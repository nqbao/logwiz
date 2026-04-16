package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/nqbao/logwiz/internal/buffer"
)

// LogViewport renders log lines directly from *buffer.Buffer at View() time —
// no intermediate copy, no SetContent call needed.
type LogViewport struct {
	Width  int
	Height int

	YOffset  int // top visible logical row index
	xOffset  int // horizontal scroll in terminal columns
	Selected int // selected row index; -1 = none

	longestLineWidth int // updated each View() call; used to clamp horizontal scroll
}

func newLogViewport() LogViewport {
	return LogViewport{Selected: -1}
}

// clamp returns v clamped to [lo, hi].
func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// maxYOffset returns the largest valid YOffset for the given buffer.
func (v *LogViewport) maxYOffset(buf *buffer.Buffer) int {
	m := buf.Len() - v.Height
	if m < 0 {
		m = 0
	}
	return m
}

// AtTop reports whether the viewport is scrolled to the very top.
func (v *LogViewport) AtTop() bool {
	return v.YOffset <= 0
}

// AtBottom reports whether the viewport is scrolled to the very bottom.
func (v *LogViewport) AtBottom(buf *buffer.Buffer) bool {
	return v.YOffset >= v.maxYOffset(buf)
}

// ScrollUp scrolls up by n logical rows (clamped at top).
func (v *LogViewport) ScrollUp(n int) {
	v.YOffset -= n
	if v.YOffset < 0 {
		v.YOffset = 0
	}
}

// ScrollDown scrolls down by n logical rows (clamped at bottom).
func (v *LogViewport) ScrollDown(n int, buf *buffer.Buffer) {
	v.YOffset += n
	if m := v.maxYOffset(buf); v.YOffset > m {
		v.YOffset = m
	}
}

// PageUp scrolls up by one viewport height.
func (v *LogViewport) PageUp() {
	v.ScrollUp(v.Height)
}

// PageDown scrolls down by one viewport height.
func (v *LogViewport) PageDown(buf *buffer.Buffer) {
	v.ScrollDown(v.Height, buf)
}

// HalfPageUp scrolls up by half a viewport height.
func (v *LogViewport) HalfPageUp() {
	v.ScrollUp(v.Height / 2)
}

// HalfPageDown scrolls down by half a viewport height.
func (v *LogViewport) HalfPageDown(buf *buffer.Buffer) {
	v.ScrollDown(v.Height/2, buf)
}

// GotoTop scrolls to the very beginning.
func (v *LogViewport) GotoTop() {
	v.YOffset = 0
}

// GotoBottom scrolls to the very end.
func (v *LogViewport) GotoBottom(buf *buffer.Buffer) {
	v.YOffset = v.maxYOffset(buf)
}

// ScrollLeft scrolls left by n columns (clamped at 0).
func (v *LogViewport) ScrollLeft(n int) {
	v.xOffset -= n
	if v.xOffset < 0 {
		v.xOffset = 0
	}
}

// ScrollRight scrolls right by n columns (clamped at longestLineWidth - Width).
func (v *LogViewport) ScrollRight(n int) {
	v.xOffset += n
	if m := v.longestLineWidth - v.Width; m > 0 && v.xOffset > m {
		v.xOffset = m
	}
}

// ScrollPercent returns the scroll position as a value in [0, 1].
func (v *LogViewport) ScrollPercent(buf *buffer.Buffer) float64 {
	maxY := v.maxYOffset(buf)
	if maxY == 0 {
		return 1.0
	}
	return float64(v.YOffset) / float64(maxY)
}

// SelectAt maps a screen-relative Y coordinate to a logical row index.
// Call this from a tea.MouseMsg left-click handler.
func (v *LogViewport) SelectAt(screenY int, buf *buffer.Buffer) {
	n := buf.Len()
	if n == 0 {
		v.Selected = -1
		return
	}
	row := v.YOffset + screenY
	v.Selected = clamp(row, 0, n-1)
}

// View renders the visible portion of buf into a fixed Width×Height string.
// Lines are ANSI-aware: horizontal scroll via xOffset, and the selected row
// is rendered with selectedStyle.
func (v *LogViewport) View(buf *buffer.Buffer, selectedStyle lipgloss.Style) string {
	n := buf.Len()
	top := clamp(v.YOffset, 0, max(0, n-1))
	bot := clamp(top+v.Height, top, n)

	var sb strings.Builder
	v.longestLineWidth = 0

	for i := top; i < bot; i++ {
		line := buf.Line(i)

		// Track longest for horizontal-scroll clamping.
		if w := ansi.StringWidth(line); w > v.longestLineWidth {
			v.longestLineWidth = w
		}

		// Horizontal scroll.
		if v.xOffset > 0 || ansi.StringWidth(line) > v.Width {
			line = ansi.Cut(line, v.xOffset, v.xOffset+v.Width)
		}

		// Row highlight.
		if i == v.Selected {
			line = selectedStyle.Width(v.Width).Render(line)
		}

		if i > top {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
	}

	return lipgloss.NewStyle().
		Width(v.Width).
		Height(v.Height).
		MaxHeight(v.Height).
		MaxWidth(v.Width).
		Render(sb.String())
}
