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
	if got[0].Extra.Len() != 0 {
		t.Errorf("Extra.Len() = %d, want 0 for top-level array", got[0].Extra.Len())
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
	if m.Level != "INFO " {
		t.Errorf("Level = %q, want \"INFO \"", m.Level)
	}
	if m.Message != "req done" {
		t.Errorf("Message = %q, want \"req done\"", m.Message)
	}
	if m.Extra.Get("user") != "bob" {
		t.Errorf("Extra[user] = %q, want \"bob\"", m.Extra.Get("user"))
	}
	if m.Extra.Get("latency_ms") != "12" {
		t.Errorf("Extra[latency_ms] = %q, want \"12\"", m.Extra.Get("latency_ms"))
	}
	if m.Extra.Len() != 2 {
		t.Errorf("Extra.Len() = %d, want 2", m.Extra.Len())
	}
}

func TestJSONParser_FieldAliases(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"@timestamp":"2026-04-21T14:02:31Z","severity":"warning","message":"down"}`)
	m := got[0]
	if m.Time == "" {
		t.Error("@timestamp should populate Time")
	}
	if m.Level != "WARN " {
		t.Errorf("Level = %q, want \"WARN \"", m.Level)
	}
	if m.Message != "down" {
		t.Errorf("Message = %q, want \"down\"", m.Message)
	}
}

func TestJSONParser_NestedObjectInExtra(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"msg":"ok","meta":{"region":"us-east"}}`)
	m := got[0]
	if m.Message != "ok" {
		t.Errorf("Message = %q, want \"ok\"", m.Message)
	}
	if m.Extra.Get("meta") != `{"region":"us-east"}` {
		t.Errorf("Extra[meta] = %q, want JSON-encoded nested object", m.Extra.Get("meta"))
	}
}

func TestJSONParser_BoolAndNullScalars(t *testing.T) {
	p := JSONParser{}
	got := p.Parse(`{"a":true,"b":false,"c":null,"d":1}`)
	m := got[0]
	if m.Extra.Get("a") != "true" {
		t.Errorf("Extra[a] = %q, want \"true\"", m.Extra.Get("a"))
	}
	if m.Extra.Get("b") != "false" {
		t.Errorf("Extra[b] = %q, want \"false\"", m.Extra.Get("b"))
	}
	if m.Extra.Get("c") != "null" {
		t.Errorf("Extra[c] = %q, want \"null\"", m.Extra.Get("c"))
	}
	if m.Extra.Get("d") != "1" {
		t.Errorf("Extra[d] = %q, want \"1\"", m.Extra.Get("d"))
	}
}

func TestJSONParser_MalformedJSONKeepsFormat(t *testing.T) {
	p := JSONParser{}
	// Malformed but bracket-balanced: still gets format hint for highlighter.
	got := p.Parse(`{bad json}`)
	if got[0].Format != "json" {
		t.Errorf("Format = %q, want \"json\"", got[0].Format)
	}
	if got[0].Extra.Len() != 0 {
		t.Errorf("Extra.Len() = %d, want 0 for malformed JSON", got[0].Extra.Len())
	}
}

func TestJSONParser_UnbalancedBracketsNotJSON(t *testing.T) {
	p := JSONParser{}
	cases := []string{
		"[DEBUG] 12:34:56: hello world",
		"[INFO] starting server",
		"[ERROR] something went wrong",
		"{startup} config loaded",
	}
	for _, line := range cases {
		got := p.Parse(line)
		if got[0].Format != "" {
			t.Errorf("Parse(%q).Format = %q, want \"\" (unbalanced brackets should not get json format)", line, got[0].Format)
		}
	}
}
