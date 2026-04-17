package tools

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/logmsg"
)

func makeBuffer(lines ...string) *buffer.Buffer {
	b := buffer.New(len(lines) + 1)
	for _, l := range lines {
		b.Append(logmsg.LogMessage{Raw: l})
	}
	return b
}

func TestReadBasic(t *testing.T) {
	buf := makeBuffer("a", "b", "c", "d", "e")
	args := json.RawMessage(`{"start":1,"n":3}`)
	result, err := readTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "total=5") {
		t.Errorf("missing total header: %q", result)
	}
	if !strings.Contains(result, "line 1: b") {
		t.Errorf("missing line 1: %q", result)
	}
	if !strings.Contains(result, "line 3: d") {
		t.Errorf("missing line 3: %q", result)
	}
	if strings.Contains(result, "line 4") {
		t.Errorf("unexpected line 4: %q", result)
	}
}

func TestReadClampsN(t *testing.T) {
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = "x"
	}
	buf := makeBuffer(lines...)
	args := json.RawMessage(`{"start":0,"n":600}`)
	result, err := readTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(result, "line ")
	if count > MaxToolLines {
		t.Errorf("returned %d lines, want <= %d", count, MaxToolLines)
	}
}

func TestReadOutOfRange(t *testing.T) {
	buf := makeBuffer("a", "b")
	args := json.RawMessage(`{"start":10,"n":5}`)
	result, err := readTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "no lines") {
		t.Errorf("expected 'no lines' message: %q", result)
	}
}

func TestReadBadArgs(t *testing.T) {
	buf := makeBuffer("a")
	_, err := readTool{}.Execute(json.RawMessage(`{not json}`), buf)
	if err == nil {
		t.Fatal("expected error for bad JSON args")
	}
}

func TestTailBasic(t *testing.T) {
	buf := makeBuffer("a", "b", "c", "d", "e")
	args := json.RawMessage(`{"n":3}`)
	result, err := tailTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "total=5") {
		t.Errorf("missing total: %q", result)
	}
	if !strings.Contains(result, "line 2: c") {
		t.Errorf("missing line 2: %q", result)
	}
	if !strings.Contains(result, "line 4: e") {
		t.Errorf("missing line 4: %q", result)
	}
	if strings.Contains(result, "line 1:") {
		t.Errorf("unexpected line 1: %q", result)
	}
}

func TestTailClampsN(t *testing.T) {
	lines := make([]string, 600)
	for i := range lines {
		lines[i] = "x"
	}
	buf := makeBuffer(lines...)
	args := json.RawMessage(`{"n":600}`)
	result, err := tailTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(result, "line ")
	if count > MaxToolLines {
		t.Errorf("returned %d lines, want <= %d", count, MaxToolLines)
	}
}

func TestTailEmptyBuffer(t *testing.T) {
	buf := buffer.New(10)
	args := json.RawMessage(`{"n":5}`)
	result, err := tailTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "empty") {
		t.Errorf("expected empty message: %q", result)
	}
}

func TestSearchBasic(t *testing.T) {
	buf := makeBuffer("foo bar", "baz qux", "foo baz", "nope")
	args := json.RawMessage(`{"pattern":"foo"}`)
	result, err := searchTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "total_matches=2") {
		t.Errorf("expected total_matches=2: %q", result)
	}
	if !strings.Contains(result, "line 0: foo bar") {
		t.Errorf("missing line 0: %q", result)
	}
	if !strings.Contains(result, "line 2: foo baz") {
		t.Errorf("missing line 2: %q", result)
	}
}

func TestSearchCaseInsensitive(t *testing.T) {
	buf := makeBuffer("ERROR: something", "info: ok", "Error: again")
	args := json.RawMessage(`{"pattern":"error","case_insensitive":true}`)
	result, err := searchTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "total_matches=2") {
		t.Errorf("expected total_matches=2: %q", result)
	}
}

func TestSearchPagination(t *testing.T) {
	buf := makeBuffer("hit0", "miss", "hit1", "hit2", "miss", "hit3")
	// First page: max=2
	args := json.RawMessage(`{"pattern":"hit","max":2}`)
	result, err := searchTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, "total_matches=4") {
		t.Errorf("expected total_matches=4: %q", result)
	}
	if !strings.Contains(result, "returned=2") {
		t.Errorf("expected returned=2: %q", result)
	}
	// next_since_line should be 3 (index after hit1 at index 2).
	if !strings.Contains(result, "next_since_line=3") {
		t.Errorf("expected next_since_line=3: %q", result)
	}
}

func TestSearchBadRegex(t *testing.T) {
	buf := makeBuffer("a")
	args := json.RawMessage(`{"pattern":"[invalid"}`)
	_, err := searchTool{}.Execute(args, buf)
	if err == nil {
		t.Fatal("expected error for bad regex")
	}
}

func TestSearchBadArgs(t *testing.T) {
	buf := makeBuffer("a")
	_, err := searchTool{}.Execute(json.RawMessage(`{bad}`), buf)
	if err == nil {
		t.Fatal("expected error for bad JSON args")
	}
}

func TestCountBasic(t *testing.T) {
	buf := makeBuffer("ERROR: a", "INFO: ok", "ERROR: b", "WARN: c")
	args := json.RawMessage(`{"pattern":"ERROR"}`)
	result, err := countTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if result != "total_matches=2\n" {
		t.Errorf("count result = %q, want total_matches=2", result)
	}
}

func TestCountCaseInsensitive(t *testing.T) {
	buf := makeBuffer("error: x", "Error: y", "INFO: z")
	args := json.RawMessage(`{"pattern":"error","case_insensitive":true}`)
	result, err := countTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if result != "total_matches=2\n" {
		t.Errorf("count result = %q, want total_matches=2", result)
	}
}

func TestCountSinceLine(t *testing.T) {
	buf := makeBuffer("hit0", "hit1", "hit2")
	args := json.RawMessage(`{"pattern":"hit","since_line":1}`)
	result, err := countTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if result != "total_matches=2\n" {
		t.Errorf("count result = %q, want total_matches=2", result)
	}
}

func TestCountBadRegex(t *testing.T) {
	buf := makeBuffer("a")
	_, err := countTool{}.Execute(json.RawMessage(`{"pattern":"[bad"}`), buf)
	if err == nil {
		t.Fatal("expected error for bad regex")
	}
}

func TestCountEmptyBuffer(t *testing.T) {
	buf := buffer.New(5)
	args := json.RawMessage(`{"pattern":"x"}`)
	result, err := countTool{}.Execute(args, buf)
	if err != nil {
		t.Fatal(err)
	}
	if result != "total_matches=0\n" {
		t.Errorf("count result = %q, want total_matches=0", result)
	}
}

func TestRegistrySpecs(t *testing.T) {
	r := NewDefault()
	specs := r.Specs()
	if len(specs) != 4 {
		t.Fatalf("specs len = %d, want 4", len(specs))
	}
	names := map[string]bool{}
	for _, s := range specs {
		names[s.Name] = true
	}
	for _, want := range []string{"read", "tail", "search_regex", "count_regex"} {
		if !names[want] {
			t.Errorf("missing tool %q in specs", want)
		}
	}
}

func TestRegistryRunUnknown(t *testing.T) {
	r := NewDefault()
	result := r.Run("nonexistent", nil, buffer.New(1))
	if !strings.Contains(result, "error") {
		t.Errorf("expected error for unknown tool: %q", result)
	}
}

func TestRegistryRunRead(t *testing.T) {
	r := NewDefault()
	buf := makeBuffer("hello", "world")
	result := r.Run("read", json.RawMessage(`{"start":0,"n":2}`), buf)
	if !strings.Contains(result, "hello") {
		t.Errorf("expected 'hello' in result: %q", result)
	}
}
