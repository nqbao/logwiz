package source

import (
	"context"
	"os"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// File reads a file from the beginning to EOF, then emits EOFMsg. It
// does not follow the file for appends (that is a post-v1 feature;
// see PLAN.md §8a).
//
// Parser is applied to each line before batching. Nil falls back to
// logmsg.PassthroughParser.
type File struct {
	Path   string
	Parser logmsg.Parser
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

		pump(ctx, file, sender, f.Parser)
	}()
}
