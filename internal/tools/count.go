package tools

import (
	"encoding/json"
	"fmt"
	"regexp"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
)

type countTool struct{}

func (countTool) Spec() agent.ToolSpec {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern":          {"type": "string",  "description": "RE2 regular expression to match against log lines"},
			"case_insensitive": {"type": "boolean", "description": "If true, match case-insensitively"},
			"since_line":       {"type": "integer", "description": "Start counting from this chronological index (0 = beginning)"}
		},
		"required": ["pattern"]
	}`)
	return agent.ToolSpec{
		Name:        "count_regex",
		Description: "Count log lines matching a regex without returning their content. Use this to cheaply assess scale before deciding whether to call search_regex.",
		Parameters:  params,
	}
}

func (countTool) Execute(args json.RawMessage, buf *buffer.Buffer) (string, error) {
	var p struct {
		Pattern         string `json:"pattern"`
		CaseInsensitive bool   `json:"case_insensitive"`
		SinceLine       int    `json:"since_line"`
	}
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("count_regex: bad arguments: %w", err)
	}

	reStr := p.Pattern
	if p.CaseInsensitive {
		reStr = "(?i)" + reStr
	}
	re, err := regexp.Compile(reStr)
	if err != nil {
		return "", fmt.Errorf("count_regex: invalid pattern %q: %w", p.Pattern, err)
	}

	n := buf.CountRegex(re, p.SinceLine)
	return fmt.Sprintf("total_matches=%d\n", n), nil
}
