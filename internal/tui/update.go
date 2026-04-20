package tui

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/source"
)

// ChatReplyMsg is kept for backward compatibility.
type ChatReplyMsg struct{ Content string }

// spinnerTickMsg drives the spinner animation while chatLoading is true.
type spinnerTickMsg struct{}

// scrollFlushMsg is fired as an immediate command after the first wheel event
// in a burst. By the time it arrives on the event loop, subsequent wheel
// events have typically already incremented pendingScrollY/X, so one flush
// applies all of them at once instead of one tiny viewport move per event.
type scrollFlushMsg struct{}

var spinnerFrames = []rune("⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏")

// Update handles all tea messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		return m.onResize(msg.Width, msg.Height), nil

	case source.NewLinesMsg:
		evictions := 0
		for _, lm := range msg.Lines {
			if evictedID, evicted := m.buf.Append(lm); evicted {
				m.highlighter.Evict(evictedID)
				evictions++
			}
		}
		m.logs.OnNewLines(len(msg.Lines))
		if m.searchRe != nil {
			m.updateSearch(len(msg.Lines), evictions)
		}
		if len(m.filterRules) > 0 {
			m.updateFilter(len(msg.Lines), evictions)
		}
		if len(m.hlRules) > 0 {
			updateHL(&m, len(msg.Lines), evictions)
		}
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

	// Legacy message kept for backward compatibility.
	case ChatReplyMsg:
		m.chatLoading = false
		m.chatAgent = nil
		m.chatMsgs = append(m.chatMsgs, agent.Message{Role: "assistant", Content: msg.Content})
		m.refreshConvoContent()
		return m, nil

	case AgentStepMsg:
		if msg.Agent == m.chatAgent {
			return m.onChatToolCalls(msg)
		}
		if msg.Agent == m.cmdAgent {
			return executeCmdTool(m, msg)
		}
		return m, nil // stale agent, ignore

	case AgentDoneMsg:
		if msg.Agent == m.chatAgent {
			return m.onChatDone(msg)
		}
		return m, nil // stale agent, ignore

	case AgentErrMsg:
		if msg.Agent == m.chatAgent {
			m.chatAgent = nil
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
		}
		if msg.Agent == m.cmdAgent {
			m.cmdAgent = nil
			m.cmdErr = msg.Err.Error()
			m.cmdInfo = ""
			return m, nil
		}
		return m, nil // stale agent, ignore

	case spinnerTickMsg:
		if m.chatLoading {
			m.spinnerTick++
			m.refreshConvoContent()
			return m, spinnerCmd()
		}
		return m, nil

	case tea.KeyMsg:
		return m.onKey(msg)

	case scrollFlushMsg:
		m = applyPendingScroll(m)
		return m, nil

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return m, nil
		}

		if m.sidebarOpen {
			if msg.Shift {
				switch msg.Button {
				case tea.MouseButtonWheelUp:
					m.convo.ScrollUp(3)
				case tea.MouseButtonWheelDown:
					m.convo.ScrollDown(3)
				}
				return m, nil
			}
			// Block clicks so the sidebar doesn't accidentally trigger
			// log-row highlighting. Wheel events (all four directions)
			// fall through to the coalescing logic below.
			switch msg.Button {
			case tea.MouseButtonWheelUp, tea.MouseButtonWheelDown,
				tea.MouseButtonWheelLeft, tea.MouseButtonWheelRight:
				// fall through
			default:
				return m, nil
			}
		}

		// Accumulate wheel events into a pending delta rather than moving the
		// viewport on every tick. A scrollFlushMsg goroutine is scheduled to
		// drain the delta, but on a fast trackpad the input reader goroutine
		// wins the channel race every time and the flush never fires. The cap
		// (maxPendingScroll) forces an immediate apply after every 5 events so
		// the viewport stays responsive regardless of goroutine scheduling.
		const (
			scrollStep      = 3
			maxPendingScroll = scrollStep * 5
		)
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			m.pendingScrollY -= scrollStep
		case tea.MouseButtonWheelDown:
			m.pendingScrollY += scrollStep
		case tea.MouseButtonWheelLeft:
			m.pendingScrollX -= scrollStep
		case tea.MouseButtonWheelRight:
			m.pendingScrollX += scrollStep
		default:
			// Non-wheel events (clicks) are handled immediately.
			m.logs.Update(msg)
			return m, nil
		}

		yFull := m.pendingScrollY >= maxPendingScroll || m.pendingScrollY <= -maxPendingScroll
		xFull := m.pendingScrollX >= maxPendingScroll || m.pendingScrollX <= -maxPendingScroll
		if yFull || xFull {
			m = applyPendingScroll(m)
			return m, nil
		}

		if !m.flushScrollPending {
			m.flushScrollPending = true
			return m, func() tea.Msg { return scrollFlushMsg{} }
		}
		return m, nil
	}
	return m, nil
}

// applyPendingScroll moves the viewport by the accumulated pending deltas and
// resets them. It is called both from scrollFlushMsg and from the inline cap
// path when enough wheel events have arrived before the goroutine could fire.
func applyPendingScroll(m Model) Model {
	m.flushScrollPending = false
	if m.pendingScrollY != 0 {
		if m.pendingScrollY > 0 {
			m.logs.ScrollDown(m.pendingScrollY)
			if m.logs.AtBottom() {
				m.logs.autoScroll = true
			}
		} else {
			m.logs.ScrollUp(-m.pendingScrollY)
			m.logs.autoScroll = false
		}
		m.pendingScrollY = 0
	}
	if m.pendingScrollX != 0 {
		if m.pendingScrollX > 0 {
			m.logs.ScrollRight(m.pendingScrollX)
		} else {
			m.logs.ScrollLeft(-m.pendingScrollX)
		}
		m.pendingScrollX = 0
	}
	return m
}


// spinnerCmd schedules one spinner tick after 100 ms.
func spinnerCmd() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// cmdRows returns the number of terminal rows consumed by the command palette.
func (m Model) cmdRows() int {
	if !m.cmdActive {
		return 0
	}
	if m.cmdErr != "" || m.cmdInfo != "" {
		return 4 // border_top + input + info/error + border_bottom
	}
	return 3 // border_top + input + border_bottom
}

func (m Model) onResize(w, h int) Model {
	m.width = w
	m.height = h
	lay := computeLayout(w, h, m.cmdRows(), m.sidebarOpen, m.cfg.SidebarWidth)

	m.logs.SetSize(lay.logsWidth, lay.paneHeight)

	// Keep the command input the same width as the terminal interior.
	if w > 2 {
		m.cmdInput.Width = w - 2
	}

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

	// Command palette is active: route all input to it.
	if m.cmdActive {
		switch msg.String() {
		case keyEscape:
			m.cmdActive = false
			m.cmdErr = ""
			m.cmdInfo = ""
			m.cmdAgent = nil
			m.cmdInput.Blur()
			m = m.onResize(m.width, m.height)
			return m, nil
		case keyEnter:
			if m.cmdAgent != nil {
				return m, nil
			}
			return m.dispatchCommand(m.cmdInput.Value())
		default:
			if m.cmdAgent != nil {
				return m, nil
			}
			var cmd tea.Cmd
			m.cmdInput, cmd = m.cmdInput.Update(msg)
			return m, cmd
		}
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
		m.cmdActive = true
		m.cmdInput.SetValue("")
		m.cmdErr = ""
		m = m.onResize(m.width, m.height)
		return m, m.cmdInput.Focus()
	case keyColon:
		m.cmdActive = true
		m.cmdInput.SetValue(":")
		m.cmdErr = ""
		m = m.onResize(m.width, m.height)
		return m, m.cmdInput.Focus()
	case keyFilterToggle:
		if len(m.filterRules) == 0 {
			return m, nil
		}
		if m.filterEnabled {
			m.pauseFilter()
		} else {
			m.resumeFilter()
		}
		return m, nil
	case keyFilterClear:
		if len(m.filterRules) > 0 {
			m.clearFilter()
		}
		return m, nil
	case keyHlToggle:
		if len(m.hlRules) > 0 {
			if m.hlEnabled {
				m.hlEnabled = false
				m.logs.hlMatchMap = nil
			} else {
				m.hlEnabled = true
				rebuildHlMatchMap(&m)
			}
		}
		return m, nil
	case keyEscape:
		if m.help {
			m.help = false
			return m, nil
		}
		if m.searchRe != nil && !m.sidebarOpen {
			m.clearSearch()
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
		m = clearChat(m)
		return m, nil
	case keyWrap:
		m.logs.ToggleWrap()
		return m, nil
	case keyNextMatch:
		m.nextMatch()
		return m, nil
	case keyPrevMatch:
		m.prevMatch()
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
	switch {
	case m.cmdActive:
		right = styleHelp.Render(" enter:run  esc:cancel ")
	case len(m.filterRules) > 0 && m.filterEnabled:
		visible := len(m.filterIndices)
		total := m.buf.Len()
		n := len(m.filterRules)
		right = styleHelp.Render(fmt.Sprintf(" %d filter(s) %d/%d  f:pause  F:clear ", n, visible, total))
	case len(m.filterRules) > 0 && !m.filterEnabled:
		right = styleHelp.Render(fmt.Sprintf(" %d filter(s) paused  f:resume  F:clear ", len(m.filterRules)))
	case m.searchRe != nil:
		total := len(m.searchMatches)
		cur := 0
		if total > 0 {
			cur = m.searchCursor + 1
		}
		right = styleHelp.Render(fmt.Sprintf(" n/N:next/prev  esc:clear  match %d/%d ", cur, total))
	case m.sidebarOpen:
		if m.chatLoading {
			right = styleStatusItem.Render("  thinking... ")
		} else if m.chatErr != "" {
			right = styleErr.Render(" ! " + truncate(m.chatErr, 40) + " ")
		} else {
			right = styleHelp.Render(" tab:focus  esc:close  enter:send ")
		}
	default:
		right = styleHelp.Render(" /:cmd  ?:help  q:quit ")
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
	case "count_regex":
		return "counting logs"
	default:
		return "thinking"
	}
}
