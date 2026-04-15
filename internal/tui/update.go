package tui

import (
	"context"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/source"
)

// ChatReplyMsg carries a successful assistant response back to the UI.
type ChatReplyMsg struct{ Content string }

// ChatErrMsg carries a chat request failure.
type ChatErrMsg struct{ Err error }

// Update handles all tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg.Width, msg.Height), nil

	case source.NewLineMsg:
		m.buf.Append(msg.Line)
		m.refreshLogsContent()
		return m, nil

	case source.EOFMsg:
		m.eof = true
		if m.sourceKind == SourceFile {
			m.autoScroll = false
			m.logs.GotoTop()
		}
		return m, nil

	case source.ErrMsg:
		m.chatErr = "source error: " + msg.Err.Error()
		m.eof = true
		return m, nil

	case ChatReplyMsg:
		m.chatLoading = false
		m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "assistant", Content: msg.Content})
		m.refreshConvoContent()
		return m, nil

	case ChatErrMsg:
		m.chatLoading = false
		m.chatErr = msg.Err.Error()
		m.refreshConvoContent()
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}
		if m.sidebarOpen {
			switch msg.Button {
			case tea.MouseButtonWheelUp:
				m.convo.ScrollUp(3)
			case tea.MouseButtonWheelDown:
				m.convo.ScrollDown(3)
			}
			return m, nil
		}
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.logs.ScrollUp(3)
			m.autoScroll = false
		case tea.MouseButtonWheelDown:
			m.logs.ScrollDown(3)
			if m.logs.AtBottom() {
				m.autoScroll = true
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) onResize(w, h int) Model {
	m.width = w
	m.height = h
	lay := computeLayout(w, h, m.sidebarOpen, m.cfg.SidebarWidth)

	m.logs.Width = lay.logsWidth
	m.logs.Height = lay.paneHeight

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

	m.refreshLogsContent()
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
		return m, nil
	}

	// Pass remaining keys (j/k, PgUp/PgDn, arrows) to the log viewport.
	var cmd tea.Cmd
	prevAtBottom := m.logs.AtBottom()
	m.logs, cmd = m.logs.Update(msg)
	// If user scrolls up, disable auto-scroll; re-enable when they
	// return to the bottom.
	if prevAtBottom && !m.logs.AtBottom() {
		m.autoScroll = false
	}
	if m.logs.AtBottom() {
		m.autoScroll = true
	}
	return m, cmd
}

func (m *Model) refreshLogsContent() {
	content := strings.Join(m.buf.Snapshot(), "\n")
	m.logs.SetContent(content)
	if m.autoScroll {
		m.logs.GotoBottom()
	}
}

func (m *Model) refreshConvoContent() {
	w := m.convo.Width
	if w < 1 {
		return
	}
	var lines []string
	for _, msg := range m.chatMsgs {
		lines = append(lines, renderMessage(msg, w))
		lines = append(lines, "")
	}
	if m.chatErr != "" {
		lines = append(lines, styleErr.Width(w).Render("error: "+m.chatErr))
	}
	m.convo.SetContent(strings.Join(lines, "\n"))
	m.convo.GotoBottom()
}

// submitChat takes the current input, validates it, and kicks off a
// chat request.
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

	m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "user", Content: q})
	m.chatErr = ""
	m.chatLoading = true
	m.input.Reset()
	m.refreshConvoContent()

	logLines := m.buf.LastN(m.cfg.ContextLines)
	history := m.chatMsgs[:len(m.chatMsgs)-1] // prior turns only
	msgs := buildChatMessages(m.cfg.SystemPrompt, history, logLines, q)

	client := m.chatClient
	ctx := m.ctx
	return m, func() tea.Msg {
		reply, err := client.Complete(ctx, msgs)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil
			}
			return ChatErrMsg{Err: err}
		}
		return ChatReplyMsg{Content: reply}
	}
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
// lipgloss styles embed escape sequences, but for status-bar padding
// the rendered rune count of the raw text is close enough.
func visualWidth(s string) int {
	// Strip ANSI escapes: ESC [ ... <letter>
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
