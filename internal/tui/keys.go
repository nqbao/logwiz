package tui

import tea "github.com/charmbracelet/bubbletea"

// key names for matching tea.KeyMsg.String().
const (
	keyQuit        = "q"
	keyQuitNow     = "qq"
	keyCtrlC       = "ctrl+c"
	keyOpen        = "/"
	keyColon       = ":"
	keyEscape      = "esc"
	keyTab         = "tab"
	keyEnter       = "enter"
	keyAltEnter    = "ctrl+j"
	keyHelp        = "?"
	keyClear       = "c"
	keyBufferClear = "C"
	keyWrap        = "w"

	keyNextMatch = "n"
	keyPrevMatch = "N"

	keyFilterToggle = "f" // pause/resume active filter
	keyFilterClear  = "F" // wipe all filter rules

	keyHlToggle     = "H" // toggle highlights on/off (h is scroll-left)
	keyPrettyToggle = "p" // toggle pretty mode (humanlog-style layout)
	keyYank         = "y" // copy selected line to clipboard

	// Log viewport scroll keys (mirrors viewport.DefaultKeyMap).
	keyScrollDown       = "j"
	keyScrollUp         = "k"
	keyArrowDown        = "down"
	keyArrowUp          = "up"
	keyPageDown         = "pgdown"
	keyPageDownSpace    = " "
	keyPageUp           = "pgup"
	keyPageUpB          = "b"
	keyHalfPageDown     = "d"
	keyHalfPageDownCtrl = "ctrl+d"
	keyHalfPageUp       = "u"
	keyHalfPageUpCtrl   = "ctrl+u"
	keyScrollLeft       = "h"
	keyArrowLeft        = "left"
	keyScrollRight      = "l"
	keyArrowRight       = "right"
)

// nextQuitState returns the updated pending-quit state and whether the
// key sequence should quit. Only logs-focused key handling uses this, so
// plain q still works inside text inputs.
func nextQuitState(pending bool, msg tea.KeyMsg) (bool, bool) {
	switch msg.String() {
	case keyCtrlC:
		return false, true
	case keyQuit:
		if pending {
			return false, true
		}
		return true, false
	default:
		return false, false
	}
}
