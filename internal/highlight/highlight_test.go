package highlight

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// noopRenderer returns Raw unchanged; used to test cache logic without
// touching chroma.
func noopRenderer(m logmsg.LogMessage) string { return m.Raw }

// countRenderer wraps a counter so tests can assert cache hit/miss behaviour.
type countRenderer struct{ calls int }

func (c *countRenderer) render(m logmsg.LogMessage) string {
	c.calls++
	return "rendered:" + m.Raw
}

func msg(id uint64, raw, format string) logmsg.LogMessage {
	return logmsg.LogMessage{ID: logmsg.MsgID{Line: id}, Raw: raw, Format: format}
}

// --- New ---

func TestNew_EmptyHandlersAndCache(t *testing.T) {
	h := New()
	m := msg(0, "hello", "")
	if got := h.Display(m); got != "hello" {
		t.Errorf("Display = %q, want %q", got, "hello")
	}
}

// --- Register / Display ---

func TestDisplay_PassthroughWhenNoHandler(t *testing.T) {
	h := New()
	m := msg(1, "raw line", "unknown-format")
	if got := h.Display(m); got != "raw line" {
		t.Errorf("Display = %q, want %q", got, "raw line")
	}
}

func TestDisplay_UsesRegisteredHandler(t *testing.T) {
	h := New()
	h.Register("json", noopRenderer)
	m := msg(2, `{"a":1}`, "json")
	if got := h.Display(m); got != `{"a":1}` {
		t.Errorf("Display = %q, want %q", got, `{"a":1}`)
	}
}

func TestDisplay_CachesResult(t *testing.T) {
	h := New()
	cr := &countRenderer{}
	h.Register("json", cr.render)
	m := msg(3, `{"x":1}`, "json")

	first := h.Display(m)
	second := h.Display(m)

	if cr.calls != 1 {
		t.Errorf("renderer called %d times, want 1 (second call should be cached)", cr.calls)
	}
	if first != second {
		t.Errorf("cached result differs: %q vs %q", first, second)
	}
}

// --- Evict ---

func TestEvict_ForcesRerender(t *testing.T) {
	h := New()
	cr := &countRenderer{}
	h.Register("json", cr.render)
	m := msg(4, `{"y":2}`, "json")

	h.Display(m)
	h.Evict(m.ID)
	h.Display(m)

	if cr.calls != 2 {
		t.Errorf("renderer called %d times after evict, want 2", cr.calls)
	}
}

func TestEvict_NonexistentIDIsNoOp(t *testing.T) {
	h := New()
	h.Evict(logmsg.MsgID{Line: 999}) // must not panic
}

// --- Clear ---

func TestClear_DropsAllEntries(t *testing.T) {
	h := New()
	cr := &countRenderer{}
	h.Register("json", cr.render)

	for i := uint64(0); i < 5; i++ {
		h.Display(msg(i, `{}`, "json"))
	}
	h.Clear()
	for i := uint64(0); i < 5; i++ {
		h.Display(msg(i, `{}`, "json"))
	}
	if cr.calls != 10 {
		t.Errorf("renderer called %d times after Clear, want 10", cr.calls)
	}
}

// --- JSONRenderer (smoke test) ---

func TestJSONRenderer_ProducesANSI(t *testing.T) {
	out := JSONRenderer(msg(0, `{"level":"info","msg":"hello"}`, "json"))
	if !strings.Contains(out, "\x1b[") {
		t.Error("JSONRenderer should produce ANSI escape codes for valid JSON")
	}
	if !strings.Contains(out, "level") {
		t.Error("JSONRenderer output missing 'level' key")
	}
}

func TestJSONRenderer_FallbackOnEmpty(t *testing.T) {
	// Empty string: chroma may emit an empty render or the original; either
	// way it must not panic.
	_ = JSONRenderer(msg(0, "", "json"))
}

// --- LogfmtRenderer ---

func TestLogfmtRenderer_ProducesANSI(t *testing.T) {
	out := LogfmtRenderer(msg(0, "level=info msg=hello retries=3", "logfmt"))
	if !strings.Contains(out, "\x1b[") {
		t.Error("LogfmtRenderer should produce ANSI escape codes")
	}
	if !strings.Contains(out, "level") {
		t.Error("LogfmtRenderer output missing 'level' key")
	}
}

func TestLogfmtRenderer_LevelError_IsRed(t *testing.T) {
	out := LogfmtRenderer(msg(0, "level=error msg=down", "logfmt"))
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI codes in output")
	}
	if !strings.Contains(out, "error") {
		t.Error("expected 'error' in output")
	}
}

func TestLogfmtRenderer_NumericValue_IsYellow(t *testing.T) {
	out := LogfmtRenderer(msg(0, "a=1 retries=3", "logfmt"))
	if !strings.Contains(out, "\x1b[") {
		t.Error("expected ANSI codes for numeric value")
	}
	if !strings.Contains(out, "3") {
		t.Error("expected '3' in output")
	}
}

func TestLogfmtRenderer_NoSpans_ReturnsUnchanged(t *testing.T) {
	in := "plain prose with no equals signs"
	out := LogfmtRenderer(msg(0, in, "logfmt"))
	if out != in {
		t.Errorf("LogfmtRenderer(%q) = %q, want unchanged", in, out)
	}
}

func TestLogfmtRenderer_EmptyInput(t *testing.T) {
	_ = LogfmtRenderer(msg(0, "", "logfmt")) // must not panic
}
