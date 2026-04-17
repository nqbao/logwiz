package tools

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
)

type searchTool struct{}

func (searchTool) Spec() chat.ToolSpec {
	params := json.RawMessage(`{
		"type": "object",
		"properties": {
			"pattern":          {"type": "string",  "description": "RE2 regular expression to match against log lines"},
			"max":              {"type": "integer", "description": "Maximum number of matching lines to return (default 50, clamped to 500)"},
			"case_insensitive": {"type": "boolean", "description": "If true, match case-insensitively"},
			"since_line":       {"type": "integer", "description": "Start searching from this chronological index (0 = beginning); use next_since_line from a prior result to paginate"}
		},
		"required": ["pattern"]
	}`)
	return chat.ToolSpec{
		Name:        "search_regex",
		Description: "Search log lines matching a regex. Returns matched lines with indices, total match count, and next_since_line for pagination.",
		Parameters:  params,
	}
}

func (searchTool) Execute(args json.RawMessage, buf *buffer.Buffer) (string, error) {
	var p struct {
		Pattern         string `json:"pattern"`
		Max             int    `json:"max"`
		CaseInsensitive bool   `json:"case_insensitive"`
		SinceLine       int    `json:"since_line"`
	}
	p.Max = 50 // default
	if err := json.Unmarshal(args, &p); err != nil {
		return "", fmt.Errorf("search_regex: bad arguments: %w", err)
	}
	if p.Max <= 0 {
		p.Max = 50
	}
	if p.Max > MaxToolLines {
		p.Max = MaxToolLines
	}

	reStr := p.Pattern
	if p.CaseInsensitive {
		reStr = "(?i)" + reStr
	}
	re, err := regexp.Compile(reStr)
	if err != nil {
		return "", fmt.Errorf("search_regex: invalid pattern %q: %w", p.Pattern, err)
	}

	matches, total, nextSince := buf.SearchRegex(re, p.SinceLine, p.Max)

	var b strings.Builder
	fmt.Fprintf(&b, "total_matches=%d returned=%d next_since_line=%d\n", total, len(matches), nextSince)
	for _, il := range matches {
		fmt.Fprintf(&b, "line %d: %s\n", il.Index, il.Msg.Raw)
	}
	return b.String(), nil
}
