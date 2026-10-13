package logmsg

import (
	"regexp"
	"strconv"
	"strings"
)

// pipeTimeRe matches the timestamp that must open a pipe-delimited header
// line: ISO-ish dates ("2024-01-02 15:04:05,123", "2024-01-02T15:04:05.123Z")
// or a bare clock ("15:04:05.123").
var pipeTimeRe = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2}[T ])?\d{2}:\d{2}:\d{2}([.,]\d+)?(Z|[+-]\d{2}:?\d{2})?$`)

// stackLineRe matches lines that continue a Java-style record: indented
// frames ("\tat com.foo.Bar(Bar.java:42)"), "Caused by:", "Suppressed:",
// "... 12 more", and a bare exception header ("java.lang.Foo: msg").
var stackLineRe = regexp.MustCompile(`^(\s+\S|Caused by:|Suppressed:|\.\.\. \d+ more|[\w$]+(\.[\w$]+)+(Exception|Error|Throwable)\b)`)

// pipeLoggerRe matches a dotted logger/class name ("com.example.Foo").
var pipeLoggerRe = regexp.MustCompile(`^[\w$]+(\.[\w$]+)+$`)

// PipeParser parses pipe-delimited lines common in Java logging setups:
//
//	2024-01-02 15:04:05.123 | INFO  | main | com.example.App | Started in 1.2s
//
// The first field must be a timestamp. A field holding a known level becomes
// Level; short single-token fields become Extra ("thread", "logger", or
// "fieldN"); the first field containing whitespace (or the last field) starts
// the Message, which runs to the end of the line so a "|" inside the message
// is kept.
//
// Lines that follow a header but are not headers themselves (stack traces)
// are emitted as separate passthrough messages that inherit the header's Time
// and Level, so level/time filters keep a record and its trace together.
// When Strict is false (auto-detect mode), only stack-trace-looking lines
// are treated as continuations.
type PipeParser struct {
	Strict bool

	last    LogMessage
	hasLast bool
}

func (p *PipeParser) Parse(raw string) []LogMessage {
	if msg, ok := parsePipe(raw); ok {
		p.last, p.hasLast = msg, true
		return []LogMessage{msg}
	}
	if msg, ok := p.continuation(raw); ok {
		return []LogMessage{msg}
	}
	p.hasLast = false
	return []LogMessage{{Raw: raw}}
}

// continuation returns raw as a continuation of the last header, if any.
func (p *PipeParser) continuation(raw string) (LogMessage, bool) {
	if !p.hasLast || strings.TrimSpace(raw) == "" {
		return LogMessage{}, false
	}
	if !p.Strict && !stackLineRe.MatchString(raw) {
		return LogMessage{}, false
	}
	return LogMessage{Raw: raw, Time: p.last.Time, Level: p.last.Level}, true
}

func (p *PipeParser) Flush() []LogMessage { return nil }

// reset drops continuation state; used by AutoParser when another format wins.
func (p *PipeParser) reset() { p.hasLast = false }

// parsePipe builds a LogMessage from a pipe-delimited header line.
func parsePipe(raw string) (LogMessage, bool) {
	first := strings.IndexByte(raw, '|')
	if first < 0 || strings.ContainsRune(raw, '\x1b') {
		return LogMessage{}, false
	}
	ts := strings.TrimSpace(raw[:first])
	if !pipeTimeRe.MatchString(ts) {
		return LogMessage{}, false
	}

	msg := LogMessage{Raw: raw, Format: "pipe", Time: ts}
	rest := raw[first+1:]
	n := 0
	for rest != "" {
		field, tail, more := strings.Cut(rest, "|")
		f := strings.TrimSpace(field)
		switch {
		case !more || strings.ContainsAny(f, " \t"):
			// Message: from here to end of line, separators included.
			msg.Message = strings.TrimSpace(rest)
			rest = ""
			continue
		case msg.Level == "" && isPipeLevel(f):
			msg.Level = NormalizeLevel(f)
		case f == "":
		default:
			n++
			msg.Extra.Set(pipeFieldName(&msg, f, n), strings.Trim(f, "[]"))
		}
		rest = tail
	}
	return msg, true
}

func isPipeLevel(s string) bool {
	switch strings.ToUpper(s) {
	case "TRACE", "DEBUG", "INFO", "WARN", "WARNING", "ERROR", "ERR", "FATAL", "SEVERE":
		return true
	}
	return false
}

// pipeFieldName picks an attribute name for a metadata field: dotted names
// are the logger, the first other field is the thread, the rest are fieldN.
func pipeFieldName(msg *LogMessage, f string, n int) string {
	switch {
	case pipeLoggerRe.MatchString(f) && msg.Extra.Get("logger") == "":
		return "logger"
	case msg.Extra.Get("thread") == "" && msg.Extra.Get("logger") == "":
		return "thread"
	}
	return "field" + strconv.Itoa(n)
}

var _ Parser = (*PipeParser)(nil)
