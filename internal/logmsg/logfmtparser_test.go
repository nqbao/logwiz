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

func TestLogfmtParser_ANSIColouredProseNotDetected(t *testing.T) {
	// modd-style coloured prose with embedded `key=val` payloads must not be
	// claimed as logfmt — re-rendering through go-logfmt mangles the SGR
	// escapes and leaves literal `[90m` fragments on screen.
	p := LogfmtParser{}
	line := "\x1b[90m12:00:00\x1b[0m INFO request_id=abc123 \x1b[36mstatus=\x1b[0mok"
	got := p.Parse(line)
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for ANSI-coloured prose", got[0].Format)
	}
	if got[0].Raw != line {
		t.Errorf("Raw mutated: got %q", got[0].Raw)
	}
}

func TestLogfmtParser_ProseWithTrailingPairsNotDetected(t *testing.T) {
	// Plain prose that happens to contain ≥2 `key=val` payloads at the END of
	// the line decodes into bare-keyed prose tokens followed by real pairs.
	// Real logfmt records lead with key=val, so requiring the first two
	// scanned pairs to carry values rejects this case.
	p := LogfmtParser{}
	line := "12:00:00 INFO loaded request: user_id=42 status=ok"
	got := p.Parse(line)
	if got[0].Format != "" {
		t.Errorf("Format = %q, want \"\" for prose with trailing pairs", got[0].Format)
	}
	if got[0].Raw != line {
		t.Errorf("Raw mutated: got %q", got[0].Raw)
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

func TestLogfmtParser_BracketPrefix(t *testing.T) {
	p := LogfmtParser{}
	cases := []struct {
		raw      string
		isLogfmt bool
	}{
		{`[DEBUG] YY:MM:DD`, false},
		{`[DEBUG] YY:MM:DD level=debug msg=hello`, false},
		{`[DEBUG] ts=12:34:56 level=debug msg=hello`, false},
		{`[DEBUG]  2026/05/11 12:34:56 level=info msg=started`, false},
		{`[DEBUG]=YY:MM:DD level=debug msg=hello`, true},
		{`ts=2026-04-21 level=info msg="hello world"`, true},
		{`level=debug msg=hello [DEBUG] YY:MM:DD`, true},
		{`msg="hello world" ts=now level=info`, true},
		{`key==val key2=val2`, false},
	}
	for i, tc := range cases {
		got := p.Parse(tc.raw)
		isLogfmt := got[0].Format == "logfmt"
		if isLogfmt != tc.isLogfmt {
			t.Errorf("[%d] %q: Format=%q, want isLogfmt=%v", i, tc.raw, got[0].Format, tc.isLogfmt)
		}
	}
}
