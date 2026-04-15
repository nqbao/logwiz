// Package buffer provides a fixed-size rolling line buffer used as the
// single source of truth for logs displayed by the TUI and sent to the
// chat model.
package buffer

// Buffer is a fixed-size ring of strings. Appending past capacity
// overwrites the oldest entry. Not safe for concurrent use; the TUI
// keeps all mutations on the Bubble Tea event loop.
type Buffer struct {
	lines []string
	head  int // index of the next slot to write
	size  int // number of valid entries
	cap   int
}

// New creates a Buffer that retains at most capLines lines. capLines
// must be >= 1.
func New(capLines int) *Buffer {
	if capLines < 1 {
		capLines = 1
	}
	return &Buffer{
		lines: make([]string, capLines),
		cap:   capLines,
	}
}

// Cap returns the maximum number of lines retained.
func (b *Buffer) Cap() int { return b.cap }

// Len returns the number of lines currently stored.
func (b *Buffer) Len() int { return b.size }

// Append adds a line to the buffer, evicting the oldest line if full.
func (b *Buffer) Append(line string) {
	b.lines[b.head] = line
	b.head = (b.head + 1) % b.cap
	if b.size < b.cap {
		b.size++
	}
}

// Snapshot returns all stored lines in chronological order. The
// returned slice is a fresh copy and safe for the caller to retain.
func (b *Buffer) Snapshot() []string {
	out := make([]string, b.size)
	start := (b.head - b.size + b.cap) % b.cap
	for i := 0; i < b.size; i++ {
		out[i] = b.lines[(start+i)%b.cap]
	}
	return out
}

// LastN returns the most recent n lines in chronological order. If n
// exceeds the number of stored lines, the full snapshot is returned.
func (b *Buffer) LastN(n int) []string {
	if n <= 0 {
		return []string{}
	}
	if n >= b.size {
		return b.Snapshot()
	}
	out := make([]string, n)
	start := (b.head - n + b.cap) % b.cap
	for i := 0; i < n; i++ {
		out[i] = b.lines[(start+i)%b.cap]
	}
	return out
}
