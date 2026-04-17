// Package buffer provides a fixed-size rolling line buffer used as the
// single source of truth for logs displayed by the TUI and sent to the
// chat model.
package buffer

import (
	"regexp"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// Buffer is a fixed-size ring of LogMessages. Appending past capacity
// overwrites the oldest entry. Not safe for concurrent use; the TUI
// keeps all mutations on the Bubble Tea event loop.
type Buffer struct {
	lines    []logmsg.LogMessage
	head     int    // index of the next slot to write
	size     int    // number of valid entries
	cap      int
	nextLine uint64 // monotonically increasing line number
}

// New creates a Buffer that retains at most capLines lines. capLines
// must be >= 1.
func New(capLines int) *Buffer {
	if capLines < 1 {
		capLines = 1
	}
	return &Buffer{
		lines: make([]logmsg.LogMessage, capLines),
		cap:   capLines,
	}
}

// Cap returns the maximum number of lines retained.
func (b *Buffer) Cap() int { return b.cap }

// Len returns the number of lines currently stored.
func (b *Buffer) Len() int { return b.size }

// Append adds a message to the buffer, evicting the oldest if full.
// The buffer assigns the message's MsgID.Line; any value the caller
// set is overwritten so IDs stay monotonic and unique.
// Returns the evicted message's ID and true when an eviction occurred.
func (b *Buffer) Append(msg logmsg.LogMessage) (logmsg.MsgID, bool) {
	msg.ID.Line = b.nextLine
	b.nextLine++
	evicted := b.size == b.cap
	var evictedID logmsg.MsgID
	if evicted {
		// When full, b.head points at the oldest slot (about to be overwritten).
		evictedID = b.lines[b.head].ID
	}
	b.lines[b.head] = msg
	b.head = (b.head + 1) % b.cap
	if b.size < b.cap {
		b.size++
	}
	return evictedID, evicted
}

// Snapshot returns all stored messages in chronological order. The
// returned slice is a fresh copy and safe for the caller to retain.
func (b *Buffer) Snapshot() []logmsg.LogMessage {
	out := make([]logmsg.LogMessage, b.size)
	start := (b.head - b.size + b.cap) % b.cap
	for i := 0; i < b.size; i++ {
		out[i] = b.lines[(start+i)%b.cap]
	}
	return out
}

// Line returns the i-th message in chronological order (0 = oldest).
// Returns a zero-value LogMessage for out-of-range indices.
func (b *Buffer) Line(i int) logmsg.LogMessage {
	if i < 0 || i >= b.size {
		return logmsg.LogMessage{}
	}
	start := (b.head - b.size + b.cap) % b.cap
	return b.lines[(start+i)%b.cap]
}

// LastN returns the most recent n messages in chronological order. If
// n exceeds the number of stored messages, the full snapshot is returned.
func (b *Buffer) LastN(n int) []logmsg.LogMessage {
	if n <= 0 {
		return []logmsg.LogMessage{}
	}
	if n >= b.size {
		return b.Snapshot()
	}
	out := make([]logmsg.LogMessage, n)
	start := (b.head - n + b.cap) % b.cap
	for i := 0; i < n; i++ {
		out[i] = b.lines[(start+i)%b.cap]
	}
	return out
}

// IndexedLine pairs a chronological index with a log message.
type IndexedLine struct {
	Index int
	Msg   logmsg.LogMessage
}

// Range returns up to n messages starting at chronological index start,
// each paired with its absolute index. Indices outside [0, Len()) are
// clamped silently.
func (b *Buffer) Range(start, n int) []IndexedLine {
	if b.size == 0 || n <= 0 {
		return nil
	}
	if start < 0 {
		start = 0
	}
	if start >= b.size {
		return nil
	}
	end := start + n
	if end > b.size {
		end = b.size
	}
	out := make([]IndexedLine, 0, end-start)
	for i := start; i < end; i++ {
		out = append(out, IndexedLine{Index: i, Msg: b.Line(i)})
	}
	return out
}

// SearchRegex iterates the buffer starting at chronological index since,
// counts every line matching re as a total match, and collects up to limit
// of them. It returns:
//   - matches: the first limit matching lines with their indices
//   - total: the total number of matches from since onward
//   - nextSince: the chronological index immediately after the last returned
//     match (use as since for the next page), or -1 if all matches were returned
func (b *Buffer) SearchRegex(re *regexp.Regexp, since, limit int) (matches []IndexedLine, total int, nextSince int) {
	if since < 0 {
		since = 0
	}
	nextSince = -1
	for i := since; i < b.size; i++ {
		msg := b.Line(i)
		if re.MatchString(msg.Raw) {
			total++
			if len(matches) < limit {
				matches = append(matches, IndexedLine{Index: i, Msg: msg})
				if len(matches) == limit {
					// Record next_since as the index after this last collected match.
					nextSince = i + 1
				}
			}
		}
	}
	// If we collected fewer than limit (all matches fit), nextSince stays -1.
	if total <= limit {
		nextSince = -1
	}
	return matches, total, nextSince
}
