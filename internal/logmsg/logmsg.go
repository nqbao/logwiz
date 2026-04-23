// Package logmsg defines the structured log line type used throughout
// logwiz, plus the Parser interface that turns a raw line into a
// LogMessage.
//
// Today only PassthroughParser ships; future parsers (regex, JSON,
// logfmt) will live in this package and populate Attributes.
package logmsg

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
// attributes (timestamp, level, source, etc.).
//
// ID is assigned by the buffer on Append; parsers leave it zero.
// Raw is the original line text after carriage-return sanitization
// (see source.lastCRSegment). Attributes is nil for unparsed messages
// — callers should use Attr to read individual keys safely.
// Format is a hint to the syntax highlighter ("json", "" = passthrough).
type LogMessage struct {
	ID         MsgID
	Raw        string
	Format     string // "json", "" = passthrough
	Attributes map[string]string
}

// Attr returns the attribute for key, or "" if the map is nil or the
// key is absent.
func (m LogMessage) Attr(key string) string {
	if m.Attributes == nil {
		return ""
	}
	return m.Attributes[key]
}

// Copy returns a deep copy of m. The Attributes map is cloned so the
// caller can safely retain the copy past a ring-buffer eviction (or
// mutate Attributes) without affecting the source.
func (m LogMessage) Copy() LogMessage {
	c := m
	if m.Attributes != nil {
		c.Attributes = make(map[string]string, len(m.Attributes))
		for k, v := range m.Attributes {
			c.Attributes[k] = v
		}
	}
	return c
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
// is the input line and whose Attributes is nil.
func (PassthroughParser) Parse(raw string) []LogMessage {
	return []LogMessage{{Raw: raw}}
}

// Flush returns nil; PassthroughParser never buffers.
func (PassthroughParser) Flush() []LogMessage { return nil }
