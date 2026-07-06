package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

func run(args []string) error {
	return runWithContext(context.Background(), args)
}

func runWithContext(ctx context.Context, args []string) error {
	var (
		baseURLArg         string
		baseURLShortArg    string
		modelArg           string
		apiKeyArg          string
		formatArg          string
		temperatureArg     string
		topPArg            string
		maxTokensArg       int
		requestTimeoutArg  string
		idleTimeoutArg     string
		reasoningEffortArg string
	)

	collector := &messageCollector{}
	flags := flag.NewFlagSet("spit", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	flags.StringVar(&baseURLArg, "base-url", "", "base URL (env: OPENAI_BASE_URL)")
	flags.StringVar(&baseURLShortArg, "u", "", "base URL (shorthand)")
	flags.StringVar(&modelArg, "model", "", "model name (env: OPENAI_MODEL)")
	flags.StringVar(&modelArg, "m", "", "model name (shorthand)")
	flags.StringVar(&apiKeyArg, "api-key", "", "API key (env: OPENAI_API_KEY)")
	flags.StringVar(&formatArg, "format", "text", "response format: text or json")
	flags.StringVar(&formatArg, "f", "text", "response format (shorthand)")
	flags.StringVar(&temperatureArg, "temperature", "", "sampling temperature (env: OPENAI_TEMPERATURE)")
	flags.StringVar(&topPArg, "top-p", "", "nucleus sampling top_p (env: OPENAI_TOP_P)")
	flags.IntVar(&maxTokensArg, "max_tokens", -1, "max tokens to generate (env: OPENAI_MAX_TOKENS)")
	flags.StringVar(&requestTimeoutArg, "request-timeout", "", "request timeout duration (env: OPENAI_REQUEST_TIMEOUT)")
	flags.StringVar(&idleTimeoutArg, "idle-timeout", "", "idle stream timeout duration (env: OPENAI_IDLE_TIMEOUT)")
	flags.StringVar(&reasoningEffortArg, "reasoning-effort", "", "reasoning effort value (env: OPENAI_REASONING_EFFORT)")
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
		fmt.Fprintf(os.Stderr, "Usage: %s --base-url <url> [options]\n\n", os.Args[0])
		fmt.Fprintln(os.Stderr, "Options:")
		fmt.Fprintln(os.Stderr, "  --base-url, -u <url>              Base URL (env: OPENAI_BASE_URL)")
		fmt.Fprintln(os.Stderr, "  --api-key <key>                   API key (optional, env: OPENAI_API_KEY)")
		fmt.Fprintln(os.Stderr, "  --model, -m <name>                Model name (required; env: OPENAI_MODEL)")
		fmt.Fprintln(os.Stderr, "  --format, -f <text|json>          Response format mode (default: text)")
		fmt.Fprintln(os.Stderr, "  --temperature <float>             Sampling temperature (env: OPENAI_TEMPERATURE)")
		fmt.Fprintln(os.Stderr, "  --top-p <float>                   Nucleus sampling top_p (env: OPENAI_TOP_P)")
		fmt.Fprintln(os.Stderr, "  --max_tokens <int>                Max tokens to generate (env: OPENAI_MAX_TOKENS)")
		fmt.Fprintln(os.Stderr, "  --request-timeout <duration>      Request timeout (env: OPENAI_REQUEST_TIMEOUT)")
		fmt.Fprintln(os.Stderr, "  --idle-timeout <duration>         Idle stream timeout (env: OPENAI_IDLE_TIMEOUT)")
		fmt.Fprintln(os.Stderr, "  --reasoning-effort <value>        Reasoning effort (env: OPENAI_REASONING_EFFORT)")
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

	cfg, err := resolveConfig(
		baseURLArg, baseURLShortArg,
		modelArg, apiKeyArg,
		formatArg, temperatureArg, topPArg, maxTokensArg,
		requestTimeoutArg, idleTimeoutArg,
		reasoningEffortArg,
	)
	if err != nil {
		return err
	}
	cfg.Messages = collector.messages

	return executeStreamingRequestWithContext(ctx, cfg, os.Stdout)
}

func (c *messageCollector) add(role, content string) error {
	value := strings.TrimSpace(content)
	if value == "" {
		return fmt.Errorf("%s prompt cannot be empty", role)
	}

	c.messages = append(c.messages, chatMessage{Role: role, Content: value})
	return nil
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
