package logmsg

import (
	"encoding/json"
	"strings"
)

// JSONParser detects JSON-formatted lines and tags them so the syntax
// highlighter can colour them. Non-JSON lines are passed through as-is.
// For valid JSON objects, top-level keys are also parsed into Attributes.
type JSONParser struct{}

func (JSONParser) Parse(raw string) []LogMessage {
	if !looksLikeJSON(raw) {
		return []LogMessage{{Raw: raw}}
	}
	attrs, ok := parseJSONAttrs(raw)
	if !ok {
		// Malformed or non-object JSON (e.g. top-level array): keep the format
		// hint for the highlighter but leave attributes nil.
		return []LogMessage{{Raw: raw, Format: "json"}}
	}
	return []LogMessage{{Raw: raw, Format: "json", Attributes: attrs}}
}

func (JSONParser) Flush() []LogMessage { return nil }

// parseJSONAttrs streams top-level keys from a JSON object and returns them
// as a flat map[string]string. Nested objects and arrays are stored as their
// JSON encoding. Returns false for non-object JSON or parse errors.
func parseJSONAttrs(raw string) (map[string]string, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return nil, false
	}
	attrs := make(map[string]string)
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return nil, false
		}
		key, ok := keyTok.(string)
		if !ok {
			return nil, false
		}
		var val any
		if err := dec.Decode(&val); err != nil {
			return nil, false
		}
		attrs[key] = jsonValToStr(val)
	}
	return attrs, true
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
