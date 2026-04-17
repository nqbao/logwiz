package tui

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/source"
)

// ChatReplyMsg is kept for backward compatibility with existing tests.
// New code uses ChatStepMsg internally.
type ChatReplyMsg struct{ Content string }

// ChatStepMsg carries one assistant response (may have tool calls) back
// to the update loop.
type ChatStepMsg struct{ Assistant chat.Message }

// ChatErrMsg carries a chat request failure.
type ChatErrMsg struct{ Err error }

// spinnerTickMsg drives the spinner animation while chatLoading is true.
type spinnerTickMsg struct{}

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Update handles all tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg.Width, msg.Height), nil

	case source.NewLinesMsg:
		for _, lm := range msg.Lines {
			if evictedID, evicted := m.buf.Append(lm); evicted {
				m.highlighter.Evict(evictedID)
			}
		}
		m.logs.OnNewLines(len(msg.Lines))
		return m, nil

	case source.EOFMsg:
		m.eof = true
		if m.sourceKind == SourceFile {
			m.logs.autoScroll = false
			m.logs.GotoTop()
		}
		return m, nil

	case source.ErrMsg:
		m.chatErr = "source error: " + msg.Err.Error()
		m.eof = true
		return m, nil

	// Legacy message kept for test compatibility.
	case ChatReplyMsg:
		m.chatLoading = false
		m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "assistant", Content: msg.Content})
		m.refreshConvoContent()
		return m, nil

	case ChatStepMsg:
		return m.onChatStep(msg.Assistant)

	case ChatErrMsg:
		m.chatLoading = false
		m.chatErr = msg.Err.Error()
		if m.activeTurn >= 0 {
			m.turns[m.activeTurn].Done = true
			m.turns[m.activeTurn].Errored = true
			m.turns[m.activeTurn].FinishedAt = time.Now()
			m.activeTurn = -1
		}
		m.refreshConvoContent()
		return m, nil

	case spinnerTickMsg:
		if m.chatLoading {
			m.spinnerTick++
			m.refreshConvoContent()
			return m, spinnerCmd()
		}
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		if msg.Shift && m.sidebarOpen {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.convo.ScrollUp(3)
			case tea.MouseButtonWheelDown:
				m.convo.ScrollDown(3)
			}
			return m, nil
		}

		m.logs.Update(msg)
		return m, nil
	}
	return m, nil
}

// onChatStep processes one assistant response in the agent loop.
func (m Model) onChatStep(asst chat.Message) (tea.Model, tea.Cmd) {
	m.chatMsgs = append(m.chatMsgs, asst)

	// No tool calls: the assistant gave a final answer.
	if len(asst.ToolCalls) == 0 {
		if m.activeTurn >= 0 {
			m.turns[m.activeTurn].Done = true
			m.turns[m.activeTurn].FinishedAt = time.Now()
			m.turnIdxByMsg[len(m.chatMsgs)-1] = m.activeTurn
			m.activeTurn = -1
		}
		m.chatLoading = false
		m.refreshConvoContent()
		return m, nil
	}

	// Tool calls present: execute them and continue the loop.
	if m.activeTurn >= 0 {
		ta := &m.turns[m.activeTurn]
		for _, call := range asst.ToolCalls {
			ta.LastTool = call.Name
			ta.LastLabel = labelFor(call.Name)
			var result string
			if m.toolsReg != nil {
				result = m.toolsReg.Run(call.Name, []byte(call.Arguments), m.buf)
			} else {
				result = "error: tools not configured"
			}
			m.chatMsgs = append(m.chatMsgs, chat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    result,
			})
			ta.ToolCount++
		}

		m.toolBudget--
		if m.toolBudget <= 0 {
			// Inject a budget-exhaustion notice into the last tool message.
			if len(m.chatMsgs) > 0 {
				last := &m.chatMsgs[len(m.chatMsgs)-1]
				if last.Role == "tool" {
					last.Content += "\n\n[Tool budget exhausted — answer with information gathered so far.]"
				}
			}
		}
	}

	m.refreshConvoContent()
	return m, m.runChatStep()
}

// runChatStep fires an async chat request using the current chatMsgs.
func (m Model) runChatStep() tea.Cmd {
	msgs := buildChatMessages(
		m.cfg.EffectiveSystemPrompt(),
		m.sessionSample,
		m.chatMsgs,
		"", // question already appended; history covers it
	)

	var specs []chat.ToolSpec
	if m.toolsReg != nil && m.toolBudget > 0 {
		specs = m.toolsReg.Specs()
	}

	client := m.chatClient
	ctx := m.ctx
	return func() tea.Msg {
		reply, err := client.Complete(ctx, msgs, specs)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil
			}
			return ChatErrMsg{Err: err}
		}
		return ChatStepMsg{Assistant: reply}
	}
}

// spinnerCmd schedules one spinner tick after 100 ms.
func spinnerCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

func (m Model) onResize(w, h int) Model {
	m.width = w
	m.height = h
	lay := computeLayout(w, h, m.sidebarOpen, m.cfg.SidebarWidth)

	m.logs.SetSize(lay.logsWidth, lay.paneHeight)

	// Sidebar outer border: -2. Input box inner border: -2.
	innerW := lay.sidebarWidth - 2
	if innerW < 4 {
		innerW = 4
	}
	inputWidth := innerW - 2
	if inputWidth < 10 {
		inputWidth = 10
	}
	m.input.SetWidth(inputWidth)
	m.input.SetHeight(3)

	// convo viewport: paneHeight minus header(1) minus inputBox(5) minus outer border(2)
	const inputBoxRows = 5
	convoH := lay.paneHeight - 1 - inputBoxRows - 2
	if convoH < 1 {
		convoH = 1
	}
	m.convo.Width = innerW
	m.convo.Height = convoH

	m.refreshConvoContent()
	return m
}

func (m Model) onKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Ctrl+C always quits.
	if msg.String() == keyCtrlC {
		m.Cancel()
		return m, tea.Quit
	}

	// When the chat input has focus, let it swallow characters first.
	if m.focus == focusChat && m.sidebarOpen {
		// While a request is in flight, block all input except Esc/Tab to blur.
		if m.chatLoading {
			switch msg.String() {
			case keyEscape, keyTab:
				m.input.Blur()
				m.focus = focusLogs
			}
			return m, nil
		}
		switch msg.String() {
		case keyEscape:
			m.input.Blur()
			m.focus = focusLogs
			return m, nil
		case keyTab:
			m.input.Blur()
			m.focus = focusLogs
			return m, nil
		case keyAltEnter:
			m.input.InsertString("\n")
			return m, nil
		case keyEnter:
			return m.submitChat()
		default:
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	// Logs-focused keybindings.
	switch msg.String() {
	case keyQuit:
		m.Cancel()
		return m, tea.Quit
	case keyOpen:
		m.sidebarOpen = true
		m.focus = focusChat
		cmd := m.input.Focus()
		m = m.onResize(m.width, m.height)
		return m, cmd
	case keyEscape:
		if m.help {
			m.help = false
			return m, nil
		}
		if m.sidebarOpen {
			m.sidebarOpen = false
			m = m.onResize(m.width, m.height)
		}
		return m, nil
	case keyTab:
		if m.sidebarOpen {
			m.focus = focusChat
			cmd := m.input.Focus()
			return m, cmd
		}
		return m, nil
	case keyHelp:
		m.help = !m.help
		return m, nil
	case keyClear:
		m.chatMsgs = nil
		m.chatErr = ""
		m.turns = nil
		m.turnIdxByMsg = make(map[int]int)
		m.activeTurn = -1
		m.sessionSample = nil
		return m, nil
	case keyWrap:
		m.logs.ToggleWrap()
		return m, nil
	}

	// Forward remaining keys to the log viewport.
	m.logs.Update(msg)
	return m, nil
}

func (m *Model) refreshConvoContent() {
	w := m.convo.Width
	if w < 1 {
		return
	}
	var lines []string
	for i, msg := range m.chatMsgs {
		// Skip intermediate tool call/result messages — they are not rendered
		// directly. Only render user messages and final assistant messages.
		if msg.Role == "tool" {
			continue
		}
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 && msg.Content == "" {
			continue
		}
		if msg.Role == "assistant" {
			lines = append(lines, renderMessageWithActivity(msg, w, m.turnIdxByMsg[i], m.turns))
		} else {
			lines = append(lines, renderMessage(msg, w))
		}
		lines = append(lines, "")
	}

	// Live activity placeholder while loading.
	if m.chatLoading && m.activeTurn >= 0 {
		lines = append(lines, renderActivityLine(m.turns[m.activeTurn], m.spinnerTick, w))
		lines = append(lines, "")
	}

	if m.chatErr != "" {
		lines = append(lines, styleErr.Width(w).Render("error: "+m.chatErr))
	}
	m.convo.SetContent(strings.Join(lines, "\n"))
	m.convo.GotoBottom()
}

// submitChat takes the current input, validates it, and kicks off the agent loop.
func (m Model) submitChat() (tea.Model, tea.Cmd) {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		return m, nil
	}
	if m.chatLoading {
		return m, nil
	}
	if m.chatClient == nil {
		m.chatErr = "chat is not configured (set OPENAI_API_KEY or --api-key)"
		return m, nil
	}

	// Capture session sample on the very first submit.
	if len(m.chatMsgs) == 0 && m.cfg.ContextLines > 0 {
		recent := m.buf.LastN(m.cfg.ContextLines)
		m.sessionSample = make([]string, len(recent))
		for i, lm := range recent {
			m.sessionSample[i] = lm.Raw
		}
	}

	m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "user", Content: q})
	m.chatErr = ""
	m.chatLoading = true
	m.input.Reset()

	// Register a new turn.
	m.turns = append(m.turns, turnActivity{
		Started:   time.Now(),
		LastLabel: "thinking",
	})
	m.activeTurn = len(m.turns) - 1
	m.toolBudget = MaxToolTurns

	m.refreshConvoContent()
	return m, tea.Batch(m.runChatStep(), spinnerCmd())
}

// statusLine composes the bottom bar text.
func (m Model) statusLine(lay layout) string {
	mode := string(m.sourceKind)
	lines := m.buf.Len()

	var badge string
	if m.eof {
		badge = styleStatusBadgeEOF.Render("eof")
	} else {
		badge = styleStatusBadgeLive.Render("live")
	}

	left := styleStatusBar.Render(fmt.Sprintf(" logwiz [%s]  %d lines ", mode, lines)) + " " + badge

	var right string
	if m.sidebarOpen {
		if m.chatLoading {
			right = styleStatusItem.Render("  thinking... ")
		} else if m.chatErr != "" {
			right = styleErr.Render(" ! " + truncate(m.chatErr, 40) + " ")
		} else {
			right = styleHelp.Render(" tab:focus  esc:close  enter:send ")
		}
	} else {
		right = styleHelp.Render(" /:chat  ?:help  q:quit ")
	}

	pad := lay.logsWidth + lay.sidebarWidth + 1 - visualWidth(left) - visualWidth(right)
	if pad < 1 {
		pad = 1
	}
	return left + strings.Repeat(" ", pad) + right
}

// visualWidth is a cheap visual length helper that ignores ANSI codes.
func visualWidth(s string) int {
	n := 0
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		n++
	}
	return n
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

// labelFor maps a tool name to a human-readable activity label.
func labelFor(name string) string {
	switch name {
	case "tail":
		return "sampling logs"
	case "read":
		return "reading logs"
	case "search_regex":
		return "searching logs"
	default:
		return "thinking"
	}
}
