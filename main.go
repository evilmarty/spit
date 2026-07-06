package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatCompletionRequest struct {
	Model           string          `json:"model"`
	Messages        []chatMessage   `json:"messages"`
	Stream          bool            `json:"stream,omitempty"`
	ResponseFormat  *responseFormat `json:"response_format,omitempty"`
	Temperature     *float64        `json:"temperature,omitempty"`
	TopP            *float64        `json:"top_p,omitempty"`
	MaxTokens       *int            `json:"max_tokens,omitempty"`
	ReasoningEffort string          `json:"reasoning_effort,omitempty"`
	Reasoning       json.RawMessage `json:"reasoning,omitempty"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatCompletionResponse struct {
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
}

type apiErrorResponse struct {
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
}

type config struct {
	Endpoint        string
	Port            int
	Model           string
	APIKey          string
	Format          string
	Temperature     *float64
	TopP            *float64
	MaxTokens       *int
	ReasoningEffort string
	Reasoning       json.RawMessage
	Messages        []chatMessage
}

type messageCollector struct {
	messages []chatMessage
}

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

func (c *messageCollector) add(role, content string) error {
	value := strings.TrimSpace(content)
	if value == "" {
		return fmt.Errorf("%s prompt cannot be empty", role)
	}

	c.messages = append(c.messages, chatMessage{Role: role, Content: value})
	return nil
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	var (
		endpointArg        string
		endpointShortArg   string
		portArg            int
		modelArg           string
		apiKeyArg          string
		formatArg          string
		temperatureArg     string
		topPArg            string
		maxTokensArg       int
		reasoningEffortArg string
		reasoningArg       string
	)

	collector := &messageCollector{}
	flags := flag.NewFlagSet("spit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&endpointArg, "endpoint", "", "endpoint host or URL (env: OPENAI_ENDPOINT)")
	flags.StringVar(&endpointShortArg, "e", "", "endpoint host or URL (shorthand)")
	flags.IntVar(&portArg, "port", -1, "endpoint port (env: OPENAI_PORT)")
	flags.StringVar(&modelArg, "model", "", "model name (env: OPENAI_MODEL, default: gpt-4o-mini)")
	flags.StringVar(&modelArg, "m", "", "model name (shorthand)")
	flags.StringVar(&apiKeyArg, "api-key", "", "API key (env: OPENAI_API_KEY)")
	flags.StringVar(&formatArg, "format", "text", "response format: text or json")
	flags.StringVar(&formatArg, "f", "text", "response format (shorthand)")
	flags.StringVar(&temperatureArg, "temperature", "", "sampling temperature (env: OPENAI_TEMPERATURE)")
	flags.StringVar(&topPArg, "top-p", "", "nucleus sampling top_p (env: OPENAI_TOP_P)")
	flags.IntVar(&maxTokensArg, "max_tokens", -1, "max tokens to generate (env: OPENAI_MAX_TOKENS)")
	flags.StringVar(&reasoningEffortArg, "reasoning-effort", "", "reasoning effort value (env: OPENAI_REASONING_EFFORT)")
	flags.StringVar(&reasoningArg, "reasoning", "", "reasoning payload as JSON string (env: OPENAI_REASONING)")
	flags.Func("system", "append a system prompt; repeat to add more", func(value string) error {
		return collector.add("system", value)
	})
	flags.Func("s", "append a system prompt; repeat to add more", func(value string) error {
		return collector.add("system", value)
	})
	flags.Func("prompt", "append a user prompt; repeat to add more", func(value string) error {
		return collector.add("user", value)
	})
	flags.Func("p", "append a user prompt; repeat to add more", func(value string) error {
		return collector.add("user", value)
	})
	flags.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s --endpoint <host|url> [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintln(os.Stderr, "  --endpoint, -e <host|url>         Endpoint host or URL (env: OPENAI_ENDPOINT)")
		fmt.Fprintln(os.Stderr, "  --port <int>                      Endpoint port override (env: OPENAI_PORT)")
		fmt.Fprintln(os.Stderr, "  --api-key <key>                   API key (optional, env: OPENAI_API_KEY)")
		fmt.Fprintln(os.Stderr, "  --model, -m <name>                Model name (env: OPENAI_MODEL, default: gpt-4o-mini)")
		fmt.Fprintln(os.Stderr, "  --format, -f <text|json>          Response format mode (default: text)")
		fmt.Fprintln(os.Stderr, "  --temperature <float>             Sampling temperature (env: OPENAI_TEMPERATURE)")
		fmt.Fprintln(os.Stderr, "  --top-p <float>                   Nucleus sampling top_p (env: OPENAI_TOP_P)")
		fmt.Fprintln(os.Stderr, "  --max_tokens <int>                Max tokens to generate (env: OPENAI_MAX_TOKENS)")
		fmt.Fprintln(os.Stderr, "  --reasoning-effort <value>        Reasoning effort (env: OPENAI_REASONING_EFFORT)")
		fmt.Fprintln(os.Stderr, "  --reasoning <json>                Reasoning payload JSON (env: OPENAI_REASONING)")
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "Message options (preserve CLI order in payload):")
		fmt.Fprintln(os.Stderr, "  --system, -s <text>               Add a system message (repeatable)")
		fmt.Fprintln(os.Stderr, "  --prompt, -p <text>               Add a user message (repeatable)")
		fmt.Fprintln(os.Stderr, "  [arg ...]                         Positional args are combined into one user message")
		fmt.Fprintln(os.Stderr, "  stdin                             If provided, appended as the final user message")
	}

	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		flags.Usage()
		return err
	}

	if positional := strings.TrimSpace(strings.Join(flags.Args(), " ")); positional != "" {
		if err := collector.add("user", positional); err != nil {
			return err
		}
	}

	stdinPrompt, err := readStdinPrompt(os.Stdin)
	if err != nil {
		return err
	}
	if stdinPrompt != "" {
		if err := collector.add("user", stdinPrompt); err != nil {
			return err
		}
	}

	if !hasUserMessage(collector.messages) {
		flags.Usage()
		return errors.New("at least one user prompt is required")
	}

	cfg, err := resolveConfig(endpointArg, endpointShortArg, portArg, modelArg, apiKeyArg, formatArg, temperatureArg, topPArg, maxTokensArg, reasoningEffortArg, reasoningArg)
	if err != nil {
		return err
	}
	cfg.Messages = collector.messages

	return executeStreamingRequest(cfg, os.Stdout)
}

func readStdinPrompt(stdin *os.File) (string, error) {
	info, err := stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("unable to inspect stdin: %w", err)
	}

	if info.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}

	data, err := io.ReadAll(stdin)
	if err != nil {
		return "", fmt.Errorf("unable to read stdin: %w", err)
	}

	return strings.TrimSpace(string(data)), nil
}

func hasUserMessage(messages []chatMessage) bool {
	for _, message := range messages {
		if message.Role == "user" {
			return true
		}
	}

	return false
}

func resolveConfig(endpointArg, endpointShortArg string, portArg int, modelArg, apiKeyArg, formatArg, temperatureArg, topPArg string, maxTokensArg int, reasoningEffortArg, reasoningArg string) (config, error) {
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
	reasoning, err := resolveOptionalJSON(reasoningArg, "OPENAI_REASONING")
	if err != nil {
		return config{}, err
	}

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
		Reasoning:       reasoning,
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

func resolveOptionalJSON(argValue, envKey string) (json.RawMessage, error) {
	value := resolveString(argValue, envKey, "")
	if value == "" {
		return nil, nil
	}

	raw := json.RawMessage(strings.TrimSpace(value))
	if !json.Valid(raw) {
		return nil, fmt.Errorf("invalid %s JSON payload", envKeyOrFlag(envKey))
	}

	return raw, nil
}

func envKeyOrFlag(envKey string) string {
	if strings.TrimSpace(envKey) == "" {
		return "flag"
	}
	return envKey
}

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
		Reasoning:       cfg.Reasoning,
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
		Reasoning:       cfg.Reasoning,
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
