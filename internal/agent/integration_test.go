package agent

import (
	"context"
	"os"
	"testing"
)

// TestCompleteIntegration makes a real LLM call.
// Requires OPENAI_API_KEY; optionally OPENAI_BASE_URL and OPENAI_MODEL.
func TestCompleteIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set — skipping integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	c := New(baseURL, apiKey, model)
	reply, err := c.Complete(context.Background(), []Message{
		{Role: "user", Content: "Reply with exactly the word PONG and nothing else."},
	}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.Content == "" {
		t.Fatal("empty response")
	}
	t.Logf("model=%s reply=%q", model, reply.Content)
}

// TestCompleteWithToolsIntegration makes a real tool-call.
func TestCompleteWithToolsIntegration(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set — skipping integration test")
	}
	baseURL := os.Getenv("OPENAI_BASE_URL")
	if baseURL == "" {
		baseURL = "https://api.openai.com/v1"
	}
	model := os.Getenv("OPENAI_MODEL")
	if model == "" {
		model = "gpt-4o-mini"
	}

	c := New(baseURL, apiKey, model)
	tools := CmdToolDefs()
	tc, err := c.CompleteWithTools(context.Background(), []Message{
		{Role: "system", Content: "Translate the user's command into exactly one tool call."},
		{Role: "user", Content: "show me lines matching ERROR"},
	}, tools)
	if err != nil {
		t.Fatalf("CompleteWithTools: %v", err)
	}
	t.Logf("tool=%s args=%s", tc.Name, tc.Arguments)
	if tc.Name == "" {
		t.Error("empty tool name")
	}
}
