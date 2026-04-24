package buffer

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// raw wraps a string into a LogMessage for terser test setup.
func raw(s string) logmsg.LogMessage { return logmsg.LogMessage{Raw: s} }

// raws projects the Raw field of a LogMessage slice for comparisons
// that don't care about the buffer-assigned MsgID.
func raws(msgs []logmsg.LogMessage) []string {
	out := make([]string, len(msgs))
	for i, m := range msgs {
		out[i] = m.Raw
	}
	return out
}

func TestNewClampsCap(t *testing.T) {
	b := New(0)
	if b.Cap() != 1 {
		t.Fatalf("cap = %d, want 1", b.Cap())
	}
	b = New(-5)
	if b.Cap() != 1 {
		t.Fatalf("cap = %d, want 1", b.Cap())
	}
}

func TestAppendAndSnapshotBelowCap(t *testing.T) {
	b := New(5)
	b.Append(raw("a"))
	b.Append(raw("b"))
	b.Append(raw("c"))
	if got, want := b.Len(), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if got, want := raws(b.Snapshot()), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot raws = %v, want %v", got, want)
	}
}

func TestAppendWrapsPastCap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}
	if got, want := b.Len(), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if got, want := raws(b.Snapshot()), []string{"c", "d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot raws = %v, want %v", got, want)
	}
}

func TestLastN(t *testing.T) {
	b := New(5)
	for _, line := range []string{"a", "b", "c", "d"} {
		b.Append(raw(line))
	}
	tests := []struct {
		n    int
		want []string
	}{
		{0, []string{}},
		{-1, []string{}},
		{2, []string{"c", "d"}},
		{4, []string{"a", "b", "c", "d"}},
		{10, []string{"a", "b", "c", "d"}}, // n > size
	}
	for _, tc := range tests {
		got := raws(b.LastN(tc.n))
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("LastN(%d) raws = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestLastNAfterWrap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}
	if got, want := raws(b.LastN(2)), []string{"d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LastN(2) raws = %v, want %v", got, want)
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	b := New(3)
	b.Append(raw("a"))
	b.Append(raw("b"))
	snap := b.Snapshot()
	snap[0] = raw("mutated")
	again := b.Snapshot()
	if again[0].Raw != "a" {
		t.Fatalf("buffer mutated by caller via snapshot: %v", again)
	}
}

func TestLine(t *testing.T) {
	// In-range, then ring-wrap case.
	b := New(3)
	b.Append(raw("a"))
	b.Append(raw("b"))
	b.Append(raw("c"))

	if got := b.Line(0).Raw; got != "a" {
		t.Errorf("Line(0) = %q, want %q", got, "a")
	}
	if got := b.Line(1).Raw; got != "b" {
		t.Errorf("Line(1) = %q, want %q", got, "b")
	}
	if got := b.Line(2).Raw; got != "c" {
		t.Errorf("Line(2) = %q, want %q", got, "c")
	}

	// Out-of-range.
	if got := b.Line(-1); got.Raw != "" {
		t.Errorf("Line(-1) = %q, want \"\"", got.Raw)
	}
	if got := b.Line(3); got.Raw != "" {
		t.Errorf("Line(3) = %q, want \"\"", got.Raw)
	}

	// Ring-wrap: append "d" evicts "a", buffer is now [b, c, d].
	b.Append(raw("d"))
	if got := b.Line(0).Raw; got != "b" {
		t.Errorf("after wrap Line(0) = %q, want %q", got, "b")
	}
	if got := b.Line(1).Raw; got != "c" {
		t.Errorf("after wrap Line(1) = %q, want %q", got, "c")
	}
	if got := b.Line(2).Raw; got != "d" {
		t.Errorf("after wrap Line(2) = %q, want %q", got, "d")
	}
}

func TestLineEmptyBuffer(t *testing.T) {
	b := New(4)
	if got := b.Line(0); got.Raw != "" {
		t.Errorf("Line(0) on empty buffer = %q, want \"\"", got.Raw)
	}
}

func TestEmptyBuffer(t *testing.T) {
	b := New(4)
	if got, want := b.Snapshot(), []logmsg.LogMessage{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty Snapshot = %v, want %v", got, want)
	}
	if got, want := b.LastN(5), []logmsg.LogMessage{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty LastN = %v, want %v", got, want)
	}
}

func TestAttributesRoundTrip(t *testing.T) {
	b := New(3)
	msg := logmsg.LogMessage{
		Raw:   "raw line",
		Time:  "2026-04-16",
		Level: "INFO",
	}
	msg.Attrs.Set("user", "bob")
	b.Append(msg)

	got := b.Line(0)
	if got.Raw != "raw line" {
		t.Errorf("Raw = %q, want %q", got.Raw, "raw line")
	}
	if got.Attr("level") != "INFO" {
		t.Errorf("Attr(level) = %q, want INFO", got.Attr("level"))
	}
	if got.Attr("ts") != "2026-04-16" {
		t.Errorf("Attr(ts) = %q, want 2026-04-16", got.Attr("ts"))
	}
	if got.Attr("user") != "bob" {
		t.Errorf("Attr(user) = %q, want bob", got.Attr("user"))
	}
}

// TestAppendAssignsMonotonicIDs covers the stable-identity guarantee:
// every appended message gets a unique, monotonically increasing
// MsgID.Line, and the counter keeps climbing across ring evictions
// (evicted IDs are not reused).
func TestAppendAssignsMonotonicIDs(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}
	// Ring holds c, d, e with IDs 2, 3, 4.
	got := b.Snapshot()
	wantIDs := []uint64{2, 3, 4}
	for i, m := range got {
		if m.ID.Line != wantIDs[i] {
			t.Errorf("Snapshot()[%d].ID.Line = %d, want %d", i, m.ID.Line, wantIDs[i])
		}
	}
}

// TestAppendOverridesCallerID ensures parsers can't spoof IDs: whatever
// the caller sets is overwritten by the buffer's counter.
func TestAppendOverridesCallerID(t *testing.T) {
	b := New(2)
	b.Append(logmsg.LogMessage{ID: logmsg.MsgID{Line: 999}, Raw: "first"})
	if got := b.Line(0).ID.Line; got != 0 {
		t.Errorf("Line(0).ID.Line = %d, want 0 (caller value ignored)", got)
	}
}

// TestAppendEvictionSignal verifies that Append reports no eviction while the
// buffer has free slots, and reports the correct evicted MsgID once full.
func TestAppendEvictionSignal(t *testing.T) {
	b := New(3)

	for i, line := range []string{"a", "b", "c"} {
		_, evicted := b.Append(raw(line))
		if evicted {
			t.Errorf("Append #%d: evicted=true, want false (buffer not yet full)", i)
		}
	}

	// Now full: next append evicts "a" (ID=0).
	evictedID, evicted := b.Append(raw("d"))
	if !evicted {
		t.Fatal("Append when full: evicted=false, want true")
	}
	if evictedID.Line != 0 {
		t.Errorf("evicted ID = %d, want 0 (oldest)", evictedID.Line)
	}

	// Next append evicts "b" (ID=1).
	evictedID, evicted = b.Append(raw("e"))
	if !evicted {
		t.Fatal("second eviction: evicted=false, want true")
	}
	if evictedID.Line != 1 {
		t.Errorf("evicted ID = %d, want 1", evictedID.Line)
	}
}

func TestRange(t *testing.T) {
	b := New(5)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}

	tests := []struct {
		start, n int
		want     []string
	}{
		{0, 3, []string{"a", "b", "c"}},
		{3, 2, []string{"d", "e"}},
		{3, 10, []string{"d", "e"}}, // clamp to end
		{5, 2, nil},                 // start past end
		{0, 0, nil},                 // n=0
	}
	for _, tc := range tests {
		got := b.Range(tc.start, tc.n)
		var gotRaws []string
		for _, il := range got {
			gotRaws = append(gotRaws, il.Msg.Raw)
		}
		if !reflect.DeepEqual(gotRaws, tc.want) {
			t.Errorf("Range(%d,%d) = %v, want %v", tc.start, tc.n, gotRaws, tc.want)
		}
	}
}

func TestRangeAfterWrap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}
	// Buffer holds c,d,e at indices 0,1,2.
	got := b.Range(1, 2)
	if len(got) != 2 || got[0].Msg.Raw != "d" || got[1].Msg.Raw != "e" {
		t.Errorf("Range after wrap = %v", got)
	}
	if got[0].Index != 1 || got[1].Index != 2 {
		t.Errorf("Range indices wrong: %v", got)
	}
}

func TestSearchRegexBasic(t *testing.T) {
	b := New(10)
	for _, line := range []string{"foo bar", "baz", "foo qux", "nope"} {
		b.Append(raw(line))
	}

	re := regexp.MustCompile("foo")
	matches, total, nextSince := b.SearchRegex(re, 0, 10)
	if total != 2 {
		t.Errorf("total = %d, want 2", total)
	}
	if nextSince != -1 {
		t.Errorf("nextSince = %d, want -1 (all fit)", nextSince)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	if matches[0].Msg.Raw != "foo bar" || matches[1].Msg.Raw != "foo qux" {
		t.Errorf("unexpected matches: %v", matches)
	}
}

func TestSearchRegexLimit(t *testing.T) {
	b := New(10)
	for _, line := range []string{"hit1", "miss", "hit2", "hit3", "miss"} {
		b.Append(raw(line))
	}

	re := regexp.MustCompile("hit")
	matches, total, nextSince := b.SearchRegex(re, 0, 2)
	if total != 3 {
		t.Errorf("total = %d, want 3", total)
	}
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	// nextSince should be the index after the last collected match (index 2).
	if nextSince != 3 {
		t.Errorf("nextSince = %d, want 3", nextSince)
	}
}

func TestSearchRegexSinceLine(t *testing.T) {
	b := New(10)
	for _, line := range []string{"hit0", "hit1", "hit2"} {
		b.Append(raw(line))
	}

	re := regexp.MustCompile("hit")
	matches, total, _ := b.SearchRegex(re, 1, 10)
	if total != 2 {
		t.Errorf("total = %d, want 2 (starting from index 1)", total)
	}
	if len(matches) != 2 || matches[0].Msg.Raw != "hit1" {
		t.Errorf("unexpected matches: %v", matches)
	}
}

func TestSearchRegexEmptyBuffer(t *testing.T) {
	b := New(5)
	re := regexp.MustCompile("x")
	matches, total, nextSince := b.SearchRegex(re, 0, 10)
	if len(matches) != 0 || total != 0 || nextSince != -1 {
		t.Errorf("empty buffer: matches=%v total=%d nextSince=%d", matches, total, nextSince)
	}
}

func TestSearchRegexAfterWrap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "hit1", "b", "hit2", "c"} {
		b.Append(raw(line))
	}
	// Buffer holds: b, hit2, c at indices 0,1,2
	re := regexp.MustCompile("hit")
	matches, total, _ := b.SearchRegex(re, 0, 10)
	if total != 1 || len(matches) != 1 || matches[0].Msg.Raw != "hit2" {
		t.Errorf("SearchRegex after wrap: total=%d matches=%v", total, matches)
	}
}
