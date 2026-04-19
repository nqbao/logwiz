package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/openai/openai-go/packages/param"
	"github.com/openai/openai-go/shared"
)

// thinkingParam is the Anthropic extended-thinking request body field.
type thinkingParam struct {
	Type         string `json:"type"`
	BudgetTokens int    `json:"budget_tokens"`
}

// Client talks to an OpenAI-compatible /chat/completions endpoint.
type Client struct {
	apiKey          string
	model           string
	thinkingBudget  int    // 0 = disabled; >= 1024 enables Anthropic extended thinking
	reasoningEffort string // "", "low", "medium", "high" — OpenAI/Minimax style
	inner           openai.Client
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

// ThinkingBudget returns the current extended-thinking token budget.
func (c *Client) ThinkingBudget() int { return c.thinkingBudget }

// SetThinkingBudget sets the token budget for Anthropic extended thinking.
// Pass 0 to disable, or a value >= 1024 to enable.
func (c *Client) SetThinkingBudget(n int) { c.thinkingBudget = n }

// ReasoningEffort returns the current reasoning effort level.
func (c *Client) ReasoningEffort() string { return c.reasoningEffort }

// SetReasoningEffort sets the reasoning effort for OpenAI/Minimax-style providers.
// Accepted values: "", "low", "medium", "high".
func (c *Client) SetReasoningEffort(v string) { c.reasoningEffort = v }

// thinkingOpts returns request options for reasoning/thinking when configured.
// Only one mode is active: thinking-budget (Anthropic) takes precedence over
// reasoning-effort (OpenAI/Minimax) if both are somehow set.
func (c *Client) thinkingOpts() []option.RequestOption {
	if c.thinkingBudget > 0 {
		return []option.RequestOption{
			option.WithJSONSet("thinking", thinkingParam{
				Type:         "enabled",
				BudgetTokens: c.thinkingBudget,
			}),
		}
	}
	if c.reasoningEffort != "" {
		return []option.RequestOption{
			option.WithJSONSet("reasoning_effort", c.reasoningEffort),
		}
	}
	return nil
}

// CompleteWithTools sends msgs with tool_choice "required", forcing the model
// to call exactly one of the provided tools. Returns the first tool call.
func (c *Client) CompleteWithTools(ctx context.Context, msgs []Message, tools []ToolSpec) (ToolCall, error) {
	if len(tools) == 0 {
		return ToolCall{}, errors.New("chat: no tools provided")
	}
	msg, err := c.complete(ctx, msgs, tools, "required")
	if err != nil {
		return ToolCall{}, err
	}
	if len(msg.ToolCalls) == 0 {
		return ToolCall{}, errors.New("chat: model did not call a tool")
	}
	return msg.ToolCalls[0], nil
}

// Complete sends msgs to the chat-completions endpoint and returns the
// assistant's reply. Pass nil or empty tools when not using function calling.
func (c *Client) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, error) {
	return c.complete(ctx, msgs, tools, "")
}

func (c *Client) complete(ctx context.Context, msgs []Message, tools []ToolSpec, toolChoice string) (Message, error) {
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
	if toolChoice != "" {
		req.ToolChoice = openai.ChatCompletionToolChoiceOptionUnionParam{
			OfAuto: openai.String(toolChoice),
		}
	}

	slog.Debug("chat request", "model", c.model, "msgs", len(msgs), "tools", len(tools),
		"tool_choice", toolChoice, "thinking_budget", c.thinkingBudget, "reasoning_effort", c.reasoningEffort)

	completion, err := c.inner.Chat.Completions.New(ctx, req, c.thinkingOpts()...)
	if err != nil {
		var apiErr *openai.Error
		if errors.As(err, &apiErr) {
			if apiErr.Message != "" {
				slog.Error("chat request failed", "status", apiErr.StatusCode, "msg", apiErr.Message)
				return Message{}, fmt.Errorf("chat: %s (status %d)", apiErr.Message, apiErr.StatusCode)
			}
			slog.Error("chat request failed", "status", apiErr.StatusCode)
			return Message{}, fmt.Errorf("chat: unexpected status %d", apiErr.StatusCode)
		}
		slog.Error("chat request failed", "err", err)
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

	slog.Debug("chat response", "tool_calls", len(out.ToolCalls),
		"prompt_tokens", completion.Usage.PromptTokens,
		"completion_tokens", completion.Usage.CompletionTokens,
		"reasoning_tokens", completion.Usage.CompletionTokensDetails.ReasoningTokens,
		"cached_tokens", completion.Usage.PromptTokensDetails.CachedTokens)

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
