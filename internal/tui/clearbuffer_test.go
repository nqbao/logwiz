package tui

import (
	"regexp"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/config"
	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/source"
	"github.com/nqbao/logwiz/internal/tools"
)

// newFileModel creates a test model in file (non-streaming) mode.
func newFileModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	cfg := config.Config{MaxLines: 100, ContextLines: 10, SidebarWidth: 40}
	m := New(cfg, SourceFile, buffer.New(cfg.MaxLines), nil, tools.NewDefault(nil), nil, nil)
	mi, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 30})
	return mi.(Model)
}

// sendLines appends lines to the model via NewLinesMsg.
func sendLines(m Model, raws ...string) Model {
	msgs := make([]logmsg.LogMessage, len(raws))
	for i, r := range raws {
		msgs[i] = logmsg.LogMessage{Raw: r}
	}
	m2, _ := m.Update(source.NewLinesMsg{Lines: msgs})
	return m2.(Model)
}

func TestClearBufferEmptiesBuffer(t *testing.T) {
	m := newTestModel(t)
	m = sendLines(m, "line1", "line2", "line3")
	if m.buf.Len() != 3 {
		t.Fatalf("pre-clear buf.Len = %d, want 3", m.buf.Len())
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	mm := m2.(Model)

	if mm.buf.Len() != 0 {
		t.Fatalf("post-clear buf.Len = %d, want 0", mm.buf.Len())
	}
	if mm.logs.effectiveLen() != 0 {
		t.Fatalf("viewport effectiveLen = %d, want 0", mm.logs.effectiveLen())
	}
}

func TestClearBufferClearsSearchState(t *testing.T) {
	m := newTestModel(t)
	m = sendLines(m, "error: foo", "info: bar", "error: baz")
	runSearch(&m, regexp.MustCompile("error"))
	if len(m.searchMatches) == 0 {
		t.Fatal("expected search matches before clear")
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	mm := m2.(Model)

	if mm.searchRe != nil {
		t.Error("searchRe should be nil after buffer clear")
	}
	if len(mm.searchMatches) != 0 {
		t.Errorf("searchMatches = %v, want empty", mm.searchMatches)
	}
}

// TestClearBufferWithFilterThenNewLines verifies that active filter rules are
// correctly re-applied to lines that arrive after a buffer clear — this exercises
// the updateFilter path on a freshly reset model.
func TestClearBufferWithFilterThenNewLines(t *testing.T) {
	m := newTestModel(t)
	m = sendLines(m, "error: a", "info: b", "error: c")
	addFilter(&m, regexp.MustCompile("error"), filterInclude)
	if !m.filterEnabled {
		t.Fatal("filter not enabled")
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	mm := m2.(Model)

	if mm.buf.Len() != 0 {
		t.Fatalf("buf not empty after clear: len=%d", mm.buf.Len())
	}
	// Filter rules must survive the clear.
	if len(mm.filterRules) == 0 {
		t.Fatal("filterRules should be preserved after buffer clear")
	}

	// New lines arriving after the clear must not panic and must be filtered.
	mm = sendLines(mm, "error: new", "info: skip", "error: also")
	if mm.buf.Len() != 3 {
		t.Fatalf("buf.Len = %d, want 3", mm.buf.Len())
	}
	if len(mm.filterIndices) != 2 {
		t.Errorf("filterIndices = %v, want 2 matches", mm.filterIndices)
	}
}

// TestClearBufferWithHighlightThenNewLines verifies that active highlight rules
// survive the clear and are re-applied to new lines without panicking.
func TestClearBufferWithHighlightThenNewLines(t *testing.T) {
	m := newTestModel(t)
	m = sendLines(m, "warn: a", "error: b")
	addHL(&m, regexp.MustCompile("error"))
	if len(m.hlRules) == 0 {
		t.Fatal("no hl rules after addHL")
	}

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	mm := m2.(Model)

	// Rules must survive; per-rule match slices must be re-allocated (not nil).
	if len(mm.hlRules) == 0 {
		t.Fatal("hlRules should be preserved after buffer clear")
	}
	if mm.hlMatches == nil || len(mm.hlMatches) != len(mm.hlRules) {
		t.Fatalf("hlMatches has wrong shape: %v (want len %d)", mm.hlMatches, len(mm.hlRules))
	}

	// New lines must not panic and must produce correct match indices.
	mm = sendLines(mm, "info: x", "error: y", "error: z")
	if len(mm.hlMatches) == 0 || len(mm.hlMatches[0]) != 2 {
		t.Errorf("hlMatches[0] = %v, want 2 matches", mm.hlMatches[0])
	}
}

func TestClearBufferNotAvailableInFileMode(t *testing.T) {
	m := newFileModel(t)
	m = sendLines(m, "line1", "line2")

	m2, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'C'}})
	mm := m2.(Model)

	// Buffer must be unchanged — C is a no-op for file sources.
	if mm.buf.Len() != 2 {
		t.Fatalf("file mode: buf.Len = %d after C, want 2 (no-op)", mm.buf.Len())
	}
}

func TestClearBufferCommandFileMode(t *testing.T) {
	m := newFileModel(t)
	m = sendLines(m, "line1")

	_, _, err := cmdClear(m, "buffer")
	if err == nil {
		t.Fatal(":clear buffer should return an error in file mode")
	}
}

func TestClearBufferCommandStreaming(t *testing.T) {
	m := newTestModel(t)
	m = sendLines(m, "a", "b", "c")

	m2, _, err := cmdClear(m, "buffer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m2.buf.Len() != 0 {
		t.Fatalf("buf.Len = %d, want 0", m2.buf.Len())
	}
}
