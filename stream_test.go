package main

import (
	"strings"
	"testing"
)

func TestEnsureTrailingNewlineBehavior(t *testing.T) {
	var out strings.Builder
	writer := &newlineTrackingWriter{writer: &out}

	if err := writer.ensureTrailingNewline(); err != nil {
		t.Fatalf("unexpected error with empty writer: %v", err)
	}
	if out.String() != "" {
		t.Fatalf("expected no output, got %q", out.String())
	}

	if _, err := writer.Write([]byte("hello")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.ensureTrailingNewline(); err != nil {
		t.Fatalf("ensureTrailingNewline failed: %v", err)
	}
	if out.String() != "hello\n" {
		t.Fatalf("expected trailing newline added, got %q", out.String())
	}

	if _, err := writer.Write([]byte("already\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := writer.ensureTrailingNewline(); err != nil {
		t.Fatalf("ensureTrailingNewline failed: %v", err)
	}
	if out.String() != "hello\nalready\n" {
		t.Fatalf("expected no duplicate newline, got %q", out.String())
	}
}

func TestWriteStreamingContentErrorPaths(t *testing.T) {
	err := writeStreamingContent(strings.NewReader("data: {not-json}\n\n"), &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "unable to parse streaming chunk") {
		t.Fatalf("expected chunk parse error, got %v", err)
	}

	err = writeStreamingContent(strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n"), &failingWriter{})
	if err == nil || !strings.Contains(err.Error(), "unable to write streaming output") {
		t.Fatalf("expected write error, got %v", err)
	}

	err = writeStreamingContent(&failingReader{}, &strings.Builder{})
	if err == nil || !strings.Contains(err.Error(), "stream read failed") {
		t.Fatalf("expected scanner error, got %v", err)
	}
}
