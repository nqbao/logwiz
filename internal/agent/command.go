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

// CmdInspectToolDefs returns the read-only inspection tools (find, count, tail, read).
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
		{
			Name:        "tail",
			Description: "Read the last N lines of the log buffer (n ∈ [1, 200]). Use when the user asks about recent activity or wants a summary of what just happened.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"n": {"type": "integer", "description": "Number of lines to read from the end (1-200)"}
				},
				"required": ["n"]
			}`),
		},
		{
			Name:        "read",
			Description: "Read a specific inclusive 1-based line range from the log buffer (max 200 lines). Use when the user references a specific line range.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"start": {"type": "integer", "description": "First line to read (1-based, inclusive)"},
					"end":   {"type": "integer", "description": "Last line to read (1-based, inclusive)"}
				},
				"required": ["start", "end"]
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
					"enum": ["grep", "clear_grep", "jump", "highlight", "clear_highlight", "set_model", "clear_chat", "quit", "answer", "reject"],
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
// history contains prior user/assistant pairs from this palette session (may be nil).
func BuildCmdPrompt(question, logContext string, history []Message) []Message {
	sys := `You are a log-viewer assistant. Translate the user's natural-language input into a logwiz action or answer.

Role: Use inspection tools (find, count, tail, read) when you need to look at logs, then commit with pick_tool. Go straight to pick_tool when the request is unambiguous.

Rules:
- Always end with exactly one pick_tool call.
- Pattern strings are Go regex. In JSON, write one backslash for a literal dot: "\." not "\\.".
- Inspection tools are read-only — they do not modify the view.
- Use find before jump when the user names something that might match unrelated lines (e.g. "500 error" could match timestamps — verify with find first).
- Use pick_tool(action="answer", reason="…") for questions about the logs. Keep reason to 1–2 lines (≤200 chars), no markdown.
- Use tail(n) to read the most recent lines when the user asks about recent activity or wants a summary.
- Use read(start, end) when the user references a specific line range.
- Use count when the user asks "how many".
- Use highlight to colour-band matching lines without hiding others; use clear_highlight to remove all colour bands.
- Unprefixed input may be an action or a question — decide from context.
- Use pick_tool(action="reject") only for gibberish, out-of-scope requests, or when you cannot tell what the user wants.
- If there is prior conversation history, use it for context when the user asks follow-up questions (e.g. "why?", "show me those lines").
- Log format samples may be provided below. They are NOT comprehensive and NOT necessarily recent — do not answer questions from samples alone; always use inspection tools.

Examples:

User: "filter to only errors"
→ pick_tool(action="grep", pattern="error", mode="include", flags="i")

User: "go to first 500 error"
→ find(pattern="500", flags="") → {count: 8, first_line: 45, samples: ["2024-01-01T00:00:500ms ...", "HTTP 500 ..."]}
→ pick_tool(action="jump", pattern="HTTP 500", position="first")

User: "highlight warnings"
→ pick_tool(action="highlight", pattern="warn", flags="i")

User: "clear highlights"
→ pick_tool(action="clear_highlight")

User: "how many warnings"
→ count(pattern="warn", flags="i") → {count: 12 of 200 lines}
→ pick_tool(action="answer", reason="12 of 200 lines (6%) match 'warn'")

User: "what's the error rate?"
→ count(pattern="(?i)error") → {count: 37 of 500}
→ pick_tool(action="answer", reason="~7.4% of lines (37/500) match 'error'")

User: "summarize the last 50 lines"
→ tail(n=50) → {50 lines: ...}
→ pick_tool(action="answer", reason="Mostly healthy requests; two 500s from /api/checkout at 12:03.")

User: "what happened on lines 120-140?"
→ read(start=120, end=140) → {...}
→ pick_tool(action="answer", reason="Worker retry loop on Redis timeout; 5 retries before fallback.")

User: "exit"
→ pick_tool(action="quit")

User: "what is the capital of France"
→ pick_tool(action="reject", reason="not a log viewer command")`
	if logContext != "" {
		sys += "\n\n" + logContext
	}
	msgs := []Message{{Role: "system", Content: sys}}
	msgs = append(msgs, history...)
	msgs = append(msgs, Message{Role: "user", Content: question})
	return msgs
}
