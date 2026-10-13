package source

import (
	"context"
	"sync"
)

// Queue is a Sender that never blocks. Messages are buffered and forwarded
// to dst in order by a background goroutine, so a source keeps draining its
// reader even while dst is busy (e.g. the TUI is suspended for $EDITOR and
// tea.Program.Send blocks). Without this, an upstream process piping into
// logwiz would stall once the OS pipe buffer filled.
//
// At most maxLines log lines are held; beyond that the oldest queued lines
// are dropped, matching what the ring buffer would evict anyway. Control
// messages (EOFMsg, ErrMsg, …) are never dropped. Adjacent NewLinesMsg
// batches are merged when forwarded.
type Queue struct {
	mu       sync.Mutex
	items    []any
	lines    int
	maxLines int
	notify   chan struct{}
}

// NewQueue starts a Queue forwarding to dst until ctx is cancelled.
// maxLines <= 0 means unbounded.
func NewQueue(ctx context.Context, dst Sender, maxLines int) *Queue {
	q := &Queue{maxLines: maxLines, notify: make(chan struct{}, 1)}
	go q.run(ctx, dst)
	return q
}

// Send enqueues msg and returns immediately.
func (q *Queue) Send(msg any) {
	q.mu.Lock()
	q.items = append(q.items, msg)
	if nl, ok := msg.(NewLinesMsg); ok {
		q.lines += len(nl.Lines)
		q.trimLocked()
	}
	q.mu.Unlock()
	select {
	case q.notify <- struct{}{}:
	default:
	}
}

// trimLocked drops the oldest queued lines until at most maxLines remain.
func (q *Queue) trimLocked() {
	for i := 0; q.maxLines > 0 && q.lines > q.maxLines && i < len(q.items); {
		nl, ok := q.items[i].(NewLinesMsg)
		if !ok {
			i++
			continue
		}
		drop := min(q.lines-q.maxLines, len(nl.Lines))
		q.lines -= drop
		if drop == len(nl.Lines) {
			q.items = append(q.items[:i], q.items[i+1:]...)
			continue
		}
		q.items[i] = NewLinesMsg{Lines: nl.Lines[drop:]}
	}
}

// take removes and returns everything queued, merging adjacent line batches.
func (q *Queue) take() []any {
	q.mu.Lock()
	items := q.items
	q.items = nil
	q.lines = 0
	q.mu.Unlock()

	out := items[:0:0]
	for _, it := range items {
		if nl, ok := it.(NewLinesMsg); ok && len(out) > 0 {
			if prev, ok := out[len(out)-1].(NewLinesMsg); ok {
				out[len(out)-1] = NewLinesMsg{Lines: append(prev.Lines, nl.Lines...)}
				continue
			}
		}
		out = append(out, it)
	}
	return out
}

func (q *Queue) run(ctx context.Context, dst Sender) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-q.notify:
		}
		for _, msg := range q.take() {
			if ctx.Err() != nil {
				return
			}
			dst.Send(msg)
		}
	}
}
