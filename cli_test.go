package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
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
				"-e", server.URL,
				"--api-key", "key",
				"-m", "model-short",
				"-f", "json",
				"-s", "sys 1",
				"-p", "user 1",
				"--system", "sys 2",
				"--prompt", "user 2",
				"--top-p", "0.7",
				"--reasoning-effort", "high",
				"--reasoning", `{"mode":"test"}`,
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
	if string(captured.Reasoning) != `{"mode":"test"}` {
		t.Fatalf("expected reasoning JSON, got %s", string(captured.Reasoning))
	}
}

func TestRunRequiresUserMessage(t *testing.T) {
	err := withStdin(t, "", func() error {
		return run([]string{
			"--endpoint", "http://example.com:1234",
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
			"-e", server.URL,
			"--api-key", "key",
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
		"-endpoint",
		"-e",
		"-api-key",
		"-model",
		"-m",
		"-format",
		"-f",
		"-temperature",
		"-top-p",
		"-max_tokens",
		"-reasoning-effort",
		"-reasoning",
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
