// Package redact replaces sensitive patterns with [REDACTED] in strings
// before they are sent to the LLM. The display layer is never affected.
package redact

import (
	"fmt"
	"regexp"
)

// Redactor holds a compiled set of patterns. A nil *Redactor is safe to
// use — all methods are no-ops, so callers need no nil guards.
type Redactor struct {
	rules []*regexp.Regexp
}

// New compiles patterns into a Redactor. Returns (nil, nil) when
// patterns is empty so callers can treat a nil Redactor as "disabled".
func New(patterns []string) (*Redactor, error) {
	if len(patterns) == 0 {
		return nil, nil
	}
	rules := make([]*regexp.Regexp, len(patterns))
	for i, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("redact: invalid pattern %q: %w", p, err)
		}
		rules[i] = re
	}
	return &Redactor{rules: rules}, nil
}

// Redact replaces every match of every rule in s with [REDACTED].
func (r *Redactor) Redact(s string) string {
	if r == nil {
		return s
	}
	for _, re := range r.rules {
		s = re.ReplaceAllString(s, "[REDACTED]")
	}
	return s
}

// RedactLines returns a new slice with Redact applied to every element.
func (r *Redactor) RedactLines(lines []string) []string {
	if r == nil {
		return lines
	}
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = r.Redact(l)
	}
	return out
}
