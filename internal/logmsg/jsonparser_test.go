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
	if got[0].Attributes != nil {
		t.Errorf("Attributes = %v, want nil for top-level array", got[0].Attributes)
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

func TestJSONParser_AttributesFlatObject(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"level":"info","msg":"hello","n":42}`)
	attrs := got[0].Attributes
	if attrs == nil {
		t.Fatal("Attributes is nil, want populated map")
	}
	cases := map[string]string{
		"level": "info",
		"msg":   "hello",
		"n":     "42",
	}
	for k, want := range cases {
		if got := attrs[k]; got != want {
			t.Errorf("attrs[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestJSONParser_AttributesNestedObject(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"a":{"b":1},"c":"d"}`)
	attrs := got[0].Attributes
	if attrs == nil {
		t.Fatal("Attributes is nil")
	}
	if attrs["a"] != `{"b":1}` {
		t.Errorf("attrs[\"a\"] = %q, want %q", attrs["a"], `{"b":1}`)
	}
	if attrs["c"] != "d" {
		t.Errorf("attrs[\"c\"] = %q, want \"d\"", attrs["c"])
	}
}

func TestJSONParser_AttributesBoolAndNull(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"ok":true,"x":false,"y":null}`)
	attrs := got[0].Attributes
	if attrs == nil {
		t.Fatal("Attributes is nil")
	}
	if attrs["ok"] != "true" {
		t.Errorf("attrs[\"ok\"] = %q, want \"true\"", attrs["ok"])
	}
	if attrs["x"] != "false" {
		t.Errorf("attrs[\"x\"] = %q, want \"false\"", attrs["x"])
	}
	if attrs["y"] != "null" {
		t.Errorf("attrs[\"y\"] = %q, want \"null\"", attrs["y"])
	}
}

func TestJSONParser_AttributesNilForArray(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`[1,2,3]`)
	if got[0].Attributes != nil {
		t.Errorf("Attributes = %v, want nil for top-level array", got[0].Attributes)
	}
}

func TestJSONParser_AttributesNilForMalformed(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{bad json`)
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
	if got[0].Attributes != nil {
		t.Errorf("Attributes = %v, want nil for malformed JSON", got[0].Attributes)
	}
}
