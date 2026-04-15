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
		body = lipgloss.JoinHorizontal(lipgloss.Top, logsView, m.renderSidebar(lay))
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
	// Each rounded border costs 2 cols (left+right) and 2 rows (top+bottom).
	const borderW = 2
	innerW := lay.sidebarWidth - borderW
	if innerW < 1 {
		innerW = 1
	}

	headerText := "chat"
	if m.chatLoading {
		headerText = "chat · thinking..."
	}
	header := styleSidebarHeader.Width(innerW).Render(headerText)

	convoBox := lipgloss.NewStyle().
		Width(innerW).
		Height(m.convo.Height).
		Render(m.convo.View())

	inputInnerW := innerW - borderW
	if inputInnerW < 1 {
		inputInnerW = 1
	}
	inputView := styleInputBox.Width(inputInnerW).Render(m.input.View())

	inner := lipgloss.JoinVertical(lipgloss.Left, header, convoBox, inputView)
	return styleSidebarBox.Width(innerW).Render(inner)
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
		"  ctrl+j    newline in chat input",
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

