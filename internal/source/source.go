// Package source provides readers that pump lines from a file or from
// stdin into a Bubble Tea program as tea.Msg values.
//
// Readers run in their own goroutine and never touch TUI state
// directly; they push messages via a Sender (satisfied by
// *tea.Program.Send).
package source

import (
	"bufio"
	"context"
	"io"
	"strings"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// Sender is the subset of *tea.Program used by sources to push messages.
// Declaring it as an interface keeps sources trivially testable without
// pulling in a full Bubble Tea runtime.
type Sender interface {
	Send(msg any)
}

// NewLinesMsg carries one or more parsed log messages read from the source.
// Batched at the source to avoid O(n) work per line downstream.
type NewLinesMsg struct{ Lines []logmsg.LogMessage }

// EOFMsg indicates the source has finished (file EOF, or stdin closed).
type EOFMsg struct{}

// ErrMsg carries a read error from the source.
type ErrMsg struct{ Err error }

// FollowingMsg is sent by a File source once the initial read is done and
// the watcher loop has started. The TUI uses it to flip the "following" badge.
type FollowingMsg struct{}

// FileFollower is implemented by *File and allows the TUI to toggle follow
// at runtime without restarting the whole source.
type FileFollower interface {
	StopFollow()
	StartFollow()
}

// Source is a reader that can be started and stopped. Start is
// non-blocking: it spawns a goroutine that pushes messages to sender
// until ctx is cancelled or the underlying reader ends.
type Source interface {
	Start(ctx context.Context, sender Sender)
}

// pumpLines reads lines from r into sender as NewLinesMsg batches.
// Returns the scanner error (nil on clean EOF). Does NOT send EOFMsg/ErrMsg.
// Callers must check ctx.Err() after return to distinguish cancel from EOF.
//
// Lines are coalesced into batches; a batch is flushed at maxBatch lines or
// after flushDelay with no new lines. If parser is nil, PassthroughParser is used.
func pumpLines(ctx context.Context, r io.Reader, sender Sender, parser logmsg.Parser) error {
	if parser == nil {
		parser = logmsg.PassthroughParser{}
	}

	scanner := bufio.NewScanner(r)
	const maxLine = 1 << 20
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	const (
		maxBatch   = 100
		flushDelay = 10 * time.Millisecond
		idleDelay  = 200 * time.Millisecond
	)
	batch := make([]logmsg.LogMessage, 0, maxBatch)

	flush := func() {
		if len(batch) == 0 {
			return
		}
		out := make([]logmsg.LogMessage, len(batch))
		copy(out, batch)
		sender.Send(NewLinesMsg{Lines: out})
		batch = batch[:0]
	}

	lineCh := make(chan string)
	go func() {
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		close(lineCh)
	}()

	ticker := time.NewTicker(flushDelay)
	defer ticker.Stop()

	idleTimer := time.NewTimer(idleDelay)
	defer idleTimer.Stop()

	resetIdle := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleDelay)
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			flush()
		case <-idleTimer.C:
			batch = append(batch, parser.Flush()...)
			flush()
		case line, ok := <-lineCh:
			if !ok {
				batch = append(batch, parser.Flush()...)
				flush()
				return scanner.Err()
			}
			batch = append(batch, parser.Parse(lastCRSegment(line))...)
			resetIdle()
			if len(batch) >= maxBatch {
				flush()
			}
		}
	}
}

// pump calls pumpLines and sends a terminal EOFMsg or ErrMsg.
func pump(ctx context.Context, r io.Reader, sender Sender, parser logmsg.Parser) {
	err := pumpLines(ctx, r, sender, parser)
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		sender.Send(ErrMsg{Err: err})
		return
	}
	sender.Send(EOFMsg{})
}

// lastCRSegment mimics terminal carriage-return behaviour: when a line
// contains \r, each segment overwrites the previous one, so only the
// last non-empty segment is what the user would have seen on screen.
func lastCRSegment(s string) string {
	last := ""
	for _, seg := range strings.Split(s, "\r") {
		if seg != "" {
			last = seg
		}
	}
	return last
}

// wrap around a sender func so tests can use a plain function.
type senderFunc func(msg any)

func (f senderFunc) Send(msg any) { f(msg) }
