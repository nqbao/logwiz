package config

import (
	"io"
	"testing"
)

func envFromMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestParseDefaults(t *testing.T) {
	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.BaseURL != DefaultBaseURL {
		t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, DefaultBaseURL)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
	}
	if cfg.APIKey != "" {
		t.Errorf("APIKey = %q, want empty", cfg.APIKey)
	}
	if cfg.MaxLines != DefaultMaxLines {
		t.Errorf("MaxLines = %d, want %d", cfg.MaxLines, DefaultMaxLines)
	}
	if cfg.File != "" {
		t.Errorf("File = %q, want empty", cfg.File)
	}
}

func TestParseEnvFallback(t *testing.T) {
	env := envFromMap(map[string]string{
		EnvAPIKey:  "sk-env",
		EnvBaseURL: "https://env.example/v1",
		EnvModel:   "env-model",
	})
	cfg, err := Parse(nil, env, io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.APIKey != "sk-env" {
		t.Errorf("APIKey = %q", cfg.APIKey)
	}
	if cfg.BaseURL != "https://env.example/v1" {
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "env-model" {
		t.Errorf("Model = %q", cfg.Model)
	}
}

func TestParseFlagOverridesEnv(t *testing.T) {
	env := envFromMap(map[string]string{
		EnvAPIKey:  "sk-env",
		EnvBaseURL: "https://env.example/v1",
		EnvModel:   "env-model",
	})
	args := []string{
		"--api-key", "sk-flag",
		"--base-url", "https://flag.example/v1/",
		"--model", "flag-model",
	}
	cfg, err := Parse(args, env, io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.APIKey != "sk-flag" {
		t.Errorf("APIKey = %q, want sk-flag", cfg.APIKey)
	}
	if cfg.BaseURL != "https://flag.example/v1" { // trailing slash stripped
		t.Errorf("BaseURL = %q", cfg.BaseURL)
	}
	if cfg.Model != "flag-model" {
		t.Errorf("Model = %q", cfg.Model)
	}
}

func TestParsePositionalFile(t *testing.T) {
	cfg, err := Parse([]string{"server.log"}, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.File != "server.log" {
		t.Errorf("File = %q", cfg.File)
	}
}

func TestParseTooManyPositionals(t *testing.T) {
	_, err := Parse([]string{"a.log", "b.log"}, envFromMap(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error for two positional args")
	}
}

func TestParseInvalidNumbers(t *testing.T) {
	cases := [][]string{
		{"--max-lines", "0"},
		{"--context-lines", "0"},
		{"--sidebar-width", "5"},
	}
	for _, args := range cases {
		if _, err := Parse(args, envFromMap(nil), io.Discard); err == nil {
			t.Errorf("Parse(%v) = nil err, want error", args)
		}
	}
}

func TestParseEmptyEnvTreatedAsUnset(t *testing.T) {
	// An env var present but empty should fall back to the default.
	env := envFromMap(map[string]string{EnvModel: ""})
	cfg, err := Parse(nil, env, io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Model != DefaultModel {
		t.Errorf("Model = %q, want %q", cfg.Model, DefaultModel)
	}
}
