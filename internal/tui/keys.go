package tui

import tea "github.com/charmbracelet/bubbletea"

// key names for matching tea.KeyMsg.String().
const (
	keyQuit     = "q"
	keyCtrlC    = "ctrl+c"
	keyOpen     = "/"
	keyColon    = ":"
	keyEscape   = "esc"
	keyTab      = "tab"
	keyEnter    = "enter"
	keyAltEnter = "ctrl+j"
	keyHelp     = "?"
	keyClear    = "c"
	keyWrap     = "w"

	keyNextMatch = "n"
	keyPrevMatch = "N"

	keyFilterToggle = "f" // pause/resume active filter
	keyFilterClear  = "F" // wipe all filter rules

	// Log viewport scroll keys (mirrors viewport.DefaultKeyMap).
	keyScrollDown       = "j"
	keyScrollUp         = "k"
	keyArrowDown        = "down"
	keyArrowUp          = "up"
	keyPageDown         = "pgdown"
	keyPageDownSpace    = " "
	keyPageUp           = "pgup"
	keyPageUpB          = "b"
	keyHalfPageDown    = "d"
	keyHalfPageDownCtrl = "ctrl+d"
	keyHalfPageUp      = "u"
	keyHalfPageUpCtrl  = "ctrl+u"
	keyScrollLeft      = "h"
	keyArrowLeft       = "left"
	keyScrollRight     = "l"
	keyArrowRight      = "right"
)

// isQuitKey returns true for q or ctrl+c when the logs pane is focused
// (so that 'q' still works normally inside the chat input).
func isQuitKey(msg tea.KeyMsg) bool {
	s := msg.String()
	return s == keyQuit || s == keyCtrlC
}
