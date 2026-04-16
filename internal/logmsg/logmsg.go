// Package logmsg defines the structured log line type used throughout
// logwiz, plus the Parser interface that turns a raw line into a
// LogMessage.
//
// Today only PassthroughParser ships; future parsers (regex, JSON,
// logfmt) will live in this package and populate Attributes.
package logmsg

// LogMessage is a single log line, optionally enriched with parsed
// attributes (timestamp, level, source, etc.).
//
// Raw is the original line text after carriage-return sanitization
// (see source.lastCRSegment). Attributes is nil for unparsed messages
// — callers should use Attr to read individual keys safely.
type LogMessage struct {
	Raw        string
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

// Parser converts a raw log line into a LogMessage.
type Parser interface {
	Parse(raw string) LogMessage
}

// PassthroughParser stores the line as-is with no attributes. It is
// the default parser used until a real one is plugged in.
type PassthroughParser struct{}

// Parse returns a LogMessage whose Raw is the input line and whose
// Attributes is nil.
func (PassthroughParser) Parse(raw string) LogMessage {
	return LogMessage{Raw: raw}
}
