package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nqbao/logwiz/internal/chat"
)

// View renders the full UI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	lay := computeLayout(m.width, m.height, m.sidebarOpen, m.cfg.SidebarWidth)

	logsView := lipgloss.NewStyle().
		Width(lay.logsWidth).
		Height(lay.paneHeight).
		Render(m.logs.View())

	var body string
	if m.sidebarOpen {
		divider := styleSidebarBorder.Render(strings.Repeat("│", lay.paneHeight))
		body = lipgloss.JoinHorizontal(lipgloss.Top, logsView, divider, m.renderSidebar(lay))
	} else {
		body = logsView
	}

	status := m.statusLine(lay)

	full := lipgloss.JoinVertical(lipgloss.Left, body, status)
	if m.help {
		return m.renderHelpOverlay(full)
	}
	return full
}

func (m Model) renderSidebar(lay layout) string {
	headerText := "chat"
	if m.chatLoading {
		headerText = "chat · thinking..."
	}
	header := styleSidebarHeader.
		Width(lay.sidebarWidth).
		Render(headerText)

	// Conversation body.
	var lines []string
	for _, msg := range m.chatMsgs {
		lines = append(lines, renderMessage(msg, lay.sidebarWidth))
		lines = append(lines, "")
	}
	if m.chatErr != "" {
		lines = append(lines, styleErr.
			Width(lay.sidebarWidth).
			Render("error: "+m.chatErr))
	}

	// Reserve 3 rows for the input textarea plus 1 header + 1 blank.
	const inputRows = 3
	convoHeight := lay.paneHeight - inputRows - 2
	if convoHeight < 1 {
		convoHeight = 1
	}

	convo := strings.Join(lines, "\n")
	convoBox := lipgloss.NewStyle().
		Width(lay.sidebarWidth).
		Height(convoHeight).
		Render(trimToHeight(convo, convoHeight))

	dashCount := lay.sidebarWidth - 8
	if dashCount < 0 {
		dashCount = 0
	}
	inputLabel := styleDim.Render("─ input " + strings.Repeat("─", dashCount))
	inputView := m.input.View()

	return lipgloss.JoinVertical(lipgloss.Left, header, convoBox, inputLabel, inputView)
}

func renderMessage(msg chat.Message, width int) string {
	var roleLabel string
	switch msg.Role {
	case "user":
		roleLabel = styleRoleUser.Render("you")
	case "assistant":
		roleLabel = styleRoleAssistant.Render("logwiz")
	default:
		roleLabel = styleDim.Render(msg.Role)
	}

	body := lipgloss.NewStyle().Width(width).Render(msg.Content)
	return roleLabel + "\n" + body
}

// trimToHeight keeps the last h lines of s, which is what we want for
// a chat log (show the most recent messages).
func trimToHeight(s string, h int) string {
	if s == "" {
		return s
	}
	lines := strings.Split(s, "\n")
	if len(lines) <= h {
		return s
	}
	return strings.Join(lines[len(lines)-h:], "\n")
}

func (m Model) renderHelpOverlay(underneath string) string {
	help := strings.Join([]string{
		"logwiz keys:",
		"  /         open chat sidebar",
		"  esc       close sidebar / blur input",
		"  tab       switch focus",
		"  enter     send chat message (in input)",
		"  c         clear chat history",
		"  j / ↓     scroll logs down",
		"  k / ↑     scroll logs up",
		"  pgup/pgdn page logs",
		"  ?         toggle this help",
		"  q         quit (ctrl+c always quits)",
	}, "\n")
	box := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Render(help)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "))
}

