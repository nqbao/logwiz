package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/nqbao/logwiz/internal/buffer"
)

// LogViewport renders log lines directly from *buffer.Buffer at View() time —
// no intermediate copy, no SetContent call needed.
//
// When Wrap is false (default) the viewport scrolls horizontally and YOffset
// counts logical lines. When Wrap is true YOffset counts screen rows; long
// lines are soft-wrapped and a prefix-sum cache of per-line heights is kept
// in sync incrementally via OnNewLines.
type LogViewport struct {
	Width  int
	Height int
	Wrap   bool // soft-wrap long lines; disables horizontal scroll

	YOffset    int  // first visible unit (screen row when Wrap, logical line otherwise)
	xOffset    int  // horizontal scroll columns (Wrap=false only)
	Selected   int  // selected logical line index; -1 = none
	autoScroll bool // follow the tail when new lines arrive

	buf *buffer.Buffer

	// Wrap=true: per-line wrap-height cache keyed to wrapCacheWidth.
	wrapCache      []int
	wrapCacheWidth int
	totalRows      int // sum of wrapCache

	// Wrap=false: longest visible line width for horizontal-scroll clamping.
	longestLineWidth int
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

// --- wrap-height cache ---

// lineWrapHeight returns the number of screen rows required to display line
// at the current viewport width.
func (v *LogViewport) lineWrapHeight(line string) int {
	if v.Width <= 0 {
		return 1
	}
	w := ansi.StringWidth(line)
	if w == 0 {
		return 1
	}
	return (w + v.Width - 1) / v.Width
}

// recomputeWrapCache rebuilds the full cache in O(N).
// Called on resize (width change) or when the cache is found stale.
func (v *LogViewport) recomputeWrapCache() {
	n := v.buf.Len()
	if cap(v.wrapCache) >= n {
		v.wrapCache = v.wrapCache[:n]
	} else {
		v.wrapCache = make([]int, n)
	}
	v.totalRows = 0
	for i := 0; i < n; i++ {
		h := v.lineWrapHeight(v.buf.Line(i).Raw)
		v.wrapCache[i] = h
		v.totalRows += h
	}
	v.wrapCacheWidth = v.Width
}

// updateWrapCache incrementally appends heights for newLines newly-appended
// lines and evicts heights for any lines the ring buffer has overwritten.
// O(newLines) when the cache is already at the right width.
func (v *LogViewport) updateWrapCache(newLines int) {
	if v.Width <= 0 {
		return
	}
	if v.wrapCacheWidth != v.Width {
		v.recomputeWrapCache()
		return
	}

	bufLen := v.buf.Len()
	bufCap := v.buf.Cap()

	// How many old entries were overwritten by the ring?
	evictions := max(0, len(v.wrapCache)+newLines-bufCap)
	if evictions > len(v.wrapCache) {
		// Cache is more stale than expected; full rebuild.
		v.recomputeWrapCache()
		return
	}

	// Remove evicted heights from the front.
	for i := 0; i < evictions; i++ {
		v.totalRows -= v.wrapCache[i]
	}
	if evictions > 0 {
		copy(v.wrapCache, v.wrapCache[evictions:])
		v.wrapCache = v.wrapCache[:len(v.wrapCache)-evictions]
	}

	// Append heights for the new lines (last newLines entries in the buffer).
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}
	for i := start; i < bufLen; i++ {
		h := v.lineWrapHeight(v.buf.Line(i).Raw)
		v.wrapCache = append(v.wrapCache, h)
		v.totalRows += h
	}
}

// ensureWrapCache is a safety net: rebuilds the cache if it is stale.
// It is O(1) when the cache is current (the common path).
func (v *LogViewport) ensureWrapCache() {
	if v.Width <= 0 {
		return
	}
	if v.wrapCacheWidth == v.Width && len(v.wrapCache) == v.buf.Len() {
		return
	}
	v.recomputeWrapCache()
}

// screenRowToLine maps a screen-row index to (logicalLine, wrapRow within
// that line). wrapRow is 0-based.
func (v *LogViewport) screenRowToLine(row int) (logicalLine, wrapRow int) {
	cum := 0
	for i, h := range v.wrapCache {
		if cum+h > row {
			return i, row - cum
		}
		cum += h
	}
	// Past the end: clamp to the last row of the last line.
	n := len(v.wrapCache)
	if n == 0 {
		return 0, 0
	}
	return n - 1, v.wrapCache[n-1] - 1
}

// --- geometry ---

func (v *LogViewport) totalScreenRows() int {
	if v.Wrap {
		return v.totalRows
	}
	return v.buf.Len()
}

func (v *LogViewport) maxYOffset() int {
	m := v.totalScreenRows() - v.Height
	if m < 0 {
		m = 0
	}
	return m
}

// ToggleWrap switches between soft-wrap and horizontal-scroll modes.
// YOffset is clamped to the new maxYOffset after the mode switch.
func (v *LogViewport) ToggleWrap() {
	v.Wrap = !v.Wrap
	if v.Wrap {
		v.recomputeWrapCache()
	}
	v.YOffset = clamp(v.YOffset, 0, v.maxYOffset())
	if v.autoScroll {
		v.GotoBottom()
	}
}

// SetSize updates dimensions and re-applies auto-scroll.
// Triggers a full cache rebuild when Wrap is on and the width changed.
func (v *LogViewport) SetSize(width, height int) {
	widthChanged := width != v.Width
	v.Width = width
	v.Height = height
	if v.Wrap && widthChanged {
		v.recomputeWrapCache()
	}
	if v.autoScroll {
		v.GotoBottom()
	}
}

// --- scroll API ---

func (v *LogViewport) AtTop() bool    { return v.YOffset <= 0 }
func (v *LogViewport) AtBottom() bool { return v.YOffset >= v.maxYOffset() }

func (v *LogViewport) ScrollUp(n int) {
	v.YOffset -= n
	if v.YOffset < 0 {
		v.YOffset = 0
	}
}

func (v *LogViewport) ScrollDown(n int) {
	v.YOffset += n
	if m := v.maxYOffset(); v.YOffset > m {
		v.YOffset = m
	}
}

func (v *LogViewport) PageUp()        { v.ScrollUp(v.Height) }
func (v *LogViewport) PageDown()      { v.ScrollDown(v.Height) }
func (v *LogViewport) HalfPageUp()    { v.ScrollUp(v.Height / 2) }
func (v *LogViewport) HalfPageDown()  { v.ScrollDown(v.Height / 2) }
func (v *LogViewport) GotoTop()       { v.YOffset = 0 }
func (v *LogViewport) GotoBottom()    { v.YOffset = v.maxYOffset() }

func (v *LogViewport) ScrollLeft(n int) {
	if v.Wrap {
		return
	}
	v.xOffset -= n
	if v.xOffset < 0 {
		v.xOffset = 0
	}
}

func (v *LogViewport) ScrollRight(n int) {
	if v.Wrap {
		return
	}
	v.xOffset += n
	if m := v.longestLineWidth - v.Width; m > 0 && v.xOffset > m {
		v.xOffset = m
	}
}

func (v *LogViewport) ScrollPercent() float64 {
	m := v.maxYOffset()
	if m == 0 {
		return 1.0
	}
	return float64(v.YOffset) / float64(m)
}

// SelectAt maps a screen-relative Y coordinate to a logical row index.
func (v *LogViewport) SelectAt(screenY int) {
	n := v.buf.Len()
	if n == 0 {
		v.Selected = -1
		return
	}
	if !v.Wrap {
		v.Selected = clamp(v.YOffset+screenY, 0, n-1)
		return
	}
	v.ensureWrapCache()
	line, _ := v.screenRowToLine(v.YOffset + screenY)
	v.Selected = clamp(line, 0, n-1)
}

// OnNewLines must be called after n lines have been appended to the buffer.
// It keeps the wrap-height cache in sync and applies auto-scroll.
func (v *LogViewport) OnNewLines(n int) {
	if v.Wrap {
		v.updateWrapCache(n)
	}
	if v.autoScroll {
		v.GotoBottom()
	}
}

// Update handles tea.KeyMsg and tea.MouseMsg for the log pane.
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
func (v *LogViewport) View(selectedStyle lipgloss.Style) string {
	if v.Wrap {
		return v.viewWrap(selectedStyle)
	}
	return v.viewNoWrap(selectedStyle)
}

func (v *LogViewport) viewNoWrap(selectedStyle lipgloss.Style) string {
	n := v.buf.Len()
	top := clamp(v.YOffset, 0, max(0, n-1))
	bot := clamp(top+v.Height, top, n)

	var sb strings.Builder
	v.longestLineWidth = 0

	for i := top; i < bot; i++ {
		line := v.buf.Line(i).Raw

		if w := ansi.StringWidth(line); w > v.longestLineWidth {
			v.longestLineWidth = w
		}
		if v.xOffset > 0 || ansi.StringWidth(line) > v.Width {
			line = ansi.Cut(line, v.xOffset, v.xOffset+v.Width)
		}
		if i == v.Selected {
			line = selectedStyle.Width(v.Width).Render(line)
		}

		if i > top {
			sb.WriteByte('\n')
		}
		sb.WriteString(line)
	}

	return lipgloss.NewStyle().
		Width(v.Width).Height(v.Height).MaxHeight(v.Height).MaxWidth(v.Width).
		Render(sb.String())
}

func (v *LogViewport) viewWrap(selectedStyle lipgloss.Style) string {
	v.ensureWrapCache()

	n := v.buf.Len()
	if n == 0 || v.Width <= 0 {
		return lipgloss.NewStyle().
			Width(v.Width).Height(v.Height).MaxHeight(v.Height).
			Render("")
	}

	yOff := clamp(v.YOffset, 0, v.maxYOffset())
	startLine, startWrapRow := v.screenRowToLine(yOff)

	var sb strings.Builder
	rowsWritten := 0
	first := true

	for i := startLine; i < n && rowsWritten < v.Height; i++ {
		line := v.buf.Line(i).Raw
		h := 1
		if i < len(v.wrapCache) {
			h = v.wrapCache[i]
		}

		wStart := 0
		if i == startLine {
			wStart = startWrapRow
		}

		for wr := wStart; wr < h && rowsWritten < v.Height; wr++ {
			segment := ansi.Cut(line, wr*v.Width, (wr+1)*v.Width)
			if i == v.Selected {
				segment = selectedStyle.Width(v.Width).Render(segment)
			}
			if !first {
				sb.WriteByte('\n')
			}
			sb.WriteString(segment)
			first = false
			rowsWritten++
		}
	}

	return lipgloss.NewStyle().
		Width(v.Width).Height(v.Height).MaxHeight(v.Height).MaxWidth(v.Width).
		Render(sb.String())
}
