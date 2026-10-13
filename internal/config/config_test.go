package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func envFromMap(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestParseDefaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())
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
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse([]string{"server.log"}, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.File != "server.log" {
		t.Errorf("File = %q", cfg.File)
	}
}

func TestParseManagedCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse([]string{"--max-lines", "123", "--", "bash", "-lc", "echo hi"}, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.File != "" {
		t.Errorf("File = %q, want empty", cfg.File)
	}
	if got, want := cfg.Command, []string{"bash", "-lc", "echo hi"}; len(got) != len(want) {
		t.Fatalf("Command len = %d, want %d (%v)", len(got), len(want), got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("Command[%d] = %q, want %q (full=%v)", i, got[i], want[i], got)
			}
		}
	}
	if cfg.MaxLines != 123 {
		t.Errorf("MaxLines = %d, want 123", cfg.MaxLines)
	}
}

func TestParseRejectsFileAndManagedCommand(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"server.log", "--", "echo", "hi"}, envFromMap(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error for file + managed command")
	}
}

func TestParseRejectsBareDoubleDash(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"--"}, envFromMap(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error for bare --")
	}
}

func TestParseTooManyPositionals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"a.log", "b.log"}, envFromMap(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error for two positional args")
	}
}

func TestParseInvalidNumbers(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := [][]string{
		{"--max-lines", "0"},
		{"--sidebar-width", "5"},
	}
	for _, args := range cases {
		if _, err := Parse(args, envFromMap(nil), io.Discard); err == nil {
			t.Errorf("Parse(%v) = nil err, want error", args)
		}
	}
}

func TestLoadFileConfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logwiz")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{
		"openai_api_key": "sk-file",
		"openai_base_url": "https://file.example/v1",
		"openai_model": "file-model"
	}`), 0600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOME", dir)
	fc, err := loadFileConfig()
	if err != nil {
		t.Fatalf("loadFileConfig: %v", err)
	}
	if fc.OpenAIAPIKey != "sk-file" {
		t.Errorf("OpenAIAPIKey = %q, want sk-file", fc.OpenAIAPIKey)
	}
	if fc.OpenAIBaseURL != "https://file.example/v1" {
		t.Errorf("OpenAIBaseURL = %q", fc.OpenAIBaseURL)
	}
	if fc.OpenAIModel != "file-model" {
		t.Errorf("OpenAIModel = %q", fc.OpenAIModel)
	}
}

func TestParseUsesFileConfig(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logwiz")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{
		"openai_api_key": "sk-file",
		"openai_base_url": "https://file.example/v1",
		"openai_model": "file-model"
	}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.APIKey != "sk-file" {
		t.Errorf("APIKey = %q, want sk-file", cfg.APIKey)
	}
	if cfg.BaseURL != "https://file.example/v1" {
		t.Errorf("BaseURL = %q, want https://file.example/v1", cfg.BaseURL)
	}
	if cfg.Model != "file-model" {
		t.Errorf("Model = %q, want file-model", cfg.Model)
	}
}

func TestFileConfigBelowEnv(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logwiz")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "config.json"), []byte(`{"openai_api_key":"sk-file"}`), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	env := envFromMap(map[string]string{EnvAPIKey: "sk-env"})
	cfg, err := Parse(nil, env, io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.APIKey != "sk-env" {
		t.Errorf("APIKey = %q, want sk-env (env should beat file)", cfg.APIKey)
	}
}

func TestParseEmptyEnvTreatedAsUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
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

func TestParseFormat(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, format := range []string{"auto", "json", "logfmt", "passthrough"} {
		cfg, err := Parse([]string{"--format", format}, envFromMap(nil), io.Discard)
		if err != nil {
			t.Errorf("--format %s: unexpected error: %v", format, err)
			continue
		}
		if cfg.Format != format {
			t.Errorf("--format %s: got %q", format, cfg.Format)
		}
	}
}

func TestParseFormatDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Format != "auto" {
		t.Errorf("default Format = %q, want \"auto\"", cfg.Format)
	}
}

func TestParseFormatInvalid(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := Parse([]string{"--format", "bogus"}, envFromMap(nil), io.Discard)
	if err == nil {
		t.Fatal("expected error for --format bogus")
	}
}

func TestParseViewFlag(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, mode := range []string{"raw", "color", "pretty"} {
		cfg, err := Parse([]string{"--view", mode}, envFromMap(nil), io.Discard)
		if err != nil {
			t.Fatalf("Parse --view %s: %v", mode, err)
		}
		if cfg.View != mode {
			t.Errorf("cfg.View should be %q when --view %s flag is set, got %q", mode, mode, cfg.View)
		}
	}
}

func TestParseViewDefaultRaw(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.View != "" {
		t.Errorf("cfg.View should be empty (raw) by default, got %q", cfg.View)
	}
}

func TestParseLoadsMemory(t *testing.T) {
	dir := t.TempDir()
	cfgDir := filepath.Join(dir, ".logwiz")
	if err := os.MkdirAll(cfgDir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "memory.md"), []byte("- /healthz is noise\n\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", dir)

	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Memory != "- /healthz is noise" {
		t.Errorf("Memory = %q", cfg.Memory)
	}
	p := cfg.EffectiveSystemPrompt()
	if !strings.HasPrefix(p, cfg.SystemPrompt) {
		t.Errorf("prompt does not start with base system prompt: %q", p)
	}
	if !strings.HasSuffix(p, "## Memory\nFacts the user asked you to remember. Treat as background context; the logs take precedence if they disagree.\n\n- /healthz is noise") {
		t.Errorf("prompt missing memory section: %q", p)
	}
}

func TestParseMissingMemory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cfg, err := Parse(nil, envFromMap(nil), io.Discard)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Memory != "" || cfg.MemorySection() != "" {
		t.Errorf("expected no memory, got %q", cfg.Memory)
	}
	if cfg.EffectiveSystemPrompt() != cfg.SystemPrompt {
		t.Errorf("prompt changed without memory: %q", cfg.EffectiveSystemPrompt())
	}
}
