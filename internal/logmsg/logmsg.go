// Package logmsg defines the structured log line type used throughout
// logwiz, plus the Parser interface that turns a raw line into a
// LogMessage.
package logmsg

import "strings"

// MsgID is the stable identity of a LogMessage. It survives ring-buffer
// eviction (the message is gone but its ID is never reused) which makes
// it usable as a map key for per-message caches — e.g. a render/colour
// cache keyed by ID.
//
// Kept as a single-field struct so new dimensions (stream id, shard, …)
// can be added without breaking construction sites that use named fields.
type MsgID struct {
	Line uint64
}

// LogMessage is a single log line, optionally enriched with parsed
// canonical fields and attributes.
//
// ID is assigned by the buffer on Append; parsers leave it zero.
// Raw is the original line text after carriage-return sanitization
// (see source.lastCRSegment). Format is a hint to the syntax highlighter
// ("json", "logfmt", "" = passthrough).
//
// Time, Level, and Message hold the canonical fields extracted at parse
// time (empty string = absent). Level is stored normalized (5-char
// uppercase, e.g. "INFO ", "ERROR"). Time and Message are stored raw;
// renderers are responsible for time normalization. Extra holds the
// remaining non-canonical key=value pairs.
type LogMessage struct {
	ID     MsgID
	Raw    string
	Format string

	Time    string
	Level   string
	Message string

	Extra Extra
}

// Canonical field aliases. First match wins at parse time,
// case-insensitive. Exported so renderers can skip these keys when
// iterating Extra.
var (
	TimeKeys    = []string{"time", "ts", "@timestamp", "timestamp"}
	LevelKeys   = []string{"level", "lvl", "severity"}
	MessageKeys = []string{"msg", "message"}
)

// Attr returns the value for key. Canonical aliases (time/ts/@timestamp,
// level/lvl/severity, msg/message) resolve to the corresponding Time,
// Level, or Message field; all other keys are looked up in Extra.
// Returns "" if the key is absent.
func (m LogMessage) Attr(key string) string {
	k := strings.ToLower(key)
	if containsStr(TimeKeys, k) {
		return m.Time
	}
	if containsStr(LevelKeys, k) {
		return m.Level
	}
	if containsStr(MessageKeys, k) {
		return m.Message
	}
	return m.Extra.Get(key)
}

// Parser converts a raw log line into zero or more LogMessages.
// Multi-line parsers may buffer lines internally and emit 0, 1, or N
// messages per call. Flush drains any buffered partial message.
type Parser interface {
	Parse(raw string) []LogMessage
	Flush() []LogMessage
}

// PassthroughParser stores the line as-is with no attributes. It is
// the default parser used until a real one is plugged in.
type PassthroughParser struct{}

// Parse returns a single-element slice containing a LogMessage whose Raw
// is the input line and whose Extra is empty.
func (PassthroughParser) Parse(raw string) []LogMessage {
	return []LogMessage{{Raw: raw}}
}

// Flush returns nil; PassthroughParser never buffers.
func (PassthroughParser) Flush() []LogMessage { return nil }
