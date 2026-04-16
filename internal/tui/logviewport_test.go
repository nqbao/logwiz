package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/muesli/termenv"

	"github.com/nqbao/logwiz/internal/buffer"
)

// TestMain forces TrueColor output so lipgloss renders ANSI escape codes
// even when stdout is not a TTY (i.e., during `go test`).
func TestMain(m *testing.M) {
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}

// noStyle is a no-op style used wherever a selectedStyle is required but
// no visual selection is being tested.
var noStyle = lipgloss.NewStyle()

// newTestViewport creates a LogViewport backed by a fresh buffer filled with
// the given lines.
func newTestViewport(width, height int, lines ...string) *LogViewport {
	b := buffer.New(max(len(lines), 1))
	for _, l := range lines {
		b.Append(l)
	}
	v := newLogViewport(b)
	v.Width = width
	v.Height = height
	return &v
}

// --- clamp ---

func TestClamp(t *testing.T) {
	if got := clamp(5, 0, 10); got != 5 {
		t.Errorf("clamp(5,0,10) = %d, want 5", got)
	}
	if got := clamp(-1, 0, 10); got != 0 {
		t.Errorf("clamp(-1,0,10) = %d, want 0", got)
	}
	if got := clamp(11, 0, 10); got != 10 {
		t.Errorf("clamp(11,0,10) = %d, want 10", got)
	}
}

// --- AtTop / AtBottom ---

func TestAtTopAndBottom_EmptyBuffer(t *testing.T) {
	v := newTestViewport(80, 10)
	if !v.AtTop() {
		t.Error("AtTop() should be true for empty buffer")
	}
	if !v.AtBottom() {
		t.Error("AtBottom() should be true for empty buffer")
	}
}

func TestAtTop_AfterScrollDown(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.ScrollDown(2)
	if v.AtTop() {
		t.Error("AtTop() should be false after scrolling down")
	}
}

func TestAtBottom_WhenFewLines(t *testing.T) {
	v := newTestViewport(80, 10, "a", "b")
	if !v.AtBottom() {
		t.Error("AtBottom() should be true when lines <= height")
	}
}

// --- ScrollUp / ScrollDown ---

func TestScrollDownClampedAtMax(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e") // maxYOffset = 5-3 = 2
	v.ScrollDown(100)
	if v.YOffset != 2 {
		t.Errorf("YOffset = %d, want 2", v.YOffset)
	}
	if !v.AtBottom() {
		t.Error("should be at bottom after over-scroll")
	}
}

func TestScrollUpClampedAtZero(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.ScrollDown(2)
	v.ScrollUp(100)
	if v.YOffset != 0 {
		t.Errorf("YOffset = %d, want 0", v.YOffset)
	}
	if !v.AtTop() {
		t.Error("should be at top after over-scroll up")
	}
}

// --- PageUp / PageDown ---

func TestPageDown(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f")
	v.PageDown()
	if v.YOffset != 3 {
		t.Errorf("after PageDown YOffset = %d, want 3", v.YOffset)
	}
}

func TestPageUp(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f")
	v.GotoBottom()
	want := v.maxYOffset() - v.Height
	if want < 0 {
		want = 0
	}
	v.PageUp()
	if v.YOffset != want {
		t.Errorf("after PageUp YOffset = %d, want %d", v.YOffset, want)
	}
}

// --- HalfPageUp / HalfPageDown ---

func TestHalfPage(t *testing.T) {
	v := newTestViewport(80, 4, "a", "b", "c", "d", "e", "f", "g", "h")
	v.HalfPageDown()
	if v.YOffset != 2 {
		t.Errorf("after HalfPageDown YOffset = %d, want 2", v.YOffset)
	}
	v.HalfPageUp()
	if v.YOffset != 0 {
		t.Errorf("after HalfPageUp YOffset = %d, want 0", v.YOffset)
	}
}

// --- GotoTop / GotoBottom ---

func TestGotoTopAndBottom(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.GotoBottom()
	if !v.AtBottom() {
		t.Error("GotoBottom should put viewport at bottom")
	}
	v.GotoTop()
	if !v.AtTop() {
		t.Error("GotoTop should put viewport at top")
	}
}

// --- ScrollPercent ---

func TestScrollPercent(t *testing.T) {
	v := newTestViewport(80, 2, "a", "b", "c", "d") // maxYOffset = 2
	if got := v.ScrollPercent(); got != 0.0 {
		t.Errorf("ScrollPercent at top = %v, want 0.0", got)
	}
	v.GotoBottom()
	if got := v.ScrollPercent(); got != 1.0 {
		t.Errorf("ScrollPercent at bottom = %v, want 1.0", got)
	}
}

func TestScrollPercentEmptyBuffer(t *testing.T) {
	v := newTestViewport(80, 10)
	if got := v.ScrollPercent(); got != 1.0 {
		t.Errorf("ScrollPercent on empty = %v, want 1.0", got)
	}
}

// --- SetSize ---

func TestSetSize_AutoScrollGotosBottom(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.autoScroll = true
	v.SetSize(80, 2) // maxYOffset is now 3
	if !v.AtBottom() {
		t.Errorf("SetSize with autoScroll should stay at bottom; YOffset=%d", v.YOffset)
	}
}

// --- OnNewLines ---

func TestOnNewLines_AutoScrollEnabled(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c")
	v.autoScroll = true
	v.buf.Append("d")
	v.buf.Append("e")
	v.OnNewLines()
	if !v.AtBottom() {
		t.Errorf("OnNewLines with autoScroll should scroll to bottom; YOffset=%d", v.YOffset)
	}
}

func TestOnNewLines_AutoScrollDisabled(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c")
	v.autoScroll = false
	v.YOffset = 0
	v.buf.Append("d")
	v.buf.Append("e")
	v.OnNewLines()
	if v.YOffset != 0 {
		t.Errorf("OnNewLines without autoScroll should not move; YOffset=%d", v.YOffset)
	}
}

// --- SelectAt ---

func TestSelectAt(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c", "d", "e")
	v.SelectAt(2) // screen row 2, YOffset=0 → logical row 2
	if v.Selected != 2 {
		t.Errorf("Selected = %d, want 2", v.Selected)
	}
}

func TestSelectAt_WithYOffset(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.ScrollDown(2) // YOffset = 2
	v.SelectAt(1)   // logical row = 2+1 = 3 → "d"
	if v.Selected != 3 {
		t.Errorf("Selected = %d, want 3", v.Selected)
	}
}

func TestSelectAt_EmptyBuffer(t *testing.T) {
	v := newTestViewport(80, 5)
	v.SelectAt(0)
	if v.Selected != -1 {
		t.Errorf("Selected = %d on empty buffer, want -1", v.Selected)
	}
}

func TestSelectAt_ClampedToLast(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c")
	v.SelectAt(100)
	if v.Selected != 2 {
		t.Errorf("Selected = %d, want 2 (last line)", v.Selected)
	}
}

// --- Update (KeyMsg) ---

func TestUpdate_KeyScrollDown(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.autoScroll = false
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if v.YOffset != 1 {
		t.Errorf("j key: YOffset = %d, want 1", v.YOffset)
	}
}

func TestUpdate_KeyScrollUp(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.autoScroll = false
	v.ScrollDown(2)
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if v.YOffset != 1 {
		t.Errorf("k key: YOffset = %d, want 1", v.YOffset)
	}
}

func TestUpdate_KeyPageDown(t *testing.T) {
	v := newTestViewport(80, 2, "a", "b", "c", "d", "e", "f")
	v.autoScroll = false
	v.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if v.YOffset != 2 {
		t.Errorf("pgdown key: YOffset = %d, want 2", v.YOffset)
	}
}

func TestUpdate_KeyPageUp(t *testing.T) {
	v := newTestViewport(80, 2, "a", "b", "c", "d", "e", "f")
	v.autoScroll = false
	v.GotoBottom()
	v.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	if v.YOffset != 2 { // maxYOffset=4, 4-2=2
		t.Errorf("pgup key: YOffset = %d, want 2", v.YOffset)
	}
}

func TestUpdate_KeyScrollDisablesAutoScroll(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.autoScroll = true
	v.GotoBottom()
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'k'}})
	if v.autoScroll {
		t.Error("scrolling up should disable autoScroll")
	}
}

func TestUpdate_ReachingBottomEnablesAutoScroll(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	v.autoScroll = false
	v.ScrollDown(100)
	// Already at bottom; pressing j again should re-enable autoScroll.
	v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	if !v.autoScroll {
		t.Error("reaching bottom should re-enable autoScroll")
	}
}

// --- Update (MouseMsg) ---

func TestUpdate_MouseWheelUp(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f", "g")
	v.autoScroll = false
	v.ScrollDown(4)
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if v.YOffset != 1 {
		t.Errorf("wheel up: YOffset = %d, want 1", v.YOffset)
	}
}

func TestUpdate_MouseWheelDown(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f")
	v.autoScroll = false
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if v.YOffset != 3 {
		t.Errorf("wheel down: YOffset = %d, want 3", v.YOffset)
	}
}

func TestUpdate_MouseWheelDownAtBottomEnablesAutoScroll(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d")
	v.autoScroll = false
	v.GotoBottom()
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if !v.autoScroll {
		t.Error("wheel down at bottom should re-enable autoScroll")
	}
}

func TestUpdate_MouseLeftClickSelects(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c", "d", "e")
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, Y: 2})
	if v.Selected != 2 {
		t.Errorf("left click Y=2: Selected = %d, want 2", v.Selected)
	}
}

func TestUpdate_MouseReleaseIgnored(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e")
	before := v.YOffset
	v.Update(tea.MouseMsg{Action: tea.MouseActionRelease, Button: tea.MouseButtonWheelDown})
	if v.YOffset != before {
		t.Error("mouse release should be ignored")
	}
}

// --- View ---

func TestView_RendersVisibleLines(t *testing.T) {
	v := newTestViewport(20, 3, "line0", "line1", "line2", "line3", "line4")
	out := v.View(noStyle)
	for _, want := range []string{"line0", "line1", "line2"} {
		if !strings.Contains(out, want) {
			t.Errorf("View() missing %q; got:\n%s", want, out)
		}
	}
	if strings.Contains(out, "line3") {
		t.Errorf("View() should not show line3 at YOffset=0, height=3")
	}
}

func TestView_AfterScrollDown(t *testing.T) {
	v := newTestViewport(20, 3, "line0", "line1", "line2", "line3", "line4")
	v.ScrollDown(2)
	out := v.View(noStyle)
	for _, want := range []string{"line2", "line3", "line4"} {
		if !strings.Contains(out, want) {
			t.Errorf("after scroll View() missing %q; got:\n%s", want, out)
		}
	}
}

func TestView_EmptyBuffer(t *testing.T) {
	v := newTestViewport(20, 5)
	out := v.View(noStyle)
	if out == "" {
		t.Error("View() on empty buffer returned empty string")
	}
}

func TestView_SelectedRowHighlighted(t *testing.T) {
	b := buffer.New(3)
	for _, l := range []string{"alpha", "beta", "gamma"} {
		b.Append(l)
	}
	vSel := newLogViewport(b)
	vSel.Width = 30
	vSel.Height = 5
	vSel.Selected = 1
	sel := lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("255"))
	outSel := vSel.View(sel)

	vNone := newLogViewport(b)
	vNone.Width = 30
	vNone.Height = 5
	vNone.Selected = -1
	outNone := vNone.View(noStyle)

	if outSel == outNone {
		t.Error("selected row render should differ from unselected render")
	}
	if !strings.Contains(outSel, "\x1b[") {
		t.Error("selected row should produce ANSI escape codes")
	}
}

func TestView_HorizontalScroll(t *testing.T) {
	v := newTestViewport(5, 3, "abcdefghij", "0123456789", "ABCDEFGHIJ")
	out := v.View(noStyle)
	if !strings.Contains(out, "abcde") {
		t.Errorf("expected 'abcde' at xOffset=0, got:\n%s", out)
	}
	v.ScrollRight(5)
	out = v.View(noStyle)
	if !strings.Contains(out, "fghij") {
		t.Errorf("expected 'fghij' after ScrollRight(5), got:\n%s", out)
	}
	if strings.Contains(out, "abcde") {
		t.Errorf("should not contain 'abcde' after scroll right")
	}
}

func TestView_ScrollLeft(t *testing.T) {
	v := newTestViewport(5, 2, "abcdefghij", "0123456789")
	v.ScrollRight(5)
	v.View(noStyle) // populate longestLineWidth
	v.ScrollLeft(5)
	out := v.View(noStyle)
	if !strings.Contains(out, "abcde") {
		t.Errorf("after ScrollLeft expected 'abcde', got:\n%s", out)
	}
}
