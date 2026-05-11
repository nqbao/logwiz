package logmsg

import (
	"regexp"
	"strings"

	"github.com/go-logfmt/logfmt"
)

// twoPairAtStart matches lines that begin with at least two key=value
// pairs. Used as a quick heuristic to reject prose before invoking the
// full decoder (e.g. "[DEBUG] YY:MM:DD level=debug msg=hello" is prose;
// "level=debug msg=hello" is logfmt).
var twoPairAtStart = regexp.MustCompile(`^[^\s=]+=[^\s=].*?\s[^\s=]+=[^\s=]`)

// LogfmtParser detects logfmt-formatted lines and parses their key=value
// pairs, splitting into canonical Time/Level/Message fields plus Extra
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
	// ANSI-coloured prose can satisfy the '=' heuristic (e.g. modd's coloured
	// output around a `key=val` payload), but go-logfmt's scanner mangles ESC
	// bytes during reconstruction in LogfmtRenderer, leaving literal `[90m`
	// fragments on screen. Treat any line carrying SGR escapes as prose.
	if strings.ContainsRune(raw, '\x1b') {
		return LogMessage{}, false
	}
	// Require at least two key=value pairs at the start of the line.
	// This rejects plain prose and lines where a prose prefix precedes
	// logfmt pairs (e.g. "[DEBUG] YY:MM:DD level=debug msg=hello").
	if !twoPairAtStart.MatchString(raw) {
		return LogMessage{}, false
	}
	dec := logfmt.NewDecoder(strings.NewReader(raw))
	if !dec.ScanRecord() {
		return LogMessage{}, false
	}

	msg := LogMessage{Raw: raw, Format: "logfmt"}
	seenTime, seenLevel, seenMsg := false, false, false
	total := 0
	leadingValued := 0 // count of valued pairs before the first bare-key token

	for dec.ScanKeyval() {
		total++
		key := string(dec.Key())
		val := string(dec.Value())
		if total == leadingValued+1 && val != "" {
			leadingValued++
		}
		keyLower := strings.ToLower(key)
		switch {
		case !seenTime && containsStr(TimeKeys, keyLower):
			msg.Time = val
			seenTime = true
		case !seenLevel && containsStr(LevelKeys, keyLower):
			msg.Level = NormalizeLevel(val)
			seenLevel = true
		case !seenMsg && containsStr(MessageKeys, keyLower):
			msg.Message = val
			seenMsg = true
		default:
			msg.Extra.Set(key, val)
		}
	}
	// Real logfmt records lead with key=val pairs. Prose with embedded
	// payloads (e.g. "Loaded loop: id(loop)=42 source=Python") decodes its
	// leading prose words as bare-key tokens; accepting them would let
	// LogfmtRenderer re-emit each prose word as `word=`. Require the first
	// two scanned pairs to both carry values.
	if dec.Err() != nil || total < 2 || leadingValued < 2 {
		return LogMessage{}, false
	}
	return msg, true
}

var _ Parser = LogfmtParser{}
