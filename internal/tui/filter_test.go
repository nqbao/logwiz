package tui

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/source"
)

// --- addFilter / applyFilters ---

func TestAddFilter_Include(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: foo", "info: bar", "error: baz", "debug: qux")

	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want 2 entries", m.filterIndices)
	}
	if m.filterIndices[0] != 0 || m.filterIndices[1] != 2 {
		t.Errorf("filterIndices = %v, want [0 2]", m.filterIndices)
	}
}

func TestAddFilter_Exclude(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: foo", "info: bar", "error: baz", "debug: qux")

	addFilter(&m, regexp.MustCompile(`^error`), filterExclude)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want 2 entries", m.filterIndices)
	}
	if m.filterIndices[0] != 1 || m.filterIndices[1] != 3 {
		t.Errorf("filterIndices = %v, want [1 3]", m.filterIndices)
	}
}

func TestAddFilter_NoMatch(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "info: a", "debug: b")

	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	if m.filterIndices == nil {
		t.Fatal("filterIndices should be non-nil (empty slice) when rules are active")
	}
	if len(m.filterIndices) != 0 {
		t.Errorf("filterIndices = %v, want empty", m.filterIndices)
	}
}

func TestAddFilter_Compounding(t *testing.T) {
	m := newTestModel(t)
	// Lines: error+foo, error+bar, info+foo, debug+baz
	seedBuf(m.buf, "error foo", "error bar", "info foo", "debug baz")

	addFilter(&m, regexp.MustCompile(`^error`), filterInclude) // → [0,1]
	addFilter(&m, regexp.MustCompile(`foo`), filterInclude)    // → [0] (error AND foo)

	if len(m.filterRules) != 2 {
		t.Fatalf("filterRules = %d, want 2", len(m.filterRules))
	}
	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", m.filterIndices)
	}
}

func TestAddFilter_IncludeAndExclude(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error foo", "error bar", "info foo", "error foo extra")

	addFilter(&m, regexp.MustCompile(`^error`), filterInclude) // → [0,1,3]
	addFilter(&m, regexp.MustCompile(`bar`), filterExclude)    // → [0,3]

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want [0 3]", m.filterIndices)
	}
	if m.filterIndices[0] != 0 || m.filterIndices[1] != 3 {
		t.Errorf("filterIndices = %v, want [0 3]", m.filterIndices)
	}
}

// --- updateFilter ---

func TestUpdateFilter_NewMatchesAdded(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: a", "info: b")
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	m.buf.Append(logmsg.LogMessage{Raw: "error: c"})
	m.updateFilter(1, 0)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want 2 entries", m.filterIndices)
	}
	if m.filterIndices[1] != 2 {
		t.Errorf("new match at index %d, want 2", m.filterIndices[1])
	}
}

func TestUpdateFilter_NonMatchingNewLine(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: a")
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	m.buf.Append(logmsg.LogMessage{Raw: "info: b"})
	m.updateFilter(1, 0)

	if len(m.filterIndices) != 1 {
		t.Errorf("filterIndices = %v, want 1 entry (no new match)", m.filterIndices)
	}
}

func TestUpdateFilter_EvictionsShiftIndices(t *testing.T) {
	m := newTestModel(t)
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)
	m.filterIndices = []int{1, 3}

	m.updateFilter(0, 1)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want [0 2]", m.filterIndices)
	}
	if m.filterIndices[0] != 0 || m.filterIndices[1] != 2 {
		t.Errorf("after eviction: %v, want [0 2]", m.filterIndices)
	}
}

func TestUpdateFilter_EvictedEntryDropped(t *testing.T) {
	m := newTestModel(t)
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)
	m.filterIndices = []int{0, 3}

	m.updateFilter(0, 1)

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 2 {
		t.Fatalf("filterIndices = %v, want [2]", m.filterIndices)
	}
}

// --- clearFilter ---

func TestClearFilter_ResetsState(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: a", "info: b")
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	m.clearFilter()

	if len(m.filterRules) != 0 {
		t.Errorf("filterRules = %v, should be empty", m.filterRules)
	}
	if m.filterIndices != nil {
		t.Errorf("filterIndices = %v, should be nil", m.filterIndices)
	}
	if m.logs.filterIndices != nil {
		t.Error("viewport filterIndices should be nil after clearFilter")
	}
}

func TestClearFilter_RestoresSearchMatches(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error foo", "info foo", "error bar")

	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)
	runSearch(&m, regexp.MustCompile(`foo`))
	if len(m.searchMatches) != 1 || m.searchMatches[0] != 0 {
		t.Fatalf("pre-clear searchMatches = %v, want [0]", m.searchMatches)
	}

	m.clearFilter()
	if len(m.searchMatches) != 2 {
		t.Errorf("post-clear searchMatches = %v, want 2 entries", m.searchMatches)
	}
}

// --- intersectSearch ---

func TestIntersectSearch_RemovesNonFilteredMatches(t *testing.T) {
	m := newTestModel(t)
	m.filterIndices = []int{0, 2}
	m.searchRe = regexp.MustCompile(`foo`)
	m.searchMatches = []int{0, 1, 2, 3}
	m.searchCursor = 2

	m.intersectSearch()

	if len(m.searchMatches) != 2 {
		t.Fatalf("searchMatches = %v, want [0 2]", m.searchMatches)
	}
	if m.searchMatches[0] != 0 || m.searchMatches[1] != 2 {
		t.Errorf("searchMatches = %v, want [0 2]", m.searchMatches)
	}
	if m.searchCursor != 1 {
		t.Errorf("searchCursor = %d, want 1 (clamped)", m.searchCursor)
	}
}

// --- viewport filter ---

func TestViewport_FilteredLen(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c", "d", "e")
	v.SetFilter([]int{0, 2, 4})
	if got := v.effectiveLen(); got != 3 {
		t.Errorf("effectiveLen = %d, want 3", got)
	}
}

func TestViewport_FilteredView_ShowsOnlyMatching(t *testing.T) {
	v := newTestViewport(20, 5, "alpha", "beta", "gamma", "delta", "epsilon")
	v.SetFilter([]int{1, 3})
	out := v.View(noStyle)
	if !strings.Contains(out, "beta") {
		t.Errorf("filtered view missing 'beta'; got:\n%s", out)
	}
	if !strings.Contains(out, "delta") {
		t.Errorf("filtered view missing 'delta'; got:\n%s", out)
	}
	if strings.Contains(out, "alpha") {
		t.Errorf("filtered view should not contain 'alpha'; got:\n%s", out)
	}
}

func TestViewport_FilterEmpty_ShowsPlaceholder(t *testing.T) {
	v := newTestViewport(40, 5, "a", "b", "c")
	v.SetFilter([]int{})
	out := v.View(noStyle)
	if !strings.Contains(out, "0 lines match filter") {
		t.Errorf("empty filter view missing placeholder; got:\n%s", out)
	}
}

func TestViewport_FilteredJumpTo(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c", "d", "e", "f", "g", "h", "i", "j")
	v.autoScroll = false
	v.SetFilter([]int{1, 3, 5, 7, 9})
	v.GotoTop()

	v.JumpTo(7)
	if v.Selected != 7 {
		t.Errorf("Selected = %d, want 7", v.Selected)
	}
	if v.YOffset > 3 || v.YOffset+v.Height <= 3 {
		t.Errorf("filter pos 3 not in viewport: YOffset=%d Height=%d", v.YOffset, v.Height)
	}
}

func TestViewport_FilteredJumpTo_NotInFilter_NoOp(t *testing.T) {
	v := newTestViewport(80, 5, "a", "b", "c", "d", "e")
	v.autoScroll = false
	v.SetFilter([]int{1, 3})
	v.YOffset = 0
	v.Selected = -1

	v.JumpTo(2)
	if v.Selected != -1 {
		t.Errorf("JumpTo non-filtered index changed Selected to %d", v.Selected)
	}
	if v.YOffset != 0 {
		t.Errorf("JumpTo non-filtered index changed YOffset to %d", v.YOffset)
	}
}

// --- via NewLinesMsg ---

func TestUpdateFilter_ViaNewLinesMsg(t *testing.T) {
	m := newTestModel(t)
	seedBuf(m.buf, "error: a", "info: b", "error: c")
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude)

	if len(m.filterIndices) != 2 {
		t.Fatalf("pre-msg filterIndices = %v, want 2 entries", m.filterIndices)
	}

	m2, _ := m.Update(source.NewLinesMsg{Lines: []logmsg.LogMessage{{Raw: "error: d"}}})
	mm := m2.(Model)

	if len(mm.filterIndices) != 3 {
		t.Errorf("post-msg filterIndices = %v, want 3 entries", mm.filterIndices)
	}
	if mm.filterIndices[2] != 3 {
		t.Errorf("new filter index = %d, want 3", mm.filterIndices[2])
	}
}
