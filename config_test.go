package main

import (
	"strings"
	"testing"
)

func TestResolveConfigPrecedenceAndOptions(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT":         "env.example",
		"OPENAI_API_KEY":          "env-key",
		"OPENAI_MODEL":            "env-model",
		"OPENAI_PORT":             "8123",
		"OPENAI_TEMPERATURE":      "0.42",
		"OPENAI_TOP_P":            "0.9",
		"OPENAI_MAX_TOKENS":       "111",
		"OPENAI_REASONING_EFFORT": "medium",
	}, func() {
		cfg, err := resolveConfig(
			"arg.example", "short.example", -1,
			"arg-model", "arg-key",
			"json", "0.7", "0.8", 99,
			"high",
		)
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}

		if cfg.Endpoint != "arg.example" {
			t.Fatalf("expected endpoint from --endpoint, got %q", cfg.Endpoint)
		}
		if cfg.Model != "arg-model" {
			t.Fatalf("expected model from flag, got %q", cfg.Model)
		}
		if cfg.APIKey != "arg-key" {
			t.Fatalf("expected api key from flag, got %q", cfg.APIKey)
		}
		if cfg.Format != "json" {
			t.Fatalf("expected format json, got %q", cfg.Format)
		}
		if cfg.Port != 8123 {
			t.Fatalf("expected port from env fallback, got %d", cfg.Port)
		}
		if cfg.Temperature == nil || *cfg.Temperature != 0.7 {
			t.Fatalf("expected temperature 0.7, got %#v", cfg.Temperature)
		}
		if cfg.TopP == nil || *cfg.TopP != 0.8 {
			t.Fatalf("expected top_p 0.8, got %#v", cfg.TopP)
		}
		if cfg.MaxTokens == nil || *cfg.MaxTokens != 99 {
			t.Fatalf("expected max_tokens 99, got %#v", cfg.MaxTokens)
		}
		if cfg.ReasoningEffort != "high" {
			t.Fatalf("expected reasoning_effort from flag, got %q", cfg.ReasoningEffort)
		}
	})
}

func TestResolveConfigUsesShortEndpointBeforeEnv(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "env.example",
		"OPENAI_API_KEY":  "",
	}, func() {
		cfg, err := resolveConfig("", "short.example", -1, "", "", "", "", "", -1, "")
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}
		if cfg.Endpoint != "short.example" {
			t.Fatalf("expected endpoint from -e, got %q", cfg.Endpoint)
		}
		if cfg.APIKey != "" {
			t.Fatalf("expected empty API key when unset, got %q", cfg.APIKey)
		}
	})
}

func TestResolveConfigDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "",
		"OPENAI_MODEL":    "",
	}, func() {
		cfg, err := resolveConfig("example.com", "", -1, "", "", "", "", "", -1, "")
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}
		if cfg.Model != "gpt-4o-mini" {
			t.Fatalf("expected default model, got %q", cfg.Model)
		}
		if cfg.Format != "text" {
			t.Fatalf("expected default format text, got %q", cfg.Format)
		}
	})
}

func TestResolveConfigErrors(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "",
		"OPENAI_API_KEY":  "",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "", "", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "missing endpoint") {
			t.Fatalf("expected missing endpoint error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "example.com",
		"OPENAI_API_KEY":  "key",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "", "x", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "OPENAI_TEMPERATURE") {
			t.Fatalf("expected OPENAI_TEMPERATURE parse error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_ENDPOINT": "example.com",
	}, func() {
		_, err := resolveConfig("", "", -1, "", "", "xml", "", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "supported values are text or json") {
			t.Fatalf("expected format validation error, got %v", err)
		}
	})
}

func TestBuildRequestURL(t *testing.T) {
	urlValue, err := buildRequestURL("example.com", 8080)
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "http://example.com:8080/v1/chat/completions" {
		t.Fatalf("unexpected URL: %s", urlValue)
	}

	urlValue, err = buildRequestURL("https://api.example.com:9999/base", 443)
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "https://api.example.com:443/base" {
		t.Fatalf("unexpected URL with endpoint path: %s", urlValue)
	}
}

func TestResolvePortFallbacks(t *testing.T) {
	port, err := resolvePort(3001, "example.com")
	if err != nil {
		t.Fatalf("resolvePort arg returned error: %v", err)
	}
	if port != 3001 {
		t.Fatalf("expected explicit port, got %d", port)
	}

	withEnv(t, map[string]string{"OPENAI_PORT": "7788"}, func() {
		port, err = resolvePort(-1, "example.com")
		if err != nil {
			t.Fatalf("resolvePort env returned error: %v", err)
		}
		if port != 7788 {
			t.Fatalf("expected OPENAI_PORT, got %d", port)
		}
	})

	withEnv(t, map[string]string{"OPENAI_PORT": ""}, func() {
		port, err = resolvePort(-1, "https://example.com")
		if err != nil {
			t.Fatalf("resolvePort https default returned error: %v", err)
		}
		if port != 443 {
			t.Fatalf("expected https default 443, got %d", port)
		}
	})
}

func TestResolvePortAndOptionalIntErrorPaths(t *testing.T) {
	if _, err := resolvePort(0, "example.com"); err == nil {
		t.Fatal("expected invalid port 0 error")
	}

	withEnv(t, map[string]string{"OPENAI_PORT": "not-a-number"}, func() {
		_, err := resolvePort(-1, "example.com")
		if err == nil || !strings.Contains(err.Error(), "invalid OPENAI_PORT value") {
			t.Fatalf("expected invalid OPENAI_PORT error, got %v", err)
		}
	})

	withEnv(t, map[string]string{"OPENAI_MAX_TOKENS": "bad"}, func() {
		_, err := resolveOptionalInt(-1, "OPENAI_MAX_TOKENS")
		if err == nil || !strings.Contains(err.Error(), "OPENAI_MAX_TOKENS") {
			t.Fatalf("expected OPENAI_MAX_TOKENS parse error, got %v", err)
		}
	})

	if envKeyOrFlag("") != "flag" {
		t.Fatalf("expected envKeyOrFlag(\"\") to return flag, got %q", envKeyOrFlag(""))
	}
}

func TestFormatAPIErrorAndBuildRequestURLErrorPaths(t *testing.T) {
	err := formatAPIError("400 Bad Request", []byte(`{"error":{"message":"bad"}}`))
	if err == nil || !strings.Contains(err.Error(), "bad") {
		t.Fatalf("expected json error message, got %v", err)
	}

	err = formatAPIError("500 Internal Server Error", []byte(""))
	if err == nil || !strings.Contains(err.Error(), "<empty response body>") {
		t.Fatalf("expected empty body marker, got %v", err)
	}

	if _, err := buildRequestURL("", 80); err == nil {
		t.Fatal("expected empty endpoint error")
	}
	if _, err := buildRequestURL("http:///", 80); err == nil {
		t.Fatal("expected invalid host error")
	}
	if _, err := buildRequestURL("example.com", 0); err == nil {
		t.Fatal("expected invalid port error")
	}
}
