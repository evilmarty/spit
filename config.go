package main

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

func resolveConfig(
	baseURLArg,
	modelArg, apiKeyArg,
	formatArg, temperatureArg, topPArg string,
	maxTokensArg int,
	requestTimeoutArg, idleTimeoutArg,
	reasoningEffortArg string,
	maxRetriesArg int,
) (config, error) {
	baseURL := resolveString(baseURLArg, "OPENAI_BASE_URL", "")
	if baseURL == "" {
		return config{}, errors.New("missing base URL; set --base-url or OPENAI_BASE_URL")
	}

	apiKey := resolveString(apiKeyArg, "OPENAI_API_KEY", "")

	model := resolveString(modelArg, "OPENAI_MODEL", "")
	if model == "" {
		return config{}, errors.New("missing model; set --model or OPENAI_MODEL")
	}
	format, err := resolveFormat(formatArg)
	if err != nil {
		return config{}, err
	}
	temperature, err := resolveOptionalFloatInRange(temperatureArg, "OPENAI_TEMPERATURE", 0, 2)
	if err != nil {
		return config{}, err
	}
	topP, err := resolveOptionalFloatInRange(topPArg, "OPENAI_TOP_P", 0, 1)
	if err != nil {
		return config{}, err
	}
	maxTokens, err := resolveOptionalInt(maxTokensArg, "OPENAI_MAX_TOKENS")
	if err != nil {
		return config{}, err
	}
	requestTimeout, err := resolveOptionalDuration(requestTimeoutArg, "OPENAI_REQUEST_TIMEOUT")
	if err != nil {
		return config{}, err
	}
	idleTimeout, err := resolveOptionalDuration(idleTimeoutArg, "OPENAI_IDLE_TIMEOUT")
	if err != nil {
		return config{}, err
	}
	reasoningEffort := resolveString(reasoningEffortArg, "OPENAI_REASONING_EFFORT", "")

	maxRetries, err := resolveMaxRetries(maxRetriesArg)
	if err != nil {
		return config{}, err
	}

	return config{
		BaseURL:         baseURL,
		Model:           model,
		APIKey:          apiKey,
		Format:          format,
		Temperature:     temperature,
		TopP:            topP,
		MaxTokens:       maxTokens,
		RequestTimeout:  requestTimeout,
		IdleTimeout:     idleTimeout,
		ReasoningEffort: reasoningEffort,
		MaxRetries:      maxRetries,
	}, nil
}

func resolveFormat(value string) (string, error) {
	format := strings.ToLower(strings.TrimSpace(value))
	if format == "" {
		return "text", nil
	}

	switch format {
	case "text", "json":
		return format, nil
	default:
		return "", fmt.Errorf("invalid format %q; supported values are text or json", value)
	}
}

func buildResponseFormat(format string) *responseFormat {
	if format == "json" {
		return &responseFormat{Type: "json_object"}
	}
	return nil
}

func resolveString(argValue, envKey, fallback string) string {
	if value := strings.TrimSpace(argValue); value != "" {
		return value
	}

	if envValue := strings.TrimSpace(os.Getenv(envKey)); envValue != "" {
		return envValue
	}

	return fallback
}

func resolveOptionalFloat(argValue, envKey string) (*float64, error) {
	value := resolveString(argValue, envKey, "")
	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s value %q", envKeyOrFlag(envKey), value)
	}

	return &parsed, nil
}

func resolveOptionalFloatInRange(argValue, envKey string, min, max float64) (*float64, error) {
	parsed, err := resolveOptionalFloat(argValue, envKey)
	if err != nil {
		return nil, err
	}
	if parsed == nil {
		return nil, nil
	}
	if *parsed < min || *parsed > max {
		return nil, fmt.Errorf("invalid %s value %q; expected between %g and %g", envKeyOrFlag(envKey), strconv.FormatFloat(*parsed, 'f', -1, 64), min, max)
	}
	return parsed, nil
}

func resolveOptionalInt(argValue int, envKey string) (*int, error) {
	if argValue >= 0 {
		parsed := argValue
		return &parsed, nil
	}

	envValue := strings.TrimSpace(os.Getenv(envKey))
	if envValue == "" {
		return nil, nil
	}

	parsed, err := strconv.Atoi(envValue)
	if err != nil || parsed < 0 {
		return nil, fmt.Errorf("invalid %s value %q", envKeyOrFlag(envKey), envValue)
	}

	return &parsed, nil
}

func resolveOptionalDuration(argValue, envKey string) (*time.Duration, error) {
	value := resolveString(argValue, envKey, "")
	if value == "" {
		return nil, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return nil, fmt.Errorf("invalid %s value %q", envKeyOrFlag(envKey), value)
	}

	return &parsed, nil
}

func envKeyOrFlag(envKey string) string {
	if strings.TrimSpace(envKey) == "" {
		return "flag"
	}
	return envKey
}

func resolveMaxRetries(argValue int) (int, error) {
	if argValue >= 0 {
		return argValue, nil
	}

	envValue := strings.TrimSpace(os.Getenv("OPENAI_MAX_RETRIES"))
	if envValue == "" {
		return 3, nil
	}

	parsed, err := strconv.Atoi(envValue)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("invalid OPENAI_MAX_RETRIES value %q", envValue)
	}

	return parsed, nil
}

func buildRequestURL(baseURL string) (string, error) {
	normalized := strings.TrimSpace(baseURL)
	if normalized == "" {
		return "", errors.New("base URL cannot be empty")
	}

	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}

	parsedEndpoint, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid base URL %q: %w", baseURL, err)
	}

	hostname := parsedEndpoint.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("base URL %q does not contain a valid host", baseURL)
	}

	basePath := strings.TrimSuffix(parsedEndpoint.Path, "/")
	if basePath == "" {
		parsedEndpoint.Path = "/chat/completions"
	} else {
		parsedEndpoint.Path = basePath + "/chat/completions"
	}

	parsedEndpoint.RawQuery = ""
	parsedEndpoint.Fragment = ""
	return parsedEndpoint.String(), nil
}
