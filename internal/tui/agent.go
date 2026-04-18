package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/chat"
)

// Agent manages one LLM call chain — either a multi-turn sidebar chat or a
// single-shot palette command. It owns the message history for its chain.
//
// The caller drives the loop:
//  1. Call Step() to fire an async LLM request.
//  2. If AgentStepMsg arrives, execute the tool calls, then call
//     FeedResults(asst, results) and Step() again.
//  3. If AgentDoneMsg arrives, the agent is done.
type Agent struct {
	client     *chat.Client
	ctx        context.Context
	msgs       []chat.Message  // accumulated LLM message history
	tools      []chat.ToolSpec // nil disables tool use for the next Step
	toolChoice string          // "required" forces a tool call (cmd agents)
}

// AgentStepMsg is delivered when the LLM requests tool execution.
// The caller executes Calls, then calls agent.FeedResults and agent.Step.
type AgentStepMsg struct {
	Agent *Agent
	Asst  chat.Message // assistant message carrying the tool calls
	Calls []chat.ToolCall
}

// AgentDoneMsg is delivered when the LLM produces a final text response.
type AgentDoneMsg struct {
	Agent   *Agent
	Content string
}

// AgentErrMsg is delivered when the LLM call fails.
type AgentErrMsg struct {
	Agent *Agent
	Err   error
}

// Step fires one async LLM call against the current message history.
func (a *Agent) Step() tea.Cmd {
	client := a.client
	ctx := a.ctx
	msgs := append([]chat.Message(nil), a.msgs...) // snapshot — no races
	tools := a.tools
	toolChoice := a.toolChoice
	agent := a
	return func() tea.Msg {
		var (
			reply chat.Message
			err   error
		)
		if toolChoice == "required" {
			tc, e := client.CompleteWithTools(ctx, msgs, tools)
			if e != nil {
				if ctx.Err() != nil {
					return nil
				}
				return AgentErrMsg{Agent: agent, Err: e}
			}
			reply = chat.Message{Role: "assistant", ToolCalls: []chat.ToolCall{tc}}
		} else {
			reply, err = client.Complete(ctx, msgs, tools)
			if err != nil {
				if ctx.Err() != nil {
					return nil
				}
				return AgentErrMsg{Agent: agent, Err: err}
			}
		}
		if len(reply.ToolCalls) > 0 {
			return AgentStepMsg{Agent: agent, Asst: reply, Calls: reply.ToolCalls}
		}
		return AgentDoneMsg{Agent: agent, Content: reply.Content}
	}
}

// FeedResults appends the assistant tool-call message and caller-supplied
// tool results to the agent's history, readying it for the next Step.
func (a *Agent) FeedResults(asst chat.Message, results []chat.Message) {
	a.msgs = append(a.msgs, asst)
	a.msgs = append(a.msgs, results...)
}

// newChatAgent creates a long-lived agent for the sidebar chat pipeline.
func newChatAgent(m Model, initialMsgs []chat.Message) *Agent {
	var specs []chat.ToolSpec
	if m.toolsReg != nil {
		specs = m.toolsReg.Specs()
	}
	return &Agent{
		client: m.chatClient,
		ctx:    m.ctx,
		msgs:   initialMsgs,
		tools:  specs,
	}
}

// newCmdAgent creates a short-lived agent for palette agent commands.
func newCmdAgent(m Model, msgs []chat.Message) *Agent {
	return &Agent{
		client:     m.chatClient,
		ctx:        m.ctx,
		msgs:       msgs,
		tools:      agentCmdToolDefs(),
		toolChoice: "required",
	}
}

// ── Agent command: tool definitions, prompt, dispatcher ─────────────────────

// agentCmdToolDefs returns the static tool schema for agent command routing.
func agentCmdToolDefs() []chat.ToolSpec {
	must := func(s string) json.RawMessage { return json.RawMessage(s) }
	return []chat.ToolSpec{
		{
			Name:        "grep",
			Description: "Filter log lines to show only those matching (or not matching) a pattern.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"},
					"mode":    {"type": "string", "enum": ["include", "exclude"]},
					"flags":   {"type": "string", "enum": ["i", ""], "description": "i = case-insensitive"}
				},
				"required": ["pattern", "mode"]
			}`),
		},
		{
			Name:        "clear_grep",
			Description: "Remove all active log filters.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "search",
			Description: "Highlight log lines matching a pattern without hiding others.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern": {"type": "string", "description": "Go regex pattern"}
				},
				"required": ["pattern"]
			}`),
		},
		{
			Name:        "jump",
			Description: "Jump the log viewport to the first or last line matching a pattern.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"pattern":  {"type": "string", "description": "Go regex pattern"},
					"position": {"type": "string", "enum": ["first", "last"]}
				},
				"required": ["pattern", "position"]
			}`),
		},
		{
			Name:        "set_model",
			Description: "Change the AI model used for chat.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"model": {"type": "string", "description": "model name, e.g. gpt-4o"}
				},
				"required": ["model"]
			}`),
		},
		{
			Name:        "clear_chat",
			Description: "Reset the chat conversation history.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "answer",
			Description: "Open the chat sidebar and submit a question about the logs. Use for any analysis or question request.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"question": {"type": "string", "description": "the question to ask about the logs"}
				},
				"required": ["question"]
			}`),
		},
		{
			Name:        "quit",
			Description: "Exit logwiz.",
			Parameters:  must(`{"type":"object","properties":{}}`),
		},
		{
			Name:        "reject",
			Description: "Reject the command when it does not map to any logwiz operation — e.g. random text, gibberish, or requests unrelated to log viewing.",
			Parameters: must(`{
				"type": "object",
				"properties": {
					"reason": {"type": "string", "description": "short explanation shown to the user"}
				},
				"required": ["reason"]
			}`),
		},
	}
}

// buildAgentCmdContext returns a sampled-lines block for the agent system prompt.
// The caller caches the result on m.agentCmdContext.
func buildAgentCmdContext(m Model) string {
	n := m.buf.Len()
	if n == 0 {
		return ""
	}
	const sampleSize = 10
	step := n / sampleSize
	if step < 1 {
		step = 1
	}
	var lines []string
	for i := 0; i < n && len(lines) < sampleSize; i += step {
		lines = append(lines, m.buf.Line(i).Raw)
	}
	var b strings.Builder
	b.WriteString("Log sample (for format awareness):\n```\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteByte('\n')
	}
	b.WriteString("```")
	return b.String()
}

// buildAgentCmdPrompt constructs the message list for an agent command routing call.
func buildAgentCmdPrompt(question, logContext string) []chat.Message {
	sys := "You are a log-viewer assistant. Translate the user's natural-language command into exactly one tool call.\n\n" +
		"Rules:\n" +
		"- Always call exactly one tool.\n" +
		"- Use grep/search/jump to operate on log lines; produce valid Go regex.\n" +
		"- Use answer for any question or analysis request about the logs.\n" +
		"- Call reject when the input is random text, gibberish, or completely unrelated to log viewing.\n"
	if logContext != "" {
		sys += "\n" + logContext
	}
	return []chat.Message{
		{Role: "system", Content: sys},
		{Role: "user", Content: question},
	}
}

// startAgentCmd creates a short-lived cmd agent and fires its first Step.
func startAgentCmd(m Model, raw string) (Model, tea.Cmd) {
	msgs := buildAgentCmdPrompt(raw, m.agentCmdContext)
	m.cmdAgent = newCmdAgent(m, msgs)
	return m, m.cmdAgent.Step()
}

// executeCmdTool routes the agent command's single tool call to the appropriate
// TUI handler. Called from Update when AgentStepMsg arrives for a cmdAgent.
func executeCmdTool(m Model, msg AgentStepMsg) (Model, tea.Cmd) {
	m.cmdAgent = nil
	if len(msg.Calls) == 0 {
		return agentCmdErr(m, "agent returned no tool call")
	}
	call := msg.Calls[0]
	args := json.RawMessage(call.Arguments)

	switch call.Name {
	case "grep":
		var a struct {
			Pattern string `json:"pattern"`
			Mode    string `json:"mode"`
			Flags   string `json:"flags"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		reStr := a.Pattern
		if a.Flags == "i" {
			reStr = "(?i)" + reStr
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		mode := filterInclude
		if a.Mode == "exclude" {
			mode = filterExclude
		}
		addFilter(&m, re, mode)
		return agentClosePalette(m), nil

	case "clear_grep":
		m.clearFilter()
		return agentClosePalette(m), nil

	case "search":
		var a struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		re, err := regexp.Compile(a.Pattern)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		return agentClosePalette(m), nil

	case "jump":
		var a struct {
			Pattern  string `json:"pattern"`
			Position string `json:"position"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		re, err := regexp.Compile(a.Pattern)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		if len(m.searchMatches) > 0 && a.Position == "last" {
			m.searchCursor = len(m.searchMatches) - 1
			m.logs.JumpTo(m.searchMatches[m.searchCursor])
		}
		return agentClosePalette(m), nil

	case "set_model":
		var a struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		newM, cmd, err := cmdConfig(m, "model "+a.Model)
		if err != nil {
			return agentCmdErr(m, err.Error())
		}
		return agentClosePalette(newM), cmd

	case "clear_chat":
		m = clearChat(m)
		return agentClosePalette(m), nil

	case "answer":
		var a struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		m.cmdActive = false
		m.cmdErr = ""
		m.cmdInfo = ""
		m.cmdInput.Blur()
		m.sidebarOpen = true
		m.focus = focusChat
		m.input.SetValue(a.Question)
		m = m.onResize(m.width, m.height)
		model, cmd := m.submitChat()
		return model.(Model), cmd

	case "quit":
		m.Cancel()
		return m, tea.Quit

	case "reject":
		var a struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return agentCmdErr(m, "not a recognized command")
		}
		return agentCmdErr(m, a.Reason)

	default:
		return agentCmdErr(m, fmt.Sprintf("unknown agent tool: %s", call.Name))
	}
}

// agentCmdErr shows an error in the palette without closing it.
func agentCmdErr(m Model, msg string) (Model, tea.Cmd) {
	m.cmdErr = msg
	m.cmdInfo = ""
	return m, nil
}

// agentClosePalette closes the palette after a successful agent command.
func agentClosePalette(m Model) Model {
	m.cmdActive = false
	m.cmdErr = ""
	m.cmdInfo = ""
	m.cmdInput.Blur()
	m = m.onResize(m.width, m.height)
	return m
}

// ── Chat agent: submit, tool loop ────────────────────────────────���───────────

// submitChat starts a new chat agent turn from the sidebar input.
func (m Model) submitChat() (tea.Model, tea.Cmd) {
	q := strings.TrimSpace(m.input.Value())
	if q == "" {
		return m, nil
	}
	if m.chatLoading {
		return m, nil
	}

	if q == "/clear" {
		m.input.Reset()
		m = clearChat(m)
		m.refreshConvoContent()
		return m, nil
	}

	if m.chatClient == nil {
		m.chatErr = "chat is not configured (set OPENAI_API_KEY or --api-key)"
		return m, nil
	}

	// Capture session sample on the very first submit.
	if len(m.chatMsgs) == 0 && m.cfg.ContextLines > 0 {
		recent := m.buf.LastN(m.cfg.ContextLines)
		m.sessionSample = make([]string, len(recent))
		for i, lm := range recent {
			m.sessionSample[i] = lm.Raw
		}
	}

	m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "user", Content: q})
	m.chatErr = ""
	m.chatLoading = true
	m.input.Reset()
	m.hintIdx = (m.hintIdx + 1) % len(inputHints)
	m.input.Placeholder = inputHints[m.hintIdx]

	m.turns = append(m.turns, turnActivity{
		Started:   time.Now(),
		LastLabel: "thinking",
	})
	m.activeTurn = len(m.turns) - 1
	m.toolBudget = MaxToolTurns

	initialMsgs := buildChatMessages(m.cfg.EffectiveSystemPrompt(), m.sessionSample, m.chatMsgs, "")
	m.chatAgent = newChatAgent(m, initialMsgs)

	m.refreshConvoContent()
	return m, tea.Batch(m.chatAgent.Step(), spinnerCmd())
}

// onChatToolCalls is called by Update when the chat agent requests tool execution.
// It runs the tools via toolsReg, feeds results back to the agent, and steps again.
func (m Model) onChatToolCalls(msg AgentStepMsg) (tea.Model, tea.Cmd) {
	m.chatMsgs = append(m.chatMsgs, msg.Asst)

	if m.activeTurn < 0 {
		m.refreshConvoContent()
		return m, msg.Agent.Step()
	}

	ta := &m.turns[m.activeTurn]
	var results []chat.Message
	for _, call := range msg.Calls {
		ta.LastTool = call.Name
		ta.LastLabel = labelFor(call.Name)
		var result string
		if m.toolsReg != nil {
			result = m.toolsReg.Run(call.Name, []byte(call.Arguments), m.buf)
		} else {
			result = "error: tools not configured"
		}
		results = append(results, chat.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    result,
		})
		ta.ToolCount++
	}

	m.toolBudget--
	if m.toolBudget <= 0 && len(results) > 0 {
		results[len(results)-1].Content += "\n\n[Tool budget exhausted — answer with information gathered so far.]"
		msg.Agent.tools = nil // force text response on next step
	}

	m.chatMsgs = append(m.chatMsgs, results...)
	msg.Agent.FeedResults(msg.Asst, results)

	m.refreshConvoContent()
	return m, msg.Agent.Step()
}

// onChatDone is called by Update when the chat agent produces a final answer.
func (m Model) onChatDone(msg AgentDoneMsg) (tea.Model, tea.Cmd) {
	m.chatAgent = nil
	m.chatMsgs = append(m.chatMsgs, chat.Message{Role: "assistant", Content: msg.Content})
	if m.activeTurn >= 0 {
		m.turns[m.activeTurn].Done = true
		m.turns[m.activeTurn].FinishedAt = time.Now()
		m.turnIdxByMsg[len(m.chatMsgs)-1] = m.activeTurn
		m.activeTurn = -1
	}
	m.chatLoading = false
	m.refreshConvoContent()
	return m, nil
}
