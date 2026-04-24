package logmsg

import "testing"

func TestLogfmtParser_ThreePairs(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`ts=2026-04-21 level=info msg="hello world"`)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	m := got[0]
	if m.Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", m.Format)
	}
	if m.Time != "2026-04-21" {
		t.Errorf("Time = %q, want \"2026-04-21\"", m.Time)
	}
	if m.Level != "INFO " {
		t.Errorf("Level = %q, want \"INFO \"", m.Level)
	}
	if m.Message != "hello world" {
		t.Errorf("Message = %q, want \"hello world\"", m.Message)
	}
	if m.Extra.Len() != 0 {
		t.Errorf("Extra.Len() = %d, want 0 (all three canonical)", m.Extra.Len())
	}
}

func TestLogfmtParser_NonCanonicalGoToExtra(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("a=1 b=2 c=3")
	m := got[0]
	if m.Format != "logfmt" {
		t.Errorf("Format = %q, want \"logfmt\"", m.Format)
	}
	if m.Extra.Get("a") != "1" || m.Extra.Get("b") != "2" || m.Extra.Get("c") != "3" {
		t.Errorf("unexpected attrs: a=%q b=%q c=%q",
			m.Extra.Get("a"), m.Extra.Get("b"), m.Extra.Get("c"))
	}
	if m.Extra.Len() != 3 {
		t.Errorf("Extra.Len() = %d, want 3", m.Extra.Len())
	}
}

func TestLogfmtParser_SinglePairNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("level=error")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for single pair", got[0].Format)
	}
}

func TestLogfmtParser_QuotedValueUnescaped(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`a=1 key="with \"quotes\" inside"`)
	m := got[0]
	if m.Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", m.Format)
	}
	if m.Extra.Get("key") != `with "quotes" inside` {
		t.Errorf("Extra[key] = %q, want %q", m.Extra.Get("key"), `with "quotes" inside`)
	}
}

func TestLogfmtParser_ProseAfterPairNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("user=bob logged in doing stuff")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for prose after pair", got[0].Format)
	}
}

func TestLogfmtParser_PlainTextNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("2026-04-21 INFO server started")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for plain text", got[0].Format)
	}
}

func TestLogfmtParser_JSONNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`{"a":1}`)
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for JSON", got[0].Format)
	}
}

func TestLogfmtParser_EmptyNotDetected(t *testing.T) {
	p := LogfmtParser{}
	if got := p.Parse(""); got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for empty", got[0].Format)
	}
	if got := p.Parse("   "); got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace", got[0].Format)
	}
}

func TestLogfmtParser_AttrCanonicalLookup(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("level=error msg=down")
	if got[0].Attr("level") != "ERROR" {
		t.Errorf("Attr(level) = %q, want \"ERROR\"", got[0].Attr("level"))
	}
	if got[0].Attr("msg") != "down" {
		t.Errorf("Attr(msg) = %q, want \"down\"", got[0].Attr("msg"))
	}
}

func TestLogfmtParser_FieldAliases(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`ts=2026-04-21 severity=warning message="nope"`)
	m := got[0]
	if m.Time != "2026-04-21" {
		t.Errorf("Time = %q", m.Time)
	}
	if m.Level != "WARN " {
		t.Errorf("Level = %q, want \"WARN \"", m.Level)
	}
	if m.Message != "nope" {
		t.Errorf("Message = %q", m.Message)
	}
}

func TestLogfmtParser_Flush(t *testing.T) {
	p := LogfmtParser{}
	if got := p.Flush(); got != nil {
		t.Errorf("Flush = %v, want nil", got)
	}
}

func TestLogfmtParser_ImplementsParser(t *testing.T) {
	var _ Parser = LogfmtParser{}
}
