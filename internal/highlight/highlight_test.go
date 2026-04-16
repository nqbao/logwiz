package highlight

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// noopRenderer returns the input unchanged; used to test cache logic without
// touching chroma.
func noopRenderer(text string) string { return text }

// countRenderer wraps a counter so tests can assert cache hit/miss behaviour.
type countRenderer struct{ calls int }

func (c *countRenderer) render(text string) string {
	c.calls++
	return "rendered:" + text
}

func msg(id uint64, raw, format string) logmsg.LogMessage {
	return logmsg.LogMessage{ID: logmsg.MsgID{Line: id}, Raw: raw, Format: format}
}

// --- New ---

func TestNew_EmptyHandlersAndCache(t *testing.T) {
	h := New()
	// Display on a plain message should return Raw.
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
	out := JSONRenderer(`{"level":"info","msg":"hello"}`)
	if !strings.Contains(out, "\x1b[") {
		t.Error("JSONRenderer should produce ANSI escape codes for valid JSON")
	}
	// The raw key/value text should still appear somewhere in the output.
	if !strings.Contains(out, "level") {
		t.Error("JSONRenderer output missing 'level' key")
	}
}

func TestJSONRenderer_FallbackOnEmpty(t *testing.T) {
	// Empty string: chroma may emit an empty render or the original; either
	// way it must not panic.
	_ = JSONRenderer("")
}
