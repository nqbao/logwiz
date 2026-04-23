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
	"strconv"
	"strings"
)

// Defaults.
const (
	DefaultBaseURL          = "https://api.openai.com/v1"
	DefaultModel            = "gpt-4o-mini"
	DefaultMaxLines         = 50000
	DefaultContextLines     = 10
	DefaultSidebarWidth     = 50
	DefaultCmdHistoryTurns  = 20
	DefaultSystemPrompt     = "You are logwiz, an assistant analyzing terminal logs. You have access to tools: tail(n), read(start, n), count_regex(pattern, case_insensitive, since_line), search_regex(pattern, max, case_insensitive, since_line). Use count_regex first to assess scale cheaply, then search_regex to fetch matching lines. Use them to fetch the log data you need — do not guess. Indexes are absolute line numbers in the current buffer (0 = oldest). Cite line numbers when giving findings. Keep answers concise."
	DefaultFormat           = "auto"
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

	// LogFile is the path to write internal debug logs to. Empty disables logging.
	LogFile string

	// Format selects the line parser: "json" or "passthrough".
	Format string

	// Pretty enables humanlog-style rendering (time LEVEL message key=val …)
	// for json and logfmt lines instead of raw syntax-coloured output.
	Pretty bool

	// RedactPatterns is a list of Go regex patterns. Any match in a log
	// line is replaced with [REDACTED] before the line is sent to the LLM.
	// The display layer is never affected.
	RedactPatterns []string

	// CmdHistoryTurns is the maximum number of palette Q&A pairs carried
	// across invocations for conversational follow-ups.
	CmdHistoryTurns int

	// ThinkingBudget is the token budget for extended thinking (Anthropic
	// models only). 0 disables thinking; values >= 1024 enable it.
	ThinkingBudget int

	// ReasoningEffort selects the reasoning level for OpenAI/Minimax-style
	// providers. Accepted values: "" (disabled), "low", "medium", "high".
	ReasoningEffort string
}

// ConfigurableKeys lists the keys accepted by ApplyKey and SaveKey.
var ConfigurableKeys = []string{
	"model", "api-key", "base-url", "sidebar-width", "context-lines", "system-prompt", "thinking-budget", "reasoning-effort", "pretty",
}

// fileConfig holds the subset of Config that can be set in
// ~/.logwiz/config.json.
type fileConfig struct {
	OpenAIAPIKey   string   `json:"openai_api_key,omitempty"`
	OpenAIBaseURL  string   `json:"openai_base_url,omitempty"`
	OpenAIModel    string   `json:"openai_model,omitempty"`
	SidebarWidth   int      `json:"sidebar_width,omitempty"`
	ContextLines   *int     `json:"context_lines,omitempty"`
	SystemPrompt   string   `json:"system_prompt,omitempty"`
	RedactPatterns []string `json:"redact_patterns,omitempty"`
	CmdHistoryTurns int    `json:"cmd_history_turns,omitempty"`
	ThinkingBudget  int    `json:"thinking_budget,omitempty"`
	ReasoningEffort string `json:"reasoning_effort,omitempty"`
	Pretty          bool   `json:"pretty,omitempty"`
}

// stringSliceFlag implements flag.Value for a repeatable --redact flag.
type stringSliceFlag []string

func (f *stringSliceFlag) String() string        { return strings.Join(*f, ",") }
func (f *stringSliceFlag) Set(v string) error    { *f = append(*f, v); return nil }

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

	defaultSidebarWidth := DefaultSidebarWidth
	if fc.SidebarWidth >= 10 {
		defaultSidebarWidth = fc.SidebarWidth
	}
	defaultContextLines := DefaultContextLines
	if fc.ContextLines != nil {
		defaultContextLines = *fc.ContextLines
	}
	defaultSystemPrompt := DefaultSystemPrompt
	if fc.SystemPrompt != "" {
		defaultSystemPrompt = fc.SystemPrompt
	}
	defaultCmdHistoryTurns := DefaultCmdHistoryTurns
	if fc.CmdHistoryTurns > 0 {
		defaultCmdHistoryTurns = fc.CmdHistoryTurns
	}
	defaultThinkingBudget := fc.ThinkingBudget
	defaultReasoningEffort := fc.ReasoningEffort
	defaultPretty := fc.Pretty

	var (
		cfg           Config
		redactFlag    stringSliceFlag = append(stringSliceFlag(nil), fc.RedactPatterns...)
	)
	fs.Var(&redactFlag, "redact",
		"Go regex; matching spans are replaced with [REDACTED] before sending to LLM (repeatable)")
	fs.StringVar(&cfg.APIKey, "api-key", defaultAPIKey,
		"API key for the OpenAI-compatible endpoint (env OPENAI_API_KEY, ~/.logwiz/config.json)")
	fs.StringVar(&cfg.BaseURL, "base-url", defaultBaseURL,
		"Base URL for the chat completions endpoint (env OPENAI_BASE_URL, ~/.logwiz/config.json)")
	fs.StringVar(&cfg.Model, "model", defaultModel,
		"Model name (env OPENAI_MODEL, ~/.logwiz/config.json)")
	fs.IntVar(&cfg.MaxLines, "max-lines", DefaultMaxLines,
		"Rolling buffer size in lines")
	fs.IntVar(&cfg.ContextLines, "context-lines", defaultContextLines,
		"Recent lines sampled once at chat session start; 0 disables")
	fs.IntVar(&cfg.SidebarWidth, "sidebar-width", defaultSidebarWidth,
		"Chat sidebar width in columns")
	fs.StringVar(&cfg.SystemPrompt, "system-prompt", defaultSystemPrompt,
		"System prompt sent with each chat request")
	fs.StringVar(&cfg.InstructionsFile, "instructions-file", "",
		"Path to a file with extra instructions appended to the system prompt")
	fs.StringVar(&cfg.Format, "format", DefaultFormat,
		`Line parser: "auto" (default, detects per line), "json", "logfmt", or "passthrough"`)
	fs.BoolVar(&cfg.Pretty, "pretty", defaultPretty,
		"Enable pretty mode: render json/logfmt lines as \"time LEVEL message key=val …\" (toggle with P inside the viewer, persist with :config pretty on/off)")
	fs.StringVar(&cfg.LogFile, "log-file", "",
		"Write internal debug logs to `file`")
	fs.IntVar(&cfg.CmdHistoryTurns, "cmd-history-turns", defaultCmdHistoryTurns,
		"Maximum palette Q&A pairs carried across invocations for follow-up questions")
	fs.IntVar(&cfg.ThinkingBudget, "thinking-budget", defaultThinkingBudget,
		"Token budget for extended thinking (Anthropic models only); 0 disables, minimum 1024 when enabled")
	fs.StringVar(&cfg.ReasoningEffort, "reasoning-effort", defaultReasoningEffort,
		`Reasoning level for OpenAI/Minimax-style providers: "low", "medium", "high", or "" to disable`)

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

	cfg.RedactPatterns = []string(redactFlag)
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
	switch cfg.Format {
	case "auto", "json", "logfmt", "passthrough":
	default:
		return Config{}, errors.New(`--format must be "auto", "json", "logfmt", or "passthrough"`)
	}
	if cfg.ThinkingBudget != 0 && cfg.ThinkingBudget < 1024 {
		return Config{}, errors.New("--thinking-budget must be 0 (disabled) or >= 1024")
	}
	if cfg.ReasoningEffort != "" && cfg.ReasoningEffort != "low" && cfg.ReasoningEffort != "medium" && cfg.ReasoningEffort != "high" {
		return Config{}, errors.New(`--reasoning-effort must be "low", "medium", or "high"`)
	}
	if cfg.ThinkingBudget > 0 && cfg.ReasoningEffort != "" {
		return Config{}, errors.New("--thinking-budget and --reasoning-effort are mutually exclusive")
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

// saveFileConfig writes fc to ~/.logwiz/config.json, creating the
// directory if needed.
func saveFileConfig(fc fileConfig) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	dir := filepath.Join(home, ".logwiz")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("creating ~/.logwiz: %w", err)
	}
	data, err := json.MarshalIndent(fc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "config.json"), append(data, '\n'), 0600)
}

// ApplyKey validates and applies a key=value pair to cfg in place.
// It returns an error for unknown keys or invalid values.
func ApplyKey(cfg *Config, key, value string) error {
	switch key {
	case "model":
		cfg.Model = value
	case "api-key":
		cfg.APIKey = value
	case "base-url":
		cfg.BaseURL = strings.TrimRight(value, "/")
	case "sidebar-width":
		n, err := strconv.Atoi(value)
		if err != nil || n < 10 {
			return fmt.Errorf("sidebar-width must be an integer >= 10")
		}
		cfg.SidebarWidth = n
	case "context-lines":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("context-lines must be an integer >= 0")
		}
		cfg.ContextLines = n
	case "system-prompt":
		cfg.SystemPrompt = value
	case "thinking-budget":
		n, err := strconv.Atoi(value)
		if err != nil || (n != 0 && n < 1024) {
			return fmt.Errorf("thinking-budget must be 0 (disabled) or an integer >= 1024")
		}
		cfg.ThinkingBudget = n
		if n > 0 {
			cfg.ReasoningEffort = ""
		}
	case "reasoning-effort":
		if value == "off" {
			value = ""
		}
		if value != "" && value != "low" && value != "medium" && value != "high" {
			return fmt.Errorf(`reasoning-effort must be "low", "medium", "high", or "off" to disable`)
		}
		cfg.ReasoningEffort = value
		if value != "" {
			cfg.ThinkingBudget = 0
		}
	case "pretty":
		switch strings.ToLower(value) {
		case "on", "true", "1":
			cfg.Pretty = true
		case "off", "false", "0":
			cfg.Pretty = false
		default:
			return fmt.Errorf(`pretty must be "on" or "off"`)
		}
	case "cmd-history-turns":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 {
			return fmt.Errorf("cmd-history-turns must be an integer >= 1")
		}
		cfg.CmdHistoryTurns = n
	default:
		return fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(ConfigurableKeys, ", "))
	}
	return nil
}

// SaveKey persists a single key=value to ~/.logwiz/config.json,
// merging with any existing values already in the file.
// Caller should have already validated the value via ApplyKey.
func SaveKey(key, value string) error {
	fc, err := loadFileConfig()
	if err != nil {
		return err
	}
	switch key {
	case "model":
		fc.OpenAIModel = value
	case "api-key":
		fc.OpenAIAPIKey = value
	case "base-url":
		fc.OpenAIBaseURL = strings.TrimRight(value, "/")
	case "sidebar-width":
		n, _ := strconv.Atoi(value)
		fc.SidebarWidth = n
	case "context-lines":
		n, _ := strconv.Atoi(value)
		fc.ContextLines = &n
	case "system-prompt":
		fc.SystemPrompt = value
	case "thinking-budget":
		n, _ := strconv.Atoi(value)
		fc.ThinkingBudget = n
		if n > 0 {
			fc.ReasoningEffort = ""
		}
	case "reasoning-effort":
		if value == "off" {
			value = ""
		}
		fc.ReasoningEffort = value
		if value != "" {
			fc.ThinkingBudget = 0
		}
	case "pretty":
		fc.Pretty = strings.ToLower(value) == "on" || strings.ToLower(value) == "true" || value == "1"
	case "cmd-history-turns":
		n, _ := strconv.Atoi(value)
		fc.CmdHistoryTurns = n
	default:
		return fmt.Errorf("unknown key %q", key)
	}
	return saveFileConfig(fc)
}

// EffectiveSystemPrompt returns the system prompt with any
// instructions-file content appended.
func (c Config) EffectiveSystemPrompt() string {
	if c.Instructions == "" {
		return c.SystemPrompt
	}
	return c.SystemPrompt + "\n\n" + c.Instructions
}
