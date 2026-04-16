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
	if got.Raw != "some raw line" {
		t.Errorf("Raw = %q, want %q", got.Raw, "some raw line")
	}
	if got.Attributes != nil {
		t.Errorf("Attributes = %v, want nil", got.Attributes)
	}
}
