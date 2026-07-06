package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

type newlineTrackingWriter struct {
	writer        io.Writer
	wroteAnything bool
	lastByte      byte
}

func (w *newlineTrackingWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	if n > 0 {
		w.wroteAnything = true
		w.lastByte = p[n-1]
	}
	return n, err
}

func (w *newlineTrackingWriter) ensureTrailingNewline() error {
	if !w.wroteAnything || w.lastByte == '\n' {
		return nil
	}

	_, err := io.WriteString(w.writer, "\n")
	return err
}

func writeStreamingContent(input io.Reader, output io.Writer) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type streamChunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || !strings.HasPrefix(line, "data:") {
			continue
		}

		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return nil
		}

		var chunk streamChunk
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			return fmt.Errorf("unable to parse streaming chunk: %w", err)
		}

		for _, choice := range chunk.Choices {
			if choice.Delta.Content == "" {
				continue
			}
			if _, err := io.WriteString(output, choice.Delta.Content); err != nil {
				return fmt.Errorf("unable to write streaming output: %w", err)
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("stream read failed: %w", err)
	}

	return nil
}
