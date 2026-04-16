package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	"github.com/nqbao/logwiz/internal/buffer"
)

// LogViewport renders log lines directly from *buffer.Buffer at View() time —
// no intermediate copy, no SetContent call needed.
type LogViewport struct {
	Width  int
	Height int

	YOffset    int  // top visible logical row index
	xOffset    int  // horizontal scroll in terminal columns
	Selected   int  // selected row index; -1 = none
	autoScroll bool // follow the tail when new lines arrive

	buf              *buffer.Buffer
	longestLineWidth int // updated each View() call; used to clamp horizontal scroll
}

func newLogViewport(buf *buffer.Buffer) LogViewport {
	return LogViewport{buf: buf, Selected: -1, autoScroll: true}
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

func (v *LogViewport) maxYOffset() int {
	m := v.buf.Len() - v.Height
	if m < 0 {
		m = 0
	}
	return m
}

// SetSize updates the viewport dimensions and re-applies auto-scroll if active.
func (v *LogViewport) SetSize(width, height int) {
	v.Width = width
	v.Height = height
	if v.autoScroll {
		v.GotoBottom()
	}
}

// AtTop reports whether the viewport is scrolled to the very top.
func (v *LogViewport) AtTop() bool {
	return v.YOffset <= 0
}

// AtBottom reports whether the viewport is scrolled to the very bottom.
func (v *LogViewport) AtBottom() bool {
	return v.YOffset >= v.maxYOffset()
}

// ScrollUp scrolls up by n logical rows (clamped at top).
func (v *LogViewport) ScrollUp(n int) {
	v.YOffset -= n
	if v.YOffset < 0 {
		v.YOffset = 0
	}
}

// ScrollDown scrolls down by n logical rows (clamped at bottom).
func (v *LogViewport) ScrollDown(n int) {
	v.YOffset += n
	if m := v.maxYOffset(); v.YOffset > m {
		v.YOffset = m
	}
}

// PageUp scrolls up by one viewport height.
func (v *LogViewport) PageUp() { v.ScrollUp(v.Height) }

// PageDown scrolls down by one viewport height.
func (v *LogViewport) PageDown() { v.ScrollDown(v.Height) }

// HalfPageUp scrolls up by half a viewport height.
func (v *LogViewport) HalfPageUp() { v.ScrollUp(v.Height / 2) }

// HalfPageDown scrolls down by half a viewport height.
func (v *LogViewport) HalfPageDown() { v.ScrollDown(v.Height / 2) }

// GotoTop scrolls to the very beginning.
func (v *LogViewport) GotoTop() { v.YOffset = 0 }

// GotoBottom scrolls to the very end.
func (v *LogViewport) GotoBottom() { v.YOffset = v.maxYOffset() }

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
func (v *LogViewport) ScrollPercent() float64 {
	maxY := v.maxYOffset()
	if maxY == 0 {
		return 1.0
	}
	return float64(v.YOffset) / float64(maxY)
}

// SelectAt maps a screen-relative Y coordinate to a logical row index.
func (v *LogViewport) SelectAt(screenY int) {
	n := v.buf.Len()
	if n == 0 {
		v.Selected = -1
		return
	}
	v.Selected = clamp(v.YOffset+screenY, 0, n-1)
}

// OnNewLines should be called after lines are appended to the buffer.
// It scrolls to the bottom when auto-scroll is active.
func (v *LogViewport) OnNewLines() {
	if v.autoScroll {
		v.GotoBottom()
	}
}

// Update handles tea.KeyMsg and tea.MouseMsg for the log pane.
// The caller is responsible for filtering out messages that belong to other
// widgets (e.g. shift+wheel for the chat pane).
func (v *LogViewport) Update(msg tea.Msg) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		prevAtBottom := v.AtBottom()
		switch msg.String() {
		case keyScrollDown, keyArrowDown:
			v.ScrollDown(1)
		case keyScrollUp, keyArrowUp:
			v.ScrollUp(1)
		case keyPageDown, keyPageDownSpace, keyPageDownF:
			v.PageDown()
		case keyPageUp, keyPageUpB:
			v.PageUp()
		case keyHalfPageDown, keyHalfPageDownCtrl:
			v.HalfPageDown()
		case keyHalfPageUp, keyHalfPageUpCtrl:
			v.HalfPageUp()
		case keyScrollLeft, keyArrowLeft:
			v.ScrollLeft(1)
		case keyScrollRight, keyArrowRight:
			v.ScrollRight(1)
		}
		if prevAtBottom && !v.AtBottom() {
			v.autoScroll = false
		}
		if v.AtBottom() {
			v.autoScroll = true
		}

	case tea.MouseMsg:
		if msg.Action != tea.MouseActionPress {
			return
		}
		switch msg.Button {
		case tea.MouseButtonLeft:
			v.SelectAt(msg.Y)
		case tea.MouseButtonWheelUp:
			v.ScrollUp(3)
			v.autoScroll = false
		case tea.MouseButtonWheelDown:
			v.ScrollDown(3)
			if v.AtBottom() {
				v.autoScroll = true
			}
		case tea.MouseButtonWheelLeft:
			v.ScrollLeft(3)
		case tea.MouseButtonWheelRight:
			v.ScrollRight(3)
		}
	}
}

// View renders the visible portion of the buffer into a fixed Width×Height string.
// Lines are ANSI-aware: horizontal scroll via xOffset, and the selected row
// is rendered with selectedStyle.
func (v *LogViewport) View(selectedStyle lipgloss.Style) string {
	n := v.buf.Len()
	top := clamp(v.YOffset, 0, max(0, n-1))
	bot := clamp(top+v.Height, top, n)

	var sb strings.Builder
	v.longestLineWidth = 0

	for i := top; i < bot; i++ {
		line := v.buf.Line(i)

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
