package source

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

type recordingSender struct {
	mu   sync.Mutex
	msgs []any
	done chan struct{}
}

func newRecorder() *recordingSender {
	return &recordingSender{done: make(chan struct{})}
}

func (r *recordingSender) Send(msg any) {
	r.mu.Lock()
	r.msgs = append(r.msgs, msg)
	// Signal on terminal messages so tests don't poll.
	switch msg.(type) {
	case EOFMsg, ErrMsg:
		select {
		case <-r.done:
		default:
			close(r.done)
		}
	}
	r.mu.Unlock()
}

func (r *recordingSender) snapshot() []any {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]any, len(r.msgs))
	copy(out, r.msgs)
	return out
}

func (r *recordingSender) waitForTerminal(t *testing.T) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for terminal msg; got %v", r.snapshot())
	}
}

// collectRaws extracts the Raw field of every LogMessage emitted.
func collectRaws(msgs []any) []string {
	var lines []string
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			for _, lm := range nl.Lines {
				lines = append(lines, lm.Raw)
			}
		}
	}
	return lines
}

func TestStdinEmitsLinesThenEOF(t *testing.T) {
	rec := newRecorder()
	src := &Stdin{R: strings.NewReader("one\ntwo\nthree\n")}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	want := []string{"one", "two", "three"}
	if got := collectRaws(msgs); !equalStrings(got, want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	if _, ok := msgs[len(msgs)-1].(EOFMsg); !ok {
		t.Fatalf("last msg = %T, want EOFMsg", msgs[len(msgs)-1])
	}
}

func TestFileEmitsLinesThenEOF(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.log")
	if err := os.WriteFile(path, []byte("alpha\nbeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rec := newRecorder()
	src := &File{Path: path}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	if got := collectRaws(msgs); !equalStrings(got, []string{"alpha", "beta"}) {
		t.Fatalf("lines = %v", got)
	}
	if _, ok := msgs[len(msgs)-1].(EOFMsg); !ok {
		t.Fatalf("last msg = %T, want EOFMsg", msgs[len(msgs)-1])
	}
}

func TestFileMissingEmitsErr(t *testing.T) {
	rec := newRecorder()
	src := &File{Path: "/definitely/not/a/real/path-xyzzy"}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	if len(msgs) == 0 {
		t.Fatal("no messages")
	}
	errMsg, ok := msgs[0].(ErrMsg)
	if !ok {
		t.Fatalf("first msg = %T, want ErrMsg", msgs[0])
	}
	if errMsg.Err == nil {
		t.Fatal("ErrMsg.Err is nil")
	}
}

func TestStdinCancelStops(t *testing.T) {
	rec := newRecorder()
	// Pipe we never close; pump would block forever without cancel.
	pr, pw := io.Pipe()
	defer pw.Close()

	ctx, cancel := context.WithCancel(context.Background())
	src := &Stdin{R: pr}
	src.Start(ctx, rec)

	// Push a line, then cancel.
	go func() {
		_, _ = pw.Write([]byte("hello\n"))
		time.Sleep(20 * time.Millisecond)
		cancel()
		_ = pw.CloseWithError(errors.New("test cancel"))
	}()

	// Give the pump time to exit.
	time.Sleep(200 * time.Millisecond)

	// We don't require a terminal msg here — on cancel, pump exits
	// silently. Just confirm we captured the line.
	msgs := rec.snapshot()
	found := false
	for _, line := range collectRaws(msgs) {
		if line == "hello" {
			found = true
		}
	}
	if !found {
		t.Fatalf("did not observe 'hello' line; got %v", msgs)
	}
}

func TestPumpBatchesLines(t *testing.T) {
	const n = 500
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteString("line\n")
	}
	rec := newRecorder()
	src := &Stdin{R: strings.NewReader(sb.String())}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	var batches int
	var lines int
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			batches++
			lines += len(nl.Lines)
		}
	}
	if lines != n {
		t.Fatalf("total lines = %d, want %d", lines, n)
	}
	if batches >= n/2 {
		t.Fatalf("batches = %d, want < %d (batching not working)", batches, n/2)
	}
}

// stubParser tags every line with a fixed attribute so tests can confirm
// the parser is actually invoked by pump.
type stubParser struct{}

func (stubParser) Parse(raw string) logmsg.LogMessage {
	return logmsg.LogMessage{
		Raw:        raw,
		Attributes: map[string]string{"tag": "stubbed"},
	}
}

func TestPumpAppliesParser(t *testing.T) {
	rec := newRecorder()
	src := &Stdin{R: strings.NewReader("a\nb\n"), Parser: stubParser{}}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	var seen []logmsg.LogMessage
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			seen = append(seen, nl.Lines...)
		}
	}
	if len(seen) != 2 {
		t.Fatalf("got %d messages, want 2", len(seen))
	}
	for i, lm := range seen {
		if lm.Attr("tag") != "stubbed" {
			t.Errorf("msg %d: Attr(tag) = %q, want stubbed", i, lm.Attr("tag"))
		}
	}
}

func TestFileTailSeekSkipsHead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.log")
	// 200 lines × 100 bytes each = 20 KiB. With tailBytes=1 KiB, only
	// the last ~10 lines should survive.
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString(strings.Repeat("x", 99))
		sb.WriteString("\n")
	}
	full := sb.String()
	if err := os.WriteFile(path, []byte(full), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, TailBytes: 1024}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	lines := collectRaws(msgs)
	if len(lines) == 0 {
		t.Fatalf("no lines emitted")
	}
	// First emitted line must be the truncation marker.
	if !strings.HasPrefix(lines[0], "[logwiz: skipped ") {
		t.Errorf("first line = %q, want truncation marker", lines[0])
	}
	// Every surviving log line (after the marker) must be a full
	// 99-x line — we must not have split a line mid-byte.
	for i, l := range lines[1:] {
		if l != strings.Repeat("x", 99) {
			t.Errorf("line %d after marker = %q, want 99 'x'", i+1, l)
		}
	}
	// We seeked to ~1 KiB from EOF, so we should keep far fewer than
	// the original 200 lines but still something.
	retained := len(lines) - 1
	if retained == 0 || retained > 50 {
		t.Errorf("retained %d lines, want 1 < n <= 50", retained)
	}
}

func TestFileNoTailSeekWhenSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, TailBytes: 1 << 20}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	got := collectRaws(rec.snapshot())
	if !equalStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("lines = %v, want [a b c] (no truncation marker expected)", got)
	}
}

func TestFileTailSeekDisabledWhenZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "any.log")
	if err := os.WriteFile(path, []byte("one\ntwo\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, TailBytes: 0}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	got := collectRaws(rec.snapshot())
	if !equalStrings(got, []string{"one", "two"}) {
		t.Fatalf("lines = %v, want [one two]", got)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
