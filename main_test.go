package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestResolveConfigPrecedenceAndOptions(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT":         "env.example",
		"OPENAI_API_KEY":          "env-key",
		"OPENAI_MODEL":            "env-model",
		"OPENAI_PORT":             "8123",
		"OPENAI_TEMPERATURE":      "0.42",
		"OPENAI_TOP_P":            "0.9",
		"OPENAI_MAX_TOKENS":       "111",
		"OPENAI_REASONING_EFFORT": "medium",
		"OPENAI_REASONING":        `{"depth":"low"}`,
	}, func() {
		cfg, err := resolveConfig(
			"arg.example", "short.example", -1,
			"arg-model", "arg-key",
			"json", "0.7", "0.8", 99,
			"high", `{"enabled":true}`,
		)
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}

		if cfg.Endpoint != "arg.example" {
			t.Fatalf("expected endpoint from --endpoint, got %q", cfg.Endpoint)
		}
		if cfg.Model != "arg-model" {
			t.Fatalf("expected model from flag, got %q", cfg.Model)
		}
		if cfg.APIKey != "arg-key" {
			t.Fatalf("expected api key from flag, got %q", cfg.APIKey)
		}
		if cfg.Format != "json" {
			t.Fatalf("expected format json, got %q", cfg.Format)
		}
		if cfg.Port != 8123 {
			t.Fatalf("expected port from env fallback, got %d", cfg.Port)
		}
		if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
			t.Fatalf("expected temperature 0.7, got %#v", cfg.Temperature)
		}
		if cfg.TopP == nil || *cfg.TopP != 0.8 {
			t.Fatalf("expected top_p 0.8, got %#v", cfg.TopP)
		}
		if cfg.MaxTokens == nil || *cfg.MaxTokens != 99 {
			t.Fatalf("expected max_tokens 99, got %#v", cfg.MaxTokens)
		}
		if cfg.ReasoningEffort != "high" {
			t.Fatalf("expected reasoning_effort from flag, got %q", cfg.ReasoningEffort)
		}
		if string(cfg.Reasoning) != `{"enabled":true}` {
			t.Fatalf("expected reasoning JSON from flag, got %s", string(cfg.Reasoning))
		}
	})
}

func TestResolveConfigUsesShortEndpointBeforeEnv(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "env.example",
		"OPENAI_API_KEY":  "",
	}, func() {
		cfg, err := resolveConfig("", "short.example", -1, "", "", "", "", "", -1, "", "")
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}
		if cfg.Endpoint != "short.example" {
			t.Fatalf("expected endpoint from -e, got %q", cfg.Endpoint)
		}
		if cfg.APIKey != "" {
			t.Fatalf("expected empty API key when unset, got %q", cfg.APIKey)
		}
	})
}

func TestResolveConfigErrors(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "",
		"OPENAI_API_KEY":  "",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "", "", "", -1, "", "")
		if err == nil || !strings.Contains(err.Error(), "missing endpoint") {
			t.Fatalf("expected missing endpoint error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "example.com",
		"OPENAI_API_KEY":  "key",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "", "x", "", -1, "", "")
		if err == nil || !strings.Contains(err.Error(), "OPENAI_TEMPERATURE") {
			t.Fatalf("expected OPENAI_TEMPERATURE parse error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "example.com",
		"OPENAI_API_KEY":  "key",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "", "", "", -1, "", "{bad json")
		if err == nil || !strings.Contains(err.Error(), "OPENAI_REASONING") {
			t.Fatalf("expected OPENAI_REASONING JSON error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "example.com",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "xml", "", "", -1, "", "")
		if err == nil || !strings.Contains(err.Error(), "supported values are text or json") {
			t.Fatalf("expected format validation error, got %v", err)
		}
	})
}

func TestBuildRequestURL(t *testing.T) {
	urlValue, err := buildRequestURL("example.com", 8080)
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "http://example.com:8080/v1/chat/completions" {
		t.Fatalf("unexpected URL: %s", urlValue)
	}

	urlValue, err = buildRequestURL("https://api.example.com:9999/base", 443)
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "https://api.example.com:443/base" {
		t.Fatalf("unexpected URL with endpoint path: %s", urlValue)
	}
}

func TestResolvePortFallbacks(t *testing.T) {
	port, err := resolvePort(3001, "example.com")
	if err != nil {
		t.Fatalf("resolvePort arg returned error: %v", err)
	}
	if port != 3001 {
		t.Fatalf("expected explicit port, got %d", port)
	}

	withEnv(t, map[string]string{"OPENAI_PORT": "7788"}, func() {
		port, err = resolvePort(-1, "example.com")
		if err != nil {
			t.Fatalf("resolvePort env returned error: %v", err)
		}
		if port != 7788 {
			t.Fatalf("expected OPENAI_PORT, got %d", port)
		}
	})

	withEnv(t, map[string]string{"OPENAI_PORT": ""}, func() {
		port, err = resolvePort(-1, "https://example.com")
		if err != nil {
			t.Fatalf("resolvePort https default returned error: %v", err)
		}
		if port != 443 {
			t.Fatalf("expected https default 443, got %d", port)
		}
	})
}

func TestExecuteRequestSuccessAndPayload(t *testing.T) {
	var captured chatCompletionRequest
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST method, got %s", r.Method)
		}
		authHeader = r.Header.Get("Authorization")

		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("failed to read body: %v", err)
		}
		if authHeader != "Bearer test-key" {
			t.Fatalf("expected Authorization header, got %q", authHeader)
		}
		if err := json.Unmarshal(body, &captured); err != nil {
			t.Fatalf("failed to unmarshal request: %v", err)
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	host, port := serverHostPort(t, server.URL)
	temperature := 0.25
	topP := 0.95
	maxTokens := 256
	cfg := config{
		Endpoint:        host,
		Port:            port,
		Model:           "gpt-4o-mini",
		APIKey:          "test-key",
		Temperature:     &temperature,
		TopP:            &topP,
		MaxTokens:       &maxTokens,
		ReasoningEffort: "medium",
		Reasoning:       json.RawMessage(`{"foo":"bar"}`),
		Messages: []chatMessage{
			{Role: "system", Content: "sys"},
			{Role: "user", Content: "hello"},
		},
	}

	content, err := executeRequest(cfg)
	if err != nil {
		t.Fatalf("executeRequest returned error: %v", err)
	}
	if content != "ok" {
		t.Fatalf("expected assistant content ok, got %q", content)
	}

	if captured.Model != "gpt-4o-mini" {
		t.Fatalf("captured model mismatch: %q", captured.Model)
	}
	if len(captured.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(captured.Messages))
	}
	if captured.Messages[0].Role != "system" || captured.Messages[1].Role != "user" {
		t.Fatalf("unexpected message roles: %#v", captured.Messages)
	}
	if captured.Temperature == nil || *captured.Temperature != 0.25 {
		t.Fatalf("temperature missing or wrong: %#v", captured.Temperature)
	}
	if captured.TopP == nil || *captured.TopP != 0.95 {
		t.Fatalf("top_p missing or wrong: %#v", captured.TopP)
	}
	if captured.MaxTokens == nil || *captured.MaxTokens != 256 {
		t.Fatalf("max_tokens missing or wrong: %#v", captured.MaxTokens)
	}
	if captured.ReasoningEffort != "medium" {
		t.Fatalf("reasoning_effort mismatch: %q", captured.ReasoningEffort)
	}
	if captured.ResponseFormat != nil {
		t.Fatalf("expected text format to omit response_format, got %#v", captured.ResponseFormat)
	}
	if string(captured.Reasoning) != `{"foo":"bar"}` {
		t.Fatalf("reasoning mismatch: %s", string(captured.Reasoning))
	}
}

func TestExecuteRequestErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad input"}}`))
	}))
	defer server.Close()

	host, port := serverHostPort(t, server.URL)
	_, err := executeRequest(config{
		Endpoint: host,
		Port:     port,
		Model:    "m",
		APIKey:   "k",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "bad input") {
		t.Fatalf("expected API error including body message, got %v", err)
	}
}

func TestExecuteStreamingRequestWithoutAPIKey(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	host, port := serverHostPort(t, server.URL)
	var out strings.Builder
	err := executeStreamingRequest(config{
		Endpoint: host,
		Port:     port,
		Model:    "gpt-4o-mini",
		APIKey:   "",
		Messages: []chatMessage{{Role: "user", Content: "hello"}},
	}, &out)
	if err != nil {
		t.Fatalf("executeStreamingRequest returned error: %v", err)
	}
	if out.String() != "ok\n" {
		t.Fatalf("expected streamed output 'ok\\n', got %q", out.String())
	}
	if authHeader != "" {
		t.Fatalf("expected no Authorization header when api key unset, got %q", authHeader)
	}
}

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

func withEnv(t *testing.T, values map[string]string, fn func()) {
	t.Helper()
	original := make(map[string]*string, len(values))
	for key, value := range values {
		v, ok := os.LookupEnv(key)
		if ok {
			copy := v
			original[key] = &copy
		} else {
			original[key] = nil
		}
		if value == "" {
			if err := os.Unsetenv(key); err != nil {
				t.Fatalf("failed to unset env %s: %v", key, err)
			}
			continue
		}
		if err := os.Setenv(key, value); err != nil {
			t.Fatalf("failed to set env %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for key, value := range original {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}
			_ = os.Setenv(key, *value)
		}
	})

	fn()
}

func withStdin(t *testing.T, input string, fn func() error) error {
	t.Helper()

	oldStdin := os.Stdin
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdin pipe: %v", err)
	}
	if _, err := writePipe.WriteString(input); err != nil {
		t.Fatalf("failed writing to stdin pipe: %v", err)
	}
	if err := writePipe.Close(); err != nil {
		t.Fatalf("failed closing stdin write pipe: %v", err)
	}

	os.Stdin = readPipe
	defer func() {
		os.Stdin = oldStdin
		_ = readPipe.Close()
	}()

	return fn()
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stdout pipe: %v", err)
	}

	os.Stdout = writePipe
	callErr := fn()

	if err := writePipe.Close(); err != nil {
		t.Fatalf("failed closing stdout writer: %v", err)
	}
	os.Stdout = oldStdout

	out, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("failed reading captured stdout: %v", err)
	}
	_ = readPipe.Close()

	return string(out), callErr
}

func captureStderr(t *testing.T, fn func() error) (string, error) {
	t.Helper()

	oldStderr := os.Stderr
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create stderr pipe: %v", err)
	}

	os.Stderr = writePipe
	callErr := fn()

	if err := writePipe.Close(); err != nil {
		t.Fatalf("failed closing stderr writer: %v", err)
	}
	os.Stderr = oldStderr

	out, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatalf("failed reading captured stderr: %v", err)
	}
	_ = readPipe.Close()

	return string(out), callErr
}

func serverHostPort(t *testing.T, rawURL string) (string, int) {
	t.Helper()

	trimmed := strings.TrimPrefix(rawURL, "http://")
	trimmed = strings.TrimPrefix(trimmed, "https://")
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		t.Fatalf("unexpected test server URL %q", rawURL)
	}

	port, err := strconv.Atoi(parts[1])
	if err != nil {
		t.Fatalf("invalid test server port in %q: %v", rawURL, err)
	}
	return parts[0], port
}
