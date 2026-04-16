package logmsg

import "testing"

func TestJSONParser_DetectsObject(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"level":"info","msg":"hello"}`)
	if got.Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got.Format)
	}
	if got.Raw != `{"level":"info","msg":"hello"}` {
		t.Errorf("Raw changed unexpectedly: %q", got.Raw)
	}
}

func TestJSONParser_DetectsArray(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`[1, 2, 3]`)
	if got.Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got.Format)
	}
}

func TestJSONParser_DetectsWithLeadingWhitespace(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("  { \"a\": 1 }")
	if got.Format != "json" {
		t.Errorf("Format = %q, want \"json\" for leading-whitespace JSON", got.Format)
	}
}

func TestJSONParser_PlainLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("2026-04-16 INFO server started")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for plain line", got.Format)
	}
}

func TestJSONParser_EmptyLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for empty line", got.Format)
	}
}

func TestJSONParser_WhitespaceOnlyLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("   ")
	if got.Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace-only line", got.Format)
	}
}

func TestJSONParser_ImplementsParser(t *testing.T) {
	// Compile-time check that JSONParser satisfies Parser.
	var _ Parser = JSONParser{}
}
