package logmsg

import "sort"

// Attrs is an opaque key-value store for log line attributes. The zero
// value is usable (empty). Mutations are not safe for concurrent use;
// parsers build an Attrs once and callers treat it as read-only.
//
// Iteration via Range is sorted by key so pretty rendering is stable
// across similar lines. Callers that need insertion order must track it
// externally.
type Attrs struct {
	m map[string]string
}

// Set inserts or overwrites key=value.
func (a *Attrs) Set(key, value string) {
	if a.m == nil {
		a.m = make(map[string]string)
	}
	a.m[key] = value
}

// Get returns the value for key, or "" if the key is absent or the
// store is empty.
func (a Attrs) Get(key string) string {
	if a.m == nil {
		return ""
	}
	return a.m[key]
}

// Len returns the number of stored pairs.
func (a Attrs) Len() int { return len(a.m) }

// Range calls fn for each pair in sorted key order. Return false from
// fn to stop iteration early.
func (a Attrs) Range(fn func(key, value string) bool) {
	if len(a.m) == 0 {
		return
	}
	keys := make([]string, 0, len(a.m))
	for k := range a.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !fn(k, a.m[k]) {
			return
		}
	}
}
