package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/source"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	cfg := config.Config{
		MaxLines:     100,
		ContextLines: 10,
		SidebarWidth: 40,
		SystemPrompt: "sys",
		Model:        "m",
	}
	m := New(cfg, SourceStdin, buffer.New(cfg.MaxLines), nil)
	// Simulate an initial resize so layout-dependent code works.
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return mi.(Model)
}

func TestNewLineMsgAppendsToBuffer(t *testing.T) {
	m := newTestModel(t)
	m2, _ := m.Update(source.NewLineMsg{Line: "hello"})
	mm := m2.(Model)
	if mm.buf.Len() != 1 {
		t.Fatalf("len = %d, want 1", mm.buf.Len())
	}
	if got := mm.buf.Snapshot()[0]; got != "hello" {
		t.Fatalf("line = %q", got)
	}
}

func TestEOFMsgSetsFlag(t *testing.T) {
	m := newTestModel(t)
	m2, _ := m.Update(source.EOFMsg{})
	if !m2.(Model).eof {
		t.Fatal("eof not set")
	}
}

func TestSlashOpensSidebarAndFocusesInput(t *testing.T) {
	m := newTestModel(t)
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := m2.(Model)
	if !mm.sidebarOpen {
		t.Fatal("sidebar not open after '/'")
	}
	if mm.focus != focusChat {
		t.Fatalf("focus = %v, want focusChat", mm.focus)
	}
}

func TestEscClosesSidebarFromLogsFocus(t *testing.T) {
	m := newTestModel(t)
	// Open sidebar.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := m2.(Model)
	// Blur (Esc while input focused).
	m3, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm = m3.(Model)
	if mm.focus != focusLogs {
		t.Fatalf("focus = %v, want focusLogs", mm.focus)
	}
	if !mm.sidebarOpen {
		t.Fatal("first Esc should blur, not close")
	}
	// Second Esc (logs focus) should close the sidebar.
	m4, _ := mm.Update(tea.KeyMsg{Type: tea.KeyEsc})
	mm = m4.(Model)
	if mm.sidebarOpen {
		t.Fatal("second Esc should close sidebar")
	}
}

func TestQKeyInLogsFocusQuits(t *testing.T) {
	m := newTestModel(t)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}})
	if cmd == nil {
		t.Fatal("q produced no cmd")
	}
	// tea.Quit is a tea.Cmd that returns tea.QuitMsg{} when invoked.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Fatalf("q cmd = %T, want tea.QuitMsg", msg)
	}
}

func TestChatReplyAppendsAssistantMessage(t *testing.T) {
	m := newTestModel(t)
	m.chatLoading = true
	m2, _ := m.Update(ChatReplyMsg{Content: "it said"})
	mm := m2.(Model)
	if mm.chatLoading {
		t.Fatal("chatLoading still true")
	}
	if len(mm.chatMsgs) != 1 || mm.chatMsgs[0].Role != "assistant" {
		t.Fatalf("chatMsgs = %+v", mm.chatMsgs)
	}
}

func TestChatErrClearsLoadingAndRecordsMessage(t *testing.T) {
	m := newTestModel(t)
	m.chatLoading = true
	m2, _ := m.Update(ChatErrMsg{Err: errExample("boom")})
	mm := m2.(Model)
	if mm.chatLoading {
		t.Fatal("chatLoading still true")
	}
	if !strings.Contains(mm.chatErr, "boom") {
		t.Fatalf("chatErr = %q", mm.chatErr)
	}
}

func TestClearEmptiesChat(t *testing.T) {
	m := newTestModel(t)
	m.chatMsgs = []chat.Message{{Role: "user", Content: "x"}}
	m.chatErr = "some err"
	// 'c' works only in logs focus; sidebar closed by default.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}})
	mm := m2.(Model)
	if len(mm.chatMsgs) != 0 {
		t.Fatalf("chatMsgs = %+v", mm.chatMsgs)
	}
	if mm.chatErr != "" {
		t.Fatalf("chatErr = %q", mm.chatErr)
	}
}

func TestSubmitWithoutClientSetsError(t *testing.T) {
	m := newTestModel(t)
	// Open sidebar and focus input.
	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	mm := m2.(Model)
	mm.input.SetValue("why?")
	// Enter with no client configured should produce an error, not a cmd.
	m3, cmd := mm.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm = m3.(Model)
	if cmd != nil {
		t.Fatalf("expected nil cmd, got %v", cmd)
	}
	if !strings.Contains(mm.chatErr, "api-key") && !strings.Contains(mm.chatErr, "API") {
		t.Fatalf("chatErr = %q", mm.chatErr)
	}
}

// --- helpers ---

type errExample string

func (e errExample) Error() string { return string(e) }
