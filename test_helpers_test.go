package main

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"testing"
)

type failingWriter struct{}

func (f *failingWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("forced write failure")
}

type failingReader struct{}

func (f *failingReader) Read(_ []byte) (int, error) {
	return 0, errors.New("forced read failure")
}

func serverWithRawResponse(t *testing.T, status int, contentType, body string) (string, int) {
	t.Helper()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", contentType)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)

	return serverHostPort(t, server.URL)
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
