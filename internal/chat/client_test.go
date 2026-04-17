package chat

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// requestBody is a minimal shape of the chat-completions request body,
// used only by tests to inspect what the SDK sends.
type requestBody struct {
	Model    string `json:"model"`
	Messages []struct {
		Role       string `json:"role"`
		Content    any    `json:"content"`
		ToolCallID string `json:"tool_call_id,omitempty"`
		ToolCalls  []struct {
			ID       string `json:"id"`
			Type     string `json:"type"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls,omitempty"`
	} `json:"messages"`
	Tools []struct {
		Type     string `json:"type"`
		Function struct {
			Name        string         `json:"name"`
			Description string         `json:"description"`
			Parameters  map[string]any `json:"parameters"`
		} `json:"function"`
	} `json:"tools,omitempty"`
	Stream bool `json:"stream"`
}

func TestCompleteHappyPath(t *testing.T) {
	var gotAuth, gotCT, gotPath string
	var gotBody requestBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		gotPath = r.URL.Path

		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"pong"}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sk-test", "gpt-mini")
	reply, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "ping"}}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply.Content != "pong" {
		t.Errorf("reply.Content = %q, want %q", reply.Content, "pong")
	}
	if reply.Role != "assistant" {
		t.Errorf("reply.Role = %q, want assistant", reply.Role)
	}
	if gotPath != "/chat/completions" {
		t.Errorf("path = %q", gotPath)
	}
	if gotAuth != "Bearer sk-test" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotCT != "application/json" {
		t.Errorf("content-type = %q", gotCT)
	}
	if gotBody.Model != "gpt-mini" {
		t.Errorf("model = %q", gotBody.Model)
	}
	if gotBody.Stream {
		t.Errorf("stream = true, want false")
	}
}

func TestCompleteTrailingSlashInBaseURL(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/v1/", "sk", "m")
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteAPIErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"auth_error","code":"401"}}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sk-bad", "m")
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "invalid api key") {
		t.Errorf("error = %v, want to contain api message", err)
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error = %v, want status code", err)
	}
}

func TestCompleteStatusWithoutStructuredError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`not json`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sk", "m")
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error = %v, want 500", err)
	}
}

func TestCompleteNoChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sk", "m")
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if err == nil {
		t.Fatal("expected error for no choices")
	}
}

func TestCompleteRequiresAPIKey(t *testing.T) {
	c := New("https://example.invalid", "", "m")
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil); err == nil {
		t.Fatal("expected error for missing api key")
	}
}

func TestCompleteRequiresModel(t *testing.T) {
	c := New("https://example.invalid", "sk", "")
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestCompleteRequiresMessages(t *testing.T) {
	c := New("https://example.invalid", "sk", "m")
	if _, err := c.Complete(context.Background(), nil, nil); err == nil {
		t.Fatal("expected error for no messages")
	}
}

func TestCompleteContextCancel(t *testing.T) {
	// Server that hangs forever.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer srv.Close()

	c := New(srv.URL, "sk", "m")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := c.Complete(ctx, []Message{{Role: "user", Content: "x"}}, nil)
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}

func TestCompleteToolsPayloadSent(t *testing.T) {
	var gotBody requestBody

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"ok"}}]}`))
	}))
	defer srv.Close()

	params := json.RawMessage(`{"type":"object","properties":{"n":{"type":"integer"}}}`)
	tools := []ToolSpec{{Name: "tail", Description: "get last N lines", Parameters: params}}

	c := New(srv.URL, "sk", "m")
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, tools)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}

	if len(gotBody.Tools) != 1 {
		t.Fatalf("tools len = %d, want 1", len(gotBody.Tools))
	}
	if gotBody.Tools[0].Function.Name != "tail" {
		t.Errorf("tool name = %q, want tail", gotBody.Tools[0].Function.Name)
	}
	if gotBody.Tools[0].Function.Description != "get last N lines" {
		t.Errorf("tool description = %q", gotBody.Tools[0].Function.Description)
	}
}

func TestCompleteToolCallResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"call_1","type":"function","function":{"name":"tail","arguments":"{\"n\":10}"}}]}}]}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "sk", "m")
	reply, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}, nil)
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(reply.ToolCalls) != 1 {
		t.Fatalf("ToolCalls len = %d, want 1", len(reply.ToolCalls))
	}
	tc := reply.ToolCalls[0]
	if tc.ID != "call_1" {
		t.Errorf("ToolCall.ID = %q, want call_1", tc.ID)
	}
	if tc.Name != "tail" {
		t.Errorf("ToolCall.Name = %q, want tail", tc.Name)
	}
	if tc.Arguments != `{"n":10}` {
		t.Errorf("ToolCall.Arguments = %q, want {\"n\":10}", tc.Arguments)
	}
}
