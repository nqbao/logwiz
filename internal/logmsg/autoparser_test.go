package logmsg

import "testing"

func TestAutoParser_DetectsJSON(t *testing.T) {
	p := AutoParser{}
	got := p.Parse(`{"a":1}`)
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
}

func TestAutoParser_DetectsLogfmt(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("a=1 b=2 c=3")
	if got[0].Format != "logfmt" {
		t.Errorf("Format = %q, want \"logfmt\"", got[0].Format)
	}
	if got[0].Attr("a") != "1" || got[0].Attr("b") != "2" {
		t.Errorf("unexpected attributes: %v", got[0].Attributes)
	}
}

func TestAutoParser_PlainTextPassthrough(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("2026-04-21 INFO server started")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for plain text", got[0].Format)
	}
}

func TestAutoParser_EmptyPassthrough(t *testing.T) {
	p := AutoParser{}
	if got := p.Parse(""); got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for empty", got[0].Format)
	}
	if got := p.Parse("   "); got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace", got[0].Format)
	}
}

func TestAutoParser_LogfmtLookingProsePassthrough(t *testing.T) {
	p := AutoParser{}
	got := p.Parse("user=bob logged in")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for prose-after-pair", got[0].Format)
	}
}

func TestAutoParser_Flush(t *testing.T) {
	p := AutoParser{}
	if got := p.Flush(); got != nil {
		t.Errorf("Flush = %v, want nil", got)
	}
}

func TestAutoParser_ImplementsParser(t *testing.T) {
	var _ Parser = AutoParser{}
}
