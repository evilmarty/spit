package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

func executeRequest(cfg config) (string, error) {
	requestURL, err := buildRequestURL(cfg.Endpoint, cfg.Port)
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

	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("unable to construct request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{Timeout: 60 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	responseBody, err := io.ReadAll(response.Body)
	if err != nil {
		return "", fmt.Errorf("unable to read API response: %w", err)
	}

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return "", formatAPIError(response.Status, responseBody)
	}

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

func executeStreamingRequest(cfg config, output io.Writer) error {
	requestURL, err := buildRequestURL(cfg.Endpoint, cfg.Port)
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

	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("unable to construct request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	if strings.TrimSpace(cfg.APIKey) != "" {
		request.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		responseBody, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			return fmt.Errorf("unable to read API response: %w", readErr)
		}
		return formatAPIError(response.Status, responseBody)
	}

	if !strings.Contains(strings.ToLower(response.Header.Get("Content-Type")), "text/event-stream") {
		responseBody, err := io.ReadAll(response.Body)
		if err != nil {
			return fmt.Errorf("unable to read API response: %w", err)
		}

		var parsed chatCompletionResponse
		if err := json.Unmarshal(responseBody, &parsed); err != nil {
			return fmt.Errorf("unable to parse API response: %w", err)
		}
		if len(parsed.Choices) == 0 {
			return errors.New("API response did not contain any choices")
		}

		content := strings.TrimSpace(parsed.Choices[0].Message.Content)
		if content == "" {
			return errors.New("API response did not contain assistant content")
		}
		if _, err := io.WriteString(trackedOutput, content); err != nil {
			return fmt.Errorf("unable to write output: %w", err)
		}
		return trackedOutput.ensureTrailingNewline()
	}

	if err := writeStreamingContent(response.Body, trackedOutput); err != nil {
		return err
	}
	return trackedOutput.ensureTrailingNewline()
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
