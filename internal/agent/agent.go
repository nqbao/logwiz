package agent

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"
)

// Agent manages one LLM call chain — either a multi-turn sidebar chat or a
// single-shot palette command. It owns the accumulated message history for
// its chain and is safe to use from the Bubble Tea event loop only.
//
// The caller drives the loop:
//  1. Call Step() to fire an async LLM request.
//  2. If StepMsg arrives, execute the tool calls, then call
//     FeedResults(asst, results) and Step() again.
//  3. If DoneMsg arrives, the agent is done.
type Agent struct {
	Client     *Client
	ctx        context.Context
	msgs       []Message  // accumulated LLM message history
	Tools      []ToolSpec // nil disables tool use for the next Step
	toolChoice string     // "required" forces a tool call (cmd agents)
}

// StepMsg is delivered when the LLM requests tool execution.
// The caller must execute Calls, then invoke FeedResults and Step again.
type StepMsg struct {
	Agent *Agent
	Asst  Message    // assistant message carrying the tool calls
	Calls []ToolCall
}

// DoneMsg is delivered when the LLM produces a final text response.
type DoneMsg struct {
	Agent   *Agent
	Content string
}

// ErrMsg is delivered when the LLM call fails.
type ErrMsg struct {
	Agent *Agent
	Err   error
}

// Step fires one async LLM call against the current message history.
func (a *Agent) Step() tea.Cmd {
	client := a.Client
	ctx := a.ctx
	msgs := append([]Message(nil), a.msgs...) // snapshot — no races
	tools := a.Tools
	toolChoice := a.toolChoice
	agent := a
	return func() tea.Msg {
		var (
			reply Message
			err   error
		)
		if toolChoice == "required" {
			tc, e := client.CompleteWithTools(ctx, msgs, tools)
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return ErrMsg{Agent: agent, Err: e}
			}
			reply = Message{Role: "assistant", ToolCalls: []ToolCall{tc}}
		} else {
			reply, err = client.Complete(ctx, msgs, tools)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return ErrMsg{Agent: agent, Err: err}
			}
		}
		if len(reply.ToolCalls) > 0 {
			return StepMsg{Agent: agent, Asst: reply, Calls: reply.ToolCalls}
		}
		return DoneMsg{Agent: agent, Content: reply.Content}
	}
}

// FeedResults appends the assistant tool-call message and caller-supplied
// tool results to the agent's history, readying it for the next Step.
func (a *Agent) FeedResults(asst Message, results []Message) {
	a.msgs = append(a.msgs, asst)
	a.msgs = append(a.msgs, results...)
}
