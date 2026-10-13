package highlight

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;]*[A-Za-z]`)

func stripANSI(s string) string { return ansiRe.ReplaceAllString(s, "") }

// jsonMsg builds a LogMessage by running raw through JSONParser.
func jsonMsg(raw string) logmsg.LogMessage {
	return logmsg.JSONParser{}.Parse(raw)[0]
}

// logfmtMsg builds a LogMessage by running raw through LogfmtParser.
func logfmtMsg(raw string) logmsg.LogMessage {
	return logmsg.LogfmtParser{}.Parse(raw)[0]
}

func TestPrettyJSONRenderer_FullLine(t *testing.T) {
	raw := `{"ts":"2026-04-21T14:02:31Z","level":"info","msg":"request done","user":"bob","status":200}`
	out := PrettyJSONRenderer(jsonMsg(raw))
	if !strings.Contains(out, "\x1b[") {
		t.Error("PrettyJSONRenderer should produce ANSI escape codes")
	}
	if !strings.Contains(out, "request done") {
		t.Error("PrettyJSONRenderer output should contain the message")
	}
	if !strings.Contains(out, "user") {
		t.Error("PrettyJSONRenderer output should contain 'user' key")
	}
	if !strings.Contains(out, "INFO") {
		t.Error("PrettyJSONRenderer output should contain level token")
	}
}

func TestPrettyJSONRenderer_MissingTimestamp(t *testing.T) {
	raw := `{"level":"warn","msg":"degraded"}`
	out := PrettyJSONRenderer(jsonMsg(raw))
	if !strings.Contains(out, "WARN") {
		t.Error("PrettyJSONRenderer output should contain level token even without ts")
	}
	if !strings.Contains(out, "degraded") {
		t.Error("PrettyJSONRenderer output should contain message even without ts")
	}
}

func TestPrettyJSONRenderer_MissingLevel(t *testing.T) {
	raw := `{"ts":"2026-04-21T14:02:31Z","msg":"hello"}`
	out := PrettyJSONRenderer(jsonMsg(raw))
	if !strings.Contains(out, "hello") {
		t.Error("PrettyJSONRenderer output should contain message even without level")
	}
}

func TestPrettyJSONRenderer_NestedField(t *testing.T) {
	raw := `{"msg":"ok","meta":{"region":"us-east"}}`
	out := PrettyJSONRenderer(jsonMsg(raw))
	if !strings.Contains(out, "meta") {
		t.Error("PrettyJSONRenderer output should contain 'meta' key")
	}
	if !strings.Contains(out, "region") {
		t.Error("PrettyJSONRenderer output should contain nested content")
	}
	if strings.Contains(out, `\"region\"`) {
		t.Error("PrettyJSONRenderer should not double-escape nested JSON")
	}
	strip := stripANSI(out)
	if strings.Contains(strip, `"{`) || strings.Contains(strip, `}"`) {
		t.Errorf("PrettyJSONRenderer should not quote-wrap nested JSON, got: %s", strip)
	}
}

func TestPrettyJSONRenderer_NestedArray(t *testing.T) {
	raw := `{"msg":"ok","tags":["a","b"]}`
	out := PrettyJSONRenderer(jsonMsg(raw))
	if strings.Contains(out, `\"a\"`) {
		t.Error("PrettyJSONRenderer should not double-escape nested JSON array")
	}
}

func TestPrettyJSONRenderer_MalformedJSON(t *testing.T) {
	raw := `{not valid json`
	out := PrettyJSONRenderer(jsonMsg(raw))
	// Falls back to JSONRenderer which processes with chroma (adding ANSI codes).
	if out == "" {
		t.Error("PrettyJSONRenderer should return non-empty output for malformed JSON")
	}
	if !strings.Contains(out, "{") {
		t.Error("PrettyJSONRenderer fallback output should contain the opening brace from raw input")
	}
}

func TestPrettyJSONRenderer_LevelAliases(t *testing.T) {
	cases := []struct {
		raw      string
		wantText string
	}{
		{`{"level":"warning","msg":"x"}`, "WARN"},
		{`{"level":"err","msg":"x"}`, "ERROR"},
		{`{"level":"panic","msg":"x"}`, "FATAL"},
	}
	for _, tc := range cases {
		out := PrettyJSONRenderer(jsonMsg(tc.raw))
		if !strings.Contains(out, tc.wantText) {
			t.Errorf("PrettyJSONRenderer(%q) output %q should contain %q", tc.raw, out, tc.wantText)
		}
	}
}

func TestPrettyLogfmtRenderer_FullLine(t *testing.T) {
	raw := `ts=2026-04-21T14:02:31Z level=info msg="request done" user=bob status=200`
	out := PrettyLogfmtRenderer(logfmtMsg(raw))
	if !strings.Contains(out, "\x1b[") {
		t.Error("PrettyLogfmtRenderer should produce ANSI escape codes")
	}
	if !strings.Contains(out, "request done") {
		t.Error("PrettyLogfmtRenderer output should contain message")
	}
	if !strings.Contains(out, "user") {
		t.Error("PrettyLogfmtRenderer output should contain 'user' key")
	}
	if !strings.Contains(out, "INFO") {
		t.Error("PrettyLogfmtRenderer output should contain level token")
	}
}

func TestPrettyLogfmtRenderer_FallsBackOnFewPairs(t *testing.T) {
	// Single k=v pair: LogfmtParser rejects → no canonical fields → falls back
	// to LogfmtRenderer, which also returns unchanged for < 2 "=" signs.
	raw := "level=error"
	out := PrettyLogfmtRenderer(logfmtMsg(raw))
	if !strings.Contains(out, "level") {
		t.Error("PrettyLogfmtRenderer fallback should preserve original text")
	}
}

func TestPrettyLogfmtRenderer_LevelAliases(t *testing.T) {
	cases := []struct {
		raw      string
		wantText string
	}{
		{`level=warning msg=x ts=1`, "WARN"},
		{`level=err msg=x ts=1`, "ERROR"},
	}
	for _, tc := range cases {
		out := PrettyLogfmtRenderer(logfmtMsg(tc.raw))
		if !strings.Contains(out, tc.wantText) {
			t.Errorf("PrettyLogfmtRenderer(%q) output %q should contain %q", tc.raw, out, tc.wantText)
		}
	}
}

func TestPrettyRenderer_NumericTimestamp(t *testing.T) {
	// Unix epoch 1234567890 → 2009-02-13 23:31:30.000 UTC
	jsonRaw := `{"ts":1234567890,"level":"info","msg":"hello"}`
	lfRaw := `ts=1234567890 level=info msg=hello`

	jsonOut := stripANSI(PrettyJSONRenderer(jsonMsg(jsonRaw)))
	lfOut := stripANSI(PrettyLogfmtRenderer(logfmtMsg(lfRaw)))

	for _, out := range []string{jsonOut, lfOut} {
		if strings.Contains(out, "1234567890") {
			t.Errorf("numeric ts should be normalized, got raw epoch in: %q", out)
		}
		if !strings.Contains(out, "2009-02-13") {
			t.Errorf("expected normalized date 2009-02-13 in: %q", out)
		}
	}
}

func TestLogfmtRenderer_NumericTimestamp(t *testing.T) {
	raw := `ts=1234567890 level=info msg=hello`
	out := stripANSI(LogfmtRenderer(logfmtMsg(raw)))
	if strings.Contains(out, "1234567890") {
		t.Errorf("LogfmtRenderer: numeric ts should be normalized, got: %q", out)
	}
	if !strings.Contains(out, "2009-02-13") {
		t.Errorf("LogfmtRenderer: expected normalized date 2009-02-13, got: %q", out)
	}
}

func TestPrettyJSONAndLogfmt_EqualOutput(t *testing.T) {
	jsonRaw := `{"ts":"2026-04-21T14:02:31Z","level":"info","msg":"hello","user":"bob"}`
	lfRaw := `ts=2026-04-21T14:02:31Z level=info msg=hello user=bob`

	jsonOut := PrettyJSONRenderer(jsonMsg(jsonRaw))
	lfOut := PrettyLogfmtRenderer(logfmtMsg(lfRaw))

	for _, text := range []string{"INFO", "hello", "user"} {
		if !strings.Contains(jsonOut, text) {
			t.Errorf("PrettyJSONRenderer output missing %q", text)
		}
		if !strings.Contains(lfOut, text) {
			t.Errorf("PrettyLogfmtRenderer output missing %q", text)
		}
	}
}

func TestPipeRenderer_PreservesText(t *testing.T) {
	raw := "2024-01-02 15:04:05 | ERROR | main | boom | x"
	msg := logmsg.LogMessage{Raw: raw, Format: "pipe", Level: "ERROR"}
	if got := stripANSI(PipeRenderer(msg)); got != raw {
		t.Errorf("stripped render = %q, want %q", got, raw)
	}
}
