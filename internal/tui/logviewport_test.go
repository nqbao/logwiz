package tui

import (
	"os"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
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

// makeBuffer fills a buffer with the given lines.
func makeBuffer(lines ...string) *buffer.Buffer {
	b := buffer.New(max(len(lines), 1))
	for _, l := range lines {
		b.Append(l)
	}
	return b
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
	v := newLogViewport()
	v.Width = 80
	v.Height = 10
	buf := makeBuffer()

	if !v.AtTop() {
		t.Error("AtTop() should be true for empty buffer")
	}
	if !v.AtBottom(buf) {
		t.Error("AtBottom() should be true for empty buffer")
	}
}

func TestAtTop_AfterScrollDown(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e")

	v.ScrollDown(2, buf)
	if v.AtTop() {
		t.Error("AtTop() should be false after scrolling down")
	}
}

func TestAtBottom_WhenFewLines(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 10
	buf := makeBuffer("a", "b")

	if !v.AtBottom(buf) {
		t.Error("AtBottom() should be true when lines <= height")
	}
}

// --- ScrollUp / ScrollDown ---

func TestScrollDownClampedAtMax(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e") // maxYOffset = 5-3 = 2

	v.ScrollDown(100, buf)
	if v.YOffset != 2 {
		t.Errorf("YOffset = %d, want 2", v.YOffset)
	}
	if !v.AtBottom(buf) {
		t.Error("should be at bottom after over-scroll")
	}
}

func TestScrollUpClampedAtZero(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e")

	v.ScrollDown(2, buf)
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
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e", "f")

	v.PageDown(buf)
	if v.YOffset != 3 {
		t.Errorf("after PageDown YOffset = %d, want 3", v.YOffset)
	}
}

func TestPageUp(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e", "f")

	v.GotoBottom(buf)
	v.PageUp()
	want := v.maxYOffset(buf) - v.Height
	if want < 0 {
		want = 0
	}
	if v.YOffset != want {
		t.Errorf("after PageUp YOffset = %d, want %d", v.YOffset, want)
	}
}

// --- HalfPageUp / HalfPageDown ---

func TestHalfPage(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 4
	buf := makeBuffer("a", "b", "c", "d", "e", "f", "g", "h")

	v.HalfPageDown(buf)
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
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e")

	v.GotoBottom(buf)
	if !v.AtBottom(buf) {
		t.Error("GotoBottom should put viewport at bottom")
	}
	v.GotoTop()
	if !v.AtTop() {
		t.Error("GotoTop should put viewport at top")
	}
}

// --- ScrollPercent ---

func TestScrollPercent(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 2
	buf := makeBuffer("a", "b", "c", "d") // maxYOffset = 2

	if got := v.ScrollPercent(buf); got != 0.0 {
		t.Errorf("ScrollPercent at top = %v, want 0.0", got)
	}
	v.GotoBottom(buf)
	if got := v.ScrollPercent(buf); got != 1.0 {
		t.Errorf("ScrollPercent at bottom = %v, want 1.0", got)
	}
}

func TestScrollPercentEmptyBuffer(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 10
	buf := makeBuffer()
	if got := v.ScrollPercent(buf); got != 1.0 {
		t.Errorf("ScrollPercent on empty = %v, want 1.0", got)
	}
}

// --- SelectAt ---

func TestSelectAt(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 5
	buf := makeBuffer("a", "b", "c", "d", "e")

	v.SelectAt(2, buf) // screen row 2, YOffset=0 → logical row 2
	if v.Selected != 2 {
		t.Errorf("Selected = %d, want 2", v.Selected)
	}
}

func TestSelectAt_WithYOffset(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 3
	buf := makeBuffer("a", "b", "c", "d", "e")

	v.ScrollDown(2, buf) // YOffset = 2
	v.SelectAt(1, buf)   // logical row = 2+1 = 3 → "d"
	if v.Selected != 3 {
		t.Errorf("Selected = %d, want 3", v.Selected)
	}
}

func TestSelectAt_EmptyBuffer(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 5
	buf := makeBuffer()

	v.SelectAt(0, buf)
	if v.Selected != -1 {
		t.Errorf("Selected = %d on empty buffer, want -1", v.Selected)
	}
}

func TestSelectAt_ClampedToLast(t *testing.T) {
	v := newLogViewport()
	v.Width = 80
	v.Height = 5
	buf := makeBuffer("a", "b", "c")

	v.SelectAt(100, buf) // way out of range
	if v.Selected != 2 {
		t.Errorf("Selected = %d, want 2 (last line)", v.Selected)
	}
}

// --- View ---

func TestView_RendersVisibleLines(t *testing.T) {
	v := newLogViewport()
	v.Width = 20
	v.Height = 3
	buf := makeBuffer("line0", "line1", "line2", "line3", "line4")

	out := v.View(buf, noStyle)
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
	v := newLogViewport()
	v.Width = 20
	v.Height = 3
	buf := makeBuffer("line0", "line1", "line2", "line3", "line4")

	v.ScrollDown(2, buf)
	out := v.View(buf, noStyle)
	for _, want := range []string{"line2", "line3", "line4"} {
		if !strings.Contains(out, want) {
			t.Errorf("after scroll View() missing %q; got:\n%s", want, out)
		}
	}
}

func TestView_EmptyBuffer(t *testing.T) {
	v := newLogViewport()
	v.Width = 20
	v.Height = 5
	buf := makeBuffer()

	// Should not panic; just produce an empty (padded) box.
	out := v.View(buf, noStyle)
	if out == "" {
		t.Error("View() on empty buffer returned empty string")
	}
}

func TestView_SelectedRowHighlighted(t *testing.T) {
	buf := makeBuffer("alpha", "beta", "gamma")

	// Render with a distinctive background color on row 1.
	vSel := newLogViewport()
	vSel.Width = 30
	vSel.Height = 5
	vSel.Selected = 1
	sel := lipgloss.NewStyle().Background(lipgloss.Color("237")).Foreground(lipgloss.Color("255"))
	outSel := vSel.View(buf, sel)

	// Render with no selection for comparison.
	vNone := newLogViewport()
	vNone.Width = 30
	vNone.Height = 5
	vNone.Selected = -1
	outNone := vNone.View(buf, noStyle)

	// The styled render must differ from the plain render.
	if outSel == outNone {
		t.Error("selected row render should differ from unselected render")
	}
	// The styled output should contain ANSI escape sequences.
	if !strings.Contains(outSel, "\x1b[") {
		t.Error("selected row should produce ANSI escape codes")
	}
}

func TestView_HorizontalScroll(t *testing.T) {
	v := newLogViewport()
	v.Width = 5
	v.Height = 3
	buf := makeBuffer("abcdefghij", "0123456789", "ABCDEFGHIJ")

	// At xOffset=0 we see the first 5 chars.
	out := v.View(buf, noStyle)
	if !strings.Contains(out, "abcde") {
		t.Errorf("expected 'abcde' at xOffset=0, got:\n%s", out)
	}

	// Scroll right by 5.
	v.ScrollRight(5)
	out = v.View(buf, noStyle)
	if !strings.Contains(out, "fghij") {
		t.Errorf("expected 'fghij' after ScrollRight(5), got:\n%s", out)
	}
	if strings.Contains(out, "abcde") {
		t.Errorf("should not contain 'abcde' after scroll right")
	}
}

func TestView_ScrollLeft(t *testing.T) {
	v := newLogViewport()
	v.Width = 5
	v.Height = 2
	buf := makeBuffer("abcdefghij", "0123456789")

	v.ScrollRight(5)
	// Force longestLineWidth to be updated.
	v.View(buf, noStyle)

	v.ScrollLeft(5)
	out := v.View(buf, noStyle)
	if !strings.Contains(out, "abcde") {
		t.Errorf("after ScrollLeft expected 'abcde', got:\n%s", out)
	}
}
