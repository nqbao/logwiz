package tui

import (
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// TestDispatchCommand_NormalizesLeadingColon verifies :search and /search
// both work the same as the bare form.
func TestDispatchCommand_NormalizesLeadingColon(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	for _, raw := range []string{"search foo", ":search foo", "/search foo"} {
		// Seed the buffer with a matching line.
		m.buf.Append(logmsg.LogMessage{Raw: "foo bar"})

		m2, _ := m.dispatchCommand(raw)
		if m2.searchRe == nil {
			t.Errorf("input %q: searchRe is nil", raw)
		}
		if m2.cmdActive {
			t.Errorf("input %q: cmdActive should be false after success", raw)
		}
		// Reset for next iteration.
		m2.searchRe = nil
		m2.searchMatches = nil
	}
}

// TestDispatchCommand_UnknownCommand leaves palette open with an error.
func TestDispatchCommand_UnknownCommand(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("frobnicate")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true after unknown command")
	}
	if m2.cmdErr == "" {
		t.Error("cmdErr should be set for unknown command")
	}
}

// TestDispatchCommand_BadRegex leaves palette open with a compile error.
func TestDispatchCommand_BadRegex(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("search (")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true after bad regex")
	}
	if m2.cmdErr == "" {
		t.Error("cmdErr should describe the compile error")
	}
	if m2.searchRe != nil {
		t.Error("searchRe should not be set after failed search")
	}
}

// TestDispatchCommand_Chat opens the sidebar and pre-fills the input.
func TestDispatchCommand_Chat(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("chat hello world")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :chat")
	}
	if !m2.sidebarOpen {
		t.Error("sidebar should be open after :chat")
	}
	if m2.focus != focusChat {
		t.Errorf("focus = %v, want focusChat", m2.focus)
	}
	if got := m2.input.Value(); got != "hello world" {
		t.Errorf("input value = %q, want %q", got, "hello world")
	}
}

// TestDispatchCommand_Clear resets conversation.
func TestDispatchCommand_Clear(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.chatErr = "some error"

	m2, _ := m.dispatchCommand("clear")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :clear")
	}
	if m2.chatErr != "" {
		t.Errorf("chatErr = %q after :clear, want empty", m2.chatErr)
	}
}

// TestDispatchCommand_Help opens the help overlay.
func TestDispatchCommand_Help(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("help")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :help")
	}
	if !m2.help {
		t.Error("help overlay should be open after :help")
	}
}

// TestDispatchCommand_EmptyInput closes the palette without error.
func TestDispatchCommand_EmptyInput(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("")
	if m2.cmdActive {
		t.Error("cmdActive should be false after empty input")
	}
	if m2.cmdErr != "" {
		t.Errorf("cmdErr = %q, want empty", m2.cmdErr)
	}
}

// TestDispatchCommand_Search sets searchRe and positions viewport.
func TestDispatchCommand_Search(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.buf.Append(logmsg.LogMessage{Raw: "alpha"})
	m.buf.Append(logmsg.LogMessage{Raw: "beta"})
	m.buf.Append(logmsg.LogMessage{Raw: "alpha again"})

	m2, _ := m.dispatchCommand("search alpha")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :search")
	}
	if m2.searchRe == nil {
		t.Fatal("searchRe is nil after :search")
	}
	if len(m2.searchMatches) != 2 {
		t.Errorf("searchMatches len = %d, want 2", len(m2.searchMatches))
	}
	if m2.searchCursor != 0 {
		t.Errorf("searchCursor = %d, want 0", m2.searchCursor)
	}
}
