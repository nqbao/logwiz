package logmsg

import (
	"strings"

	"github.com/go-logfmt/logfmt"
)

// LogfmtParser detects logfmt-formatted lines and parses their key=value
// attributes. Lines with fewer than two valid pairs, or any unparseable
// prose, are passed through as-is.
type LogfmtParser struct{}

func (LogfmtParser) Parse(raw string) LogMessage {
	attrs, ok := parseLogfmt(raw)
	if !ok {
		return LogMessage{Raw: raw}
	}
	return LogMessage{Raw: raw, Format: "logfmt", Attributes: attrs}
}

// parseLogfmt returns a key→value map for raw if it is a valid logfmt record
// with at least two pairs and no unparseable prose. Returns false otherwise.
func parseLogfmt(raw string) (map[string]string, bool) {
	if strings.TrimSpace(raw) == "" {
		return nil, false
	}
	// Require at least two explicit '=' signs before invoking the full parser.
	// This rejects plain prose and lines where bare words parse as empty-value
	// keys (e.g. "2026-04-21 INFO server started" or "user=bob logged in").
	if strings.Count(raw, "=") < 2 {
		return nil, false
	}
	dec := logfmt.NewDecoder(strings.NewReader(raw))
	if !dec.ScanRecord() {
		return nil, false
	}
	attrs := make(map[string]string)
	for dec.ScanKeyval() {
		attrs[string(dec.Key())] = string(dec.Value())
	}
	if dec.Err() != nil || len(attrs) < 2 {
		return nil, false
	}
	return attrs, true
}

var _ Parser = LogfmtParser{}
