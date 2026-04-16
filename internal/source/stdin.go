package source

import (
	"context"
	"io"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// Stdin reads lines from an io.Reader (typically os.Stdin) and pushes
// them as NewLinesMsg values containing parsed LogMessages.
//
// Parser is applied to each line before batching. Nil falls back to
// logmsg.PassthroughParser.
type Stdin struct {
	R      io.Reader
	Parser logmsg.Parser
}

// Start kicks off the read loop in a new goroutine. It returns
// immediately.
func (s *Stdin) Start(ctx context.Context, sender Sender) {
	go pump(ctx, s.R, sender, s.Parser)
}
