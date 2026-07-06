package main

import (
	"io"
	"strings"
	"testing"
	"time"
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

func TestWriteStreamingContentWithIdleTimeout(t *testing.T) {
	readPipe, writePipe := io.Pipe()
	defer readPipe.Close()

	go func() {
		_, _ = io.WriteString(writePipe, "data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n\n")
		time.Sleep(120 * time.Millisecond)
		_, _ = io.WriteString(writePipe, "data: [DONE]\n\n")
		_ = writePipe.Close()
	}()

	err := writeStreamingContentWithIdleTimeout(readPipe, &strings.Builder{}, 40*time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), "stream idle timeout after") {
		t.Fatalf("expected idle timeout error, got %v", err)
	}
}

func TestWriteStreamingContentSupportsMultilineDataEvents(t *testing.T) {
	input := strings.NewReader(
		"event: message\n" +
			"data: {\"choices\":[{\"delta\":\n" +
			"data: {\"content\":\"hello\"}}]}\n" +
			"\n" +
			"data: [DONE]\n" +
			"\n",
	)

	var out strings.Builder
	if err := writeStreamingContent(input, &out); err != nil {
		t.Fatalf("writeStreamingContent failed: %v", err)
	}
	if out.String() != "hello" {
		t.Fatalf("expected multiline event output hello, got %q", out.String())
	}
}

func TestWriteStreamingContentParsesFinalEventWithoutTrailingBlankLine(t *testing.T) {
	input := strings.NewReader("data: {\"choices\":[{\"delta\":{\"content\":\"x\"}}]}\n")

	var out strings.Builder
	if err := writeStreamingContent(input, &out); err != nil {
		t.Fatalf("writeStreamingContent failed: %v", err)
	}
	if out.String() != "x" {
		t.Fatalf("expected output x, got %q", out.String())
	}
}
