package buffer

import (
	"reflect"
	"testing"
)

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
	b.Append("a")
	b.Append("b")
	b.Append("c")
	if got, want := b.Len(), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if got, want := b.Snapshot(), []string{"a", "b", "c"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
}

func TestAppendWrapsPastCap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(line)
	}
	if got, want := b.Len(), 3; got != want {
		t.Fatalf("len = %d, want %d", got, want)
	}
	if got, want := b.Snapshot(), []string{"c", "d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %v, want %v", got, want)
	}
}

func TestLastN(t *testing.T) {
	b := New(5)
	for _, line := range []string{"a", "b", "c", "d"} {
		b.Append(line)
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
		got := b.LastN(tc.n)
		if !reflect.DeepEqual(got, tc.want) {
			t.Errorf("LastN(%d) = %v, want %v", tc.n, got, tc.want)
		}
	}
}

func TestLastNAfterWrap(t *testing.T) {
	b := New(3)
	for _, line := range []string{"a", "b", "c", "d", "e"} {
		b.Append(line)
	}
	if got, want := b.LastN(2), []string{"d", "e"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("LastN(2) = %v, want %v", got, want)
	}
}

func TestSnapshotIsCopy(t *testing.T) {
	b := New(3)
	b.Append("a")
	b.Append("b")
	snap := b.Snapshot()
	snap[0] = "mutated"
	again := b.Snapshot()
	if again[0] != "a" {
		t.Fatalf("buffer mutated by caller via snapshot: %v", again)
	}
}

func TestEmptyBuffer(t *testing.T) {
	b := New(4)
	if got, want := b.Snapshot(), []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty Snapshot = %v, want %v", got, want)
	}
	if got, want := b.LastN(5), []string{}; !reflect.DeepEqual(got, want) {
		t.Fatalf("empty LastN = %v, want %v", got, want)
	}
}
