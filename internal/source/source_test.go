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

func TestStdinEmitsLinesThenEOF(t *testing.T) {
	rec := newRecorder()
	src := &Stdin{R: strings.NewReader("one\ntwo\nthree\n")}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	want := []string{"one", "two", "three"}
	var lines []string
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			lines = append(lines, nl.Lines...)
		}
	}
	if !equalStrings(lines, want) {
		t.Fatalf("lines = %v, want %v", lines, want)
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
	var lines []string
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			lines = append(lines, nl.Lines...)
		}
	}
	if !equalStrings(lines, []string{"alpha", "beta"}) {
		t.Fatalf("lines = %v", lines)
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
	for _, m := range msgs {
		if nl, ok := m.(NewLinesMsg); ok {
			for _, line := range nl.Lines {
				if line == "hello" {
					found = true
				}
			}
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
