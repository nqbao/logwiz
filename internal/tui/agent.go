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

// AgentCmdMsg carries the LLM's chosen tool back to the update loop.
type AgentCmdMsg struct {
	Name string
	Args json.RawMessage
}

// AgentCmdErrMsg carries an agent command failure.
type AgentCmdErrMsg struct{ Err error }

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

// buildAgentCmdContext returns a sampled-lines block used in the agent system prompt.
// The caller is responsible for caching the result on m.agentCmdContext.
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

// dispatchAgentCmd fires an async LLM call and returns AgentCmdMsg or AgentCmdErrMsg.
func dispatchAgentCmd(m Model, raw string) tea.Cmd {
	ctx := m.ctx
	client := m.chatClient
	tools := agentCmdToolDefs()
	msgs := buildAgentCmdPrompt(raw, m.agentCmdContext)
	return func() tea.Msg {
		tc, err := client.CompleteWithTools(ctx, msgs, tools)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return AgentCmdErrMsg{Err: err}
		}
		return AgentCmdMsg{Name: tc.Name, Args: json.RawMessage(tc.Arguments)}
	}
}

// dispatchAgentCmdToolCall routes the LLM's chosen tool to the appropriate handler.
func dispatchAgentCmdToolCall(m Model, msg AgentCmdMsg) (Model, tea.Cmd) {
	switch msg.Name {
	case "grep":
		var args struct {
			Pattern string `json:"pattern"`
			Mode    string `json:"mode"`
			Flags   string `json:"flags"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		reStr := args.Pattern
		if args.Flags == "i" {
			reStr = "(?i)" + reStr
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		mode := filterInclude
		if args.Mode == "exclude" {
			mode = filterExclude
		}
		addFilter(&m, re, mode)
		return agentClosePalette(m), nil

	case "clear_grep":
		m.clearFilter()
		return agentClosePalette(m), nil

	case "search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		return agentClosePalette(m), nil

	case "jump":
		var args struct {
			Pattern  string `json:"pattern"`
			Position string `json:"position"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		if len(m.searchMatches) > 0 && args.Position == "last" {
			m.searchCursor = len(m.searchMatches) - 1
			m.logs.JumpTo(m.searchMatches[m.searchCursor])
		}
		return agentClosePalette(m), nil

	case "set_model":
		var args struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		newM, cmd, err := cmdConfig(m, "model "+args.Model)
		if err != nil {
			return agentCmdErr(m, err.Error())
		}
		return agentClosePalette(newM), cmd

	case "clear_chat":
		m = clearChat(m)
		return agentClosePalette(m), nil

	case "answer":
		var args struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "agent command error: "+err.Error())
		}
		m.cmdActive = false
		m.cmdErr = ""
		m.cmdInfo = ""
		m.agentCmdLoading = false
		m.cmdInput.Blur()
		m.sidebarOpen = true
		m.focus = focusChat
		m.input.SetValue(args.Question)
		m = m.onResize(m.width, m.height)
		model, cmd := m.submitChat()
		return model.(Model), cmd

	case "quit":
		m.Cancel()
		return m, tea.Quit

	case "reject":
		var args struct {
			Reason string `json:"reason"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return agentCmdErr(m, "not a recognized command")
		}
		return agentCmdErr(m, args.Reason)

	default:
		return agentCmdErr(m, fmt.Sprintf("unknown agent tool: %s", msg.Name))
	}
}

// agentCmdErr sets a command error and resets agent loading state without
// closing the palette so the user can see the message.
func agentCmdErr(m Model, msg string) (Model, tea.Cmd) {
	m.cmdErr = msg
	m.cmdInfo = ""
	m.agentCmdLoading = false
	return m, nil
}

// agentClosePalette closes the command palette after a successful agent tool dispatch.
func agentClosePalette(m Model) Model {
	m.cmdActive = false
	m.cmdErr = ""
	m.cmdInfo = ""
	m.agentCmdLoading = false
	m.cmdInput.Blur()
	m = m.onResize(m.width, m.height)
	return m
}

// submitChat takes the current input, validates it, and kicks off the chat agent loop.
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

	m.refreshConvoContent()
	return m, tea.Batch(m.runChatStep(), spinnerCmd())
}

// runChatStep fires an async chat request using the current chatMsgs.
func (m Model) runChatStep() tea.Cmd {
	msgs := buildChatMessages(
		m.cfg.EffectiveSystemPrompt(),
		m.sessionSample,
		m.chatMsgs,
		"",
	)

	var specs []chat.ToolSpec
	if m.toolsReg != nil && m.toolBudget > 0 {
		specs = m.toolsReg.Specs()
	}

	client := m.chatClient
	ctx := m.ctx
	return func() tea.Msg {
		reply, err := client.Complete(ctx, msgs, specs)
		if err != nil {
			if ctx.Err() == context.Canceled {
				return nil
			}
			return ChatErrMsg{Err: err}
		}
		return ChatStepMsg{Assistant: reply}
	}
}

// onChatStep processes one assistant response in the chat agent loop.
func (m Model) onChatStep(asst chat.Message) (tea.Model, tea.Cmd) {
	m.chatMsgs = append(m.chatMsgs, asst)

	if len(asst.ToolCalls) == 0 {
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

	if m.activeTurn >= 0 {
		ta := &m.turns[m.activeTurn]
		for _, call := range asst.ToolCalls {
			ta.LastTool = call.Name
			ta.LastLabel = labelFor(call.Name)
			var result string
			if m.toolsReg != nil {
				result = m.toolsReg.Run(call.Name, []byte(call.Arguments), m.buf)
			} else {
				result = "error: tools not configured"
			}
			m.chatMsgs = append(m.chatMsgs, chat.Message{
				Role:       "tool",
				ToolCallID: call.ID,
				Name:       call.Name,
				Content:    result,
			})
			ta.ToolCount++
		}

		m.toolBudget--
		if m.toolBudget <= 0 {
			if len(m.chatMsgs) > 0 {
				last := &m.chatMsgs[len(m.chatMsgs)-1]
				if last.Role == "tool" {
					last.Content += "\n\n[Tool budget exhausted — answer with information gathered so far.]"
				}
			}
		}
	}

	m.refreshConvoContent()
	return m, m.runChatStep()
}
