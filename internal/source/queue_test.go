package source

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// gateSender blocks every Send until the gate is opened, then records.
type gateSender struct {
	gate chan struct{}
	mu   sync.Mutex
	got  []any
}

func (g *gateSender) Send(msg any) {
	<-g.gate
	g.mu.Lock()
	g.got = append(g.got, msg)
	g.mu.Unlock()
}

func (g *gateSender) received() []any {
	g.mu.Lock()
	defer g.mu.Unlock()
	return append([]any(nil), g.got...)
}

func lines(raws ...string) NewLinesMsg {
	out := make([]logmsg.LogMessage, len(raws))
	for i, r := range raws {
		out[i] = logmsg.LogMessage{Raw: r}
	}
	return NewLinesMsg{Lines: out}
}

func waitFor(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatal("timed out")
		}
		time.Sleep(time.Millisecond)
	}
}

func rawsOf(msgs []any) []string {
	var out []string
	for _, m := range msgs {
		switch v := m.(type) {
		case NewLinesMsg:
			for _, l := range v.Lines {
				out = append(out, l.Raw)
			}
		case EOFMsg:
			out = append(out, "<eof>")
		}
	}
	return out
}

func TestQueueSendDoesNotBlock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := &gateSender{gate: make(chan struct{})}
	q := NewQueue(ctx, dst, 0)

	done := make(chan struct{})
	go func() {
		for i := 0; i < 1000; i++ {
			q.Send(lines("x"))
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Send blocked while destination was blocked")
	}
	close(dst.gate)
	waitFor(t, func() bool { return len(rawsOf(dst.received())) == 1000 })
}

func TestQueueDropsOldestLinesKeepsOrderAndControl(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	dst := &gateSender{gate: make(chan struct{})}
	q := NewQueue(ctx, dst, 3)

	// The first message is taken by the forwarder and blocks in Send, so
	// later ones accumulate in the queue.
	q.Send(lines("first"))
	waitFor(t, func() bool {
		q.mu.Lock()
		defer q.mu.Unlock()
		return len(q.items) == 0
	})
	q.Send(lines("a", "b"))
	q.Send(lines("c", "d"))
	q.Send(EOFMsg{})
	q.Send(lines("e"))
	close(dst.gate)

	want := []string{"first", "c", "d", "<eof>", "e"}
	waitFor(t, func() bool { return len(rawsOf(dst.received())) == len(want) })
	got := rawsOf(dst.received())
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
	// "c","d" were merged into a single batch on forward.
	if n := len(dst.received()); n != 4 {
		t.Errorf("forwarded %d messages, want 4", n)
	}
}
