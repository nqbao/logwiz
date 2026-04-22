package logmsg

import "testing"

func TestLogfmtParser_ThreePairs(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`ts=2026-04-21 level=info msg="hello world"`)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", got[0].Format)
	}
	if len(got[0].Attributes) != 3 {
		t.Fatalf("len(Attributes) = %d, want 3", len(got[0].Attributes))
	}
	if got[0].Attr("ts") != "2026-04-21" {
		t.Errorf("ts = %q, want \"2026-04-21\"", got[0].Attr("ts"))
	}
	if got[0].Attr("level") != "info" {
		t.Errorf("level = %q, want \"info\"", got[0].Attr("level"))
	}
	if got[0].Attr("msg") != "hello world" {
		t.Errorf("msg = %q, want \"hello world\"", got[0].Attr("msg"))
	}
}

func TestLogfmtParser_MinimalTwoPairs(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("a=1 b=2 c=3")
	if got[0].Format != "logfmt" {
		t.Errorf("Format = %q, want \"logfmt\"", got[0].Format)
	}
	if got[0].Attr("a") != "1" || got[0].Attr("b") != "2" || got[0].Attr("c") != "3" {
		t.Errorf("unexpected attributes: %v", got[0].Attributes)
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
	if got[0].Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", got[0].Format)
	}
	if got[0].Attr("key") != `with "quotes" inside` {
		t.Errorf("key = %q, want %q", got[0].Attr("key"), `with "quotes" inside`)
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

func TestLogfmtParser_AttrRoundTrip(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("level=error msg=down")
	if got[0].Attr("level") != "error" {
		t.Errorf("Attr(level) = %q, want \"error\"", got[0].Attr("level"))
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
