package logmsg

import "testing"

func TestAutoParser_DetectsJSON(t *testing.T) {
	p := AutoParser{}
	got := p.Parse(`{"a":1}`)
	if got.Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got.Format)
	}
}

func TestAutoParser_DetectsLogfmt(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("a=1 b=2 c=3")
	if got.Format != "logfmt" {
		t.Errorf("Format = %q, want \"logfmt\"", got.Format)
	}
	if got.Attr("a") != "1" || got.Attr("b") != "2" {
		t.Errorf("unexpected attributes: %v", got.Attributes)
	}
}

func TestAutoParser_PlainTextPassthrough(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("2026-04-21 INFO server started")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for plain text", got.Format)
	}
}

func TestAutoParser_EmptyPassthrough(t *testing.T) {
	p := AutoParser{}
	if got := p.Parse(""); got.Format != "" {
		t.Errorf("Format = %q, want \"\" for empty", got.Format)
	}
	if got := p.Parse("   "); got.Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace", got.Format)
	}
}

func TestAutoParser_LogfmtLookingProsePassthrough(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("user=bob logged in")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for prose-after-pair", got.Format)
	}
}

func TestAutoParser_ImplementsParser(t *testing.T) {
	var _ Parser = AutoParser{}
}
