package logmsg

import (
	"encoding/json"
	"strings"
)

// JSONParser detects JSON-formatted lines and extracts canonical fields
// (time/level/message) plus the remaining top-level keys into Extra.
// Non-JSON lines are passed through as-is.
type JSONParser struct{}

func (JSONParser) Parse(raw string) []LogMessage {
	if !looksLikeJSON(raw) {
		return []LogMessage{{Raw: raw}}
	}
	msg, ok := parseJSON(raw)
	if !ok {
		// Malformed or non-object JSON (e.g. top-level array): keep the
		// format hint for the highlighter but leave fields empty.
		return []LogMessage{{Raw: raw, Format: "json"}}
	}
	return []LogMessage{msg}
}

func (JSONParser) Flush() []LogMessage { return nil }

// parseJSON streams top-level keys from a JSON object and splits them
// into canonical fields (Time/Level/Message) and remaining Extra.
// Returns false for non-object JSON or parse errors.
func parseJSON(raw string) (LogMessage, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil || tok != json.Delim('{') {
		return LogMessage{}, false
	}

	msg := LogMessage{Raw: raw, Format: "json"}
	seenTime, seenLevel, seenMsg := false, false, false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return LogMessage{}, false
		}
		key, ok := keyTok.(string)
		if !ok {
			return LogMessage{}, false
		}
		var val any
		if err := dec.Decode(&val); err != nil {
			return LogMessage{}, false
		}
		keyLower := strings.ToLower(key)
		switch {
		case !seenTime && containsStr(TimeKeys, keyLower):
			msg.Time = jsonValToStr(val)
			seenTime = true
		case !seenLevel && containsStr(LevelKeys, keyLower):
			msg.Level = NormalizeLevel(jsonValToStr(val))
			seenLevel = true
		case !seenMsg && containsStr(MessageKeys, keyLower):
			msg.Message = jsonValToStr(val)
			seenMsg = true
		default:
			msg.Extra.Set(key, jsonValToStr(val))
		}
	}
	return msg, true
}

// looksLikeJSON is a cheap byte-level heuristic to avoid invoking the full
// JSON decoder on plain text. A line must both open and close with matching
// delimiters: '{' … '}' for objects, '[' … ']' for arrays. This rejects log
// prefixes like "[DEBUG] 12:34:56: msg" or "{startup} config loaded" that
// open with a JSON delimiter but are not JSON.
func looksLikeJSON(s string) bool {
	t := strings.TrimSpace(s)
	if len(t) == 0 {
		return false
	}
	return (t[0] == '{' && t[len(t)-1] == '}') || (t[0] == '[' && t[len(t)-1] == ']')
}
