package logmsg

import "strings"

// JSONParser detects JSON-formatted lines and tags them so the syntax
// highlighter can colour them. Non-JSON lines are passed through as-is.
type JSONParser struct{}

func (JSONParser) Parse(raw string) LogMessage {
	if looksLikeJSON(raw) {
		return LogMessage{Raw: raw, Format: "json"}
	}
	return LogMessage{Raw: raw}
}

// looksLikeJSON is a cheap byte-level heuristic: a line is treated as JSON
// if its first non-whitespace character is '{' or '['. Full validation is
// intentionally deferred to the renderer so malformed input degrades
// gracefully (chroma emits the raw text on tokenise errors).
func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) == 0 {
		return false
	}
	return t[0] == '{' || t[0] == '['
}
