// Package chat implements a minimal OpenAI-compatible chat-completions
// client backed by the official openai-go SDK.
package chat

import (
	"context"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// Client talks to an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	apiKey string
	model  string
	inner  openai.Client
}

// New returns a Client backed by the official openai-go SDK.
func New(baseURL, apiKey, model string) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		inner: openai.NewClient(
			option.WithAPIKey(apiKey),
			option.WithBaseURL(baseURL),
			option.WithMaxRetries(0),
		),
	}
}

// Complete sends msgs to the chat-completions endpoint and returns the
// assistant's reply text.
func (c *Client) Complete(ctx context.Context, msgs []Message) (string, error) {
	if c.apiKey == "" {
		return "", errors.New("chat: API key is not configured")
	}
	if c.model == "" {
		return "", errors.New("chat: model is not configured")
	}
	if len(msgs) == 0 {
		return "", errors.New("chat: no messages to send")
	}

	completion, err := c.inner.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: toSDKMessages(msgs),
	})
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Message != "" {
				return "", fmt.Errorf("chat: %s (status %d)", apiErr.Message, apiErr.StatusCode)
			}
			return "", fmt.Errorf("chat: unexpected status %d", apiErr.StatusCode)
		}
		return "", fmt.Errorf("chat: %w", err)
	}

	if len(completion.Choices) == 0 {
		return "", errors.New("chat: response contained no choices")
	}

	return completion.Choices[0].Message.Content, nil
}

func toSDKMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, len(msgs))
	for i, m := range msgs {
		switch m.Role {
		case "system":
			out[i] = openai.SystemMessage(m.Content)
		case "assistant":
			out[i] = openai.AssistantMessage(m.Content)
		default:
			out[i] = openai.UserMessage(m.Content)
		}
	}
	return out
}
