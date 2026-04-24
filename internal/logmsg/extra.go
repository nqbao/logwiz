package logmsg

import "sort"

// Extra is an opaque key-value store for non-canonical log line
// attributes — i.e. every structured field that is not Time, Level, or
// Message. The zero value is usable (empty). Mutations are not safe for
// concurrent use; parsers build an Extra once and callers treat it as
// read-only.
//
// Iteration via Range is sorted by key so pretty rendering is stable
// across similar lines. Callers that need insertion order must track it
// externally.
type Extra struct {
	m map[string]string
}

// Set inserts or overwrites key=value.
func (e *Extra) Set(key, value string) {
	if e.m == nil {
		e.m = make(map[string]string)
	}
	e.m[key] = value
}

// Get returns the value for key, or "" if the key is absent or the
// store is empty.
func (e Extra) Get(key string) string {
	if e.m == nil {
		return ""
	}
	return e.m[key]
}

// Len returns the number of stored pairs.
func (e Extra) Len() int { return len(e.m) }

// Range calls fn for each pair in sorted key order. Return false from
// fn to stop iteration early.
func (e Extra) Range(fn func(key, value string) bool) {
	if len(e.m) == 0 {
		return
	}
	keys := make([]string, 0, len(e.m))
	for k := range e.m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !fn(k, e.m[k]) {
			return
		}
	}
}
