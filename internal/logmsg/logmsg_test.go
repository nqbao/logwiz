package logmsg

import "testing"

func TestAttrEmpty(t *testing.T) {
	m := LogMessage{Raw: "hello"}
	if got := m.Attr("level"); got != "" {
		t.Errorf("Attr on empty = %q, want \"\"", got)
	}
	if got := m.Attr("anything"); got != "" {
		t.Errorf("Attr on empty = %q, want \"\"", got)
	}
}

func TestAttrCanonicalFields(t *testing.T) {
	m := LogMessage{
		Time:    "2026-04-16",
		Level:   "INFO ",
		Message: "hello",
	}
	cases := map[string]string{
		"time":       "2026-04-16",
		"ts":         "2026-04-16",
		"@timestamp": "2026-04-16",
		"timestamp":  "2026-04-16",
		"TS":         "2026-04-16",
		"level":      "INFO ",
		"lvl":        "INFO ",
		"severity":   "INFO ",
		"msg":        "hello",
		"message":    "hello",
	}
	for key, want := range cases {
		if got := m.Attr(key); got != want {
			t.Errorf("Attr(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestAttrFallsBackToExtra(t *testing.T) {
	m := LogMessage{}
	m.Extra.Set("user", "bob")
	if got := m.Attr("user"); got != "bob" {
		t.Errorf("Attr(user) = %q, want \"bob\"", got)
	}
	if got := m.Attr("missing"); got != "" {
		t.Errorf("Attr(missing) = %q, want \"\"", got)
	}
}

func TestPassthroughParser(t *testing.T) {
	var p Parser = PassthroughParser{}
	got := p.Parse("some raw line")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Raw != "some raw line" {
		t.Errorf("Raw = %q, want %q", got[0].Raw, "some raw line")
	}
	if got[0].Extra.Len() != 0 {
		t.Errorf("Extra.Len() = %d, want 0", got[0].Extra.Len())
	}
	if got[0].Time != "" || got[0].Level != "" || got[0].Message != "" {
		t.Errorf("canonical fields should be empty, got Time=%q Level=%q Message=%q",
			got[0].Time, got[0].Level, got[0].Message)
	}
}

func TestPassthroughParserFlush(t *testing.T) {
	var p Parser = PassthroughParser{}
	if got := p.Flush(); got != nil {
		t.Errorf("Flush = %v, want nil", got)
	}
}
