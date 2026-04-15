package source

import (
	"context"
	"io"
)

// Stdin reads lines from an io.Reader (typically os.Stdin) and pushes
// them as NewLineMsg values.
type Stdin struct {
	R io.Reader
}

// Start kicks off the read loop in a new goroutine. It returns
// immediately.
func (s *Stdin) Start(ctx context.Context, sender Sender) {
	go pump(ctx, s.R, sender)
}
