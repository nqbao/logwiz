package agent

import (
	"context"
	"strings"
)

// NewChatAgent creates a long-lived agent for the sidebar chat pipeline.
func NewChatAgent(client *Client, ctx context.Context, tools []ToolSpec, initialMsgs []Message) *Agent {
	return &Agent{
		Client: client,
		ctx:    ctx,
		msgs:   initialMsgs,
		Tools:  tools,
	}
}

// BuildChatMessages composes the full message list sent to the chat endpoint:
//
//  1. system prompt
//  2. If sessionSample is non-empty and history is empty (first turn): a second
//     system message with the fenced sample and a note about tools.
//  3. history verbatim (includes assistant tool_call messages and tool results)
//  4. user question (no log stuffing on follow-up turns)
//
// For agent continuation calls (where the last message in msgs is a tool
// result), pass an empty question; the function detects this and omits
// the extra user turn.
func BuildChatMessages(systemPrompt string, sessionSample []string, history []Message, question string) []Message {
	msgs := make([]Message, 0, len(history)+3)
	msgs = append(msgs, Message{Role: "system", Content: systemPrompt})

	// Inject the one-shot sample only on the very first user turn.
	if len(sessionSample) > 0 && len(history) == 0 {
		var b strings.Builder
		b.WriteString("Recent log output (session sample):\n\n```\n")
		for _, line := range sessionSample {
			b.WriteString(line)
			b.WriteByte('\n')
		}
		b.WriteString("```\n\nYou can call tools to fetch more of the buffer.")
		msgs = append(msgs, Message{Role: "system", Content: b.String()})
	}

	msgs = append(msgs, history...)

	if question != "" {
		msgs = append(msgs, Message{Role: "user", Content: question})
	}
	return msgs
}
