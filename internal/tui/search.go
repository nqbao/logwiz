package tui

import "regexp"

// runSearch scans for re, stores matching chronological indices, and jumps the
// viewport to the first hit. When a filter is active, only the visible (filtered)
// lines are scanned so search stays within the filtered set.
func runSearch(m *Model, re *regexp.Regexp) {
	m.searchRe = re
	if m.filterIndices != nil {
		var indices []int
		for _, bufIdx := range m.filterIndices {
			if re.MatchString(m.buf.Line(bufIdx).Raw) {
				indices = append(indices, bufIdx)
			}
		}
		m.searchMatches = indices
	} else {
		matches, _, _ := m.buf.SearchRegex(re, 0, 10000)
		indices := make([]int, len(matches))
		for i, il := range matches {
			indices[i] = il.Index
		}
		m.searchMatches = indices
	}
	m.searchCursor = 0
	if len(m.searchMatches) > 0 {
		m.logs.JumpTo(m.searchMatches[0])
	}
}

// updateSearch keeps searchMatches consistent after a batch of new lines is
// appended to the ring buffer. It must be called after buf.Append has run for
// the whole batch but before any viewport update.
//
// evictions is the number of lines the ring buffer evicted during this batch.
// Each eviction removes the oldest entry, so every stored index must be
// decremented by that amount; indices that go negative refer to lines that no
// longer exist and are dropped.
//
// newLines is the count of lines just appended. The function scans the tail of
// the buffer ([bufLen-newLines, bufLen)) for additional matches and appends
// their indices. Because the ring buffer maintains chronological order and
// scanning only covers the newly added tail, the result list stays sorted.
//
// The viewport's Selected is updated to track the cursor's match only when it
// is still on that match — if the user has clicked elsewhere we leave their
// selection alone. The viewport position is not moved.
func (m *Model) updateSearch(newLines, evictions int) {
	if m.searchRe == nil {
		return
	}

	// Capture whether Selected currently tracks the cursor's match before any
	// shifts. If not, the user has moved selection (e.g. mouse click) and we
	// must not snap them back.
	prevCursorMatch := -1
	if m.searchCursor < len(m.searchMatches) {
		prevCursorMatch = m.searchMatches[m.searchCursor]
	}
	trackingCursor := m.logs.Selected == prevCursorMatch

	// Shift stored indices down by eviction count; drop gone entries.
	if evictions > 0 {
		keep := 0
		for _, idx := range m.searchMatches {
			if shifted := idx - evictions; shifted >= 0 {
				m.searchMatches[keep] = shifted
				keep++
			}
		}
		m.searchMatches = m.searchMatches[:keep]
		if len(m.searchMatches) == 0 {
			m.searchCursor = 0
		} else if m.searchCursor >= len(m.searchMatches) {
			m.searchCursor = len(m.searchMatches) - 1
		}
	}

	// Scan the newly appended tail for additional matches.
	bufLen := m.buf.Len()
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}
	for i := start; i < bufLen; i++ {
		if m.searchRe.MatchString(m.buf.Line(i).Raw) {
			m.searchMatches = append(m.searchMatches, i)
		}
	}

	if !trackingCursor {
		return
	}

	// Sync Selected with the current cursor without moving the viewport.
	if len(m.searchMatches) == 0 {
		m.searchCursor = 0
		m.logs.Selected = -1
	} else {
		m.logs.Selected = m.searchMatches[m.searchCursor]
	}
}

func (m *Model) nextMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchCursor = (m.searchCursor + 1) % len(m.searchMatches)
	m.logs.JumpTo(m.searchMatches[m.searchCursor])
}

func (m *Model) prevMatch() {
	if len(m.searchMatches) == 0 {
		return
	}
	m.searchCursor = (m.searchCursor - 1 + len(m.searchMatches)) % len(m.searchMatches)
	m.logs.JumpTo(m.searchMatches[m.searchCursor])
}

func (m *Model) clearSearch() {
	m.searchRe = nil
	m.searchMatches = nil
	m.searchCursor = 0
	m.logs.Selected = -1
	m.logs.ClearMultiSelect()
}
