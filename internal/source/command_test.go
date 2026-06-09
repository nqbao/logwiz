package source

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandEmitsStdoutAndStderrThenEOF(t *testing.T) {
	rec := newRecorder()
	src := &Command{Args: []string{"sh", "-c", "printf 'one\\n'; printf 'two\\n' >&2"}}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	if got := collectRaws(msgs); !equalStrings(got, []string{"one", "two"}) {
		t.Fatalf("lines = %v, want %v", got, []string{"one", "two"})
	}
	if _, ok := msgs[len(msgs)-1].(EOFMsg); !ok {
		t.Fatalf("last msg = %T, want EOFMsg", msgs[len(msgs)-1])
	}
}

func TestCommandStartErrorEmitsErr(t *testing.T) {
	rec := newRecorder()
	src := &Command{Args: []string{"/definitely/not/a/real/command"}}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	if len(msgs) == 0 {
		t.Fatal("no messages")
	}
	if _, ok := msgs[0].(ErrMsg); !ok {
		t.Fatalf("first msg = %T, want ErrMsg", msgs[0])
	}
}

func TestCommandGetsClosedStdin(t *testing.T) {
	rec := newRecorder()
	src := &Command{Args: []string{"sh", "-c", "if IFS= read -r line; then echo got:$line; else echo eof; fi"}}
	src.Start(context.Background(), rec)
	rec.waitForTerminal(t)

	msgs := rec.snapshot()
	if got := collectRaws(msgs); !equalStrings(got, []string{"eof"}) {
		t.Fatalf("lines = %v, want %v", got, []string{"eof"})
	}
}

func TestCommandCancelInterruptsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "interrupted.txt")
	script := "trap 'echo interrupted > \"$MARKER\"; exit 0' INT; sleep 30 & wait"

	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	t.Setenv("MARKER", marker)
	src := &Command{Args: []string{"sh", "-c", script}}
	src.Start(ctx, rec)

	time.Sleep(200 * time.Millisecond)
	cancel()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(marker)
		if err == nil && strings.TrimSpace(string(data)) == "interrupted" {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("command did not observe SIGINT; marker %q was not written", marker)
}
