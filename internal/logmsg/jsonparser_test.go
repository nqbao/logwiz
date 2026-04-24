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
	if got[0].Attrs.Len() != 0 {
		t.Errorf("Attrs.Len() = %d, want 0 for top-level array", got[0].Attrs.Len())
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
	var _ Parser = JSONParser{}
}

func TestJSONParser_CanonicalFields(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"ts":"2026-04-21T14:02:31Z","level":"info","msg":"req done","user":"bob","latency_ms":12}`)
	m := got[0]
	if m.Time != "2026-04-21T14:02:31Z" {
		t.Errorf("Time = %q, want RFC3339 raw", m.Time)
	}
	if m.Level != "info" {
		t.Errorf("Level = %q, want raw \"info\"", m.Level)
	}
	if m.Message != "req done" {
		t.Errorf("Message = %q, want \"req done\"", m.Message)
	}
	if m.Attrs.Get("user") != "bob" {
		t.Errorf("Attrs[user] = %q, want \"bob\"", m.Attrs.Get("user"))
	}
	if m.Attrs.Get("latency_ms") != "12" {
		t.Errorf("Attrs[latency_ms] = %q, want \"12\"", m.Attrs.Get("latency_ms"))
	}
	if m.Attrs.Len() != 2 {
		t.Errorf("Attrs.Len() = %d, want 2", m.Attrs.Len())
	}
}

func TestJSONParser_FieldAliases(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"@timestamp":"2026-04-21T14:02:31Z","severity":"warning","message":"down"}`)
	m := got[0]
	if m.Time == "" {
		t.Error("@timestamp should populate Time")
	}
	if m.Level != "warning" {
		t.Errorf("Level = %q, want \"warning\"", m.Level)
	}
	if m.Message != "down" {
		t.Errorf("Message = %q, want \"down\"", m.Message)
	}
}

func TestJSONParser_NestedObjectInAttrs(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"msg":"ok","meta":{"region":"us-east"}}`)
	m := got[0]
	if m.Message != "ok" {
		t.Errorf("Message = %q, want \"ok\"", m.Message)
	}
	if m.Attrs.Get("meta") != `{"region":"us-east"}` {
		t.Errorf("Attrs[meta] = %q, want JSON-encoded nested object", m.Attrs.Get("meta"))
	}
}

func TestJSONParser_BoolAndNullScalars(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"a":true,"b":false,"c":null,"d":1}`)
	m := got[0]
	if m.Attrs.Get("a") != "true" {
		t.Errorf("Attrs[a] = %q, want \"true\"", m.Attrs.Get("a"))
	}
	if m.Attrs.Get("b") != "false" {
		t.Errorf("Attrs[b] = %q, want \"false\"", m.Attrs.Get("b"))
	}
	if m.Attrs.Get("c") != "null" {
		t.Errorf("Attrs[c] = %q, want \"null\"", m.Attrs.Get("c"))
	}
	if m.Attrs.Get("d") != "1" {
		t.Errorf("Attrs[d] = %q, want \"1\"", m.Attrs.Get("d"))
	}
}

func TestJSONParser_MalformedJSONKeepsFormat(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{bad json`)
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
	if got[0].Attrs.Len() != 0 {
		t.Errorf("Attrs.Len() = %d, want 0 for malformed JSON", got[0].Attrs.Len())
	}
}
