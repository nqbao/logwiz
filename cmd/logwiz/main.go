// Command logwiz is a tee-like log viewer with an integrated chat
// sidebar backed by an OpenAI-compatible endpoint.
//
// Usage:
//
//	some_command 2>&1 | logwiz
//	logwiz server.log
package main

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/redact"
	"github.com/nqbao/logwiz/internal/source"
	"github.com/nqbao/logwiz/internal/tools"
	"github.com/nqbao/logwiz/internal/tui"
)

// Version is set at build time via ldflags:
//
//	go build -ldflags "-X main.Version=$(VERSION)"
var Version = "dev"

var errNoInput = errors.New("no input")

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("logwiz", Version)
		return
	}

	if len(os.Args) >= 2 && os.Args[1] == "setup" {
		if err := config.Setup(); err != nil {
			fmt.Fprintln(os.Stderr, "logwiz:", err)
			os.Exit(1)
		}
		return
	}

	pgid, pgidErr := syscall.Getpgid(0)

	killPipelineGroup, err := run()
	if err != nil {
		if errors.Is(err, errNoInput) {
			os.Exit(2)
		}
		fmt.Fprintln(os.Stderr, "logwiz:", err)
		os.Exit(1)
	}

	if killPipelineGroup && pgidErr == nil {
		// Kill the pipeline process group so upstream processes (e.g.
		// `start_api.sh | logwiz`, `modd | logwiz`) exit when the user
		// quits logwiz. We send SIGINT (not SIGTERM) because supervisors
		// like modd put their children in separate process groups and
		// only run their cleanup logic on SIGINT — SIGTERM hits the
		// default-terminate path and leaves the grandchildren orphaned.
		signal.Ignore(syscall.SIGINT, syscall.SIGTERM)
		_ = syscall.Kill(-pgid, syscall.SIGINT)
	}
}

func run() (bool, error) {
	cfg, err := config.ParseOSArgs()
	if err != nil {
		return false, err
	}

	if cfg.LogFile != "" {
		f, err := os.OpenFile(cfg.LogFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			return false, fmt.Errorf("--log-file: %w", err)
		}
		defer f.Close()
		slog.SetDefault(slog.New(slog.NewTextHandler(f, &slog.HandlerOptions{Level: slog.LevelDebug})))
		slog.Info("logwiz started", "log_file", cfg.LogFile)
	} else {
		// No log file: discard everything so slog calls are no-ops.
		slog.SetDefault(slog.New(slog.DiscardHandler))
	}

	// Decide source: explicit file > stdin (if piped). Reject if neither.
	var parser logmsg.Parser
	switch cfg.Format {
	case "json":
		parser = logmsg.JSONParser{}
	case "logfmt":
		parser = logmsg.LogfmtParser{}
	case "pipe":
		parser = &logmsg.PipeParser{Strict: true}
	case "auto":
		parser = &logmsg.AutoParser{}
	default:
		parser = logmsg.PassthroughParser{}
	}
	var (
		src        source.Source
		kind       tui.SourceKind
		fileFollow source.FileFollower
	)
	if cfg.File != "" {
		if _, err := os.Stat(cfg.File); err != nil {
			return false, fmt.Errorf("open %s: %w", cfg.File, err)
		}
	}

	killPipelineGroup := false
	switch {
	case len(cfg.Command) > 0:
		src = &source.Command{Args: cfg.Command, Parser: parser}
		kind = tui.SourceCommand
	case cfg.File != "":
		f := &source.File{Path: cfg.File, Parser: parser, MaxLines: cfg.MaxLines, Follow: true}
		src = f
		fileFollow = f
		kind = tui.SourceFile
	case !isatty.IsTerminal(os.Stdin.Fd()):
		src = &source.Stdin{R: os.Stdin, Parser: parser}
		kind = tui.SourceStdin
		killPipelineGroup = true
	default:
		fmt.Fprintln(os.Stderr, "logwiz: no input — pipe logs in or pass a file")
		fmt.Fprintln(os.Stderr, "usage: logwiz [flags] [file]")
		fmt.Fprintln(os.Stderr, "   or: logwiz [flags] -- <command> [args...]")
		return false, errNoInput
	}

	buf := buffer.New(cfg.MaxLines)

	redactor, err := redact.New(cfg.RedactPatterns)
	if err != nil {
		return false, err
	}

	var client *agent.Client
	if cfg.APIKey != "" {
		client = agent.New(cfg.BaseURL, cfg.APIKey, cfg.Model)
		client.SetThinkingBudget(cfg.ThinkingBudget)
		client.SetReasoningEffort(cfg.ReasoningEffort)
	}

	registry := tools.NewDefault(redactor)
	model := tui.New(cfg, kind, buf, client, registry, redactor, fileFollow)

	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())

	// Start source reader; it will Send messages into the program.
	// An adapter keeps the source package free of a tea dependency.
	src.Start(model.Context(), senderAdapter{p})

	if _, err := p.Run(); err != nil {
		return killPipelineGroup, fmt.Errorf("tui: %w", err)
	}

	return killPipelineGroup, nil
}

// senderAdapter bridges *tea.Program (Send(tea.Msg)) to source.Sender
// (Send(any)). tea.Msg is defined as interface{} so the cast is a
// no-op at runtime.
type senderAdapter struct{ p *tea.Program }

func (s senderAdapter) Send(msg any) { s.p.Send(msg) }
