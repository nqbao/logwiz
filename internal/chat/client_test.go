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
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
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
	reply, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "ping"}})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if reply != "pong" {
		t.Errorf("reply = %q, want %q", reply, "pong")
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
	if len(gotBody.Messages) != 1 || gotBody.Messages[0].Content != "ping" {
		t.Errorf("messages = %+v", gotBody.Messages)
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
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}); err != nil {
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
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
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
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
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
	_, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("expected error for no choices")
	}
}

func TestCompleteRequiresAPIKey(t *testing.T) {
	c := New("https://example.invalid", "", "m")
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected error for missing api key")
	}
}

func TestCompleteRequiresModel(t *testing.T) {
	c := New("https://example.invalid", "sk", "")
	if _, err := c.Complete(context.Background(), []Message{{Role: "user", Content: "x"}}); err == nil {
		t.Fatal("expected error for missing model")
	}
}

func TestCompleteRequiresMessages(t *testing.T) {
	c := New("https://example.invalid", "sk", "m")
	if _, err := c.Complete(context.Background(), nil); err == nil {
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

	_, err := c.Complete(ctx, []Message{{Role: "user", Content: "x"}})
	if err == nil {
		t.Fatal("expected context cancellation error")
	}
}
