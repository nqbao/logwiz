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
)

// Sender is the subset of *tea.Program used by sources to push messages.
// Declaring it as an interface keeps sources trivially testable without
// pulling in a full Bubble Tea runtime.
type Sender interface {
	Send(msg any)
}

// NewLineMsg carries a single log line read from the source.
type NewLineMsg struct{ Line string }

// EOFMsg indicates the source has finished (file EOF, or stdin closed).
type EOFMsg struct{}

// ErrMsg carries a read error from the source.
type ErrMsg struct{ Err error }

// Source is a reader that can be started and stopped. Start is
// non-blocking: it spawns a goroutine that pushes messages to sender
// until ctx is cancelled or the underlying reader ends.
type Source interface {
	Start(ctx context.Context, sender Sender)
}

// pump reads lines from r, pushing NewLineMsg for each line and a
// final EOFMsg / ErrMsg when the stream ends or ctx is cancelled.
//
// Callers that wrap os.Stdin or a file should pass the reader directly;
// pump does its own buffering.
func pump(ctx context.Context, r io.Reader, sender Sender) {
	scanner := bufio.NewScanner(r)
	// Allow long lines (up to 1 MiB). Default bufio.Scanner buffer
	// maxes out at 64 KiB which is easy to blow past in real logs.
	const maxLine = 1 << 20
	scanner.Buffer(make([]byte, 64*1024), maxLine)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return
		}
		sender.Send(NewLineMsg{Line: lastCRSegment(scanner.Text())})
	}

	if ctx.Err() != nil {
		return
	}
	if err := scanner.Err(); err != nil {
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
