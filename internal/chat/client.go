// Package chat implements a minimal OpenAI-compatible chat-completions
// client backed by the official openai-go SDK.
package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
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

// Model returns the model name currently in use.
func (c *Client) Model() string { return c.model }

// SetModel updates the model used for subsequent requests.
func (c *Client) SetModel(model string) { c.model = model }

// CompleteWithTools sends msgs with tool_choice "required", forcing the model
// to call exactly one of the provided tools. Returns the first tool call.
func (c *Client) CompleteWithTools(ctx context.Context, msgs []Message, tools []ToolSpec) (ToolCall, error) {
	if c.apiKey == "" {
		return ToolCall{}, errors.New("chat: API key is not configured")
	}
	if c.model == "" {
		return ToolCall{}, errors.New("chat: model is not configured")
	}
	if len(msgs) == 0 {
		return ToolCall{}, errors.New("chat: no messages to send")
	}
	if len(tools) == 0 {
		return ToolCall{}, errors.New("chat: no tools provided")
	}

	req := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: toSDKMessages(msgs),
		Tools:    toSDKTools(tools),
		ToolChoice: openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String("required"),
		},
	}

	completion, err := c.inner.Chat.Completions.New(ctx, req)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Message != "" {
				return ToolCall{}, fmt.Errorf("chat: %s (status %d)", apiErr.Message, apiErr.StatusCode)
			}
			return ToolCall{}, fmt.Errorf("chat: unexpected status %d", apiErr.StatusCode)
		}
		return ToolCall{}, fmt.Errorf("chat: %w", err)
	}

	if len(completion.Choices) == 0 {
		return ToolCall{}, errors.New("chat: response contained no choices")
	}

	tcs := completion.Choices[0].Message.ToolCalls
	if len(tcs) == 0 {
		return ToolCall{}, errors.New("chat: model did not call a tool")
	}

	return ToolCall{
		ID:        tcs[0].ID,
		Name:      tcs[0].Function.Name,
		Arguments: tcs[0].Function.Arguments,
	}, nil
}

// Complete sends msgs to the chat-completions endpoint and returns the
// assistant's reply as a Message (with ToolCalls populated when the model
// requests tool use). Pass nil or an empty slice for tools when not using
// function calling.
func (c *Client) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, error) {
	if c.apiKey == "" {
		return Message{}, errors.New("chat: API key is not configured")
	}
	if c.model == "" {
		return Message{}, errors.New("chat: model is not configured")
	}
	if len(msgs) == 0 {
		return Message{}, errors.New("chat: no messages to send")
	}

	req := openai.ChatCompletionNewParams{
		Model:    openai.ChatModel(c.model),
		Messages: toSDKMessages(msgs),
	}
	if len(tools) > 0 {
		req.Tools = toSDKTools(tools)
	}

	completion, err := c.inner.Chat.Completions.New(ctx, req)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Message != "" {
				return Message{}, fmt.Errorf("chat: %s (status %d)", apiErr.Message, apiErr.StatusCode)
			}
			return Message{}, fmt.Errorf("chat: unexpected status %d", apiErr.StatusCode)
		}
		return Message{}, fmt.Errorf("chat: %w", err)
	}

	if len(completion.Choices) == 0 {
		return Message{}, errors.New("chat: response contained no choices")
	}

	choice := completion.Choices[0].Message
	out := Message{
		Role:    "assistant",
		Content: choice.Content,
	}
	for _, tc := range choice.ToolCalls {
		out.ToolCalls = append(out.ToolCalls, ToolCall{
			ID:        tc.ID,
			Name:      tc.Function.Name,
			Arguments: tc.Function.Arguments,
		})
	}
	return out, nil
}

func toSDKMessages(msgs []Message) []openai.ChatCompletionMessageParamUnion {
	out := make([]openai.ChatCompletionMessageParamUnion, len(msgs))
	for i, m := range msgs {
		switch m.Role {
		case "system":
			out[i] = openai.SystemMessage(m.Content)
		case "assistant":
			if len(m.ToolCalls) > 0 {
				p := openai.ChatCompletionAssistantMessageParam{}
				if m.Content != "" {
					p.Content = openai.ChatCompletionAssistantMessageParamContentUnion{
						OfString: param.NewOpt(m.Content),
					}
				}
				p.ToolCalls = make([]openai.ChatCompletionMessageToolCallParam, len(m.ToolCalls))
				for j, tc := range m.ToolCalls {
					p.ToolCalls[j] = openai.ChatCompletionMessageToolCallParam{
						ID: tc.ID,
						Function: openai.ChatCompletionMessageToolCallFunctionParam{
							Name:      tc.Name,
							Arguments: tc.Arguments,
						},
					}
				}
				out[i] = openai.ChatCompletionMessageParamUnion{OfAssistant: &p}
			} else {
				out[i] = openai.AssistantMessage(m.Content)
			}
		case "tool":
			out[i] = openai.ToolMessage(m.Content, m.ToolCallID)
		default:
			out[i] = openai.UserMessage(m.Content)
		}
	}
	return out
}

func toSDKTools(specs []ToolSpec) []openai.ChatCompletionToolParam {
	out := make([]openai.ChatCompletionToolParam, len(specs))
	for i, s := range specs {
		var fp shared.FunctionParameters
		if len(s.Parameters) > 0 {
			_ = json.Unmarshal(s.Parameters, &fp)
		}
		out[i] = openai.ChatCompletionToolParam{
			Function: shared.FunctionDefinitionParam{
				Name:        s.Name,
				Description: param.NewOpt(s.Description),
				Parameters:  fp,
			},
		}
	}
	return out
}
