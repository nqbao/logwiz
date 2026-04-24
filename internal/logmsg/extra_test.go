package logmsg

import "testing"

func TestExtra_ZeroValueIsEmpty(t *testing.T) {
	var e Extra
	if e.Len() != 0 {
		t.Errorf("Len() = %d, want 0", e.Len())
	}
	if got := e.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want \"\"", got)
	}
	e.Range(func(_, _ string) bool {
		t.Error("Range should not invoke fn on empty Extra")
		return true
	})
}

func TestExtra_SetAndGet(t *testing.T) {
	var e Extra
	e.Set("user", "bob")
	e.Set("count", "42")
	if got := e.Get("user"); got != "bob" {
		t.Errorf("Get(user) = %q, want \"bob\"", got)
	}
	if got := e.Get("count"); got != "42" {
		t.Errorf("Get(count) = %q, want \"42\"", got)
	}
	if e.Len() != 2 {
		t.Errorf("Len() = %d, want 2", e.Len())
	}
}

func TestExtra_SetOverwrites(t *testing.T) {
	var e Extra
	e.Set("k", "v1")
	e.Set("k", "v2")
	if got := e.Get("k"); got != "v2" {
		t.Errorf("Get(k) = %q, want \"v2\"", got)
	}
	if e.Len() != 1 {
		t.Errorf("Len() = %d, want 1", e.Len())
	}
}

func TestExtra_RangeSortedByKey(t *testing.T) {
	var e Extra
	e.Set("charlie", "3")
	e.Set("alpha", "1")
	e.Set("bravo", "2")

	var gotKeys []string
	var gotVals []string
	e.Range(func(k, v string) bool {
		gotKeys = append(gotKeys, k)
		gotVals = append(gotVals, v)
		return true
	})
	wantKeys := []string{"alpha", "bravo", "charlie"}
	wantVals := []string{"1", "2", "3"}
	for i, k := range wantKeys {
		if gotKeys[i] != k {
			t.Errorf("keys[%d] = %q, want %q", i, gotKeys[i], k)
		}
		if gotVals[i] != wantVals[i] {
			t.Errorf("values[%d] = %q, want %q", i, gotVals[i], wantVals[i])
		}
	}
}

func TestExtra_RangeEarlyStop(t *testing.T) {
	var e Extra
	e.Set("a", "1")
	e.Set("b", "2")
	e.Set("c", "3")

	count := 0
	e.Range(func(_, _ string) bool {
		count++
		return count < 2 // stop after 2 visits
	})
	if count != 2 {
		t.Errorf("visited %d entries, want 2", count)
	}
}
