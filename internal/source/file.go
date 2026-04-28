package source

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// File reads a file from near EOF to EOF, then optionally follows it for
// appends (Follow=true). It does not follow by default for backward
// compatibility when Follow is not set.
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
//
// When Follow is true, File sends FollowingMsg once the initial read
// completes, then watches the file for appends, truncations, and
// rotations using a 250 ms poll. StopFollow and StartFollow allow
// runtime toggling from the TUI.
type File struct {
	Path     string
	Parser   logmsg.Parser
	MaxLines int
	Follow   bool

	// follow runtime state — set in Start, safe to access after that.
	mu           sync.Mutex
	followCancel context.CancelFunc // non-nil while watcher is running
	progCtx      context.Context    // outer ctx from Start
	progSender   Sender             // sender from Start
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

// watchInterval is how often the watcher polls the file for changes.
const watchInterval = 250 * time.Millisecond

// Start opens the file and pumps lines. If opening fails, an ErrMsg
// is sent and the goroutine exits.
func (f *File) Start(ctx context.Context, sender Sender) {
	f.mu.Lock()
	f.progCtx = ctx
	f.progSender = sender
	f.mu.Unlock()

	go func() {
		file, err := os.Open(f.Path)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		// Close the file when ctx is cancelled so pumpLines unblocks.
		pumpDone := make(chan struct{})
		defer close(pumpDone)
		go func() {
			select {
			case <-ctx.Done():
				_ = file.Close()
			case <-pumpDone:
			}
		}()

		r, err := f.tailSeek(file, sender)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		if err := pumpLines(ctx, r, sender, f.Parser); err != nil || ctx.Err() != nil {
			if ctx.Err() == nil {
				sender.Send(ErrMsg{Err: err})
			}
			return
		}

		if !f.Follow {
			sender.Send(EOFMsg{})
			return
		}

		// Seek the raw fd to its current end (bypasses any bufio wrapper from
		// tailSeek that may have buffered ahead of what was scanned).
		lastOffset, err := file.Seek(0, io.SeekEnd)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		// Open a fresh fd for the watcher; the deferred pumpDone close and
		// ctx-cancel goroutine above own the original file.
		watchFile, err := os.Open(f.Path)
		if err != nil {
			sender.Send(ErrMsg{Err: err})
			return
		}

		watchCtx, cancel := context.WithCancel(ctx)
		f.mu.Lock()
		f.followCancel = cancel
		f.mu.Unlock()

		sender.Send(FollowingMsg{})
		f.watch(watchCtx, watchFile, sender, lastOffset)
	}()
}

// StopFollow cancels the running watcher. The watcher sends EOFMsg when it
// exits, so the TUI learns follow has stopped. No-op if not currently following.
func (f *File) StopFollow() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.followCancel != nil {
		f.followCancel()
		f.followCancel = nil
	}
}

// StartFollow opens a new watcher from the current EOF. It emits a marker
// line and FollowingMsg so the TUI badge updates. No-op if already following
// or if the program context has been cancelled.
func (f *File) StartFollow() {
	f.mu.Lock()
	if f.followCancel != nil {
		f.mu.Unlock()
		return // already following
	}
	ctx := f.progCtx
	sender := f.progSender
	f.mu.Unlock()

	if ctx == nil || ctx.Err() != nil {
		return
	}

	file, err := os.Open(f.Path)
	if err != nil {
		sender.Send(ErrMsg{Err: err})
		return
	}

	// Start from current EOF; skip the gap that accumulated while follow was off.
	lastOffset, err := file.Seek(0, io.SeekEnd)
	if err != nil {
		_ = file.Close()
		sender.Send(ErrMsg{Err: err})
		return
	}

	sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{
		Raw: "[logwiz: follow re-enabled, skipped to current EOF]",
	}}})

	watchCtx, cancel := context.WithCancel(ctx)
	f.mu.Lock()
	f.followCancel = cancel
	f.mu.Unlock()

	sender.Send(FollowingMsg{})
	go f.watch(watchCtx, file, sender, lastOffset)
}

// watch polls f.Path every watchInterval for size changes, truncation, and
// rotation. It owns file and closes it on exit. lastOffset is the byte offset
// at which new data should be read.
func (f *File) watch(ctx context.Context, file *os.File, sender Sender, lastOffset int64) {
	defer file.Close()

	ticker := time.NewTicker(watchInterval)
	defer ticker.Stop()

	fileInfo, err := file.Stat()
	if err != nil {
		sender.Send(ErrMsg{Err: err})
		return
	}

	for {
		select {
		case <-ctx.Done():
			f.mu.Lock()
			f.followCancel = nil
			f.mu.Unlock()
			sender.Send(EOFMsg{})
			return

		case <-ticker.C:
			pathInfo, err := os.Stat(f.Path)
			if os.IsNotExist(err) {
				continue // file being rotated away; wait
			}
			if err != nil {
				sender.Send(ErrMsg{Err: err})
				return
			}

			// Rotation: path now points to a different inode.
			if !os.SameFile(pathInfo, fileInfo) {
				_ = file.Close()
				file, err = os.Open(f.Path)
				if err != nil {
					sender.Send(ErrMsg{Err: err})
					return
				}
				fileInfo, err = file.Stat()
				if err != nil {
					sender.Send(ErrMsg{Err: err})
					return
				}
				pathInfo = fileInfo
				sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{Raw: "[logwiz: file rotated]"}}})
				lastOffset = 0
			}

			// Truncation: file shrank.
			if pathInfo.Size() < lastOffset {
				sender.Send(NewLinesMsg{Lines: []logmsg.LogMessage{{Raw: "[logwiz: file truncated]"}}})
				if _, err := file.Seek(0, io.SeekStart); err != nil {
					sender.Send(ErrMsg{Err: err})
					return
				}
				lastOffset = 0
			}

			// Append: new data available.
			if pathInfo.Size() > lastOffset {
				if _, err := file.Seek(lastOffset, io.SeekStart); err != nil {
					sender.Send(ErrMsg{Err: err})
					return
				}
				if err := pumpLines(ctx, file, sender, f.Parser); err != nil {
					if ctx.Err() == nil {
						sender.Send(ErrMsg{Err: err})
					}
					return
				}
				// Advance offset to actual fd position after scan.
				newOffset, err := file.Seek(0, io.SeekCurrent)
				if err != nil {
					sender.Send(ErrMsg{Err: err})
					return
				}
				lastOffset = newOffset
			}
		}
	}
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
