package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"
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
	return writeStreamingContentWithIdleTimeout(input, output, 0)
}

func writeStreamingContentWithIdleTimeout(input io.Reader, output io.Writer, idleTimeout time.Duration) error {
	scanner := bufio.NewScanner(input)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	type streamChunk struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}

	lineCh := make(chan string)
	errCh := make(chan error, 1)
	done := make(chan struct{})
	defer close(done)

	go func() {
		defer close(lineCh)
		for scanner.Scan() {
			select {
			case lineCh <- scanner.Text():
			case <-done:
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("stream read failed: %w", err)
			return
		}
		errCh <- nil
	}()

	var timer *time.Timer
	if idleTimeout > 0 {
		timer = time.NewTimer(idleTimeout)
		defer timer.Stop()
	}

	resetIdleTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer.Reset(idleTimeout)
	}

	for {
		var timerCh <-chan time.Time
		if timer != nil {
			timerCh = timer.C
		}

		select {
		case <-timerCh:
			if closer, ok := input.(io.Closer); ok {
				_ = closer.Close()
			}
			return fmt.Errorf("stream idle timeout after %s", idleTimeout)
		case line, ok := <-lineCh:
			if !ok {
				if err := <-errCh; err != nil {
					return err
				}
				return nil
			}
			resetIdleTimer()

			line = strings.TrimSpace(line)
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
	}
}
