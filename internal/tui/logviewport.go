package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/highlight"
	"github.com/nqbao/logwiz/internal/logmsg"
)

// LogViewport renders log lines directly from *buffer.Buffer at View() time —
// no intermediate copy, no SetContent call needed.
//
// When Wrap is false (default) the viewport scrolls horizontally and YOffset
// counts logical lines. When Wrap is true YOffset counts screen rows; long
// lines are soft-wrapped and a prefix-sum cache of per-line heights is kept
// in sync incrementally via OnNewLines.
//
// When filterIndices is non-nil only the listed buffer positions are shown.
// All scroll math and the wrapCache operate over the filtered set.
type LogViewport struct {
	Width  int
	Height int
	Wrap   bool // soft-wrap long lines; disables horizontal scroll

	YOffset    int  // first visible unit (screen row when Wrap, logical line otherwise)
	xOffset    int  // horizontal scroll columns (Wrap=false only)
	Selected   int  // selected logical line (buffer index); -1 = none
	autoScroll bool // follow the tail when new lines arrive

	buf *buffer.Buffer
	h   *highlight.Highlighter // may be nil; display() falls back to Raw

	// filterIndices, when non-nil, lists the buffer positions visible after
	// filtering. All iteration uses effectiveLen / effectiveBufIdx.
	filterIndices []int

	// Per-line visual width cache (ansi.StringWidth of the raw line).
	// Always maintained regardless of Wrap mode; indexed over the effective set.
	// Evicted/appended in O(newLines) to stay in sync with the ring buffer.
	widthCache []int

	// Wrap=true: per-line wrap-height cache keyed to wrapCacheWidth.
	// When filterIndices != nil the cache is indexed over the filtered set.
	wrapCache      []int
	wrapCacheWidth int
	totalRows      int // sum of wrapCache

	// Wrap=false: longest visible line width for horizontal-scroll clamping.
	longestLineWidth int

	// hlMatchMap maps buffer indices to their highlight background colour.
	// Set by rebuildHlMatchMap; nil when no highlights are active.
	hlMatchMap map[int]lipgloss.Color
}

func newLogViewport(buf *buffer.Buffer) LogViewport {
	return LogViewport{buf: buf, Selected: -1, autoScroll: true}
}

// SetFilter replaces the active filter index list. Pass nil to show all lines.
// Recomputes the wrap cache when Wrap is true; repositions to the tail when
// autoScroll is on. When autoScroll is off, YOffset is clamped to the new
// valid range so stale offsets don't leave a blank viewport.
func (v *LogViewport) SetFilter(indices []int) {
	v.filterIndices = indices
	v.recomputeWidthCache()
	if v.Wrap {
		v.recomputeWrapCache()
	}
	if v.autoScroll {
		v.GotoBottom()
	} else {
		v.YOffset = clamp(v.YOffset, 0, v.maxYOffset())
	}
}

// effectiveLen returns the number of visible lines (filtered or total).
func (v *LogViewport) effectiveLen() int {
	if v.filterIndices != nil {
		return len(v.filterIndices)
	}
	return v.buf.Len()
}

// effectiveBufIdx maps a visible position to the underlying buffer index.
func (v *LogViewport) effectiveBufIdx(pos int) int {
	if v.filterIndices != nil {
		return v.filterIndices[pos]
	}
	return pos
}

// filterPosOf returns the visible position of bufIdx via binary search, or -1
// when bufIdx is not in the current filter.
func (v *LogViewport) filterPosOf(bufIdx int) int {
	lo, hi := 0, len(v.filterIndices)
	for lo < hi {
		mid := (lo + hi) / 2
		switch {
		case v.filterIndices[mid] == bufIdx:
			return mid
		case v.filterIndices[mid] < bufIdx:
			lo = mid + 1
		default:
			hi = mid
		}
	}
	return -1
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

// display returns the rendered (possibly ANSI-coloured) string for msg.
// Falls back to msg.Raw when no highlighter is set.
func (v *LogViewport) display(msg logmsg.LogMessage) string {
	if v.h == nil {
		return msg.Raw
	}
	return v.h.Display(msg)
}

// --- visual-width cache ---

// recomputeWidthCache rebuilds widthCache from scratch over the effective set.
func (v *LogViewport) recomputeWidthCache() {
	n := v.effectiveLen()
	if cap(v.widthCache) >= n {
		v.widthCache = v.widthCache[:n]
	} else {
		v.widthCache = make([]int, n)
	}
	for i := 0; i < n; i++ {
		v.widthCache[i] = ansi.StringWidth(v.buf.Line(v.effectiveBufIdx(i)).Raw)
	}
}

// updateWidthCache incrementally evicts front entries and appends widths for
// the newLines lines just appended to the unfiltered buffer. O(newLines).
func (v *LogViewport) updateWidthCache(newLines int) {
	bufLen := v.buf.Len()
	bufCap := v.buf.Cap()
	evictions := max(0, len(v.widthCache)+newLines-bufCap)
	if evictions > len(v.widthCache) {
		v.recomputeWidthCache()
		return
	}
	if evictions > 0 {
		copy(v.widthCache, v.widthCache[evictions:])
		v.widthCache = v.widthCache[:len(v.widthCache)-evictions]
	}
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}
	for i := start; i < bufLen; i++ {
		v.widthCache = append(v.widthCache, ansi.StringWidth(v.buf.Line(i).Raw))
	}
}

// --- wrap-height cache ---

// contentWidth returns the usable width for log line content.
// When highlights are active a 1-column gutter is reserved on the left for
// the colour bar, so content gets v.Width-1 columns.
func (v *LogViewport) contentWidth() int {
	if v.hlMatchMap != nil {
		return v.Width - 1
	}
	return v.Width
}

// lineWrapHeight returns the number of screen rows required to display a line
// of the given visual width at the current content width.
func (v *LogViewport) lineWrapHeight(w int) int {
	cw := v.contentWidth()
	if cw <= 0 || w == 0 {
		return 1
	}
	return (w + cw - 1) / cw
}

// recomputeWrapCache rebuilds the full cache in O(N) over the effective set.
// When filterIndices != nil the cache is indexed over the filtered set.
// Called on resize (width change), filter change, or when the cache is stale.
// Reads widthCache to avoid re-scanning line content; rebuilds widthCache first
// if it is stale.
func (v *LogViewport) recomputeWrapCache() {
	n := v.effectiveLen()
	if len(v.widthCache) != n {
		v.recomputeWidthCache()
	}
	if cap(v.wrapCache) >= n {
		v.wrapCache = v.wrapCache[:n]
	} else {
		v.wrapCache = make([]int, n)
	}
	v.totalRows = 0
	for i := 0; i < n; i++ {
		h := v.lineWrapHeight(v.widthCache[i])
		v.wrapCache[i] = h
		v.totalRows += h
	}
	v.wrapCacheWidth = v.contentWidth()
}

// updateWrapCache incrementally appends heights for newLines newly-appended
// lines and evicts heights for any lines the ring buffer has overwritten.
// Also keeps widthCache in sync. O(newLines) when the cache is at the right width.
func (v *LogViewport) updateWrapCache(newLines int) {
	if v.Width <= 0 {
		return
	}
	if v.wrapCacheWidth != v.contentWidth() {
		v.recomputeWrapCache()
		return
	}

	bufLen := v.buf.Len()
	bufCap := v.buf.Cap()

	// How many old entries were overwritten by the ring?
	evictions := max(0, len(v.wrapCache)+newLines-bufCap)
	if evictions > len(v.wrapCache) {
		v.recomputeWrapCache()
		return
	}

	// Remove evicted entries from the front of both caches.
	for i := 0; i < evictions; i++ {
		v.totalRows -= v.wrapCache[i]
	}
	if evictions > 0 {
		copy(v.wrapCache, v.wrapCache[evictions:])
		v.wrapCache = v.wrapCache[:len(v.wrapCache)-evictions]
		copy(v.widthCache, v.widthCache[evictions:])
		v.widthCache = v.widthCache[:len(v.widthCache)-evictions]
	}

	// Append for the new lines (last newLines entries in the buffer).
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}
	for i := start; i < bufLen; i++ {
		w := ansi.StringWidth(v.buf.Line(i).Raw)
		v.widthCache = append(v.widthCache, w)
		h := v.lineWrapHeight(w)
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
	if v.wrapCacheWidth == v.contentWidth() && len(v.wrapCache) == v.effectiveLen() {
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
	return v.effectiveLen()
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
	if m := v.longestLineWidth - v.contentWidth(); m > 0 && v.xOffset > m {
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

// SelectAt maps a screen-relative Y coordinate to a buffer index.
func (v *LogViewport) SelectAt(screenY int) {
	n := v.effectiveLen()
	if n == 0 {
		v.Selected = -1
		return
	}
	if !v.Wrap {
		pos := clamp(v.YOffset+screenY, 0, n-1)
		v.Selected = v.effectiveBufIdx(pos)
		return
	}
	v.ensureWrapCache()
	line, _ := v.screenRowToLine(v.YOffset + screenY)
	v.Selected = v.effectiveBufIdx(clamp(line, 0, n-1))
}

// OnNewLines must be called after n lines have been appended to the buffer.
// It keeps both caches in sync and applies auto-scroll.
// When a filter is active the caches are managed by SetFilter instead.
func (v *LogViewport) OnNewLines(n int) {
	if v.filterIndices == nil {
		if v.Wrap {
			v.updateWrapCache(n) // also evicts/appends widthCache
		} else {
			v.updateWidthCache(n)
		}
	}
	if v.autoScroll {
		v.GotoBottom()
	}
}

// JumpTo scrolls the viewport so that bufIdx is visible roughly 1/3 from the
// top, selects it, and disables auto-scroll. When a filter is active and
// bufIdx is not in the filtered set, the call is a no-op.
func (v *LogViewport) JumpTo(bufIdx int) {
	n := v.effectiveLen()
	if n == 0 {
		return
	}

	// Resolve bufIdx to a visible position.
	pos := bufIdx
	if v.filterIndices != nil {
		pos = v.filterPosOf(bufIdx)
		if pos < 0 {
			return // not visible in current filter
		}
	}
	pos = clamp(pos, 0, n-1)
	v.Selected = bufIdx
	v.autoScroll = false

	if !v.Wrap {
		target := pos - v.Height/3
		v.YOffset = clamp(target, 0, v.maxYOffset())
		return
	}

	// Wrap mode: find the screen row that corresponds to pos.
	v.ensureWrapCache()
	screenRow := 0
	for i := 0; i < pos && i < len(v.wrapCache); i++ {
		screenRow += v.wrapCache[i]
	}
	target := screenRow - v.Height/3
	v.YOffset = clamp(target, 0, v.maxYOffset())
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
		case keyPageDown, keyPageDownSpace:
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
			v.ScrollUp(5)
			v.autoScroll = false
		case tea.MouseButtonWheelDown:
			v.ScrollDown(5)
			if v.AtBottom() {
				v.autoScroll = true
			}
		case tea.MouseButtonWheelLeft:
			v.ScrollLeft(10)
		case tea.MouseButtonWheelRight:
			v.ScrollRight(10)
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
	n := v.effectiveLen()

	if n == 0 && v.filterIndices != nil {
		return lipgloss.NewStyle().
			Width(v.Width).Height(v.Height).MaxHeight(v.Height).MaxWidth(v.Width).
			Render(styleDim.Render("0 lines match filter"))
	}

	top := clamp(v.YOffset, 0, max(0, n-1))
	bot := clamp(top+v.Height, top, n)

	if len(v.widthCache) != n {
		v.recomputeWidthCache()
	}

	var sb strings.Builder
	v.longestLineWidth = 0

	cw := v.contentWidth()
	for i := top; i < bot; i++ {
		bufIdx := v.effectiveBufIdx(i)
		line := v.display(v.buf.Line(bufIdx))

		w := v.widthCache[i]
		if w > v.longestLineWidth {
			v.longestLineWidth = w
		}
		if v.xOffset > 0 || w > cw {
			line = ansi.Cut(line, v.xOffset, v.xOffset+cw)
		}
		if bufIdx == v.Selected {
			line = selectedStyle.Width(cw).Render(ansi.Strip(line))
			if v.hlMatchMap != nil {
				line = " " + line
			}
		} else if col, ok := v.hlMatchMap[bufIdx]; ok {
			bar := lipgloss.NewStyle().Foreground(col).Render("▌")
			line = bar + lipgloss.NewStyle().Width(cw).Render(line)
		} else if v.hlMatchMap != nil {
			line = " " + line
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

	n := v.effectiveLen()
	if n == 0 || v.Width <= 0 {
		var placeholder string
		if v.filterIndices != nil {
			placeholder = styleDim.Render("0 lines match filter")
		}
		return lipgloss.NewStyle().
			Width(v.Width).Height(v.Height).MaxHeight(v.Height).
			Render(placeholder)
	}

	yOff := clamp(v.YOffset, 0, v.maxYOffset())
	startLine, startWrapRow := v.screenRowToLine(yOff)

	cw := v.contentWidth()
	var sb strings.Builder
	rowsWritten := 0
	first := true

	for i := startLine; i < n && rowsWritten < v.Height; i++ {
		bufIdx := v.effectiveBufIdx(i)
		line := v.display(v.buf.Line(bufIdx))
		h := 1
		if i < len(v.wrapCache) {
			h = v.wrapCache[i]
		}

		wStart := 0
		if i == startLine {
			wStart = startWrapRow
		}

		for wr := wStart; wr < h && rowsWritten < v.Height; wr++ {
			segment := ansi.Cut(line, wr*cw, (wr+1)*cw)
			if bufIdx == v.Selected {
				segment = selectedStyle.Width(cw).Render(ansi.Strip(segment))
				if v.hlMatchMap != nil {
					segment = " " + segment
				}
			} else if col, ok := v.hlMatchMap[bufIdx]; ok {
				bar := lipgloss.NewStyle().Foreground(col).Render("▌")
				segment = bar + lipgloss.NewStyle().Width(cw).Render(segment)
			} else if v.hlMatchMap != nil {
				segment = " " + segment
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
