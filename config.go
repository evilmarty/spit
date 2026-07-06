package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

func resolveConfig(endpointArg, endpointShortArg string, portArg int, modelArg, apiKeyArg, formatArg, temperatureArg, topPArg string, maxTokensArg int, reasoningEffortArg string) (config, error) {
	endpoint := resolveString(endpointArg, "", "")
	if endpoint == "" {
		endpoint = resolveString(endpointShortArg, "OPENAI_ENDPOINT", "")
	}
	if endpoint == "" {
		return config{}, errors.New("missing endpoint; set --endpoint/-e or OPENAI_ENDPOINT")
	}

	apiKey := resolveString(apiKeyArg, "OPENAI_API_KEY", "")

	model := resolveString(modelArg, "OPENAI_MODEL", "gpt-4o-mini")
	format, err := resolveFormat(formatArg)
	if err != nil {
		return config{}, err
	}
	temperature, err := resolveOptionalFloat(temperatureArg, "OPENAI_TEMPERATURE")
	if err != nil {
		return config{}, err
	}
	topP, err := resolveOptionalFloat(topPArg, "OPENAI_TOP_P")
	if err != nil {
		return config{}, err
	}
	maxTokens, err := resolveOptionalInt(maxTokensArg, "OPENAI_MAX_TOKENS")
	if err != nil {
		return config{}, err
	}
	reasoningEffort := resolveString(reasoningEffortArg, "OPENAI_REASONING_EFFORT", "")

	port, err := resolvePort(portArg, endpoint)
	if err != nil {
		return config{}, err
	}

	return config{
		Endpoint:        endpoint,
		Port:            port,
		Model:           model,
		APIKey:          apiKey,
		Format:          format,
		Temperature:     temperature,
		TopP:            topP,
		MaxTokens:       maxTokens,
		ReasoningEffort: reasoningEffort,
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

func resolvePort(portArg int, endpoint string) (int, error) {
	if portArg > 0 {
		return portArg, nil
	}
	if portArg == 0 {
		return 0, errors.New("invalid port 0")
	}

	if envPort := strings.TrimSpace(os.Getenv("OPENAI_PORT")); envPort != "" {
		parsed, err := strconv.Atoi(envPort)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid OPENAI_PORT value %q", envPort)
		}
		return parsed, nil
	}

	normalized := endpoint
	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}

	parsedEndpoint, err := url.Parse(normalized)
	if err != nil {
		return 0, fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}

	if endpointPort := parsedEndpoint.Port(); endpointPort != "" {
		parsed, err := strconv.Atoi(endpointPort)
		if err != nil || parsed <= 0 {
			return 0, fmt.Errorf("invalid endpoint port %q", endpointPort)
		}
		return parsed, nil
	}

	if parsedEndpoint.Scheme == "https" {
		return 443, nil
	}

	return 80, nil
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

func envKeyOrFlag(envKey string) string {
	if strings.TrimSpace(envKey) == "" {
		return "flag"
	}
	return envKey
}

func buildRequestURL(endpoint string, port int) (string, error) {
	normalized := strings.TrimSpace(endpoint)
	if normalized == "" {
		return "", errors.New("endpoint cannot be empty")
	}

	if !strings.Contains(normalized, "://") {
		normalized = "http://" + normalized
	}

	parsedEndpoint, err := url.Parse(normalized)
	if err != nil {
		return "", fmt.Errorf("invalid endpoint %q: %w", endpoint, err)
	}

	hostname := parsedEndpoint.Hostname()
	if hostname == "" {
		return "", fmt.Errorf("endpoint %q does not contain a valid host", endpoint)
	}

	if port <= 0 {
		return "", fmt.Errorf("invalid port %d", port)
	}

	parsedEndpoint.Host = net.JoinHostPort(hostname, strconv.Itoa(port))

	if parsedEndpoint.Path == "" || parsedEndpoint.Path == "/" {
		parsedEndpoint.Path = "/v1/chat/completions"
	}

	parsedEndpoint.RawQuery = ""
	parsedEndpoint.Fragment = ""
	return parsedEndpoint.String(), nil
}
