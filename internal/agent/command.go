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

// CmdToolDefs returns the static tool schema for agent command routing.
func CmdToolDefs() []ToolSpec {
	must := func(s string) json.RawMessage { return json.RawMessage(s) }
	return []ToolSpec{
		{
			Name:        "grep",
			Description: "Filter log lines to show only those matching (or not matching) a pattern.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"},
					"mode":    {"type": "string", "enum": ["include", "exclude"]},
					"flags":   {"type": "string", "enum": ["i", ""], "description": "i = case-insensitive"}
				},
				"required": ["pattern", "mode"]
			}`),
		},
		{
			Name:        "clear_grep",
			Description: "Remove all active log filters.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "search",
			Description: "Highlight log lines matching a pattern without hiding others.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"}
				},
				"required": ["pattern"]
			}`),
		},
		{
			Name:        "jump",
			Description: "Jump the log viewport to the first or last line matching a pattern.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern":  {"type": "string", "description": "Go regex pattern"},
					"position": {"type": "string", "enum": ["first", "last"]}
				},
				"required": ["pattern", "position"]
			}`),
		},
		{
			Name:        "set_model",
			Description: "Change the AI model used for chat.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"model": {"type": "string", "description": "model name, e.g. gpt-4o"}
				},
				"required": ["model"]
			}`),
		},
		{
			Name:        "clear_chat",
			Description: "Reset the chat conversation history.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "answer",
			Description: "Open the chat sidebar and submit a question about the logs. Use for any analysis or question request.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"question": {"type": "string", "description": "the question to ask about the logs"}
				},
				"required": ["question"]
			}`),
		},
		{
			Name:        "quit",
			Description: "Exit logwiz.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "reject",
			Description: "Reject the command when it does not map to any logwiz operation — e.g. random text, gibberish, or requests unrelated to log viewing.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"reason": {"type": "string", "description": "short explanation shown to the user"}
				},
				"required": ["reason"]
			}`),
		},
	}
}

// BuildCmdPrompt constructs the message list for an agent command routing call.
func BuildCmdPrompt(question, logContext string) []Message {
	sys := "You are a log-viewer assistant. Translate the user's natural-language command into exactly one tool call.\n\n" +
		"Rules:\n" +
		"- Always call exactly one tool.\n" +
		"- Use grep/search/jump to operate on log lines; produce valid Go regex.\n" +
		"- Use answer for any question or analysis request about the logs.\n" +
		"- Call reject when the input is random text, gibberish, or completely unrelated to log viewing.\n"
	if logContext != "" {
		sys += "\n" + logContext
	}
	return []Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: question},
	}
}
