package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

var errInterrupted = errors.New("interrupted by signal")

func isTransientError(err error, statusCode int) bool {
	if err != nil {
		var timeoutErr interface{ Timeout() bool }
		if errors.As(err, &timeoutErr) && timeoutErr.Timeout() {
			return true
		}
		return errors.Is(err, context.DeadlineExceeded)
	}
	return statusCode == http.StatusTooManyRequests || statusCode >= http.StatusInternalServerError
}

func backoffWithJitter(attempt int) time.Duration {
	base := time.Duration(100) * time.Millisecond
	exponential := base * time.Duration(1<<uint(attempt))
	jitter := time.Duration(rand.Intn(100)) * time.Millisecond
	return exponential + jitter
}

func executeRequest(cfg config) (string, error) {
	return executeRequestWithContext(context.Background(), cfg)
}

func executeRequestWithContext(ctx context.Context, cfg config) (string, error) {
	requestURL, err := buildRequestURL(cfg.BaseURL)
	if err != nil {
		return "", err
	}

	payload := chatCompletionRequest{
		Model:           cfg.Model,
		Messages:        cfg.Messages,
		Stream:          false,
		ResponseFormat:  buildResponseFormat(cfg.Format),
		Temperature:     cfg.Temperature,
		TopP:            cfg.TopP,
		MaxTokens:       cfg.MaxTokens,
		ReasoningEffort: cfg.ReasoningEffort,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("unable to encode request payload: %w", err)
	}

	timeout := 60 * time.Second
	if cfg.RequestTimeout != nil {
		timeout = *cfg.RequestTimeout
	}
	client := &http.Client{Timeout: timeout}

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
		if err != nil {
			return "", fmt.Errorf("unable to construct request: %w", err)
		}

		request.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(cfg.APIKey) != "" {
			request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		response, err := client.Do(request)
		if err != nil {
			if errors.Is(err, context.Canceled) && isInterruptedContext(ctx) {
				return "", errInterrupted
			}
			if isTransientError(err, 0) && attempt < cfg.MaxRetries {
				time.Sleep(backoffWithJitter(attempt))
				continue
			}
			return "", fmt.Errorf("request failed: %w", err)
		}
		defer response.Body.Close()

		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			return "", fmt.Errorf("unable to read API response: %w", err)
		}

		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			if isTransientError(nil, response.StatusCode) && attempt < cfg.MaxRetries {
				time.Sleep(backoffWithJitter(attempt))
				continue
			}
			return "", formatAPIError(response.Status, responseBody)
		}

		return decodeAssistantContent(responseBody)
	}

	return "", fmt.Errorf("request failed after %d retries", cfg.MaxRetries)
}

func executeStreamingRequest(cfg config, output io.Writer) error {
	return executeStreamingRequestWithContext(context.Background(), cfg, output)
}

func executeStreamingRequestWithContext(ctx context.Context, cfg config, output io.Writer) error {
	requestURL, err := buildRequestURL(cfg.BaseURL)
	if err != nil {
		return err
	}
	trackedOutput := &newlineTrackingWriter{writer: output}

	payload := chatCompletionRequest{
		Model:           cfg.Model,
		Messages:        cfg.Messages,
		Stream:          true,
		ResponseFormat:  buildResponseFormat(cfg.Format),
		Temperature:     cfg.Temperature,
		TopP:            cfg.TopP,
		MaxTokens:       cfg.MaxTokens,
		ReasoningEffort: cfg.ReasoningEffort,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("unable to encode request payload: %w", err)
	}

	client := newStreamingHTTPClient(cfg.RequestTimeout)

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("unable to construct request: %w", err)
		}

		request.Header.Set("Content-Type", "application/json")
		if strings.TrimSpace(cfg.APIKey) != "" {
			request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		response, err := client.Do(request)
		if err != nil {
			if errors.Is(err, context.Canceled) && isInterruptedContext(ctx) {
				return errInterrupted
			}
			if isTransientError(err, 0) && attempt < cfg.MaxRetries {
				time.Sleep(backoffWithJitter(attempt))
				continue
			}
			return fmt.Errorf("request failed: %w", err)
		}
		defer response.Body.Close()

		if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
			responseBody, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				return fmt.Errorf("unable to read API response: %w", readErr)
			}
			if isTransientError(nil, response.StatusCode) && attempt < cfg.MaxRetries {
				time.Sleep(backoffWithJitter(attempt))
				continue
			}
			return formatAPIError(response.Status, responseBody)
		}

		if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
			responseBody, err := io.ReadAll(response.Body)
			if err != nil {
				if errors.Is(err, context.Canceled) && isInterruptedContext(ctx) {
					_ = trackedOutput.ensureTrailingNewline()
					return errInterrupted
				}
				return fmt.Errorf("unable to read API response: %w", err)
			}

			content, err := decodeAssistantContent(responseBody)
			if err != nil {
				return err
			}
			if _, err := io.WriteString(trackedOutput, content); err != nil {
				return fmt.Errorf("unable to write output: %w", err)
			}
			return trackedOutput.ensureTrailingNewline()
		}

		idleTimeout := time.Duration(0)
		if cfg.IdleTimeout != nil {
			idleTimeout = *cfg.IdleTimeout
		}
		if err := writeStreamingContentWithIdleTimeout(response.Body, trackedOutput, idleTimeout); err != nil {
			if isInterruptedContext(ctx) {
				_ = trackedOutput.ensureTrailingNewline()
				return errInterrupted
			}
			return err
		}
		return trackedOutput.ensureTrailingNewline()
	}

	return fmt.Errorf("request failed after %d retries", cfg.MaxRetries)
}

func isInterruptedContext(ctx context.Context) bool {
	return ctx != nil && errors.Is(ctx.Err(), context.Canceled)
}

func newStreamingHTTPClient(requestTimeout *time.Duration) *http.Client {
	if requestTimeout == nil {
		return &http.Client{}
	}

	baseTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Client{}
	}
	transport := baseTransport.Clone()
	transport.ResponseHeaderTimeout = *requestTimeout

	return &http.Client{Transport: transport}
}

func formatAPIError(status string, responseBody []byte) error {
	var apiErr apiErrorResponse
	if err := json.Unmarshal(responseBody, &apiErr); err == nil && strings.TrimSpace(apiErr.Error.Message) != "" {
		return fmt.Errorf("API request failed (%s): %s", status, strings.TrimSpace(apiErr.Error.Message))
	}

	body := strings.TrimSpace(string(responseBody))
	if body == "" {
		body = "<empty response body>"
	}

	return fmt.Errorf("API request failed (%s): %s", status, body)
}

func decodeAssistantContent(responseBody []byte) (string, error) {
	var parsed chatCompletionResponse
	if err := json.Unmarshal(responseBody, &parsed); err != nil {
		return "", fmt.Errorf("unable to parse API response: %w", err)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("API response did not contain any choices")
	}

	content := strings.TrimSpace(parsed.Choices[0].Message.Content)
	if content == "" {
		return "", errors.New("API response did not contain assistant content")
	}

	return content, nil
}
