package tui

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/chat"
)

func TestBuildChatMessagesShape(t *testing.T) {
	history := []chat.Message{
		{Role: "user", Content: "earlier"},
		{Role: "assistant", Content: "ack"},
	}
	msgs := buildChatMessages("SYS", history, []string{"line1", "line2"}, "why is it failing?")

	if len(msgs) != 4 {
		t.Fatalf("len = %d, want 4; got %+v", len(msgs), msgs)
	}
	if msgs[0].Role != "system" || msgs[0].Content != "SYS" {
		t.Errorf("msgs[0] = %+v", msgs[0])
	}
	if msgs[1] != history[0] || msgs[2] != history[1] {
		t.Errorf("history not preserved")
	}
	last := msgs[3]
	if last.Role != "user" {
		t.Errorf("last role = %q", last.Role)
	}
	if !strings.Contains(last.Content, "line1\nline2") {
		t.Errorf("log lines missing from content: %q", last.Content)
	}
	if !strings.Contains(last.Content, "```") {
		t.Errorf("expected fenced block; content = %q", last.Content)
	}
	if !strings.Contains(last.Content, "why is it failing?") {
		t.Errorf("question missing: %q", last.Content)
	}
}

func TestBuildChatMessagesNoHistory(t *testing.T) {
	msgs := buildChatMessages("s", nil, []string{"a"}, "q")
	if len(msgs) != 2 {
		t.Fatalf("len = %d, want 2", len(msgs))
	}
	if msgs[0].Role != "system" || msgs[1].Role != "user" {
		t.Errorf("roles = %q,%q", msgs[0].Role, msgs[1].Role)
	}
}

func TestBuildChatMessagesEmptyLogs(t *testing.T) {
	msgs := buildChatMessages("s", nil, nil, "q")
	if !strings.Contains(msgs[1].Content, "```") {
		t.Errorf("fences still required with empty logs: %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[1].Content, "q") {
		t.Errorf("question missing")
	}
}
