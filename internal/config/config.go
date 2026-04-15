// Package config resolves logwiz configuration from CLI flags and
// environment variables. Flag values override env vars; env vars
// override built-in defaults.
package config

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Defaults.
const (
	DefaultBaseURL      = "https://api.openai.com/v1"
	DefaultModel        = "gpt-4o-mini"
	DefaultMaxLines     = 5000
	DefaultContextLines = 300
	DefaultSidebarWidth = 50
	DefaultSystemPrompt = "You are helping analyze terminal logs. Be concise and grounded in the provided log text."
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

	// File is the path to read when not reading from stdin. Empty
	// means stdin mode.
	File string
}

// Parse parses args (not including the program name) and the given
// environment lookup function, returning the resolved Config. out is
// used for usage / error output.
//
// envLookup mirrors os.LookupEnv; pass os.LookupEnv in production.
func Parse(args []string, envLookup func(string) (string, bool), out io.Writer) (Config, error) {
	fs := flag.NewFlagSet("logwiz", flag.ContinueOnError)
	fs.SetOutput(out)

	envOr := func(key, fallback string) string {
		if v, ok := envLookup(key); ok && v != "" {
			return v
		}
		return fallback
	}

	var cfg Config
	fs.StringVar(&cfg.APIKey, "api-key", envOr(EnvAPIKey, ""),
		"API key for the OpenAI-compatible endpoint (env OPENAI_API_KEY)")
	fs.StringVar(&cfg.BaseURL, "base-url", envOr(EnvBaseURL, DefaultBaseURL),
		"Base URL for the chat completions endpoint (env OPENAI_BASE_URL)")
	fs.StringVar(&cfg.Model, "model", envOr(EnvModel, DefaultModel),
		"Model name (env OPENAI_MODEL)")
	fs.IntVar(&cfg.MaxLines, "max-lines", DefaultMaxLines,
		"Rolling buffer size in lines")
	fs.IntVar(&cfg.ContextLines, "context-lines", DefaultContextLines,
		"Lines of log context sent per chat turn")
	fs.IntVar(&cfg.SidebarWidth, "sidebar-width", DefaultSidebarWidth,
		"Chat sidebar width in columns")
	fs.StringVar(&cfg.SystemPrompt, "system-prompt", DefaultSystemPrompt,
		"System prompt sent with each chat request")

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
	if cfg.ContextLines < 1 {
		return Config{}, errors.New("--context-lines must be >= 1")
	}
	if cfg.SidebarWidth < 10 {
		return Config{}, errors.New("--sidebar-width must be >= 10")
	}

	return cfg, nil
}

// ParseOSArgs is a convenience wrapper around Parse that uses
// os.Args[1:], os.LookupEnv, and os.Stderr.
func ParseOSArgs() (Config, error) {
	return Parse(os.Args[1:], os.LookupEnv, os.Stderr)
}
