package tui

import (
	"fmt"
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

// hlPalette is the rotation of background colours assigned to successive :hl rules.
var hlPalette = []lipgloss.Color{
	"130", // amber
	"25",  // blue
	"28",  // green
	"90",  // purple
	"31",  // cyan
}

type hlRule struct {
	re       *regexp.Regexp
	colorIdx int
}

// addHL appends a highlight rule, scans the full buffer for matches, and
// refreshes the viewport's hlMatchMap.
func addHL(m *Model, re *regexp.Regexp) {
	colorIdx := len(m.hlRules) % len(hlPalette)
	m.hlRules = append(m.hlRules, hlRule{re: re, colorIdx: colorIdx})
	m.hlEnabled = true

	n := m.buf.Len()
	indices := make([]int, 0)
	for i := 0; i < n; i++ {
		if re.MatchString(m.buf.Line(i).Raw) {
			indices = append(indices, i)
		}
	}
	m.hlMatches = append(m.hlMatches, indices)
	rebuildHlMatchMap(m)
}

// updateHL keeps hlMatches consistent after a batch of new lines is appended.
// Must be called after buf.Append has run for the whole batch.
func updateHL(m *Model, newLines, evictions int) {
	if len(m.hlRules) == 0 {
		return
	}

	bufLen := m.buf.Len()
	start := bufLen - newLines
	if start < 0 {
		start = 0
	}

	for ri, rule := range m.hlRules {
		if evictions > 0 {
			keep := 0
			for _, idx := range m.hlMatches[ri] {
				if shifted := idx - evictions; shifted >= 0 {
					m.hlMatches[ri][keep] = shifted
					keep++
				}
			}
			m.hlMatches[ri] = m.hlMatches[ri][:keep]
		}
		for i := start; i < bufLen; i++ {
			if rule.re.MatchString(m.buf.Line(i).Raw) {
				m.hlMatches[ri] = append(m.hlMatches[ri], i)
			}
		}
	}

	rebuildHlMatchMap(m)
}

// clearHL wipes all highlight rules and removes colours from the viewport.
func clearHL(m *Model) {
	m.hlRules = nil
	m.hlMatches = nil
	m.hlEnabled = false
	m.logs.hlMatchMap = nil
}

// rebuildHlMatchMap rebuilds logs.hlMatchMap from the current rule/match state.
// First rule wins when indices overlap. Sets nil when highlights are disabled.
func rebuildHlMatchMap(m *Model) {
	if len(m.hlRules) == 0 || !m.hlEnabled {
		m.logs.hlMatchMap = nil
		return
	}
	result := make(map[int]lipgloss.Color)
	// Iterate in reverse so earlier rules take priority (overwrite later ones).
	for ri := len(m.hlRules) - 1; ri >= 0; ri-- {
		col := hlPalette[m.hlRules[ri].colorIdx]
		for _, idx := range m.hlMatches[ri] {
			result[idx] = col
		}
	}
	m.logs.hlMatchMap = result
}

func hlInfoMsg(m Model) string {
	n := len(m.hlRules)
	if n == 0 {
		return "no active highlights"
	}
	if !m.hlEnabled {
		return fmt.Sprintf("%d highlight(s) paused — H to resume, :nohl to clear", n)
	}
	return fmt.Sprintf("%d highlight(s) active — H to toggle, :nohl to clear", n)
}
