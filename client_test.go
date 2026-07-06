package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

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

	temperature := 0.25
	topP := 0.95
	maxTokens := 256
	cfg := config{
		BaseURL:         server.URL,
		Model:           "gpt-4o-mini",
		APIKey:          "test-key",
		Temperature:     &temperature,
		TopP:            &topP,
		MaxTokens:       &maxTokens,
		ReasoningEffort: "medium",
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
}

func TestExecuteRequestErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad input"}}`))
	}))
	defer server.Close()

	_, err := executeRequest(config{
		BaseURL:  server.URL,
		Model:    "m",
		APIKey:   "k",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "bad input") {
		t.Fatalf("expected API error including body message, got %v", err)
	}
}

func TestExecuteRequestAdditionalErrorPaths(t *testing.T) {
	baseURL := serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[]}`)
	_, err := executeRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "did not contain any choices") {
		t.Fatalf("expected no choices error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[{"message":{"role":"assistant","content":"   "}}]}`)
	_, err = executeRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "did not contain assistant content") {
		t.Fatalf("expected empty assistant content error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `not-json`)
	_, err = executeRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err == nil || !strings.Contains(err.Error(), "unable to parse API response") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestExecuteRequestWithoutAPIKeyOmitsAuthHeader(t *testing.T) {
	var authHeader string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer server.Close()

	content, err := executeRequest(config{
		BaseURL:  server.URL,
		Model:    "m",
		APIKey:   "",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if err != nil {
		t.Fatalf("executeRequest returned error: %v", err)
	}
	if content != "ok" {
		t.Fatalf("expected assistant content ok, got %q", content)
	}
	if authHeader != "" {
		t.Fatalf("expected empty Authorization header, got %q", authHeader)
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

	var out strings.Builder
	err := executeStreamingRequest(config{
		BaseURL:  server.URL,
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

func TestExecuteStreamingRequestAdditionalPaths(t *testing.T) {
	baseURL := serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	var out strings.Builder
	err := executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Format:   "text",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &out)
	if err != nil {
		t.Fatalf("non-sse fallback failed: %v", err)
	}
	if out.String() != "ok\n" {
		t.Fatalf("expected fallback output with newline, got %q", out.String())
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `not-json`)
	err = executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "unable to parse API response") {
		t.Fatalf("expected non-sse parse error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusBadRequest, "text/plain", `plain error`)
	err = executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "plain error") {
		t.Fatalf("expected status/body error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[]}`)
	err = executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "did not contain any choices") {
		t.Fatalf("expected no choices error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[{"message":{"role":"assistant","content":"   "}}]}`)
	err = executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "did not contain assistant content") {
		t.Fatalf("expected empty content error, got %v", err)
	}

	baseURL = serverWithRawResponse(t, http.StatusOK, "application/json", `{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`)
	err = executeStreamingRequest(config{
		BaseURL:  baseURL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "unable to write output") {
		t.Fatalf("expected fallback write error, got %v", err)
	}
}

func TestExecuteStreamingRequestBuildURLFailure(t *testing.T) {
	err := executeStreamingRequest(config{
		BaseURL:  "",
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "base URL cannot be empty") {
		t.Fatalf("expected base URL validation error, got %v", err)
	}
}

func TestExecuteStreamingRequestRequestTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(150 * time.Millisecond)
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	requestTimeout := 20 * time.Millisecond
	err := executeStreamingRequest(config{
		BaseURL:        server.URL,
		Model:          "m",
		RequestTimeout: &requestTimeout,
		Messages:       []chatMessage{{Role: "user", Content: "x"}},
	}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "request failed") {
		t.Fatalf("expected request timeout failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "timeout awaiting response headers") &&
		!strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("expected timeout details in error, got %v", err)
	}
}

func TestExecuteStreamingRequestIdleTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("response writer does not implement http.Flusher")
		}

		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"first\"}}]}\n\n"))
		flusher.Flush()
		time.Sleep(120 * time.Millisecond)
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
		flusher.Flush()
	}))
	defer server.Close()

	idleTimeout := 40 * time.Millisecond
	var out strings.Builder
	err := executeStreamingRequest(config{
		BaseURL:     server.URL,
		Model:       "m",
		IdleTimeout: &idleTimeout,
		Messages:    []chatMessage{{Role: "user", Content: "x"}},
	}, &out)
	if err == nil || !strings.Contains(err.Error(), "stream idle timeout after") {
		t.Fatalf("expected idle timeout failure, got %v", err)
	}
	if !strings.Contains(out.String(), "first") {
		t.Fatalf("expected partial streamed output before timeout, got %q", out.String())
	}
}

func TestExecuteRequestWithContextInterrupt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := executeRequestWithContext(ctx, config{
		BaseURL:  server.URL,
		Model:    "m",
		Messages: []chatMessage{{Role: "user", Content: "x"}},
	})
	if !errors.Is(err, errInterrupted) {
		t.Fatalf("expected interrupt error, got %v", err)
	}
}

func TestDecodeAssistantContentPaths(t *testing.T) {
	content, err := decodeAssistantContent([]byte(`{"choices":[{"message":{"role":"assistant","content":" ok "}}]}`))
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if content != "ok" {
		t.Fatalf("expected trimmed content, got %q", content)
	}

	_, err = decodeAssistantContent([]byte(`not-json`))
	if err == nil || !strings.Contains(err.Error(), "unable to parse API response") {
		t.Fatalf("expected parse error, got %v", err)
	}

	_, err = decodeAssistantContent([]byte(`{"choices":[]}`))
	if err == nil || !strings.Contains(err.Error(), "did not contain any choices") {
		t.Fatalf("expected no choices error, got %v", err)
	}

	_, err = decodeAssistantContent([]byte(`{"choices":[{"message":{"role":"assistant","content":"   "}}]}`))
	if err == nil || !strings.Contains(err.Error(), "did not contain assistant content") {
		t.Fatalf("expected empty content error, got %v", err)
	}
}
