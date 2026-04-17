// Package config resolves logwiz configuration from CLI flags and
// environment variables. Flag values override env vars; env vars
// override built-in defaults.
package config

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Defaults.
const (
	DefaultBaseURL      = "https://api.openai.com/v1"
	DefaultModel        = "gpt-4o-mini"
	DefaultMaxLines     = 50000
	DefaultContextLines = 10
	DefaultSidebarWidth = 50
	DefaultSystemPrompt = "You are logwiz, an assistant analyzing terminal logs. You have access to tools: tail(n), read(start, n), count_regex(pattern, case_insensitive, since_line), search_regex(pattern, max, case_insensitive, since_line). Use count_regex first to assess scale cheaply, then search_regex to fetch matching lines. Use them to fetch the log data you need — do not guess. Indexes are absolute line numbers in the current buffer (0 = oldest). Cite line numbers when giving findings. Keep answers concise."
	DefaultFormat       = "json"
)

// Env var names.
const (
	EnvAPIKey  = "OPENAI_API_KEY"
	EnvBaseURL = "OPENAI_BASE_URL"
	EnvModel   = "OPENAI_MODEL"
)

// Config holds resolved settings for a logwiz run.
type Config struct {
	APIKey       string
	BaseURL      string
	Model        string
	MaxLines     int
	ContextLines int
	SidebarWidth int
	SystemPrompt string

	// InstructionsFile is a path to a file whose content is appended to
	// SystemPrompt at runtime.
	InstructionsFile string

	// Instructions holds the content of InstructionsFile after parsing.
	// It is not settable via flags; it is derived from InstructionsFile.
	Instructions string

	// File is the path to read when not reading from stdin. Empty
	// means stdin mode.
	File string

	// Format selects the line parser: "json" or "passthrough".
	Format string
}

// fileConfig holds the subset of Config that can be set in
// ~/.logwiz/config.json.
type fileConfig struct {
	OpenAIAPIKey  string `json:"openai_api_key"`
	OpenAIBaseURL string `json:"openai_base_url"`
	OpenAIModel   string `json:"openai_model"`
}

// loadFileConfig reads ~/.logwiz/config.json. Missing file is not an
// error; any other problem is returned.
func loadFileConfig() (fileConfig, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return fileConfig{}, nil
	}
	data, err := os.ReadFile(filepath.Join(home, ".logwiz", "config.json"))
	if os.IsNotExist(err) {
		return fileConfig{}, nil
	}
	if err != nil {
		return fileConfig{}, fmt.Errorf("reading ~/.logwiz/config.json: %w", err)
	}
	var fc fileConfig
	if err := json.Unmarshal(data, &fc); err != nil {
		return fileConfig{}, fmt.Errorf("parsing ~/.logwiz/config.json: %w", err)
	}
	return fc, nil
}

// Parse parses args (not including the program name) and the given
// environment lookup function, returning the resolved Config. out is
// used for usage / error output.
//
// Resolution order: flag > env var > ~/.logwiz/config.json > default.
// envLookup mirrors os.LookupEnv; pass os.LookupEnv in production.
func Parse(args []string, envLookup func(string) (string, bool), out io.Writer) (Config, error) {
	fc, err := loadFileConfig()
	if err != nil {
		fmt.Fprintln(out, "logwiz: warning:", err)
	}

	fs := flag.NewFlagSet("logwiz", flag.ContinueOnError)
	fs.SetOutput(out)

	fileOr := func(fileVal, fallback string) string {
		if fileVal != "" {
			return fileVal
		}
		return fallback
	}

	envOr := func(key, fallback string) string {
		if v, ok := envLookup(key); ok && v != "" {
			return v
		}
		return fallback
	}

	// env > file > default
	defaultAPIKey := envOr(EnvAPIKey, fileOr(fc.OpenAIAPIKey, ""))
	defaultBaseURL := envOr(EnvBaseURL, fileOr(fc.OpenAIBaseURL, DefaultBaseURL))
	defaultModel := envOr(EnvModel, fileOr(fc.OpenAIModel, DefaultModel))

	var cfg Config
	fs.StringVar(&cfg.APIKey, "api-key", defaultAPIKey,
		"API key for the OpenAI-compatible endpoint (env OPENAI_API_KEY, ~/.logwiz/config.json)")
	fs.StringVar(&cfg.BaseURL, "base-url", defaultBaseURL,
		"Base URL for the chat completions endpoint (env OPENAI_BASE_URL, ~/.logwiz/config.json)")
	fs.StringVar(&cfg.Model, "model", defaultModel,
		"Model name (env OPENAI_MODEL, ~/.logwiz/config.json)")
	fs.IntVar(&cfg.MaxLines, "max-lines", DefaultMaxLines,
		"Rolling buffer size in lines")
	fs.IntVar(&cfg.ContextLines, "context-lines", DefaultContextLines,
		"Recent lines sampled once at chat session start; 0 disables")
	fs.IntVar(&cfg.SidebarWidth, "sidebar-width", DefaultSidebarWidth,
		"Chat sidebar width in columns")
	fs.StringVar(&cfg.SystemPrompt, "system-prompt", DefaultSystemPrompt,
		"System prompt sent with each chat request")
	fs.StringVar(&cfg.InstructionsFile, "instructions-file", "",
		"Path to a file with extra instructions appended to the system prompt")
	fs.StringVar(&cfg.Format, "format", DefaultFormat,
		`Line parser: "json" detects and colours JSON lines, "passthrough" renders everything as plain text`)

	fs.Usage = func() {
		fmt.Fprintln(out, "Usage: logwiz [flags] [file]")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Read logs from stdin (piped) or from [file] and open an interactive")
		fmt.Fprintln(out, "viewer with an optional chat sidebar backed by an OpenAI-compatible")
		fmt.Fprintln(out, "chat completions endpoint.")
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Flags:")
		fs.PrintDefaults()
	}

	if err := fs.Parse(args); err != nil {
		return Config{}, err
	}

	rest := fs.Args()
	switch len(rest) {
	case 0:
		// stdin mode
	case 1:
		cfg.File = rest[0]
	default:
		return Config{}, fmt.Errorf("expected at most one positional argument (file), got %d", len(rest))
	}

	cfg.BaseURL = strings.TrimRight(cfg.BaseURL, "/")

	if cfg.MaxLines < 1 {
		return Config{}, errors.New("--max-lines must be >= 1")
	}
	if cfg.ContextLines < 0 {
		return Config{}, errors.New("--context-lines must be >= 0")
	}
	if cfg.SidebarWidth < 10 {
		return Config{}, errors.New("--sidebar-width must be >= 10")
	}
	if cfg.Format != "json" && cfg.Format != "passthrough" {
		return Config{}, errors.New(`--format must be "json" or "passthrough"`)
	}

	if cfg.InstructionsFile != "" {
		data, err := os.ReadFile(cfg.InstructionsFile)
		if err != nil {
			return Config{}, fmt.Errorf("--instructions-file: %w", err)
		}
		cfg.Instructions = strings.TrimRight(string(data), "\n")
	}

	return cfg, nil
}

// ParseOSArgs is a convenience wrapper around Parse that uses
// os.Args[1:], os.LookupEnv, and os.Stderr.
func ParseOSArgs() (Config, error) {
	return Parse(os.Args[1:], os.LookupEnv, os.Stderr)
}

// EffectiveSystemPrompt returns the system prompt with any
// instructions-file content appended.
func (c Config) EffectiveSystemPrompt() string {
	if c.Instructions == "" {
		return c.SystemPrompt
	}
	return c.SystemPrompt + "\n\n" + c.Instructions
}
