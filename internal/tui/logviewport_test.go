package tui

import (
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/logmsg"
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
// the given raw log lines.
func newTestViewport(width, height int, lines ...string) *LogViewport {
	b := buffer.New(max(len(lines), 1))
	for _, l := range lines {
		b.Append(logmsg.LogMessage{Raw: l})
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
	v.buf.Append(logmsg.LogMessage{Raw: "d"})
	v.buf.Append(logmsg.LogMessage{Raw: "e"})
	v.OnNewLines(2)
	if !v.AtBottom() {
		t.Errorf("OnNewLines with autoScroll should scroll to bottom; YOffset=%d", v.YOffset)
	}
}

func TestOnNewLines_AutoScrollDisabled(t *testing.T) {
	v := newTestViewport(80, 3, "a", "b", "c")
	v.autoScroll = false
	v.YOffset = 0
	v.buf.Append(logmsg.LogMessage{Raw: "d"})
	v.buf.Append(logmsg.LogMessage{Raw: "e"})
	v.OnNewLines(2)
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
	// 11 lines, height 3 → maxYOffset = 8; start at 6, wheel-up(5) → 1
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k")
	v.autoScroll = false
	v.ScrollDown(6)
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelUp})
	if v.YOffset != 1 {
		t.Errorf("wheel up: YOffset = %d, want 1", v.YOffset)
	}
}

func TestUpdate_MouseWheelDown(t *testing.T) {
	// 11 lines, height 3 → maxYOffset = 8; wheel-down(5) from 0 → 5
	v := newTestViewport(80, 3, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k")
	v.autoScroll = false
	v.Update(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonWheelDown})
	if v.YOffset != 5 {
		t.Errorf("wheel down: YOffset = %d, want 5", v.YOffset)
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
		b.Append(logmsg.LogMessage{Raw: l})
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

// --- Wrap mode ---

// newWrapViewport creates a LogViewport with Wrap=true.
func newWrapViewport(width, height int, lines ...string) *LogViewport {
	v := newTestViewport(width, height, lines...)
	v.Wrap = true
	v.recomputeWrapCache()
	return v
}

func TestWrap_LineWrapHeight(t *testing.T) {
	v := newWrapViewport(10, 20)
	cases := []struct {
		line string
		want int
	}{
		{"", 1},           // empty line occupies one row
		{"abcde", 1},      // fits exactly (5 < 10)
		{"abcdefghij", 1}, // exactly 10 chars → 1 row
		{"abcdefghijk", 2}, // 11 chars → 2 rows
		{"abcdefghijabcdefghij", 2}, // 20 chars → 2 rows
		{"abcdefghijabcdefghija", 3}, // 21 chars → 3 rows
	}
	for _, tc := range cases {
		got := v.lineWrapHeight(ansi.StringWidth(tc.line))
		if got != tc.want {
			t.Errorf("lineWrapHeight(%q) = %d, want %d", tc.line, got, tc.want)
		}
	}
}

func TestWrap_TotalRows(t *testing.T) {
	// width=5: "ab"→1 row, "abcde"→1 row, "abcdefghij"→2 rows. total=4
	v := newWrapViewport(5, 10, "ab", "abcde", "abcdefghij")
	if v.totalRows != 4 {
		t.Errorf("totalRows = %d, want 4", v.totalRows)
	}
}

func TestWrap_MaxYOffset(t *testing.T) {
	// width=5, height=3. "abcdefghij" → 2 rows. total=4 rows. maxYOffset=4-3=1
	v := newWrapViewport(5, 3, "ab", "abcde", "abcdefghij")
	if got := v.maxYOffset(); got != 1 {
		t.Errorf("maxYOffset = %d, want 1", got)
	}
}

func TestWrap_ViewRendersWrappedLine(t *testing.T) {
	// width=5, so "abcdefghij" wraps to ["abcde", "fghij"]
	v := newWrapViewport(5, 4, "hello", "abcdefghij")
	out := v.View(noStyle)
	if !strings.Contains(out, "hello") {
		t.Errorf("wrap View() missing 'hello'; got:\n%s", out)
	}
	if !strings.Contains(out, "abcde") {
		t.Errorf("wrap View() missing first wrap segment 'abcde'; got:\n%s", out)
	}
	if !strings.Contains(out, "fghij") {
		t.Errorf("wrap View() missing second wrap segment 'fghij'; got:\n%s", out)
	}
}

func TestWrap_ScrollMovesInScreenRows(t *testing.T) {
	// width=5. Line 0: "abcde" (1 row). Line 1: "abcdefghij" (2 rows). total=3 rows.
	// height=2 → maxYOffset=1.
	v := newWrapViewport(5, 2, "abcde", "abcdefghij")
	v.ScrollDown(1)
	if v.YOffset != 1 {
		t.Errorf("YOffset = %d, want 1 (screen row)", v.YOffset)
	}
	// At YOffset=1 we start in the middle of line 1 (wrapRow=0 of "abcdefghij").
	out := v.View(noStyle)
	if !strings.Contains(out, "abcde") {
		t.Errorf("after scroll, first wrap of line 1 should be visible; got:\n%s", out)
	}
}

func TestWrap_SelectAtMapsToLogicalLine(t *testing.T) {
	// width=5. Line 0: "abcde" (1 row). Line 1: "abcdefghij" (2 rows).
	v := newWrapViewport(5, 5, "abcde", "abcdefghij")
	// Screen row 1 → first wrap of line 1.
	v.SelectAt(1)
	if v.Selected != 1 {
		t.Errorf("SelectAt(1) = %d, want 1 (logical line)", v.Selected)
	}
	// Screen row 2 → second wrap of line 1, still logical line 1.
	v.SelectAt(2)
	if v.Selected != 1 {
		t.Errorf("SelectAt(2) on wrapped line = %d, want 1", v.Selected)
	}
}

func TestWrap_OnNewLinesIncrementalUpdate(t *testing.T) {
	// Use explicit capacity=3 so no eviction occurs when the third line is appended.
	b := buffer.New(3)
	b.Append(logmsg.LogMessage{Raw: "ab"})
	b.Append(logmsg.LogMessage{Raw: "abcde"})
	v := newLogViewport(b)
	v.Wrap = true
	v.Width = 5
	v.Height = 10
	v.recomputeWrapCache() // totalRows = 1+1 = 2, cache=[1,1]

	b.Append(logmsg.LogMessage{Raw: "abcdefghij"}) // 2 rows at width=5; no eviction
	v.OnNewLines(1)
	if v.totalRows != 4 {
		t.Errorf("totalRows after OnNewLines = %d, want 4", v.totalRows)
	}
	if len(v.wrapCache) != 3 {
		t.Errorf("wrapCache len = %d, want 3", len(v.wrapCache))
	}
}

func TestWrap_OnNewLinesEviction(t *testing.T) {
	// capacity=3. Fill it: "ab"(1),"abcde"(1),"abcdefghij"(2). totalRows=4, cache=[1,1,2]
	b := buffer.New(3)
	for _, l := range []string{"ab", "abcde", "abcdefghij"} {
		b.Append(logmsg.LogMessage{Raw: l})
	}
	v := newLogViewport(b)
	v.Wrap = true
	v.Width = 5
	v.Height = 10
	v.recomputeWrapCache()

	if v.totalRows != 4 {
		t.Fatalf("initial totalRows = %d, want 4", v.totalRows)
	}

	// Append "xyz" (1 row). Ring evicts "ab" (1 row). net: -1+1=0, totalRows stays 4.
	b.Append(logmsg.LogMessage{Raw: "xyz"})
	v.OnNewLines(1)
	if v.totalRows != 4 {
		t.Errorf("after eviction totalRows = %d, want 4", v.totalRows)
	}
	if len(v.wrapCache) != 3 {
		t.Errorf("wrapCache len = %d, want 3", len(v.wrapCache))
	}
}

// --- JumpTo ---

func TestJumpTo_NoWrap_BringsLineOnScreen(t *testing.T) {
	// 20 lines, height=5 → maxYOffset=15. Jump to line 15.
	lines := make([]string, 20)
	for i := range lines {
		lines[i] = "line"
	}
	v := newTestViewport(80, 5, lines...)
	v.autoScroll = false
	v.GotoTop()

	v.JumpTo(15)
	if v.Selected != 15 {
		t.Errorf("Selected = %d, want 15", v.Selected)
	}
	// Line 15 should be visible: YOffset <= 15 < YOffset+Height.
	if v.YOffset > 15 || v.YOffset+v.Height <= 15 {
		t.Errorf("line 15 not in viewport: YOffset=%d Height=%d", v.YOffset, v.Height)
	}
	if v.autoScroll {
		t.Error("JumpTo should disable autoScroll")
	}
}

func TestJumpTo_NoWrap_NearTop(t *testing.T) {
	// Jumping to line 1 with height=5 should keep YOffset at 0 (clamped).
	lines := make([]string, 10)
	for i := range lines {
		lines[i] = "x"
	}
	v := newTestViewport(80, 5, lines...)
	v.autoScroll = false
	v.GotoBottom()

	v.JumpTo(1)
	if v.Selected != 1 {
		t.Errorf("Selected = %d, want 1", v.Selected)
	}
	if v.YOffset < 0 {
		t.Errorf("YOffset = %d, want >= 0", v.YOffset)
	}
	// Line 1 must be visible.
	if v.YOffset > 1 || v.YOffset+v.Height <= 1 {
		t.Errorf("line 1 not in viewport: YOffset=%d Height=%d", v.YOffset, v.Height)
	}
}

func TestJumpTo_Wrap_BringsLineOnScreen(t *testing.T) {
	// width=5, height=4. Lines: 5×"a" (1 row each). Jump to line 4.
	lines := []string{"a", "b", "c", "d", "e", "f", "g", "h"}
	v := newWrapViewport(5, 4, lines...)
	v.autoScroll = false
	v.GotoTop()

	v.JumpTo(6)
	if v.Selected != 6 {
		t.Errorf("Selected = %d, want 6", v.Selected)
	}
	if v.autoScroll {
		t.Error("JumpTo should disable autoScroll")
	}
	// In wrap mode, the screen row for line 6 must be in [YOffset, YOffset+Height).
	screenRow := 0
	for i := 0; i < 6 && i < len(v.wrapCache); i++ {
		screenRow += v.wrapCache[i]
	}
	if screenRow < v.YOffset || screenRow >= v.YOffset+v.Height {
		t.Errorf("line 6 (screen row %d) not visible: YOffset=%d Height=%d",
			screenRow, v.YOffset, v.Height)
	}
}

func TestJumpTo_EmptyBuffer_NoOp(t *testing.T) {
	v := newTestViewport(80, 5)
	v.JumpTo(0) // should not panic
	if v.Selected != -1 {
		t.Errorf("Selected = %d on empty buffer, want -1", v.Selected)
	}
}

func TestWrap_RecomputeOnWidthChange(t *testing.T) {
	v := newWrapViewport(5, 10, "abcdefghij") // 2 rows at width=5
	if v.totalRows != 2 {
		t.Fatalf("initial totalRows = %d, want 2", v.totalRows)
	}
	// Resize to width=10: same line now fits in 1 row.
	v.SetSize(10, 10)
	if v.totalRows != 1 {
		t.Errorf("after resize totalRows = %d, want 1", v.totalRows)
	}
}

// TestSetFilter_ClampsStaleYOffset is a regression test for the blank-screen
// bug: when autoScroll is off and the user has scrolled far down, applying a
// grep/filter that returns far fewer lines must clamp YOffset so all matches
// are visible immediately without requiring manual scrolling.
func TestSetFilter_ClampsStaleYOffset(t *testing.T) {
	// 50 lines, height=10 → maxYOffset without filter = 40.
	lines := make([]string, 50)
	for i := range lines {
		lines[i] = "line"
	}
	v := newTestViewport(80, 10, lines...)
	v.autoScroll = false
	v.YOffset = 40 // user scrolled to the bottom manually

	// Apply a filter that yields only 5 matches (positions 0–4).
	indices := []int{0, 1, 2, 3, 4}
	v.SetFilter(indices)

	// With 5 filtered lines and height=10, maxYOffset=0.
	// YOffset must be clamped to 0, not left at 40.
	if v.YOffset != 0 {
		t.Errorf("after SetFilter YOffset = %d, want 0 (clamped to maxYOffset)", v.YOffset)
	}

	// The view must render all 5 matched lines, not a blank screen.
	out := v.View(noStyle)
	for i := 0; i < 5; i++ {
		if !strings.Contains(out, "line") {
			t.Errorf("View() after SetFilter with stale YOffset is blank; got:\n%s", out)
			break
		}
	}
}
