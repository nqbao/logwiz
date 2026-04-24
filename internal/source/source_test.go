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

func (stubParser) Parse(raw string) []logmsg.LogMessage {
	m := logmsg.LogMessage{Raw: raw}
	m.Extra.Set("tag", "stubbed")
	return []logmsg.LogMessage{m}
}

func (stubParser) Flush() []logmsg.LogMessage { return nil }

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
	// 2000 lines × 100 bytes each = ~200 KiB. Larger than the 64 KiB
	// sample, so estimation kicks in. With MaxLines=50 and ~100 B/line
	// the estimated budget is ~6 KiB → most of the file is skipped.
	const lineLen = 99
	const total = 2000
	var sb strings.Builder
	for i := 0; i < total; i++ {
		sb.WriteString(strings.Repeat("x", lineLen))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, MaxLines: 50}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	lines := collectRaws(rec.snapshot())
	if len(lines) == 0 {
		t.Fatalf("no lines emitted")
	}
	if !strings.HasPrefix(lines[0], "[logwiz: skipped ") {
		t.Errorf("first line = %q, want truncation marker", lines[0])
	}
	// Every surviving log line (after the marker) must be a full
	// line — we must not have split mid-byte when seeking.
	for i, l := range lines[1:] {
		if l != strings.Repeat("x", lineLen) {
			t.Errorf("line %d after marker = %q (len=%d), want %d 'x'",
				i+1, l, len(l), lineLen)
		}
	}
	retained := len(lines) - 1
	if retained == 0 || retained > total/4 {
		t.Errorf("retained %d lines, want 0 < n <= %d", retained, total/4)
	}
}

func TestFileNoTailSeekWhenSmall(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.log")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, MaxLines: 10000}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	got := collectRaws(rec.snapshot())
	if !equalStrings(got, []string{"a", "b", "c"}) {
		t.Fatalf("lines = %v, want [a b c] (no truncation marker expected)", got)
	}
}

func TestFileTailSeekDisabledWhenMaxLinesZero(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "any.log")
	// Deliberately larger than sampleSize so the only thing stopping
	// tail-seek is MaxLines=0.
	var sb strings.Builder
	for i := 0; i < 1000; i++ {
		sb.WriteString(strings.Repeat("y", 99))
		sb.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := newRecorder()
	src := &File{Path: path, MaxLines: 0}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	lines := collectRaws(rec.snapshot())
	if len(lines) != 1000 {
		t.Fatalf("got %d lines, want 1000 (no seek expected with MaxLines=0)", len(lines))
	}
	if strings.HasPrefix(lines[0], "[logwiz:") {
		t.Errorf("got truncation marker with MaxLines=0: %q", lines[0])
	}
}

func TestFileTailSeekScalesWithLineSize(t *testing.T) {
	// Files with bigger lines should produce bigger byte budgets so
	// the user still gets roughly MaxLines lines back. This is the
	// whole point of sample-and-estimate over a fixed byte cap.
	dir := t.TempDir()
	write := func(name string, lineLen, count int) string {
		path := filepath.Join(dir, name)
		var sb strings.Builder
		for i := 0; i < count; i++ {
			sb.WriteString(strings.Repeat("z", lineLen))
			sb.WriteString("\n")
		}
		if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	run := func(path string) int {
		rec := newRecorder()
		src := &File{Path: path, MaxLines: 100}
		src.Start(context.Background(), rec)
		rec.waitForTerminal(t)
		lines := collectRaws(rec.snapshot())
		// Drop marker.
		if len(lines) > 0 && strings.HasPrefix(lines[0], "[logwiz:") {
			lines = lines[1:]
		}
		return len(lines)
	}

	// 2000 × ~100 B lines → ~200 KiB file. With MaxLines=100 and
	// 1.25× headroom we expect ~125 lines retained (headroom can
	// round up a bit depending on integer division).
	shortRetained := run(write("short.log", 99, 2000))
	// 1000 × ~1 KiB lines → ~1 MiB file. Same MaxLines, budget should
	// scale to cover ~125 lines again.
	longRetained := run(write("long.log", 1023, 1000))

	for _, n := range []int{shortRetained, longRetained} {
		if n < 80 || n > 200 {
			t.Errorf("retained %d, want in [80, 200] for MaxLines=100 with 1.25× headroom", n)
		}
	}
}

func TestPumpFlushesParserOnEOF(t *testing.T) {
	var buf []string
	p := &eofFlushParser{parse: func(raw string) []logmsg.LogMessage {
		buf = append(buf, raw)
		return nil
	}, flush: func() []logmsg.LogMessage {
		var out []logmsg.LogMessage
		for _, s := range buf {
			m := logmsg.LogMessage{Raw: s}
			m.Extra.Set("tag", "flushed")
			out = append(out, m)
		}
		buf = buf[:0]
		return out
	}}

	rec := newRecorder()
	src := &Stdin{R: strings.NewReader("a\nb\n"), Parser: p}
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
		if lm.Attr("tag") != "flushed" {
			t.Errorf("msg %d: Attr(tag) = %q, want flushed", i, lm.Attr("tag"))
		}
	}
}

func TestPumpFlushesParserOnIdle(t *testing.T) {
	gotFlush := make(chan struct{}, 1)
	p := &idleFlushParser{
		parse: func(raw string) []logmsg.LogMessage {
			return nil
		},
		flush: func() []logmsg.LogMessage {
			select {
			case gotFlush <- struct{}{}:
			default:
			}
			return []logmsg.LogMessage{{Raw: "from-flush"}}
		},
	}

	pr, pw := io.Pipe()
	rec := newRecorder()
	src := &Stdin{R: pr, Parser: p}
	src.Start(context.Background(), rec)

	_, _ = pw.Write([]byte("a\n"))

	select {
	case <-gotFlush:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("Flush was not called within 500ms after last line")
	}
	_ = pw.Close()

	rec.waitForTerminal(t)
	msgs := rec.snapshot()
	raws := collectRaws(msgs)
	found := false
	for _, r := range raws {
		if r == "from-flush" {
			found = true
		}
	}
	if !found {
		t.Fatalf("did not observe 'from-flush' line; got %v", raws)
	}
}

type eofFlushParser struct {
	parse func(raw string) []logmsg.LogMessage
	flush func() []logmsg.LogMessage
}

func (p *eofFlushParser) Parse(raw string) []logmsg.LogMessage { return p.parse(raw) }
func (p *eofFlushParser) Flush() []logmsg.LogMessage            { return p.flush() }

type idleFlushParser struct {
	parse func(raw string) []logmsg.LogMessage
	flush func() []logmsg.LogMessage
}

func (p *idleFlushParser) Parse(raw string) []logmsg.LogMessage { return p.parse(raw) }
func (p *idleFlushParser) Flush() []logmsg.LogMessage            { return p.flush() }

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
