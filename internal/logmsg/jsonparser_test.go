package logmsg

import "testing"

func TestJSONParser_DetectsObject(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"level":"info","msg":"hello"}`)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
	if got[0].Raw != `{"level":"info","msg":"hello"}` {
		t.Errorf("Raw changed unexpectedly: %q", got[0].Raw)
	}
}

func TestJSONParser_DetectsArray(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`[1, 2, 3]`)
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
}

func TestJSONParser_DetectsWithLeadingWhitespace(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("  { \"a\": 1 }")
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\" for leading-whitespace JSON", got[0].Format)
	}
}

func TestJSONParser_PlainLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("2026-04-16 INFO server started")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for plain line", got[0].Format)
	}
}

func TestJSONParser_EmptyLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for empty line", got[0].Format)
	}
}

func TestJSONParser_WhitespaceOnlyLine(t *testing.T) {
	p := JSONParser{}
	got := p.Parse("   ")
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for whitespace-only line", got[0].Format)
	}
}

func TestJSONParser_Flush(t *testing.T) {
	p := JSONParser{}
	if got := p.Flush(); got != nil {
		t.Errorf("Flush = %v, want nil", got)
	}
}

func TestJSONParser_ImplementsParser(t *testing.T) {
	// Compile-time check that JSONParser satisfies Parser.
	var _ Parser = JSONParser{}
}
