// Package tui holds the Bubble Tea model that drives logwiz's UI.
//
// The model owns the rolling log buffer, the chat conversation state,
// and the two sub-widgets (log viewport + chat sidebar). All state
// transitions happen on the Bubble Tea event loop so no locking is
// required.
package tui

import (
	"context"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/config"
)

// SourceKind is a label shown in the status bar.
type SourceKind string

const (
	SourceStdin SourceKind = "stdin"
	SourceFile  SourceKind = "file"
)

type focusArea int

const (
	focusLogs focusArea = iota
	focusChat
)

// Model is the root Bubble Tea model.
type Model struct {
	cfg        config.Config
	sourceKind SourceKind

	buf *buffer.Buffer
	eof bool

	logs       LogViewport
	autoScroll bool

	sidebarOpen bool
	convo       viewport.Model // chat conversation viewport
	input       textarea.Model
	chatMsgs    []chat.Message // excludes system prompt
	chatLoading bool
	chatErr     string

	focus focusArea

	width, height int

	help bool

	chatClient *chat.Client

	// ctx controls in-flight chat requests; cancelled on quit.
	ctx    context.Context
	cancel context.CancelFunc
}

// New builds a ready-to-run Model.
func New(cfg config.Config, kind SourceKind, buf *buffer.Buffer, client *chat.Client) Model {
	ta := textarea.New()
	ta.Placeholder = "Ask a question about the logs..."
	ta.Prompt = ""
	ta.CharLimit = 2000
	ta.ShowLineNumbers = false
	ta.SetHeight(3)

	ctx, cancel := context.WithCancel(context.Background())

	m := Model{
		cfg:        cfg,
		sourceKind: kind,
		buf:        buf,
		logs:       newLogViewport(),
		convo:      viewport.New(0, 0),
		autoScroll: true,
		input:      ta,
		focus:      focusLogs,
		chatClient: client,
		ctx:        ctx,
		cancel:     cancel,
	}
	return m
}

// Init is required by tea.Model.
func (m Model) Init() tea.Cmd {
	return textarea.Blink
}

// Context returns the cancelable context used for chat requests so
// the runner can cancel it on program exit.
func (m *Model) Context() context.Context { return m.ctx }

// Cancel stops any in-flight chat requests.
func (m *Model) Cancel() {
	if m.cancel != nil {
		m.cancel()
	}
}
