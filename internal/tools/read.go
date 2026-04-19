package tools

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
)

type readTool struct{}

func (readTool) Spec() agent.ToolSpec {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"start": {"type": "integer", "description": "Chronological index of the first line to return (0 = oldest in buffer)"},
			"n":     {"type": "integer", "description": "Number of lines to return (clamped to 500)"}
		},
		"required": ["start", "n"]
	}`)
	return agent.ToolSpec{
		Name:        "read",
		Description: "Read n lines from the log buffer starting at absolute chronological index start. Returns lines with their index numbers and a total= header.",
		Parameters:  params,
	}
}

func (readTool) Execute(args json.RawMessage, buf *buffer.Buffer) (string, error) {
	var p struct {
		Start int `json:"start"`
		N     int `json:"n"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("read: bad arguments: %w", err)
	}
	if p.N > MaxToolLines {
		p.N = MaxToolLines
	}
	if p.N <= 0 {
		p.N = 1
	}

	lines := buf.Range(p.Start, p.N)
	var b strings.Builder
	fmt.Fprintf(&b, "total=%d\n", buf.Len())
	for _, il := range lines {
		fmt.Fprintf(&b, "line %d: %s\n", il.Index, il.Msg.Raw)
	}
	if len(lines) == 0 {
		fmt.Fprintf(&b, "(no lines in range start=%d n=%d)\n", p.Start, p.N)
	}
	return b.String(), nil
}
