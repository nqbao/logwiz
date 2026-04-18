package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/nqbao/logwiz/internal/chat"
)

// View renders the full UI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "loading..."
	}

	lay := computeLayout(m.width, m.height, m.cmdRows(), m.sidebarOpen, m.cfg.SidebarWidth)

	logsView := m.logs.View(styleSelectedRow)

	var body string
	if m.sidebarOpen {
		body = lipgloss.JoinHorizontal(lipgloss.Top, logsView, m.renderSidebar(lay))
	} else {
		body = logsView
	}

	status := m.statusLine(lay)

	var full string
	if m.cmdActive {
		palette := m.renderCommandPalette(m.width)
		full = lipgloss.JoinVertical(lipgloss.Left, body, palette, status)
	} else {
		full = lipgloss.JoinVertical(lipgloss.Left, body, status)
	}

	if m.help {
		return m.renderHelpOverlay(full)
	}
	return full
}

// renderCommandPalette renders the command input box docked above the status bar.
func (m Model) renderCommandPalette(width int) string {
	innerW := width - 2 // subtract left+right border
	if innerW < 1 {
		innerW = 1
	}
	content := m.cmdInput.View()
	if m.cmdErr != "" {
		content += "\n" + styleErr.Render(m.cmdErr)
	} else if m.cmdInfo != "" {
		content += "\n" + styleDim.Render(m.cmdInfo)
	}
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Width(innerW).
		Render(content)
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
	var inputContent string
	if m.chatLoading {
		inputContent = styleDim.Render("waiting for response...")
	} else {
		inputContent = m.input.View()
	}
	inputView := styleInputBox.Width(inputInnerW).Render(inputContent)

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

// renderMessageWithActivity renders an assistant message, prepending a dim
// activity summary line when the turn used tools.
func renderMessageWithActivity(msg chat.Message, width, turnIdx int, turns []turnActivity) string {
	roleLabel := styleRoleAssistant.Render("logwiz")
	var extra string
	if turnIdx >= 0 && turnIdx < len(turns) {
		ta := turns[turnIdx]
		if ta.ToolCount > 0 {
			elapsed := ta.FinishedAt.Sub(ta.Started).Round(100 * time.Millisecond)
			summary := fmt.Sprintf("✓ ran %d tool%s in %.1fs",
				ta.ToolCount, pluralS(ta.ToolCount), elapsed.Seconds())
			if ta.Errored {
				extra = styleActivityError.Render(summary) + "\n"
			} else {
				extra = styleActivity.Render(summary) + "\n"
			}
		}
	}
	body := lipgloss.NewStyle().Width(width).Render(msg.Content)
	return roleLabel + "\n" + extra + body
}

// renderActivityLine renders the live spinner line shown while the agent is
// executing.
func renderActivityLine(ta turnActivity, tick, width int) string {
	frame := string(spinnerFrames[tick%len(spinnerFrames)])
	elapsed := time.Since(ta.Started).Round(100 * time.Millisecond)
	var label string
	if ta.LastLabel != "" {
		label = ta.LastLabel
	} else {
		label = "thinking"
	}
	var tools string
	if ta.ToolCount > 0 {
		tools = fmt.Sprintf(" (%d tool%s)", ta.ToolCount, pluralS(ta.ToolCount))
	}
	line := fmt.Sprintf("%s %s...%s (%.1fs)", frame, label, tools, elapsed.Seconds())
	return styleActivity.Width(width).Render(line)
}

func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// trimToHeight keeps the last h lines of s.
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
		"  /             open command palette",
		"  :chat [q]     open chat sidebar (optionally pre-filled)",
		"  :search re    regex-search the buffer",
		"  :grep [-i] [-F] [-v] re   filter lines (stackable, rules are ANDed)",
		"    -v  exclude  -i  case-insensitive  -F  literal string",
		"    esc clears all filters",
		"  :model [name] show or change the active model",
		"  n / N         next / previous search match",
		"  esc           close sidebar / cancel palette / clear filter / clear search",
		"  tab           switch focus",
		"  enter         send chat message (in input)",
		"  ctrl+j        newline in chat input",
		"  c             clear chat history",
		"  w             toggle line wrap",
		"  j / ↓         scroll logs down",
		"  k / ↑         scroll logs up",
		"  pgup/pgdn     page logs",
		"  ?             toggle this help",
		"  q             quit (ctrl+c always quits)",
	}, "\n")
	box := lipgloss.NewStyle().
		Padding(1, 2).
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Render(help)

	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box,
		lipgloss.WithWhitespaceChars(" "))
}
