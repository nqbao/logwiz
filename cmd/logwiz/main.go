// Command logwiz is a tee-like log viewer with an integrated chat
// sidebar backed by an OpenAI-compatible endpoint.
//
// Usage:
//
//	some_command 2>&1 | logwiz
//	logwiz server.log
package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/mattn/go-isatty"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/source"
	"github.com/nqbao/logwiz/internal/tui"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "logwiz:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.ParseOSArgs()
	if err != nil {
		return err
	}

	// Decide source: explicit file > stdin (if piped). Reject if neither.
	var (
		src  source.Source
		kind tui.SourceKind
	)
	switch {
	case cfg.File != "":
		src = &source.File{Path: cfg.File}
		kind = tui.SourceFile
	case !isatty.IsTerminal(os.Stdin.Fd()):
		src = &source.Stdin{R: os.Stdin}
		kind = tui.SourceStdin
	default:
		fmt.Fprintln(os.Stderr, "logwiz: no input — pipe logs in or pass a file")
		fmt.Fprintln(os.Stderr, "usage: logwiz [flags] [file]")
		os.Exit(2)
	}

	buf := buffer.New(cfg.MaxLines)

	var client *chat.Client
	if cfg.APIKey != "" {
		client = chat.New(cfg.BaseURL, cfg.APIKey, cfg.Model)
	}

	model := tui.New(cfg, kind, buf, client)

	p := tea.NewProgram(model, tea.WithAltScreen())

	// Start source reader; it will Send messages into the program.
	// An adapter keeps the source package free of a tea dependency.
	src.Start(model.Context(), senderAdapter{p})

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("tui: %w", err)
	}
	return nil
}

// senderAdapter bridges *tea.Program (Send(tea.Msg)) to source.Sender
// (Send(any)). tea.Msg is defined as interface{} so the cast is a
// no-op at runtime.
type senderAdapter struct{ p *tea.Program }

func (s senderAdapter) Send(msg any) { s.p.Send(msg) }
