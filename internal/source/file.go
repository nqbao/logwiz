package source

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// File reads a file from the beginning to EOF, then emits EOFMsg. It
// does not follow the file for appends (that is a post-v1 feature;
// see PLAN.md §8a).
//
// Parser is applied to each line before batching. Nil falls back to
// logmsg.PassthroughParser.
//
// TailBytes, when > 0, caps how much of the file is read. If the file
// is larger than TailBytes, File seeks to approximately TailBytes
// before EOF, advances to the next line boundary, emits a synthetic
// truncation-marker LogMessage as the first line, and then pumps from
// there. This keeps startup O(TailBytes) instead of O(fileSize) and
// signals the user that content was skipped.
type File struct {
	Path      string
	Parser    logmsg.Parser
	TailBytes int64
}

// Start opens the file and pumps lines. If opening fails, an ErrMsg
// is sent and the goroutine exits.
func (f *File) Start(ctx context.Context, sender Sender) {
	go func() {
		file, err := os.Open(f.Path)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}
		defer file.Close()

		// Close the file when ctx is cancelled so pump unblocks.
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-ctx.Done():
				_ = file.Close()
			case <-done:
			}
		}()

		r, err := tailSeek(file, f.TailBytes, sender)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		pump(ctx, r, sender, f.Parser)
	}()
}

// tailSeek optionally repositions file to within tailBytes of EOF,
// discards the partial line at the seek point, and emits a truncation
// marker LogMessage via sender. The returned reader is what pump
// should consume — either the original file, or a bufio.Reader that
// has already been advanced past the partial line.
func tailSeek(file *os.File, tailBytes int64, sender Sender) (io.Reader, error) {
	if tailBytes <= 0 {
		return file, nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size() <= tailBytes {
		return file, nil
	}

	skipped := info.Size() - tailBytes
	if _, err := file.Seek(-tailBytes, io.SeekEnd); err != nil {
		return nil, err
	}
	br := bufio.NewReader(file)
	// We almost certainly landed mid-line; drop everything up to the
	// next newline so the first line we emit is well-formed. If we
	// happen to hit EOF first, the subsequent Scan will simply return
	// no lines.
	if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
		return nil, err
	}
	sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{
		Raw: fmt.Sprintf("[logwiz: skipped %s from head of file (tail-seek); raise --tail-bytes to read more]", humanBytes(skipped)),
	}}})
	return br, nil
}

// humanBytes formats n using binary units (KiB / MiB / GiB). Used for
// user-facing messages; not a general-purpose formatter.
func humanBytes(n int64) string {
	const (
		KiB = 1 << 10
		MiB = 1 << 20
		GiB = 1 << 30
	)
	switch {
	case n >= GiB:
		return fmt.Sprintf("%.1f GiB", float64(n)/GiB)
	case n >= MiB:
		return fmt.Sprintf("%.1f MiB", float64(n)/MiB)
	case n >= KiB:
		return fmt.Sprintf("%.1f KiB", float64(n)/KiB)
	default:
		return fmt.Sprintf("%d B", n)
	}
}
