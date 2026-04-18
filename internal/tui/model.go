// Package tui holds the Bubble Tea model that drives logwiz's UI.
//
// The model owns the rolling log buffer, the chat conversation state,
// and the two sub-widgets (log viewport + chat sidebar). All state
// transitions happen on the Bubble Tea event loop so no locking is
// required.
package tui

import (
	"context"
	"regexp"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/highlight"
	"github.com/nqbao/logwiz/internal/tools"
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

// MaxToolTurns is the maximum number of tool round-trips per user question.
const MaxToolTurns = 6

// turnActivity records status for one assistant turn (from user question to
// final answer). It is displayed as a dim activity line in the sidebar.
type turnActivity struct {
	Started    time.Time
	FinishedAt time.Time
	ToolCount  int
	LastTool   string // "search_regex" | "read" | "tail" | ""
	LastLabel  string // "searching logs" etc.
	Done       bool
	Errored    bool
}

// Model is the root Bubble Tea model.
type Model struct {
	cfg        config.Config
	sourceKind SourceKind

	buf         *buffer.Buffer
	eof         bool
	highlighter *highlight.Highlighter

	logs LogViewport

	sidebarOpen bool
	convo       viewport.Model // chat conversation viewport
	input       textarea.Model
	chatMsgs    []chat.Message // excludes system prompt
	chatLoading bool
	chatErr     string

	focus focusArea

	width, height int

	help bool

	// Command palette state.
	cmdActive bool
	cmdInput  textinput.Model
	cmdErr    string
	cmdInfo   string // dim informational line shown below the input

	// Search state.
	searchRe      *regexp.Regexp
	searchMatches []int
	searchCursor  int

	// Filter state (orthogonal to search: filter hides, search highlights).
	// Each :grep / :grep -v appends a rule; all rules are ANDed together.
	// filterEnabled can be toggled with 'f' without losing the rules.
	filterRules   []filterRule
	filterIndices []int
	filterEnabled bool

	chatClient *chat.Client
	toolsReg   *tools.Registry

	// Agent loop state.
	turns        []turnActivity
	turnIdxByMsg map[int]int // chatMsgs index -> turns index (final assistant only)
	activeTurn   int         // index into turns; -1 when idle
	toolBudget   int         // remaining tool round-trips in current question
	spinnerTick  int

	hintIdx int // cycles through inputHints

	// sessionSample is the one-shot log snapshot captured on first submit.
	sessionSample []string

	// Agent command state.
	agentCmdContext string // cached log sample for agent command routing; cleared on clearChat
	agentCmdLoading bool   // true while an agent command LLM call is in flight

	// ctx controls in-flight chat requests; cancelled on quit.
	ctx    context.Context
	cancel context.CancelFunc
}

// inputHints cycles through the textarea placeholder.
var inputHints = []string{
	"Ask a question about the logs...",
	"Type /clear to reset the chat",
	"e.g. what errors occurred recently?",
	"e.g. summarize the last 50 lines",
}

// New builds a ready-to-run Model.
func New(cfg config.Config, kind SourceKind, buf *buffer.Buffer, client *chat.Client, reg *tools.Registry) Model {
	ta := textarea.New()
	ta.Placeholder = inputHints[0]
	ta.Prompt = ""
	ta.CharLimit = 2000
	ta.ShowLineNumbers = false
	ta.SetHeight(3)

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = ":command  /command  or describe what you want"
	ci.CharLimit = 200

	ctx, cancel := context.WithCancel(context.Background())

	h := highlight.New()
	h.Register("json", highlight.JSONRenderer)

	lv := newLogViewport(buf)
	lv.h = h

	m := Model{
		cfg:          cfg,
		sourceKind:   kind,
		buf:          buf,
		highlighter:  h,
		logs:         lv,
		convo:        viewport.New(0, 0),
		input:        ta,
		cmdInput:     ci,
		focus:        focusLogs,
		chatClient:   client,
		toolsReg:     reg,
		turnIdxByMsg: make(map[int]int),
		activeTurn:   -1,
		ctx:          ctx,
		cancel:       cancel,
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
