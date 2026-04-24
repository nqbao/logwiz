package logmsg

import (
	"strings"

	"github.com/go-logfmt/logfmt"
)

// LogfmtParser detects logfmt-formatted lines and parses their key=value
// pairs, splitting into canonical Time/Level/Message fields plus Attrs
// for the rest. Lines with fewer than two valid pairs, or any
// unparseable prose, are passed through as-is.
type LogfmtParser struct{}

func (LogfmtParser) Parse(raw string) []LogMessage {
	msg, ok := parseLogfmt(raw)
	if !ok {
		return []LogMessage{{Raw: raw}}
	}
	return []LogMessage{msg}
}

func (LogfmtParser) Flush() []LogMessage { return nil }

// parseLogfmt builds a LogMessage from raw if it is a valid logfmt
// record with at least two pairs and no unparseable prose. Returns
// false otherwise.
func parseLogfmt(raw string) (LogMessage, bool) {
	if strings.TrimSpace(raw) == "" {
		return LogMessage{}, false
	}
	// Require at least two explicit '=' signs before invoking the full parser.
	// This rejects plain prose and lines where bare words parse as empty-value
	// keys (e.g. "2026-04-21 INFO server started" or "user=bob logged in").
	if strings.Count(raw, "=") < 2 {
		return LogMessage{}, false
	}
	dec := logfmt.NewDecoder(strings.NewReader(raw))
	if !dec.ScanRecord() {
		return LogMessage{}, false
	}

	msg := LogMessage{Raw: raw, Format: "logfmt"}
	seenTime, seenLevel, seenMsg := false, false, false
	total := 0

	for dec.ScanKeyval() {
		total++
		key := string(dec.Key())
		val := string(dec.Value())
		keyLower := strings.ToLower(key)
		switch {
		case !seenTime && containsStr(TimeKeys, keyLower):
			msg.Time = val
			seenTime = true
		case !seenLevel && containsStr(LevelKeys, keyLower):
			msg.Level = val
			seenLevel = true
		case !seenMsg && containsStr(MessageKeys, keyLower):
			msg.Message = val
			seenMsg = true
		default:
			msg.Attrs.Set(key, val)
		}
	}
	if dec.Err() != nil || total < 2 {
		return LogMessage{}, false
	}
	return msg, true
}

var _ Parser = LogfmtParser{}
