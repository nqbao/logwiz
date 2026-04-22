package logmsg

import "testing"

func TestAttrNilMap(t *testing.T) {
	m := LogMessage{Raw: "hello"}
	if got := m.Attr("level"); got != "" {
		t.Errorf("Attr on nil map = %q, want \"\"", got)
	}
}

func TestAttrPresent(t *testing.T) {
	m := LogMessage{
		Raw:        "hello",
		Attributes: map[string]string{"level": "INFO", "ts": "2026-04-16"},
	}
	if got := m.Attr("level"); got != "INFO" {
		t.Errorf("Attr(level) = %q, want INFO", got)
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
	if got[0].Attributes != nil {
		t.Errorf("Attributes = %v, want nil", got[0].Attributes)
	}
}

func TestPassthroughParserFlush(t *testing.T) {
	var p Parser = PassthroughParser{}
	if got := p.Flush(); got != nil {
		t.Errorf("Flush = %v, want nil", got)
	}
}
