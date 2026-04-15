package tui

import tea "github.com/charmbracelet/bubbletea"

// key names for matching tea.KeyMsg.String().
const (
	keyQuit    = "q"
	keyCtrlC   = "ctrl+c"
	keyOpen    = "/"
	keyEscape  = "esc"
	keyTab     = "tab"
	keyEnter   = "enter"
	keyHelp    = "?"
	keyClear   = "c"
)

// isQuitKey returns true for q or ctrl+c when the logs pane is focused
// (so that 'q' still works normally inside the chat input).
func isQuitKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == keyQuit || s == keyCtrlC
}
