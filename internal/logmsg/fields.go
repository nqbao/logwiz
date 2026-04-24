package logmsg

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// NormalizeTime parses RFC3339, epoch-sec, or epoch-ms timestamps and returns
// YYYY-MM-DD HH:MM:SS.mmm (UTC). Returns s unchanged when no format matches.
func NormalizeTime(s string) string {
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05.000")
	}
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.UTC().Format("2006-01-02 15:04:05.000")
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
		return t.UTC().Format("2006-01-02 15:04:05.000")
	}
	return s
}

// NormalizeLevel maps any level string to a 5-char uppercase form.
// Aliases: warning→WARN, err→ERROR, panic→FATAL. Unknown values are
// uppercased and padded/truncated to 5 chars.
func NormalizeLevel(s string) string {
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

// containsStr reports whether list contains s.
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

// QuoteIfNeeded wraps s in double quotes when it is empty or contains
// characters that would be ambiguous in a k=v rendering context.
func QuoteIfNeeded(s string) string {
	if s == "" || strings.ContainsAny(s, " =\"'\t\n\r") {
		return strconv.Quote(s)
	}
	return s
}
