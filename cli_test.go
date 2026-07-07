package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestRunParsesNewFlagsAndPreservesMessageOrder(t *testing.T) {
	var captured chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"do\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ne\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	stdout, err := captureStdout(t, func() error {
		return withStdin(t, "stdin message\n", func() error {
			return run([]string{
				"-u", server.URL,
				"--api-key", "key",
				"-m", "model-short",
				"-f", "json",
				"-s", "sys 1",
				"-p", "user 1",
				"--system", "sys 2",
				"--prompt", "user 2",
				"--top-p", "0.7",
				"--reasoning-effort", "high",
				"positional user",
			})
		})
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if stdout != "done\n" {
		t.Fatalf("expected stdout to be done with trailing newline, got %q", stdout)
	}
	if !captured.Stream {
		t.Fatalf("expected stream=true in request payload")
	}

	expected := []chatMessage{
		{Role: "system", Content: "sys 1"},
		{Role: "user", Content: "user 1"},
		{Role: "system", Content: "sys 2"},
		{Role: "user", Content: "user 2"},
		{Role: "user", Content: "positional user"},
		{Role: "user", Content: "stdin message"},
	}
	if len(captured.Messages) != len(expected) {
		t.Fatalf("expected %d messages, got %d: %#v", len(expected), len(captured.Messages), captured.Messages)
	}
	for i := range expected {
		if captured.Messages[i] != expected[i] {
			t.Fatalf("message[%d] mismatch: got %#v want %#v", i, captured.Messages[i], expected[i])
		}
	}

	if captured.TopP == nil || *captured.TopP != 0.7 {
		t.Fatalf("expected top-p in payload, got %#v", captured.TopP)
	}
	if captured.Model != "model-short" {
		t.Fatalf("expected model from -m, got %q", captured.Model)
	}
	if captured.ReasoningEffort != "high" {
		t.Fatalf("expected reasoning-effort high, got %q", captured.ReasoningEffort)
	}
	if captured.ResponseFormat == nil || captured.ResponseFormat.Type != "json_object" {
		t.Fatalf("expected json response_format, got %#v", captured.ResponseFormat)
	}
}

func TestRunRequiresUserMessage(t *testing.T) {
	err := withStdin(t, "", func() error {
		return run([]string{
			"--base-url", "http://example.com:1234",
			"--api-key", "key",
			"-s", "system only",
		})
	})
	if err == nil || !strings.Contains(err.Error(), "at least one user prompt is required") {
		t.Fatalf("expected missing user prompt error, got %v", err)
	}
}

func TestRunUnknownFlagReturnsParseError(t *testing.T) {
	_, err := captureStderr(t, func() error {
		return run([]string{"--not-a-real-flag"})
	})
	if err == nil || !strings.Contains(err.Error(), "flag provided but not defined") {
		t.Fatalf("expected flag parse error, got %v", err)
	}
}

func TestRunUsesDefaultModelWhenEnvUnset(t *testing.T) {
	var captured chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	withEnv(t, map[string]string{"OPENAI_MODEL": ""}, func() {
		err := run([]string{
			"--base-url", server.URL,
			"--prompt", "hello",
		})
		if err != nil {
			t.Fatalf("expected run success with default model, got %v", err)
		}
	})

	if captured.Model != "llama3" {
		t.Fatalf("expected default model llama3, got %q", captured.Model)
	}
}

func TestRunHelpIncludesDefaultBaseURLAndModel(t *testing.T) {
	stderr, err := captureStderr(t, func() error {
		return run([]string{"--help"})
	})
	if err == nil || !strings.Contains(err.Error(), "help requested") {
		t.Fatalf("expected help requested error, got %v", err)
	}
	if !strings.Contains(stderr, "http://localhost:11434/v1") {
		t.Fatalf("expected help to include default base URL, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "default: llama3") {
		t.Fatalf("expected help to include default model, got:\n%s", stderr)
	}
	if !strings.Contains(stderr, "--version") {
		t.Fatalf("expected help to include --version flag, got:\n%s", stderr)
	}
}

func TestRunVersionPrintsBuildMetadata(t *testing.T) {
	originalVersion := Version
	originalBuildDate := BuildDate
	originalCommit := Commit
	Version = "1.2.3"
	BuildDate = "2026-07-07T04:00:00Z"
	Commit = "abc123"
	t.Cleanup(func() {
		Version = originalVersion
		BuildDate = originalBuildDate
		Commit = originalCommit
	})

	stdout, err := captureStdout(t, func() error {
		return run([]string{"--version"})
	})
	if err != nil {
		t.Fatalf("expected --version to succeed, got %v", err)
	}
	if !strings.Contains(stdout, "Version: 1.2.3") {
		t.Fatalf("expected version in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "BuildDate: 2026-07-07T04:00:00Z") {
		t.Fatalf("expected build date in output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Commit: abc123") {
		t.Fatalf("expected commit in output, got %q", stdout)
	}
}

func TestRunWithContextInterruptKeepsPartialOutputAndNewline(t *testing.T) {
	started := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not implement http.Flusher")
		}

		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"partial\"}}]}\n\n"))
		flusher.Flush()
		close(started)
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-started
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	out, err := captureStdout(t, func() error {
		return runWithContext(ctx, []string{
			"--base-url", server.URL,
			"--model", "m",
			"--prompt", "hello",
		})
	})
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("expected interrupt error, got %v", err)
	}
	if out != "partial\n" {
		t.Fatalf("expected partial output with newline, got %q", out)
	}
}

func TestRunCombinesPositionalArgsIntoSingleMessage(t *testing.T) {
	var captured chatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("failed to unmarshal payload: %v", err)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	_, err := captureStdout(t, func() error {
		return run([]string{
			"-u", server.URL,
			"--api-key", "key",
			"-m", "model",
			"positional",
			"user",
			"parts",
		})
	})
	if err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	if len(captured.Messages) != 1 {
		t.Fatalf("expected exactly one message, got %d: %#v", len(captured.Messages), captured.Messages)
	}
	if captured.Messages[0].Role != "user" || captured.Messages[0].Content != "positional user parts" {
		t.Fatalf("unexpected positional message: %#v", captured.Messages[0])
	}
}

func TestHelpListsArguments(t *testing.T) {
	stderr, err := captureStderr(t, func() error {
		return run([]string{"--help"})
	})
	if err == nil || !strings.Contains(err.Error(), "help requested") {
		t.Fatalf("expected help requested error, got %v", err)
	}

	expected := []string{
		"-base-url",
		"-u",
		"-api-key",
		"-model",
		"-m",
		"-format",
		"-f",
		"-temperature",
		"-top-p",
		"-max-tokens",
		"-request-timeout",
		"-idle-timeout",
		"-reasoning-effort",
		"-version",
		"-system",
		"-s",
		"-prompt",
		"-p",
	}
	for _, token := range expected {
		if !strings.Contains(stderr, token) {
			t.Fatalf("expected help output to contain %q, got:\n%s", token, stderr)
		}
	}
}

func TestMessageCollectorAddRejectsEmpty(t *testing.T) {
	collector := &messageCollector{}
	err := collector.add("user", "   ")
	if err == nil || !strings.Contains(err.Error(), "cannot be empty") {
		t.Fatalf("expected empty prompt error, got %v", err)
	}
}

func TestReadStdinPromptCharDeviceAndPipe(t *testing.T) {
	devNull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatalf("failed to open %s: %v", os.DevNull, err)
	}
	defer devNull.Close()

	prompt, err := readStdinPrompt(devNull)
	if err != nil {
		t.Fatalf("readStdinPrompt on char device failed: %v", err)
	}
	if prompt != "" {
		t.Fatalf("expected empty prompt from char device, got %q", prompt)
	}

	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	_, _ = writePipe.WriteString("  hello from pipe  \n")
	_ = writePipe.Close()
	defer readPipe.Close()

	prompt, err = readStdinPrompt(readPipe)
	if err != nil {
		t.Fatalf("readStdinPrompt on pipe failed: %v", err)
	}
	if prompt != "hello from pipe" {
		t.Fatalf("unexpected prompt from pipe: %q", prompt)
	}

	closed, err := os.CreateTemp("", "closed-stdin-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	name := closed.Name()
	_ = closed.Close()
	_ = os.Remove(name)
	if _, err := readStdinPrompt(closed); err == nil || !strings.Contains(err.Error(), "unable to inspect stdin") {
		t.Fatalf("expected stat failure for closed file, got %v", err)
	}
}

func TestMainHelpExitZero(t *testing.T) {
	if os.Getenv("SPIT_TEST_MAIN_HELP") == "1" {
		os.Args = []string{"spit", "--help"}
		main()
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestMainHelpExitZero")
	cmd.Env = append(os.Environ(), "SPIT_TEST_MAIN_HELP=1")
	if err := cmd.Run(); err != nil {
		t.Fatalf("expected main help path to exit 0, got error: %v", err)
	}
}

func TestExitCodeForError(t *testing.T) {
	if code := exitCodeForError(nil); code != 0 {
		t.Fatalf("expected exit code 0 for nil error, got %d", code)
	}
	if code := exitCodeForError(flag.ErrHelp); code != 0 {
		t.Fatalf("expected exit code 0 for help, got %d", code)
	}
	if code := exitCodeForError(errInterrupted); code != 130 {
		t.Fatalf("expected exit code 130 for interrupt, got %d", code)
	}
	if code := exitCodeForError(errors.New("x")); code != 1 {
		t.Fatalf("expected exit code 1 for generic errors, got %d", code)
	}
}
