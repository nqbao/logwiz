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

// waitFor polls rec until cond is true or the deadline passes.
func waitForMsgs(t *testing.T, rec *recordingSender, what string, cond func([]any) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if cond(rec.snapshot()) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s; msgs = %#v", what, rec.snapshot())
}

func countRaw(msgs []any, raw string) int {
	n := 0
	for _, r := range collectRaws(msgs) {
		if r == raw {
			n++
		}
	}
	return n
}

func TestCommandRestartStopsAndRespawns(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "interrupted.txt")
	t.Setenv("MARKER", marker)
	script := "trap 'echo x >> \"$MARKER\"; exit 0' INT; echo started; sleep 30 & wait"

	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &Command{Args: []string{"sh", "-c", script}}
	src.Start(ctx, rec)

	waitForMsgs(t, rec, "first start", func(m []any) bool { return countRaw(m, "started") == 1 })
	src.Restart()
	waitForMsgs(t, rec, "second start", func(m []any) bool { return countRaw(m, "started") == 2 })

	msgs := rec.snapshot()
	var sawRestarting bool
	for _, m := range msgs {
		switch m.(type) {
		case RestartingMsg:
			sawRestarting = true
		case EOFMsg, ErrMsg:
			t.Fatalf("unexpected terminal msg %T during restart", m)
		}
	}
	if !sawRestarting {
		t.Fatal("no RestartingMsg sent")
	}
	if n := countRaw(msgs, "[logwiz: restarting: sh -c "+script+"]"); n != 1 {
		t.Fatalf("restart notice count = %d, want 1", n)
	}
	if data, _ := os.ReadFile(marker); strings.TrimSpace(string(data)) != "x" {
		t.Fatalf("old process was not interrupted; marker = %q", data)
	}

	// Shutting down interrupts the replacement too.
	cancel()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if data, _ := os.ReadFile(marker); strings.Count(string(data), "x") == 2 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("replacement process was not interrupted on cancel")
}

func TestCommandRestartAfterExit(t *testing.T) {
	rec := newRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	src := &Command{Args: []string{"sh", "-c", "echo hi"}}
	src.Start(ctx, rec)
	rec.waitForTerminal(t)

	src.Restart()
	waitForMsgs(t, rec, "second run", func(m []any) bool { return countRaw(m, "hi") == 2 })
}
