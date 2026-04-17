package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
)

type tailTool struct{}

func (tailTool) Spec() chat.ToolSpec {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"n": {"type": "integer", "description": "Number of most-recent lines to return (clamped to 500)"}
		},
		"required": ["n"]
	}`)
	return chat.ToolSpec{
		Name:        "tail",
		Description: "Return the last n lines from the log buffer. Same format as read: total= header then line INDEX: CONTENT.",
		Parameters:  params,
	}
}

func (tailTool) Execute(args json.RawMessage, buf *buffer.Buffer) (string, error) {
	var p struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("tail: bad arguments: %w", err)
	}
	if p.N > MaxToolLines {
		p.N = MaxToolLines
	}
	if p.N <= 0 {
		p.N = 1
	}

	total := buf.Len()
	start := total - p.N
	if start < 0 {
		start = 0
	}
	lines := buf.Range(start, p.N)

	var b strings.Builder
	fmt.Fprintf(&b, "total=%d\n", total)
	for _, il := range lines {
		fmt.Fprintf(&b, "line %d: %s\n", il.Index, il.Msg.Raw)
	}
	if len(lines) == 0 {
		fmt.Fprintf(&b, "(buffer is empty)\n")
	}
	return b.String(), nil
}
