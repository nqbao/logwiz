// Package chat implements a minimal OpenAI-compatible chat-completions
// client. Only the /chat/completions endpoint is used, and only in
// non-streaming mode — streaming is a post-v1 nice-to-have.
package chat

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	BaseURL string
	APIKey  string
	Model   string
	HTTP    *http.Client
}

// New returns a Client with a sensible default HTTP timeout.
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		APIKey:  apiKey,
		Model:   model,
		HTTP:    &http.Client{Timeout: 60 * time.Second},
	}
}

// Complete sends msgs to the chat-completions endpoint and returns the
// assistant's reply text. Errors are wrapped with context but never
// include the request body (which may contain log content) or the API
// key.
func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	if c.APIKey == "" {
		return "", errors.New("chat: API key is not configured")
	}
	if c.Model == "" {
		return "", errors.New("chat: model is not configured")
	}
	if len(msgs) == 0 {
		return "", errors.New("chat: no messages to send")
	}

	reqBody := Request{
		Model:    c.Model,
		Messages: msgs,
		Stream:   false,
	}
	buf, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("chat: marshal request: %w", err)
	}

	url := c.BaseURL + "/chat/completions"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(buf))
	if err != nil {
		return "", fmt.Errorf("chat: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("chat: http request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("chat: read response: %w", err)
	}

	var parsed response
	// Even on error status, try to parse a structured API error.
	_ = json.Unmarshal(body, &parsed)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("chat: %s (status %d)", parsed.Error.Message, resp.StatusCode)
		}
		return "", fmt.Errorf("chat: unexpected status %d", resp.StatusCode)
	}

	if parsed.Error != nil && parsed.Error.Message != "" {
		return "", fmt.Errorf("chat: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", errors.New("chat: response contained no choices")
	}

	return parsed.Choices[0].Message.Content, nil
}
