package logmsg

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/go-logfmt/logfmt"
)

// canonical field alias lists; first match wins, case-insensitive.
var (
	tsKeys    = []string{"time", "ts", "@timestamp", "timestamp"}
	levelKeys = []string{"level", "lvl", "severity"}
	msgKeys   = []string{"msg", "message"}
)

// KV is a single ordered key-value pair from a structured log line.
type KV struct{ Key, Value string }

// Fields holds the canonical columns extracted from a structured log line.
// Empty strings indicate absent fields; Rest preserves input key order.
type Fields struct {
	Time    string // HH:MM:SS.mmm (UTC), or "" if absent/unparseable
	Level   string // 5-char padded uppercase ("INFO ", "WARN ", "ERROR", "DEBUG", "TRACE", "FATAL")
	Message string
	Rest    []KV
}

// normalizeTime parses RFC3339, epoch-sec, or epoch-ms timestamps and returns
// HH:MM:SS.mmm (UTC). Returns s unchanged when no format matches.
func normalizeTime(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format("15:04:05.000")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("15:04:05.000")
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil && f > 0 {
		var t time.Time
		if f > 1e12 { // milliseconds
			ms := int64(f)
			t = time.Unix(ms/1000, (ms%1000)*int64(time.Millisecond))
		} else { // seconds
			sec := int64(f)
			nsec := int64(math.Round((f - float64(sec)) * 1e9))
			t = time.Unix(sec, nsec)
		}
		return t.UTC().Format("15:04:05.000")
	}
	return s
}

// normalizeLevel maps any level string to a 5-char uppercase form.
// Aliases: warning→WARN, err→ERROR, panic→FATAL.
func normalizeLevel(s string) string {
	switch strings.ToUpper(s) {
	case "TRACE":
		return "TRACE"
	case "DEBUG":
		return "DEBUG"
	case "INFO":
		return "INFO "
	case "WARN", "WARNING":
		return "WARN "
	case "ERROR", "ERR":
		return "ERROR"
	case "FATAL", "PANIC":
		return "FATAL"
	default:
		u := strings.ToUpper(s)
		if len(u) >= 5 {
			return u[:5]
		}
		return fmt.Sprintf("%-5s", u)
	}
}

// ExtractFromJSON parses raw as a JSON object and extracts canonical fields
// in key-declaration order via token streaming. Returns (Fields{}, false)
// when raw is not a valid JSON object.
func ExtractFromJSON(raw string) (Fields, bool) {
	dec := json.NewDecoder(strings.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return Fields{}, false
	}
	if tok != json.Delim('{') {
		return Fields{}, false
	}

	var f Fields
	seenTS, seenLevel, seenMsg := false, false, false

	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return Fields{}, false
		}
		key, ok := keyTok.(string)
		if !ok {
			return Fields{}, false
		}
		var val any
		if err := dec.Decode(&val); err != nil {
			return Fields{}, false
		}
		keyLower := strings.ToLower(key)
		switch {
		case !seenTS && containsStr(tsKeys, keyLower):
			f.Time = normalizeTime(jsonValToStr(val))
			seenTS = true
		case !seenLevel && containsStr(levelKeys, keyLower):
			f.Level = normalizeLevel(jsonValToStr(val))
			seenLevel = true
		case !seenMsg && containsStr(msgKeys, keyLower):
			f.Message = jsonValToStr(val)
			seenMsg = true
		default:
			f.Rest = append(f.Rest, KV{Key: key, Value: formatJSONVal(val)})
		}
	}
	return f, true
}

// ExtractFromLogfmt parses raw as a logfmt line and extracts canonical fields
// in key-declaration order. Returns (Fields{}, false) for lines with fewer
// than two key=value pairs or parse errors.
func ExtractFromLogfmt(raw string) (Fields, bool) {
	if strings.Count(raw, "=") < 2 {
		return Fields{}, false
	}
	dec := logfmt.NewDecoder(strings.NewReader(raw))
	if !dec.ScanRecord() {
		return Fields{}, false
	}

	var f Fields
	seenTS, seenLevel, seenMsg := false, false, false

	for dec.ScanKeyval() {
		key := string(dec.Key())
		val := string(dec.Value())
		keyLower := strings.ToLower(key)
		switch {
		case !seenTS && containsStr(tsKeys, keyLower):
			f.Time = normalizeTime(val)
			seenTS = true
		case !seenLevel && containsStr(levelKeys, keyLower):
			f.Level = normalizeLevel(val)
			seenLevel = true
		case !seenMsg && containsStr(msgKeys, keyLower):
			f.Message = val
			seenMsg = true
		default:
			f.Rest = append(f.Rest, KV{Key: key, Value: quoteIfNeeded(val)})
		}
	}
	if dec.Err() != nil {
		return Fields{}, false
	}
	return f, true
}

func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// jsonValToStr converts a JSON-decoded scalar to its string form.
// Objects and arrays are JSON-marshalled.
func jsonValToStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == math.Trunc(t) && t >= -1e15 && t <= 1e15 {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	case nil:
		return "null"
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// formatJSONVal formats a JSON value for display in a k=v pair. Strings are
// quoted if they contain special chars; non-scalars are rendered as inline JSON.
func formatJSONVal(v any) string {
	switch v.(type) {
	case string:
		return quoteIfNeeded(v.(string))
	case float64, bool, nil:
		return jsonValToStr(v)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

// quoteIfNeeded wraps s in double quotes when it is empty or contains
// characters that would be ambiguous in a k=v context.
func quoteIfNeeded(s string) string {
	if s == "" || strings.ContainsAny(s, " =\"'\t\n\r") {
		return strconv.Quote(s)
	}
	return s
}
