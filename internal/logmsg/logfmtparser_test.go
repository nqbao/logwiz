package logmsg

import "testing"

func TestLogfmtParser_ThreePairs(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`ts=2026-04-21 level=info msg="hello world"`)
	if got.Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", got.Format)
	}
	if len(got.Attributes) != 3 {
		t.Fatalf("len(Attributes) = %d, want 3", len(got.Attributes))
	}
	if got.Attr("ts") != "2026-04-21" {
		t.Errorf("ts = %q, want \"2026-04-21\"", got.Attr("ts"))
	}
	if got.Attr("level") != "info" {
		t.Errorf("level = %q, want \"info\"", got.Attr("level"))
	}
	if got.Attr("msg") != "hello world" {
		t.Errorf("msg = %q, want \"hello world\"", got.Attr("msg"))
	}
}

func TestLogfmtParser_MinimalTwoPairs(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("a=1 b=2 c=3")
	if got.Format != "logfmt" {
		t.Errorf("Format = %q, want \"logfmt\"", got.Format)
	}
	if got.Attr("a") != "1" || got.Attr("b") != "2" || got.Attr("c") != "3" {
		t.Errorf("unexpected attributes: %v", got.Attributes)
	}
}

func TestLogfmtParser_SinglePairNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("level=error")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for single pair", got.Format)
	}
}

func TestLogfmtParser_QuotedValueUnescaped(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`a=1 key="with \"quotes\" inside"`)
	if got.Format != "logfmt" {
		t.Fatalf("Format = %q, want \"logfmt\"", got.Format)
	}
	if got.Attr("key") != `with "quotes" inside` {
		t.Errorf("key = %q, want %q", got.Attr("key"), `with "quotes" inside`)
	}
}

func TestLogfmtParser_ProseAfterPairNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("user=bob logged in doing stuff")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for prose after pair", got.Format)
	}
}

func TestLogfmtParser_PlainTextNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("2026-04-21 INFO server started")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for plain text", got.Format)
	}
}

func TestLogfmtParser_JSONNotDetected(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse(`{"a":1}`)
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for JSON", got.Format)
	}
}

func TestLogfmtParser_EmptyNotDetected(t *testing.T) {
	p := LogfmtParser{}
	if got := p.Parse(""); got.Format != "" {
		t.Errorf("Format = %q, want \"\" for empty", got.Format)
	}
	if got := p.Parse("   "); got.Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace", got.Format)
	}
}

func TestLogfmtParser_AttrRoundTrip(t *testing.T) {
	p := LogfmtParser{}
	got := p.Parse("level=error msg=down")
	if got.Attr("level") != "error" {
		t.Errorf("Attr(level) = %q, want \"error\"", got.Attr("level"))
	}
}

func TestLogfmtParser_ImplementsParser(t *testing.T) {
	var _ Parser = LogfmtParser{}
}
