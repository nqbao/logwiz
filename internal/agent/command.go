package agent

import (
	"context"
	"encoding/json"
)

// NewCmdAgent creates a short-lived agent for palette agent commands.
func NewCmdAgent(client *Client, ctx context.Context, msgs []Message) *Agent {
	return &Agent{
		Client:     client,
		ctx:        ctx,
		msgs:       msgs,
		Tools:      CmdToolDefs(),
		toolChoice: "required",
	}
}

// CmdToolDefs returns the full tool set: inspection tools plus the commit umbrella.
func CmdToolDefs() []ToolSpec {
	defs := CmdInspectToolDefs()
	defs = append(defs, CmdCommitToolDef())
	return defs
}

// CmdInspectToolDefs returns the read-only inspection tools (find, count).
func CmdInspectToolDefs() []ToolSpec {
	must := func(s string) json.RawMessage { return json.RawMessage(s) }
	return []ToolSpec{
		{
			Name:        "find",
			Description: "Search the log buffer for lines matching a pattern. Returns count, first/last line numbers (1-based), and up to 10 sample matches. Use before committing to verify a pattern matches what you expect.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"},
					"flags":   {"type": "string", "enum": ["i", ""], "description": "i = case-insensitive"}
				},
				"required": ["pattern"]
			}`),
		},
		{
			Name:        "count",
			Description: "Count log lines matching a pattern. Returns the match count and total line count.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"},
					"flags":   {"type": "string", "enum": ["i", ""], "description": "i = case-insensitive"}
				},
				"required": ["pattern"]
			}`),
		},
	}
}

// CmdCommitToolDef returns the single pick_tool commit schema.
func CmdCommitToolDef() ToolSpec {
	must := func(s string) json.RawMessage { return json.RawMessage(s) }
	return ToolSpec{
		Name:        "pick_tool",
		Description: "Commit to an action. Call this once you know what to do — after inspection if needed, or directly for unambiguous requests.",
		Parameters: must(`{
			"type": "object",
			"properties": {
				"action": {
					"type": "string",
					"enum": ["grep", "clear_grep", "jump", "set_model", "clear_chat", "quit", "reject"],
					"description": "The action to perform"
				},
				"pattern":  {"type": "string", "description": "Go regex pattern (grep, jump)"},
				"mode":     {"type": "string", "enum": ["include", "exclude"], "description": "grep filter direction"},
				"flags":    {"type": "string", "enum": ["i", ""], "description": "i = case-insensitive (grep, jump)"},
				"position": {"type": "string", "enum": ["first", "last"], "description": "jump target position"},
				"model":    {"type": "string", "description": "model name for set_model"},
				"reason":   {"type": "string", "description": "message shown to the user for reject, or answer for informational queries like count results"}
			},
			"required": ["action"]
		}`),
	}
}

// BuildCmdPrompt constructs the message list for an agent command routing call.
func BuildCmdPrompt(question, logContext string) []Message {
	sys := `You are a log-viewer assistant. Translate the user's natural-language command into a logwiz action.

Role: Use inspection tools (find, count) when uncertain about a pattern, then commit with pick_tool. Go straight to pick_tool when the request is unambiguous.

Rules:
- Always end with exactly one pick_tool call.
- Inspection tools are read-only — they do not modify the view.
- Use find before jump when the user names something that might match unrelated lines (e.g. "500 error" could match timestamps — verify with find first).
- Use count when the user asks "how many" — then report the result via pick_tool(action="reject", reason="N of M lines match ...").
- Use pick_tool(action="reject") for gibberish, out-of-scope requests, or to report an informational result.

Examples:

User: "filter to only errors"
→ pick_tool(action="grep", pattern="error", mode="include", flags="i")

User: "go to first 500 error"
→ find(pattern="500", flags="") → {count: 8, first_line: 45, samples: ["2024-01-01T00:00:500ms ...", "HTTP 500 ..."]}
→ pick_tool(action="jump", pattern="HTTP 500", position="first")

User: "how many warnings"
→ count(pattern="warn", flags="i") → {count: 12 of 200 lines}
→ pick_tool(action="reject", reason="12 of 200 lines match 'warn'")

User: "exit"
→ pick_tool(action="quit")

User: "what is the capital of France"
→ pick_tool(action="reject", reason="not a log viewer command")`
	if logContext != "" {
		sys += "\n\n" + logContext
	}
	return []Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: question},
	}
}
