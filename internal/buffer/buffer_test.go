package buffer

import (
	"reflect"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// raw wraps a string into a LogMessage for terser test setup.
func raw(s string) logmsg.LogMessage { return logmsg.LogMessage{Raw: s} }

// rawN wraps multiple strings.
func rawN(ss ...string) []logmsg.LogMessage {
	out := make([]logmsg.LogMessage, len(ss))
	for i, s := range ss {
		out[i] = raw(s)
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
	if got, want := b.Snapshot(), rawN("a", "b", "c"); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
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
	if got, want := b.Snapshot(), rawN("c", "d", "e"); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
}

func TestLastN(t *testing.T) {
	b := New(5)
	for _, line := range []string{"a", "b", "c", "d"} {
		b.Append(raw(line))
	}
	tests := []struct {
		n    int
		want []logmsg.LogMessage
	}{
		{0, []logmsg.LogMessage{}},
		{-1, []logmsg.LogMessage{}},
		{2, rawN("c", "d")},
		{4, rawN("a", "b", "c", "d")},
		{10, rawN("a", "b", "c", "d")}, // n > size
	}
	for _, tc := range tests {
		got := b.LastN(tc.n)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("LastN(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestLastNAfterWrap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(raw(line))
	}
	if got, want := b.LastN(2), rawN("d", "e"); !reflect.DeepEqual(got, want) {
		t.Fatalf("LastN(2) = %v, want %v", got, want)
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
