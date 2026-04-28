package tui

import (
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

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

// --- :filter (attribute rules) ---

func TestAttrFilter_ExactMatch(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: `{"level":"error","msg":"boom"}`, Level: "ERROR", Message: "boom"},
		logmsg.LogMessage{Raw: `{"level":"info","msg":"ok"}`, Level: "INFO ", Message: "ok"},
		logmsg.LogMessage{Raw: `{"level":"error","msg":"crash"}`, Level: "ERROR", Message: "crash"},
	)

	re := regexp.MustCompile(`(?i)^error$`)
	addAttrFilter(&m, "level", re, filterInclude)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want [0 2]", m.filterIndices)
	}
	if m.filterIndices[0] != 0 || m.filterIndices[1] != 2 {
		t.Errorf("filterIndices = %v, want [0 2]", m.filterIndices)
	}
}

func TestAttrFilter_ExactExclude(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "DEBUG"},
		logmsg.LogMessage{Raw: "b", Level: "INFO "},
		logmsg.LogMessage{Raw: "c", Level: "DEBUG"},
	)

	re := regexp.MustCompile(`(?i)^debug$`)
	addAttrFilter(&m, "level", re, filterExclude)

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", m.filterIndices)
	}
}

func TestAttrFilter_RegexMatch(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Message: "connection timeout"},
		logmsg.LogMessage{Raw: "b", Message: "all good"},
		logmsg.LogMessage{Raw: "c", Message: "timeout exceeded"},
	)

	re := regexp.MustCompile(`timeout`)
	addAttrFilter(&m, "msg", re, filterInclude)

	if len(m.filterIndices) != 2 {
		t.Fatalf("filterIndices = %v, want [0 2]", m.filterIndices)
	}
}

func TestAttrFilter_CanonicaLevelAlias(t *testing.T) {
	m := newTestModel(t)
	// "lvl" is a canonical alias for Level
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "ERROR"},
		logmsg.LogMessage{Raw: "b", Level: "INFO "},
	)

	re := regexp.MustCompile(`(?i)^error$`)
	addAttrFilter(&m, "lvl", re, filterInclude)

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", m.filterIndices)
	}
}

func TestAttrFilter_MixedWithGrep(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "error: boom", Level: "ERROR", Message: "boom"},
		logmsg.LogMessage{Raw: "error: crash", Level: "ERROR", Message: "crash"},
		logmsg.LogMessage{Raw: "info: boom", Level: "INFO ", Message: "boom"},
	)

	// :grep error → lines 0,1,2 match raw "error" → [0,1] (info line doesn't have "error" in raw)
	// Wait: "info: boom" doesn't have "error" so grep narrows to [0,1]
	// Then :filter msg=boom narrows to [0]
	addFilter(&m, regexp.MustCompile(`^error`), filterInclude) // → [0,1]
	addAttrFilter(&m, "msg", regexp.MustCompile(`(?i)^boom$`), filterInclude) // → [0]

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", m.filterIndices)
	}
}

func TestAttrFilter_TrimSpaceOnLevel(t *testing.T) {
	m := newTestModel(t)
	// Level is stored as "INFO " (trailing space from NormalizeLevel)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "INFO "},
		logmsg.LogMessage{Raw: "b", Level: "ERROR"},
	)

	// (?i)^info$ should match "INFO " after TrimSpace → "INFO"
	re := regexp.MustCompile(`(?i)^info$`)
	addAttrFilter(&m, "level", re, filterInclude)

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0] — TrimSpace should strip trailing space from level", m.filterIndices)
	}
}

// --- cmdFilter parsing ---

func TestCmdFilter_ParseExact(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "ERROR"},
		logmsg.LogMessage{Raw: "b", Level: "INFO "},
	)

	newM, _, err := cmdFilter(m, `level="error"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterRules) != 1 {
		t.Fatalf("filterRules = %d, want 1", len(newM.filterRules))
	}
	if newM.filterRules[0].key != "level" {
		t.Errorf("key = %q, want \"level\"", newM.filterRules[0].key)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", newM.filterIndices)
	}
}

func TestCmdFilter_ParseNotEqual(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "DEBUG"},
		logmsg.LogMessage{Raw: "b", Level: "INFO "},
	)

	newM, _, err := cmdFilter(m, `level!="debug"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", newM.filterIndices)
	}
}

func TestCmdFilter_ParseRegex(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Message: "timeout"},
		logmsg.LogMessage{Raw: "b", Message: "ok"},
	)

	newM, _, err := cmdFilter(m, `msg=~"time.*"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", newM.filterIndices)
	}
}

func TestCmdFilter_ParseRegexNot(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Message: "timeout"},
		logmsg.LogMessage{Raw: "b", Message: "ok"},
	)

	newM, _, err := cmdFilter(m, `msg!~"time"`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", newM.filterIndices)
	}
}

func TestCmdFilter_NoOp_ReportsCount(t *testing.T) {
	m := newTestModel(t)
	addFilter(&m, regexp.MustCompile(`x`), filterInclude)

	_, _, err := cmdFilter(m, "")
	if err == nil {
		t.Fatal("want infoErr, got nil")
	}
	if _, ok := err.(infoErr); !ok {
		t.Errorf("want infoErr, got %T: %v", err, err)
	}
}

func TestCmdFilter_BadUsage(t *testing.T) {
	m := newTestModel(t)
	_, _, err := cmdFilter(m, "justpattern")
	if err == nil {
		t.Fatal("want error for missing operator")
	}
	if _, ok := err.(infoErr); ok {
		t.Error("want hard error, not infoErr")
	}
}

func TestCmdFilter_UnquotedValue(t *testing.T) {
	m := newTestModel(t)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Level: "ERROR"},
		logmsg.LogMessage{Raw: "b", Level: "INFO "},
	)

	newM, _, err := cmdFilter(m, "level=error")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0]", newM.filterIndices)
	}
}

// --- time comparison rules ---

func TestTimeCmp_GreaterThan_RecentLine(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	recent := now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: old},
		logmsg.LogMessage{Raw: "recent", Time: recent},
	)

	ref := func() time.Time { return time.Now().Add(-5 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: ">", ref: ref})

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1] (only recent line)", m.filterIndices)
	}
}

func TestTimeCmp_LessThan_OldLine(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	recent := now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: old},
		logmsg.LogMessage{Raw: "recent", Time: recent},
	)

	ref := func() time.Time { return time.Now().Add(-5 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: "<", ref: ref})

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Errorf("filterIndices = %v, want [0] (only old line)", m.filterIndices)
	}
}

func TestTimeCmp_MissingTimestamp_Excluded(t *testing.T) {
	m := newTestModel(t)
	recent := time.Now().Add(-1 * time.Minute).UTC().Format(time.RFC3339)

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "no-ts"},
		logmsg.LogMessage{Raw: "has-ts", Time: recent},
	)

	ref := func() time.Time { return time.Now().Add(-5 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: ">", ref: ref})

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1] (no-ts line excluded)", m.filterIndices)
	}
}

func TestTimeCmp_EpochTimestamp(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	oldEpoch := fmt.Sprintf("%d", now.Add(-10*time.Minute).Unix())
	recentEpoch := fmt.Sprintf("%d", now.Add(-1*time.Minute).Unix())

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: oldEpoch},
		logmsg.LogMessage{Raw: "recent", Time: recentEpoch},
	)

	ref := func() time.Time { return time.Now().Add(-5 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: ">", ref: ref})

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", m.filterIndices)
	}
}

// --- parseRHSTime ---

func TestParseRHSTime_Now(t *testing.T) {
	ref, err := parseRHSTime("now")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	diff := time.Since(ref())
	if diff < -time.Second || diff > time.Second {
		t.Errorf("parseRHSTime(now) returned time %v off from now", diff)
	}
}

func TestParseRHSTime_RelativeOffset(t *testing.T) {
	ref, err := parseRHSTime("now-5m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	diff := time.Since(ref())
	if diff < 4*time.Minute || diff > 6*time.Minute {
		t.Errorf("parseRHSTime(now-5m) off by %v", diff-5*time.Minute)
	}
}

func TestParseRHSTime_BareShorthand(t *testing.T) {
	ref, err := parseRHSTime("30m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	diff := time.Since(ref())
	if diff < 29*time.Minute || diff > 31*time.Minute {
		t.Errorf("parseRHSTime(30m) off by %v", diff-30*time.Minute)
	}
}

func TestParseRHSTime_AbsoluteRFC3339(t *testing.T) {
	ts := "2024-01-15T10:30:00Z"
	ref, err := parseRHSTime(ts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want, _ := time.Parse(time.RFC3339, ts)
	if !ref().Equal(want) {
		t.Errorf("parseRHSTime(%q) = %v, want %v", ts, ref(), want)
	}
}

func TestParseRHSTime_Invalid(t *testing.T) {
	_, err := parseRHSTime("yesterday")
	if err == nil {
		t.Error("expected error for invalid time expression")
	}
}

// --- cmdFilter time integration ---

func TestCmdFilter_TimeGreaterThan(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	recent := now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: old},
		logmsg.LogMessage{Raw: "recent", Time: recent},
	)

	newM, _, err := cmdFilter(m, "ts>now-5m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", newM.filterIndices)
	}
}

func TestCmdFilter_TimeShorthand(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	recent := now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)

	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: old},
		logmsg.LogMessage{Raw: "recent", Time: recent},
	)

	newM, _, err := cmdFilter(m, "ts>5m")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(newM.filterIndices) != 1 || newM.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1]", newM.filterIndices)
	}
}

func TestCmdFilter_NonTimeKeyComparison_Error(t *testing.T) {
	m := newTestModel(t)
	_, _, err := cmdFilter(m, "status>200")
	if err == nil {
		t.Error("expected error for comparison on non-time key")
	}
}

// --- filterTickMsg ---

func TestFilterTick_ReappliesTimeRule(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()

	// Both lines are currently within the 5m window.
	l1 := now.Add(-2 * time.Minute).UTC().Format(time.RFC3339)
	l2 := now.Add(-4 * time.Minute).UTC().Format(time.RFC3339)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Time: l1},
		logmsg.LogMessage{Raw: "b", Time: l2},
	)

	// Rule: ts > now-3m — only l1 matches at rule creation.
	ref := func() time.Time { return time.Now().Add(-3 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: ">", ref: ref})

	if len(m.filterIndices) != 1 || m.filterIndices[0] != 0 {
		t.Fatalf("initial filterIndices = %v, want [0]", m.filterIndices)
	}

	// Simulate tick: both lines now fall outside the 1m window.
	tightRef := func() time.Time { return time.Now().Add(-1 * time.Minute) }
	m.filterRules[0].cmp.ref = tightRef

	m2, _ := m.Update(filterTickMsg{})
	mm := m2.(Model)

	if len(mm.filterIndices) != 0 {
		t.Errorf("after tick filterIndices = %v, want [] (both lines aged out)", mm.filterIndices)
	}
}

func TestFilterTick_StopsWhenNoTimeRules(t *testing.T) {
	m := newTestModel(t)
	// No time rules — tick should return nil cmd (loop dies).
	m2, cmd := m.Update(filterTickMsg{})
	_ = m2
	if cmd != nil {
		t.Error("expected nil cmd when no time rules, got non-nil")
	}
}

func TestFilterTick_RespectsFilterPaused(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	recent := now.Add(-1 * time.Minute).UTC().Format(time.RFC3339)
	old := now.Add(-10 * time.Minute).UTC().Format(time.RFC3339)
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "old", Time: old},
		logmsg.LogMessage{Raw: "recent", Time: recent},
	)

	ref := func() time.Time { return time.Now().Add(-5 * time.Minute) }
	addTimeCmpFilter(&m, "ts", &timeCmp{op: ">", ref: ref})

	// Pause the filter — viewport should show all lines.
	m.pauseFilter()
	if m.logs.filterIndices != nil {
		t.Fatal("expected viewport filter to be nil after pauseFilter")
	}

	// Tick fires while paused — filterIndices updated but viewport stays clear.
	m2, _ := m.Update(filterTickMsg{})
	mm := m2.(Model)

	if mm.filterEnabled {
		t.Error("filterEnabled should remain false after tick while paused")
	}
	if mm.logs.filterIndices != nil {
		t.Error("viewport filter should stay nil while paused")
	}
	// But filterIndices should be maintained.
	if len(mm.filterIndices) != 1 || mm.filterIndices[0] != 1 {
		t.Errorf("filterIndices = %v, want [1] (maintained while paused)", mm.filterIndices)
	}
}

func TestCmdFilter_TimeRule_SecondCallDoesNotSpawnExtraTick(t *testing.T) {
	m := newTestModel(t)
	now := time.Now()
	seedStructured(m.buf,
		logmsg.LogMessage{Raw: "a", Time: now.UTC().Format(time.RFC3339)},
	)

	// First time rule: cmd should be non-nil (starts the tick loop).
	m1, cmd1, err := cmdFilter(m, "ts>1h")
	if err != nil {
		t.Fatalf("first cmdFilter error: %v", err)
	}
	if cmd1 == nil {
		t.Fatal("first time rule should return a non-nil tick cmd")
	}
	if !m1.filterTickRunning {
		t.Error("filterTickRunning should be true after first time rule")
	}

	// Second time rule while the tick loop is still in flight: cmd must be nil.
	_, cmd2, err := cmdFilter(m1, "ts<now")
	if err != nil {
		t.Fatalf("second cmdFilter error: %v", err)
	}
	if cmd2 != nil {
		t.Error("second time rule should NOT spawn a parallel tick loop")
	}
}

func TestFilterTick_ResetsRunningFlagWhenNoRules(t *testing.T) {
	m := newTestModel(t)
	m.filterTickRunning = true // simulate a tick loop in flight, no rules

	m2, cmd := m.Update(filterTickMsg{})
	mm := m2.(Model)

	if cmd != nil {
		t.Error("expected nil cmd when no time rules")
	}
	if mm.filterTickRunning {
		t.Error("filterTickRunning should reset to false so a future :filter can restart the loop")
	}
}
