package tui

import "regexp"

type filterMode int

const (
	filterInclude filterMode = iota
	filterExclude
)

// runFilter scans the full buffer, populates filterIndices, and passes the
// result to the log viewport. O(N) initial scan is acceptable since the buffer
// is ring-bounded.
func runFilter(m *Model, re *regexp.Regexp, mode filterMode) {
	var indices []int
	n := m.buf.Len()
	for i := 0; i < n; i++ {
		matched := re.MatchString(m.buf.Line(i).Raw)
		if matched == (mode == filterInclude) {
			indices = append(indices, i)
		}
	}
	m.filterRe = re
	m.filterMode = mode
	m.filterIndices = indices
	m.logs.SetFilter(indices)
	if m.searchRe != nil {
		m.intersectSearch()
	}
}

// updateFilter keeps filterIndices consistent after a batch of new lines is
// appended to the ring buffer. Must be called after buf.Append has run for the
// whole batch. Mirrors the updateSearch pattern.
func (m *Model) updateFilter(newLines, evictions int) {
	if m.filterRe == nil {
		return
	}

	// Shift stored indices down by eviction count; drop gone entries.
	if evictions > 0 {
		keep := 0
		for _, idx := range m.filterIndices {
			if shifted := idx - evictions; shifted >= 0 {
				m.filterIndices[keep] = shifted
				keep++
			}
		}
		m.filterIndices = m.filterIndices[:keep]
	}

	// Scan the newly appended tail for additional matches.
	bufLen := m.buf.Len()
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}
	for i := start; i < bufLen; i++ {
		matched := m.filterRe.MatchString(m.buf.Line(i).Raw)
		if matched == (m.filterMode == filterInclude) {
			m.filterIndices = append(m.filterIndices, i)
		}
	}

	m.logs.SetFilter(m.filterIndices)

	// Keep search aligned with the updated filter.
	if m.searchRe != nil {
		m.intersectSearch()
	}
}

// clearFilter resets all filter state and restores the full buffer view.
// If a search is active its matches are expanded back to the full buffer.
func (m *Model) clearFilter() {
	m.filterRe = nil
	m.filterIndices = nil
	m.logs.SetFilter(nil)
	if m.searchRe != nil {
		runSearch(m, m.searchRe)
	}
}

// intersectSearch removes searchMatches that are not visible in the current
// filter. Called whenever filter state changes while a search is active.
func (m *Model) intersectSearch() {
	if len(m.filterIndices) == 0 {
		m.searchMatches = m.searchMatches[:0]
		m.searchCursor = 0
		m.logs.Selected = -1
		return
	}

	filterSet := make(map[int]struct{}, len(m.filterIndices))
	for _, idx := range m.filterIndices {
		filterSet[idx] = struct{}{}
	}

	keep := 0
	for _, idx := range m.searchMatches {
		if _, ok := filterSet[idx]; ok {
			m.searchMatches[keep] = idx
			keep++
		}
	}
	m.searchMatches = m.searchMatches[:keep]

	if m.searchCursor >= len(m.searchMatches) {
		if len(m.searchMatches) > 0 {
			m.searchCursor = len(m.searchMatches) - 1
		} else {
			m.searchCursor = 0
		}
	}
	if len(m.searchMatches) == 0 {
		m.logs.Selected = -1
	} else {
		m.logs.Selected = m.searchMatches[m.searchCursor]
	}
}
