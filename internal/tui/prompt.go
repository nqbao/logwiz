package tui

import (
	"strings"

	"github.com/nqbao/logwiz/internal/chat"
)

// buildChatMessages composes the full message list sent to the chat
// endpoint:
//
//  1. system prompt
//  2. prior conversation turns
//  3. the user question, prefixed with a fenced code block containing
//     the most recent logLines
//
// Keeping this in a pure function makes the exact prompt shape easy to
// unit-test and tweak without touching the TUI.
func buildChatMessages(systemPrompt string, history []chat.Message, logLines []string, question string) []chat.Message {
	msgs := make([]chat.Message, 0, len(history)+2)
	msgs = append(msgs, chat.Message{Role: "system", Content: systemPrompt})
	msgs = append(msgs, history...)

	var b strings.Builder
	b.WriteString("Recent log output:\n\n```\n")
	for _, line := range logLines {
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteString("```\n\n")
	b.WriteString("Question: ")
	b.WriteString(question)

	msgs = append(msgs, chat.Message{Role: "user", Content: b.String()})
	return msgs
}
