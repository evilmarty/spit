package main

import (
	"strings"
	"testing"
)

func TestResolveConfigPrecedenceAndOptions(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_BASE_URL":         "http://env.example:8123",
		"OPENAI_API_KEY":          "env-key",
		"OPENAI_MODEL":            "env-model",
		"OPENAI_TEMPERATURE":      "0.42",
		"OPENAI_TOP_P":            "0.9",
		"OPENAI_MAX_TOKENS":       "111",
		"OPENAI_REASONING_EFFORT": "medium",
	}, func() {
		cfg, err := resolveConfig(
			"http://arg.example:9000", "http://short.example:7000",
			"arg-model", "arg-key",
			"json", "0.7", "0.8", 99,
			"high",
		)
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}

		if cfg.BaseURL != "http://arg.example:9000" {
			t.Fatalf("expected base URL from --base-url, got %q", cfg.BaseURL)
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

func TestResolveConfigUsesShortBaseURLBeforeEnv(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_BASE_URL": "http://env.example:8123",
		"OPENAI_API_KEY":  "",
	}, func() {
		cfg, err := resolveConfig("", "http://short.example:7000", "", "", "", "", "", -1, "")
		if err != nil {
			t.Fatalf("resolveConfig returned error: %v", err)
		}
		if cfg.BaseURL != "http://short.example:7000" {
			t.Fatalf("expected base URL from -u, got %q", cfg.BaseURL)
		}
		if cfg.APIKey != "" {
			t.Fatalf("expected empty API key when unset, got %q", cfg.APIKey)
		}
	})
}

func TestResolveConfigDefaults(t *testing.T) {
	withEnv(t, map[string]string{
		"OPENAI_BASE_URL": "",
		"OPENAI_MODEL":    "",
	}, func() {
		cfg, err := resolveConfig("http://example.com", "", "", "", "", "", "", -1, "")
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
		"OPENAI_BASE_URL": "",
		"OPENAI_API_KEY":  "",
	}, func() {
		_, err := resolveConfig("", "", "", "", "", "", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "missing base URL") {
			t.Fatalf("expected missing base URL error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_BASE_URL": "http://example.com",
		"OPENAI_API_KEY":  "key",
	}, func() {
		_, err := resolveConfig("", "", "", "", "", "x", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "OPENAI_TEMPERATURE") {
			t.Fatalf("expected OPENAI_TEMPERATURE parse error, got %v", err)
		}
	})

	withEnv(t, map[string]string{
		"OPENAI_BASE_URL": "http://example.com",
	}, func() {
		_, err := resolveConfig("", "", "", "", "xml", "", "", -1, "")
		if err == nil || !strings.Contains(err.Error(), "supported values are text or json") {
			t.Fatalf("expected format validation error, got %v", err)
		}
	})
}

func TestBuildRequestURL(t *testing.T) {
	urlValue, err := buildRequestURL("example.com:8080")
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "http://example.com:8080/chat/completions" {
		t.Fatalf("unexpected URL: %s", urlValue)
	}

	urlValue, err = buildRequestURL("https://api.example.com:9999/base")
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "https://api.example.com:9999/base/chat/completions" {
		t.Fatalf("unexpected URL with base path: %s", urlValue)
	}

	urlValue, err = buildRequestURL("https://api.example.com:9999/v1/")
	if err != nil {
		t.Fatalf("buildRequestURL returned error: %v", err)
	}
	if urlValue != "https://api.example.com:9999/v1/chat/completions" {
		t.Fatalf("unexpected URL with trailing slash path: %s", urlValue)
	}
}

func TestResolveOptionalIntErrorPaths(t *testing.T) {
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

	if _, err := buildRequestURL(""); err == nil {
		t.Fatal("expected empty base URL error")
	}
	if _, err := buildRequestURL("http:///"); err == nil {
		t.Fatal("expected invalid host error")
	}
}
