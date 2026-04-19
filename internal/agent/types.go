// Package agent provides the LLM client, message types, and agent lifecycle
// for logwiz's chat sidebar and palette agent commands.
package agent

import "encoding/json"

// Message is a single chat message in the OpenAI chat-completions format.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`  // role=assistant
	ToolCallID string     `json:"tool_call_id,omitempty"` // role=tool
	Name       string     `json:"name,omitempty"`         // role=tool
}

// ToolCall represents a function call requested by the assistant.
type ToolCall struct {
	ID        string
	Name      string
	Arguments string // raw JSON string
}

// ToolSpec describes a tool available to the model.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  json.RawMessage // JSON Schema object
}
