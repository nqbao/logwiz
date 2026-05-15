// Package tui holds the Bubble Tea model that drives logwiz's UI.
//
// The model owns the rolling log buffer, the chat conversation state,
// and the two sub-widgets (log viewport + chat sidebar). All state
// transitions happen on the Bubble Tea event loop so no locking is
// required.
package tui

import (
	"context"
	"math/rand"
	"regexp"
	"time"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/highlight"
	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/redact"
	"github.com/nqbao/logwiz/internal/source"
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

// viewMode controls how log lines are rendered.
type viewMode int

const (
	viewModeRaw    viewMode = iota // pass-through: no syntax coloring
	viewModeColor                  // syntax-colored (chroma/logfmt token colors)
	viewModePretty                 // humanlog-style: time LEVEL message key=val …
)

// MaxToolTurns is the maximum number of tool round-trips per user question.
const MaxToolTurns = 6

// MaxCmdToolTurns is the maximum number of inspection round-trips for a palette command.
const MaxCmdToolTurns = 4

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
	following   bool // true while file watcher is active
	fileFollow  source.FileFollower
	highlighter *highlight.Highlighter

	logs LogViewport

	sidebarOpen bool
	convo       viewport.Model // chat conversation viewport
	input       textarea.Model
	chatMsgs    []agent.Message // excludes system prompt
	chatLoading bool
	chatErr     string

	focus focusArea

	width, height int

	help bool

	quitPending bool

	// Command palette state.
	cmdActive bool
	cmdInput  textinput.Model
	cmdErr    string
	cmdInfo   string // dim informational line shown below the input
	cmdAnswer string // agent answer text (rendered in readable style)

	// Search state.
	searchRe      *regexp.Regexp
	searchMatches []int
	searchCursor  int

	// Filter state (orthogonal to search: filter hides, search highlights).
	// Each :grep / :filter appends a rule; all rules are ANDed together.
	// filterEnabled can be toggled with 'f' without losing the rules.
	// filterTickRunning is true while a 1s reapply tick is in flight for time
	// rules; prevents adding a second time rule from spawning a parallel loop.
	filterRules       []filterRule
	filterIndices     []int
	filterEnabled     bool
	filterTickRunning bool

	// Highlight state: colours matching lines without hiding non-matching ones.
	// Orthogonal to both filter and search; H toggles enabled without losing rules.
	hlRules   []hlRule
	hlMatches [][]int
	hlEnabled bool

	chatClient *agent.Client
	toolsReg   *tools.Registry
	redactor   *redact.Redactor

	// Agent loop state (shared by chat and command agents).
	turns        []turnActivity
	turnIdxByMsg map[int]int // chatMsgs index -> turns index (final assistant only)
	activeTurn   int         // index into turns; -1 when idle
	toolBudget   int         // remaining tool round-trips in current chat turn
	spinnerTick  int

	chatAgent     *Agent // non-nil while the chat agent loop is running
	cmdAgent      *Agent // non-nil while a palette agent command is in flight
	cmdToolBudget int    // remaining inspection round-trips for current palette command

	// Palette carry-over: bounded Q&A history across invocations (reset on Escape / :clear chat).
	cmdHistory []agent.Message // alternating user/assistant pairs
	cmdPending string          // raw input for the in-flight palette turn

	// cmdSample is a one-shot log snapshot captured on first palette agent use,
	// stored as copies so it's independent of ring-buffer eviction. Kept for
	// the lifetime of the run so the system prompt stays stable across turns
	// (enables provider-side prompt caching on the prefix).
	cmdSample []logmsg.LogMessage

	hintIdx    int    // cycles through inputHints
	yankNotice string // transient "yanked" flash in the status bar

	// view controls how structured lines (json/logfmt) are rendered.
	view viewMode

	// sessionSample is the one-shot log snapshot captured on first submit.
	sessionSample []string

	// ctx controls in-flight chat requests; cancelled on quit.
	ctx    context.Context
	cancel context.CancelFunc
}

// cmdHints are random placeholder tips shown in the command palette input.
var cmdHints = []string{
	"Ask anything about the logs…",
	"grep for timeout errors",
	"count how many 500 errors occurred",
	"show errors from the last 5 minutes",
	"what's causing the spike in errors?",
	"find lines matching panic",
	"how many 4xx requests happened?",
	"summarize recent activity",
	"count warnings vs errors",
	"any database connection failures?",
	"grep for slow queries",
	"filter lines with :grep timeout",
	"case-insensitive search with :grep -i warning",
	"highlight matches with :hl error",
	"clear all highlights with :nohl",
	"pretty-print structured logs with :view pretty",
	"see raw lines with :view raw",
}

// inputHints cycles through the textarea placeholder.
var inputHints = []string{
	"Ask a question about the logs...",
	"Type /clear to reset the chat",
	"e.g. what errors occurred recently?",
	"e.g. summarize the last 50 lines",
}

// New builds a ready-to-run Model.
func New(cfg config.Config, kind SourceKind, buf *buffer.Buffer, client *agent.Client, reg *tools.Registry, r *redact.Redactor, follow source.FileFollower) Model {
	ta := textarea.New()
	ta.Placeholder = inputHints[0]
	ta.Prompt = ""
	ta.CharLimit = 2000
	ta.ShowLineNumbers = false
	ta.SetHeight(3)

	ci := textinput.New()
	ci.Prompt = ""
	ci.Placeholder = cmdHints[rand.Intn(len(cmdHints))]
	ci.CharLimit = 200

	ctx, cancel := context.WithCancel(context.Background())

	h := highlight.New()

	initialView := viewModeRaw
	switch cfg.View {
	case "color":
		initialView = viewModeColor
		h.Register("json", highlight.JSONRenderer)
		h.Register("logfmt", highlight.LogfmtRenderer)
	case "pretty":
		initialView = viewModePretty
		h.Register("json", highlight.PrettyJSONRenderer)
		h.Register("logfmt", highlight.PrettyLogfmtRenderer)
	}

	lv := newLogViewport(buf)
	lv.h = h

	m := Model{
		cfg:          cfg,
		sourceKind:   kind,
		buf:          buf,
		highlighter:  h,
		view:         initialView,
		logs:         lv,
		convo:        viewport.New(0, 0),
		input:        ta,
		cmdInput:     ci,
		focus:        focusLogs,
		chatClient:   client,
		toolsReg:     reg,
		redactor:     r,
		fileFollow:   follow,
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
