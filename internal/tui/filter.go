package tui

import (
	"regexp"
)

type filterMode int

const (
	filterInclude filterMode = iota
	filterExclude
)

// filterRule is one include/exclude predicate in the active filter stack.
type filterRule struct {
	re   *regexp.Regexp
	mode filterMode
}

// passesFilters returns true when raw satisfies every rule (AND logic).
func passesFilters(rules []filterRule, raw string) bool {
	for _, r := range rules {
		matched := r.re.MatchString(raw)
		if r.mode == filterInclude && !matched {
			return false
		}
		if r.mode == filterExclude && matched {
			return false
		}
	}
	return true
}

// addFilter appends a rule to the stack and recomputes filterIndices.
// O(N) full scan; acceptable since buffer is ring-bounded and this only
// runs on an explicit user command.
func addFilter(m *Model, re *regexp.Regexp, mode filterMode) {
	m.filterRules = append(m.filterRules, filterRule{re: re, mode: mode})
	applyFilters(m)
}

// applyFilters recomputes filterIndices from the current rule stack. The
// result is always a non-nil slice so the viewport can distinguish "filter
// active with zero matches" from "no filter".
func applyFilters(m *Model) {
	indices := make([]int, 0)
	n := m.buf.Len()
	for i := 0; i < n; i++ {
		if passesFilters(m.filterRules, m.buf.Line(i).Raw) {
			indices = append(indices, i)
		}
	}
	m.filterIndices = indices
	m.logs.SetFilter(indices)
	if m.searchRe != nil {
		m.intersectSearch()
	}
}

// updateFilter keeps filterIndices consistent after a batch of new lines is
// appended to the ring buffer. Must be called after buf.Append has run for
// the whole batch. Mirrors the updateSearch pattern.
func (m *Model) updateFilter(newLines, evictions int) {
	if len(m.filterRules) == 0 {
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
		if passesFilters(m.filterRules, m.buf.Line(i).Raw) {
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
	m.filterRules = nil
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
