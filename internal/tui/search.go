package tui

import "regexp"

// runSearch compiles the regex against the full buffer, stores matching
// chronological indices on the model, and jumps the viewport to the first hit.
func runSearch(m *Model, re *regexp.Regexp) {
	matches, _, _ := m.buf.SearchRegex(re, 0, 10000)
	indices := make([]int, len(matches))
	for i, il := range matches {
		indices[i] = il.Index
	}
	m.searchRe = re
	m.searchMatches = indices
	m.searchCursor = 0
	if len(indices) > 0 {
		m.logs.JumpTo(indices[0])
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
}
