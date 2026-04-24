package logmsg

import "testing"

func TestAttrs_ZeroValueIsEmpty(t *testing.T) {
	var a Attrs
	if a.Len() != 0 {
		t.Errorf("Len() = %d, want 0", a.Len())
	}
	if got := a.Get("missing"); got != "" {
		t.Errorf("Get(missing) = %q, want \"\"", got)
	}
	a.Range(func(_, _ string) bool {
		t.Error("Range should not invoke fn on empty Attrs")
		return true
	})
}

func TestAttrs_SetAndGet(t *testing.T) {
	var a Attrs
	a.Set("user", "bob")
	a.Set("count", "42")
	if got := a.Get("user"); got != "bob" {
		t.Errorf("Get(user) = %q, want \"bob\"", got)
	}
	if got := a.Get("count"); got != "42" {
		t.Errorf("Get(count) = %q, want \"42\"", got)
	}
	if a.Len() != 2 {
		t.Errorf("Len() = %d, want 2", a.Len())
	}
}

func TestAttrs_SetOverwrites(t *testing.T) {
	var a Attrs
	a.Set("k", "v1")
	a.Set("k", "v2")
	if got := a.Get("k"); got != "v2" {
		t.Errorf("Get(k) = %q, want \"v2\"", got)
	}
	if a.Len() != 1 {
		t.Errorf("Len() = %d, want 1", a.Len())
	}
}

func TestAttrs_RangeSortedByKey(t *testing.T) {
	var a Attrs
	a.Set("charlie", "3")
	a.Set("alpha", "1")
	a.Set("bravo", "2")

	var gotKeys []string
	var gotVals []string
	a.Range(func(k, v string) bool {
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

func TestAttrs_RangeEarlyStop(t *testing.T) {
	var a Attrs
	a.Set("a", "1")
	a.Set("b", "2")
	a.Set("c", "3")

	count := 0
	a.Range(func(_, _ string) bool {
		count++
		return count < 2 // stop after 2 visits
	})
	if count != 2 {
		t.Errorf("visited %d entries, want 2", count)
	}
}
