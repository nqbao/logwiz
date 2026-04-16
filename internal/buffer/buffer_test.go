package buffer

import (
	"reflect"
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
		Raw:        "raw line",
		Attributes: map[string]string{"level": "INFO", "ts": "2026-04-16"},
	}
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
