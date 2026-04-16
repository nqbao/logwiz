package source

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// File reads a file from near EOF to EOF, then emits EOFMsg. It does
// not follow the file for appends (see PLAN.md §v1.3).
//
// Parser is applied to each line before batching. Nil falls back to
// logmsg.PassthroughParser.
//
// MaxLines drives the tail-seek heuristic: File samples ~64 KiB near
// EOF to estimate average line size, then seeks backward far enough
// to cover approximately MaxLines lines (with 25% headroom). When the
// estimate doesn't cover the whole file, File emits a synthetic
// "[logwiz: skipped N ...]" marker as the first line so the user
// knows content was dropped. MaxLines <= 0 disables tail-seek (the
// entire file is read from the start; head content may still be
// evicted by the ring buffer downstream).
type File struct {
	Path     string
	Parser   logmsg.Parser
	MaxLines int
}

// sampleSize is how many bytes we read from near EOF to estimate
// average line length. Big enough to cover hundreds of lines in the
// typical case; small enough that the read is effectively free.
const sampleSize = 64 << 10

// headroom multiplies the estimated byte budget so normal line-size
// variance doesn't leave the viewport under-filled. 25% is a
// compromise between overshooting (wasted I/O) and undershooting
// (user sees fewer lines than --max-lines requested).
const headroomNum, headroomDen = 5, 4 // 1.25×

// fallbackTailBytes is used when we can't estimate (sample contained
// no newlines — one giant line, or the tail is binary).
const fallbackTailBytes int64 = 10 << 20 // 10 MiB

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

		r, err := f.tailSeek(file, sender)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		pump(ctx, r, sender, f.Parser)
	}()
}

// tailSeek estimates a byte budget for MaxLines lines, seeks to that
// offset before EOF, discards the partial line at the seek point, and
// emits a truncation-marker LogMessage. When no seek is needed
// (MaxLines <= 0, small file, or estimate covers the whole file) the
// raw file is returned unchanged.
func (f *File) tailSeek(file *os.File, sender Sender) (io.Reader, error) {
	if f.MaxLines <= 0 {
		return file, nil
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	budget, err := estimateTailBytes(file, info.Size(), f.MaxLines)
	if err != nil {
		return nil, err
	}
	if budget <= 0 || budget >= info.Size() {
		return file, nil
	}
	if _, err := file.Seek(-budget, io.SeekEnd); err != nil {
		return nil, err
	}
	br := bufio.NewReader(file)
	// We almost certainly landed mid-line; drop everything up to the
	// next newline so the first line we emit is well-formed. EOF here
	// just means the tail contains no newline at all — pump will
	// produce nothing, which is fine.
	if _, err := br.ReadString('\n'); err != nil && err != io.EOF {
		return nil, err
	}
	skipped := info.Size() - budget
	sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{
		Raw: fmt.Sprintf("[logwiz: skipped %s from head of file (tail-seek); raise --max-lines to read more]", humanBytes(skipped)),
	}}})
	return br, nil
}

// estimateTailBytes returns the byte budget needed to cover
// approximately maxLines lines from the tail of a file of size
// fileSize. Returns 0 when the file is too small to warrant seeking.
func estimateTailBytes(file *os.File, fileSize int64, maxLines int) (int64, error) {
	if fileSize <= sampleSize {
		return 0, nil
	}
	buf := make([]byte, sampleSize)
	// ReadAt doesn't move the file offset, so subsequent Seek calls
	// still behave as if we never read.
	if _, err := file.ReadAt(buf, fileSize-sampleSize); err != nil && err != io.EOF {
		return 0, err
	}
	newlines := bytes.Count(buf, []byte{'\n'})
	if newlines < 2 {
		// Can't estimate from the sample — fall back to a fixed cap.
		if fileSize <= fallbackTailBytes {
			return 0, nil
		}
		return fallbackTailBytes, nil
	}
	avg := int64(sampleSize) / int64(newlines)
	if avg < 1 {
		avg = 1
	}
	budget := avg * int64(maxLines) * headroomNum / headroomDen
	return budget, nil
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
