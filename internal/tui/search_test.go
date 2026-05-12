package tui

import (
	"regexp"
	"testing"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/source"
)

// seedBuf appends raw lines to a buffer.
func seedBuf(buf *buffer.Buffer, lines ...string) {
	for _, l := range lines {
		buf.Append(logmsg.LogMessage{Raw: l})
	}
}

// seedStructured appends pre-built LogMessages to a buffer.
func seedStructured(buf *buffer.Buffer, msgs ...logmsg.LogMessage) {
	for _, m := range msgs {
		buf.Append(m)
	}
}

// TestUpdateSearch_NewMatchesAdded verifies that a newly appended matching
// line is appended to searchMatches without moving the viewport.
func TestUpdateSearch_NewMatchesAdded(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "foo", "bar")
	m.searchRe = regexp.MustCompile("foo")
	m.searchMatches = []int{0}
	m.searchCursor = 0
	m.logs.Selected = 0

	m.buf.Append(logmsg.LogMessage{Raw: "foo2"})
	m.updateSearch(1, 0)

	if len(m.searchMatches) != 2 {
		t.Fatalf("searchMatches = %v, want 2 entries", m.searchMatches)
	}
	if m.searchMatches[1] != 2 {
		t.Errorf("new match at index %d, want 2", m.searchMatches[1])
	}
	// Cursor stays at 0; Selected tracks it.
	if m.searchCursor != 0 {
		t.Errorf("cursor = %d, want 0", m.searchCursor)
	}
	if m.logs.Selected != 0 {
		t.Errorf("Selected = %d, want 0 (cursor unchanged)", m.logs.Selected)
	}
}

// TestUpdateSearch_NonMatchingNewLine verifies that a non-matching new line
// does not grow searchMatches.
func TestUpdateSearch_NonMatchingNewLine(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "foo")
	m.searchRe = regexp.MustCompile("foo")
	m.searchMatches = []int{0}
	m.searchCursor = 0

	m.buf.Append(logmsg.LogMessage{Raw: "bar"})
	m.updateSearch(1, 0)

	if len(m.searchMatches) != 1 {
		t.Errorf("searchMatches = %v, want 1 entry (no new match)", m.searchMatches)
	}
}

// TestUpdateSearch_EvictionsShiftIndices verifies that evictions decrement
// all stored indices correctly.
func TestUpdateSearch_EvictionsShiftIndices(t *testing.T) {
	m := newTestModel(t)
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{1, 3}
	m.searchCursor = 1

	m.updateSearch(0, 1)

	if len(m.searchMatches) != 2 {
		t.Fatalf("searchMatches = %v, want [0 2]", m.searchMatches)
	}
	if m.searchMatches[0] != 0 || m.searchMatches[1] != 2 {
		t.Errorf("after eviction: %v, want [0 2]", m.searchMatches)
	}
	if m.searchCursor != 1 {
		t.Errorf("cursor = %d, want 1 (still valid)", m.searchCursor)
	}
}

// TestUpdateSearch_EvictedMatchDropped verifies that a match whose line was
// evicted is removed and the cursor is clamped.
func TestUpdateSearch_EvictedMatchDropped(t *testing.T) {
	m := newTestModel(t)
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 3}
	m.searchCursor = 1 // was pointing at index 3
	m.logs.Selected = 3

	m.updateSearch(0, 1)

	// index 0 → -1 (dropped); index 3 → 2.
	if len(m.searchMatches) != 1 || m.searchMatches[0] != 2 {
		t.Fatalf("searchMatches = %v, want [2]", m.searchMatches)
	}
	// cursor was 1, len is now 1 → clamp to 0.
	if m.searchCursor != 0 {
		t.Errorf("cursor = %d, want 0 (clamped)", m.searchCursor)
	}
	if m.logs.Selected != 2 {
		t.Errorf("Selected = %d, want 2", m.logs.Selected)
	}
}

// TestUpdateSearch_AllEvictedClearsSelected verifies that when every match is
// evicted, Selected resets to -1 and cursor to 0.
func TestUpdateSearch_AllEvictedClearsSelected(t *testing.T) {
	m := newTestModel(t)
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 1}
	m.searchCursor = 1
	m.logs.Selected = 1

	m.updateSearch(0, 2)

	if len(m.searchMatches) != 0 {
		t.Errorf("searchMatches = %v, want empty", m.searchMatches)
	}
	if m.searchCursor != 0 {
		t.Errorf("cursor = %d, want 0", m.searchCursor)
	}
	if m.logs.Selected != -1 {
		t.Errorf("Selected = %d, want -1", m.logs.Selected)
	}
}

// TestUpdateSearch_ViaNewLinesMsg exercises the full path through Update to
// confirm evictions are counted and updateSearch is invoked.
func TestUpdateSearch_ViaNewLinesMsg(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "hit", "miss", "hit")
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 2}
	m.searchCursor = 0
	m.logs.Selected = 0

	m2, _ := m.Update(source.NewLinesMsg{Lines: []logmsg.LogMessage{{Raw: "hit"}}})
	mm := m2.(Model)

	// New match should appear at index 3 (no evictions; buffer has cap=100).
	if len(mm.searchMatches) != 3 {
		t.Errorf("searchMatches = %v, want 3 entries", mm.searchMatches)
	}
	if mm.searchMatches[2] != 3 {
		t.Errorf("new match index = %d, want 3", mm.searchMatches[2])
	}
}

// TestUpdateSearch_PreservesUserClick verifies that after the user has clicked
// a different line (Selected != cursor's match), incoming streaming batches
// don't snap Selected back to the cursor's match.
func TestUpdateSearch_PreservesUserClick(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "hit", "other", "hit")
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 2}
	m.searchCursor = 0
	// Simulate user clicking the non-match line at index 1.
	m.logs.Selected = 1

	m.buf.Append(logmsg.LogMessage{Raw: "more"})
	m.updateSearch(1, 0)

	if m.logs.Selected != 1 {
		t.Errorf("Selected = %d, want 1 (user click preserved)", m.logs.Selected)
	}
}

// TestUpdateSearch_TracksCursorWhenInSync verifies that when Selected still
// matches the cursor (no user override), eviction shifts propagate to Selected.
func TestUpdateSearch_TracksCursorWhenInSync(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "hit", "miss", "hit")
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 2}
	m.searchCursor = 1
	m.logs.Selected = 2 // tracking cursor's match

	m.updateSearch(0, 1) // 1 eviction

	// matches shift: [0,2] -> drop 0, [1]; cursor clamps to 0; Selected -> 1.
	if len(m.searchMatches) != 1 || m.searchMatches[0] != 1 {
		t.Fatalf("searchMatches = %v, want [1]", m.searchMatches)
	}
	if m.logs.Selected != 1 {
		t.Errorf("Selected = %d, want 1 (cursor tracked)", m.logs.Selected)
	}
}

// TestClearSearch_ClearsSelectedSet verifies that pressing Esc to clear search
// also removes any shift-click multi-selections.
func TestClearSearch_ClearsSelectedSet(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "a", "b", "c")
	m.searchRe = regexp.MustCompile("a")
	m.searchMatches = []int{0}
	m.logs.Selected = 0
	m.logs.ToggleAt(2) // shift-click line 2

	m.clearSearch()

	if m.logs.Selected != -1 {
		t.Errorf("Selected = %d, want -1", m.logs.Selected)
	}
	if len(m.logs.selectedSet) != 0 {
		t.Error("selectedSet should be empty after clearSearch")
	}
}

// TestNextMatch_ClearsSelectedSet verifies that navigating to the next search
// match (via JumpTo) clears any accumulated shift-click selections.
func TestNextMatch_ClearsSelectedSet(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "hit", "other", "hit")
	m.searchRe = regexp.MustCompile("hit")
	m.searchMatches = []int{0, 2}
	m.searchCursor = 0
	m.logs.Selected = 0
	m.logs.ToggleAt(1) // shift-click line 1

	m.nextMatch()

	if len(m.logs.selectedSet) != 0 {
		t.Error("selectedSet should be cleared by nextMatch")
	}
	if m.logs.Selected != 2 {
		t.Errorf("Selected = %d, want 2", m.logs.Selected)
	}
}
