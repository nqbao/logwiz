package logmsg

import (
	"regexp"
	"testing"
)

var fullTsRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}\.\d{3}$`)

func TestNormalizeTime_RFC3339Nano(t *testing.T) {
	got := NormalizeTime("2026-04-21T14:02:31.123Z")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(RFC3339Nano) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
	if got != "2026-04-21 14:02:31.123" {
		t.Errorf("NormalizeTime(RFC3339Nano UTC) = %q, want \"2026-04-21 14:02:31.123\"", got)
	}
}

func TestNormalizeTime_RFC3339(t *testing.T) {
	got := NormalizeTime("2026-04-21T14:02:31Z")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(RFC3339) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
	if got != "2026-04-21 14:02:31.000" {
		t.Errorf("NormalizeTime(RFC3339 UTC) = %q, want \"2026-04-21 14:02:31.000\"", got)
	}
}

func TestNormalizeTime_EpochSec(t *testing.T) {
	got := NormalizeTime("1745234551")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(epoch-sec) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
}

func TestNormalizeTime_EpochMs(t *testing.T) {
	got := NormalizeTime("1745234551123")
	if !fullTsRe.MatchString(got) {
		t.Errorf("NormalizeTime(epoch-ms) = %q, want YYYY-MM-DD HH:MM:SS.mmm format", got)
	}
}

func TestNormalizeTime_Unparseable(t *testing.T) {
	const s = "not-a-timestamp"
	if got := NormalizeTime(s); got != s {
		t.Errorf("NormalizeTime(unparseable) = %q, want unchanged %q", got, s)
	}
}

func TestNormalizeLevel_Aliases(t *testing.T) {
	cases := []struct{ in, want string }{
		{"info", "INFO "},
		{"INFO", "INFO "},
		{"warn", "WARN "},
		{"warning", "WARN "},
		{"WARNING", "WARN "},
		{"err", "ERROR"},
		{"error", "ERROR"},
		{"ERR", "ERROR"},
		{"fatal", "FATAL"},
		{"panic", "FATAL"},
		{"debug", "DEBUG"},
		{"trace", "TRACE"},
	}
	for _, tc := range cases {
		if got := normalizeLevel(tc.in); got != tc.want {
			t.Errorf("normalizeLevel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestNormalizeLevel_Padding(t *testing.T) {
	// Short unknown level should be padded to 5 chars.
	got := normalizeLevel("ok")
	if len(got) != 5 {
		t.Errorf("normalizeLevel(\"ok\") len = %d, want 5; got %q", len(got), got)
	}
}

func TestExtractFromJSON_Full(t *testing.T) {
	raw := `{"ts":"2026-04-21T14:02:31Z","level":"info","msg":"req done","user":"bob","latency_ms":12}`
	f, ok := ExtractFromJSON(raw)
	if !ok {
		t.Fatal("ExtractFromJSON returned false for valid JSON")
	}
	if f.Time == "" {
		t.Error("Time should be set")
	}
	if f.Level != "INFO " {
		t.Errorf("Level = %q, want \"INFO \"", f.Level)
	}
	if f.Message != "req done" {
		t.Errorf("Message = %q, want \"req done\"", f.Message)
	}
	if len(f.Rest) != 2 {
		t.Fatalf("len(Rest) = %d, want 2", len(f.Rest))
	}
	if f.Rest[0].Key != "user" || f.Rest[0].Value != "bob" {
		t.Errorf("Rest[0] = %+v, want {user bob}", f.Rest[0])
	}
	if f.Rest[1].Key != "latency_ms" || f.Rest[1].Value != "12" {
		t.Errorf("Rest[1] = %+v, want {latency_ms 12}", f.Rest[1])
	}
}

func TestExtractFromJSON_MissingTimestamp(t *testing.T) {
	raw := `{"level":"warn","msg":"degraded"}`
	f, ok := ExtractFromJSON(raw)
	if !ok {
		t.Fatal("ExtractFromJSON returned false")
	}
	if f.Time != "" {
		t.Errorf("Time = %q, want empty (no ts field)", f.Time)
	}
	if f.Level != "WARN " {
		t.Errorf("Level = %q, want \"WARN \"", f.Level)
	}
}

func TestExtractFromJSON_MissingLevel(t *testing.T) {
	raw := `{"ts":"2026-04-21T14:02:31Z","msg":"hello"}`
	f, ok := ExtractFromJSON(raw)
	if !ok {
		t.Fatal("ExtractFromJSON returned false")
	}
	if f.Level != "" {
		t.Errorf("Level = %q, want empty (no level field)", f.Level)
	}
	if f.Message != "hello" {
		t.Errorf("Message = %q, want \"hello\"", f.Message)
	}
}

func TestExtractFromJSON_NestedObject(t *testing.T) {
	raw := `{"msg":"ok","meta":{"region":"us-east"}}`
	f, ok := ExtractFromJSON(raw)
	if !ok {
		t.Fatal("ExtractFromJSON returned false")
	}
	if len(f.Rest) != 1 {
		t.Fatalf("len(Rest) = %d, want 1", len(f.Rest))
	}
	if f.Rest[0].Key != "meta" {
		t.Errorf("Rest[0].Key = %q, want \"meta\"", f.Rest[0].Key)
	}
	// Value should be the inline JSON of the nested object.
	if f.Rest[0].Value == "" {
		t.Error("nested value should not be empty")
	}
}

func TestExtractFromJSON_NotJSON(t *testing.T) {
	_, ok := ExtractFromJSON("not json at all")
	if ok {
		t.Error("ExtractFromJSON should return false for non-JSON input")
	}
}

func TestExtractFromJSON_JSONArray(t *testing.T) {
	_, ok := ExtractFromJSON(`["a","b"]`)
	if ok {
		t.Error("ExtractFromJSON should return false for JSON arrays")
	}
}

func TestExtractFromJSON_LevelAliasAtTimestamp(t *testing.T) {
	// @timestamp alias
	raw := `{"@timestamp":"2026-04-21T14:02:31Z","severity":"warning","message":"down"}`
	f, ok := ExtractFromJSON(raw)
	if !ok {
		t.Fatal("ExtractFromJSON returned false")
	}
	if f.Time == "" {
		t.Error("@timestamp should be recognised as time alias")
	}
	if f.Level != "WARN " {
		t.Errorf("Level = %q, want \"WARN \" for severity=warning", f.Level)
	}
	if f.Message != "down" {
		t.Errorf("Message = %q, want \"down\" for message alias", f.Message)
	}
}

func TestExtractFromLogfmt_Full(t *testing.T) {
	raw := `ts=2026-04-21T14:02:31Z level=info msg="req done" user=bob latency_ms=12`
	f, ok := ExtractFromLogfmt(raw)
	if !ok {
		t.Fatal("ExtractFromLogfmt returned false")
	}
	if f.Time == "" {
		t.Error("Time should be set")
	}
	if f.Level != "INFO " {
		t.Errorf("Level = %q, want \"INFO \"", f.Level)
	}
	if f.Message != "req done" {
		t.Errorf("Message = %q, want \"req done\"", f.Message)
	}
	if len(f.Rest) != 2 {
		t.Fatalf("len(Rest) = %d, want 2", len(f.Rest))
	}
	if f.Rest[0].Key != "user" {
		t.Errorf("Rest[0].Key = %q, want \"user\"", f.Rest[0].Key)
	}
}

func TestExtractFromLogfmt_LevelAliases(t *testing.T) {
	cases := []struct{ raw, wantLevel string }{
		{`level=warning msg=x ts=1`, "WARN "},
		{`level=err msg=x ts=1`, "ERROR"},
		{`lvl=panic msg=x ts=1`, "FATAL"},
		{`severity=debug msg=x ts=1`, "DEBUG"},
	}
	for _, tc := range cases {
		f, ok := ExtractFromLogfmt(tc.raw)
		if !ok {
			t.Errorf("ExtractFromLogfmt(%q) returned false", tc.raw)
			continue
		}
		if f.Level != tc.wantLevel {
			t.Errorf("ExtractFromLogfmt(%q).Level = %q, want %q", tc.raw, f.Level, tc.wantLevel)
		}
	}
}

func TestExtractFromLogfmt_TooFewPairs(t *testing.T) {
	_, ok := ExtractFromLogfmt("level=error")
	if ok {
		t.Error("ExtractFromLogfmt should return false for fewer than two pairs")
	}
}

func TestExtractFromLogfmt_PlainText(t *testing.T) {
	_, ok := ExtractFromLogfmt("2026-04-21 INFO server started")
	if ok {
		t.Error("ExtractFromLogfmt should return false for plain text")
	}
}
