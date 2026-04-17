package tui

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/chat"
)

func TestBuildChatMessagesFirstTurnShape(t *testing.T) {
	history := []chat.Message{} // first turn: no history
	msgs := buildChatMessages("SYS", []string{"line1", "line2"}, history, "why is it failing?")

	// system + sample-system + user = 3
	if len(msgs) != 3 {
		t.Fatalf("len = %d, want 3; got %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "SYS" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1].Role != "system" {
		t.Errorf("msgs[1] role = %q, want system (sample)", msgs[1].Role)
	}
	if !strings.Contains(msgs[1].Content, "line1\nline2") {
		t.Errorf("sample not in msgs[1]: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "```") {
		t.Errorf("expected fenced block in sample: %q", msgs[1].Content)
	}
	last := msgs[2]
	if last.Role != "user" {
		t.Errorf("last role = %q, want user", last.Role)
	}
	if !strings.Contains(last.Content, "why is it failing?") {
		t.Errorf("question missing: %q", last.Content)
	}
}

func TestBuildChatMessagesFollowUpTurnShape(t *testing.T) {
	// Second turn: history already has messages, no sample injection.
	history := []chat.Message{
		{Role: "user", Content: "earlier"},
		{Role: "assistant", Content: "ack"},
	}
	msgs := buildChatMessages("SYS", []string{"line1", "line2"}, history, "follow up?")

	// system + 2 history + user = 4 (NO sample message)
	if len(msgs) != 4 {
		t.Fatalf("len = %d, want 4; got %+v", len(msgs), msgs)
	}
	if msgs[1].Role != history[0].Role || msgs[1].Content != history[0].Content {
		t.Errorf("history[0] not preserved: %+v", msgs[1])
	}
	if msgs[2].Role != history[1].Role || msgs[2].Content != history[1].Content {
		t.Errorf("history[1] not preserved: %+v", msgs[2])
	}
	if msgs[3].Role != "user" || msgs[3].Content != "follow up?" {
		t.Errorf("msgs[3] = %+v", msgs[3])
	}
}

func TestBuildChatMessagesEmptySample(t *testing.T) {
	// No sample: even on first turn, no sample message injected.
	msgs := buildChatMessages("SYS", nil, nil, "q")
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Errorf("roles = %q,%q", msgs[0].Role, msgs[1].Role)
	}
}

func TestBuildChatMessagesContinuationAfterToolResult(t *testing.T) {
	// After tool execution the last message in history is a tool result;
	// pass empty question to avoid appending a duplicate user turn.
	history := []chat.Message{
		{Role: "user", Content: "q"},
		{Role: "assistant", ToolCalls: []chat.ToolCall{{ID: "1", Name: "tail", Arguments: `{"n":10}`}}},
		{Role: "tool", ToolCallID: "1", Name: "tail", Content: "total=50\nline 49: last"},
	}
	msgs := buildChatMessages("SYS", []string{"sample"}, history, "")

	// system + sample (first turn? no — history is non-empty) + 3 history = 4
	// History is non-empty so no sample injection.
	if len(msgs) != 4 {
		t.Fatalf("len = %d, want 4 (sys + 3 history); got %+v", len(msgs), msgs)
	}
	// No trailing user message.
	last := msgs[len(msgs)-1]
	if last.Role != "tool" {
		t.Errorf("last role = %q, want tool", last.Role)
	}
}
