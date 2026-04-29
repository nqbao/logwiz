package tui

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// TestDispatchCommand_NormalizesLeadingColon verifies :search and /search
// both work the same way.
func TestDispatchCommand_NormalizesLeadingColon(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	for _, raw := range []string{":search foo", "/search foo"} {
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

	m2, _ := m.dispatchCommand(":frobnicate")
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

	m2, _ := m.dispatchCommand(":search (")
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

	m2, _ := m.dispatchCommand(":chat hello world")
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

	m2, _ := m.dispatchCommand(":clear chat")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :clear chat")
	}
	if m2.chatErr != "" {
		t.Errorf("chatErr = %q after :clear chat, want empty", m2.chatErr)
	}
}

// TestDispatchCommand_Help opens the help overlay.
func TestDispatchCommand_Help(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":help")
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

// TestDispatchCommand_NoPrefixKeepsPaletteOpen verifies that unprefixed input
// leaves the palette open with an error (reserved for future natural input).
func TestDispatchCommand_NoPrefixKeepsPaletteOpen(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand("chat hello")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true for unprefixed input")
	}
	if m2.cmdErr == "" {
		t.Error("cmdErr should be set for unprefixed input")
	}
}

// TestDispatchCommand_ConfigGetModel shows current model as dim info, keeps palette open.
func TestDispatchCommand_ConfigGetModel(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.cfg.Model = "gpt-4o"

	m2, _ := m.dispatchCommand(":config model")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true when :config shows info")
	}
	if m2.cmdErr != "" {
		t.Errorf("cmdErr = %q, want empty (info should not be an error)", m2.cmdErr)
	}
	if m2.cmdInfo == "" {
		t.Error("cmdInfo should be set to current model name")
	}
	if !strings.Contains(m2.cmdInfo, "gpt-4o") {
		t.Errorf("cmdInfo = %q, should contain model name", m2.cmdInfo)
	}
}

// TestDispatchCommand_ConfigSetModel changes the model and shows a confirm
// info line in the still-open palette.
func TestDispatchCommand_ConfigSetModel(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.cfg.Model = "gpt-4o-mini"

	m2, _ := m.dispatchCommand(":config model claude-sonnet-4-6")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true to show the saved info line")
	}
	if m2.cfg.Model != "claude-sonnet-4-6" {
		t.Errorf("cfg.Model = %q, want claude-sonnet-4-6", m2.cfg.Model)
	}
	if m2.cmdErr != "" {
		t.Errorf("cmdErr = %q, want empty (save should not be an error)", m2.cmdErr)
	}
	if m2.cmdInfo == "" {
		t.Error("cmdInfo should confirm the saved value")
	}
	if !strings.Contains(m2.cmdInfo, "claude-sonnet-4-6") {
		t.Errorf("cmdInfo = %q, should contain the saved value", m2.cmdInfo)
	}
}

// --- :grep flag parsing ---

func TestParseGrepFlags_NoFlags(t *testing.T) {
	mode, ci, fs, pat, err := parseGrepFlags("error")
	if err != nil || mode != filterInclude || ci || fs || pat != "error" {
		t.Errorf("got mode=%v ci=%v fs=%v pat=%q err=%v", mode, ci, fs, pat, err)
	}
}

func TestParseGrepFlags_v(t *testing.T) {
	mode, _, _, pat, err := parseGrepFlags("-v debug")
	if err != nil || mode != filterExclude || pat != "debug" {
		t.Errorf("got mode=%v pat=%q err=%v", mode, pat, err)
	}
}

func TestParseGrepFlags_i(t *testing.T) {
	_, ci, _, pat, err := parseGrepFlags("-i Error")
	if err != nil || !ci || pat != "Error" {
		t.Errorf("got ci=%v pat=%q err=%v", ci, pat, err)
	}
}

func TestParseGrepFlags_F(t *testing.T) {
	_, _, fs, pat, err := parseGrepFlags("-F [http]")
	if err != nil || !fs || pat != "[http]" {
		t.Errorf("got fs=%v pat=%q err=%v", fs, pat, err)
	}
}

func TestParseGrepFlags_Combined_iv(t *testing.T) {
	mode, ci, _, pat, err := parseGrepFlags("-iv warn")
	if err != nil || mode != filterExclude || !ci || pat != "warn" {
		t.Errorf("got mode=%v ci=%v pat=%q err=%v", mode, ci, pat, err)
	}
}

func TestParseGrepFlags_Separate_i_v(t *testing.T) {
	mode, ci, _, pat, err := parseGrepFlags("-i -v warn")
	if err != nil || mode != filterExclude || !ci || pat != "warn" {
		t.Errorf("got mode=%v ci=%v pat=%q err=%v", mode, ci, pat, err)
	}
}

func TestParseGrepFlags_DoubleDash(t *testing.T) {
	_, _, _, pat, err := parseGrepFlags("-- -v notaflag")
	if err != nil || pat != "-v notaflag" {
		t.Errorf("got pat=%q err=%v", pat, err)
	}
}

func TestParseGrepFlags_UnknownFlag(t *testing.T) {
	_, _, _, _, err := parseGrepFlags("-z pattern")
	if err == nil {
		t.Error("expected error for unknown flag -z")
	}
	if !strings.Contains(err.Error(), "-z") {
		t.Errorf("error should mention the unknown flag, got: %v", err)
	}
}

func TestGrepCommand_FixedString_EscapesRegexChars(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.buf.Append(logmsg.LogMessage{Raw: "price: $10.00"})
	m.buf.Append(logmsg.LogMessage{Raw: "price: $20.00"})
	m.buf.Append(logmsg.LogMessage{Raw: "no match here"})

	// Without -F, "$10.00" as regex would match differently; with -F it's literal.
	m2, _ := m.dispatchCommand(":grep -F $10.00")
	if len(m2.filterIndices) != 1 {
		t.Errorf("filterIndices = %v, want 1 entry for literal match", m2.filterIndices)
	}
}

func TestGrepCommand_CaseInsensitive(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.buf.Append(logmsg.LogMessage{Raw: "ERROR: something"})
	m.buf.Append(logmsg.LogMessage{Raw: "error: other"})
	m.buf.Append(logmsg.LogMessage{Raw: "info: skip"})

	m2, _ := m.dispatchCommand(":grep -i error")
	if len(m2.filterIndices) != 2 {
		t.Errorf("filterIndices = %v, want 2 entries (case-insensitive)", m2.filterIndices)
	}
}

func TestGrepCommand_UnknownFlagKeepsPaletteOpen(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":grep -z pattern")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true after unknown flag")
	}
	if m2.cmdErr == "" {
		t.Error("cmdErr should be set for unknown flag")
	}
}

// --- :pretty command ---

func TestDispatchCommand_ViewRaw(t *testing.T) {
	m := newTestModel(t)
	m.view = viewModeColor
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":view raw")
	if m2.cmdActive {
		t.Error("cmdActive should be false after :view raw")
	}
	if m2.view != viewModeRaw {
		t.Error("view should be raw after :view raw")
	}
}

func TestDispatchCommand_ViewColor(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":view color")
	if m2.view != viewModeColor {
		t.Error("view should be color after :view color")
	}
}

func TestDispatchCommand_ViewPretty(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":view pretty")
	if m2.view != viewModePretty {
		t.Error("view should be pretty after :view pretty")
	}
}

func TestDispatchCommand_ViewBadArg(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":view badarg")
	if !m2.cmdActive {
		t.Error("cmdActive should remain true after :view with bad argument")
	}
	if m2.cmdErr == "" {
		t.Error("cmdErr should be set for unrecognised :pretty argument")
	}
}

// TestDispatchCommand_Search sets searchRe and positions viewport.
func TestDispatchCommand_Search(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m.buf.Append(logmsg.LogMessage{Raw: "alpha"})
	m.buf.Append(logmsg.LogMessage{Raw: "beta"})
	m.buf.Append(logmsg.LogMessage{Raw: "alpha again"})

	m2, _ := m.dispatchCommand(":search alpha")
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
