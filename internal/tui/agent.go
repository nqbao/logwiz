package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/agent"
)

// Type aliases so update.go and tests don't need to import agent directly.
type Agent = agent.Agent
type AgentStepMsg = agent.StepMsg
type AgentDoneMsg = agent.DoneMsg
type AgentErrMsg = agent.ErrMsg

// buildAgentCmdContext returns a sampled-lines block for the agent system prompt.
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
		lines = append(lines, m.redactor.Redact(m.buf.Line(i).Raw))
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

// startAgentCmd creates a short-lived cmd agent and fires its first Step.
func startAgentCmd(m Model, raw string) (Model, tea.Cmd) {
	msgs := agent.BuildCmdPrompt(raw, m.agentCmdContext)
	m.cmdAgent = agent.NewCmdAgent(m.chatClient, m.ctx, msgs)
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
		focusCmd := m.input.Focus()
		model, cmd := m.submitChat()
		return model.(Model), tea.Batch(focusCmd, cmd)

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
			m.sessionSample[i] = m.redactor.Redact(lm.Raw)
		}
	}

	m.chatMsgs = append(m.chatMsgs, agent.Message{Role: "user", Content: q})
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

	var specs []agent.ToolSpec
	if m.toolsReg != nil {
		specs = m.toolsReg.Specs()
	}
	initialMsgs := agent.BuildChatMessages(m.cfg.EffectiveSystemPrompt(), m.sessionSample, m.chatMsgs, "")
	m.chatAgent = agent.NewChatAgent(m.chatClient, m.ctx, specs, initialMsgs)

	m.refreshConvoContent()
	return m, tea.Batch(m.chatAgent.Step(), spinnerCmd())
}

// onChatToolCalls is called by Update when the chat agent requests tool execution.
func (m Model) onChatToolCalls(msg AgentStepMsg) (tea.Model, tea.Cmd) {
	m.chatMsgs = append(m.chatMsgs, msg.Asst)

	if m.activeTurn < 0 {
		m.refreshConvoContent()
		return m, msg.Agent.Step()
	}

	ta := &m.turns[m.activeTurn]
	var results []agent.Message
	for _, call := range msg.Calls {
		ta.LastTool = call.Name
		ta.LastLabel = labelFor(call.Name)
		var result string
		if m.toolsReg != nil {
			result = m.toolsReg.Run(call.Name, []byte(call.Arguments), m.buf)
		} else {
			result = "error: tools not configured"
		}
		results = append(results, agent.Message{
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
		msg.Agent.Tools = nil // force text response on next step
	}

	m.chatMsgs = append(m.chatMsgs, results...)
	msg.Agent.FeedResults(msg.Asst, results)

	m.refreshConvoContent()
	return m, msg.Agent.Step()
}

// onChatDone is called by Update when the chat agent produces a final answer.
func (m Model) onChatDone(msg AgentDoneMsg) (tea.Model, tea.Cmd) {
	m.chatAgent = nil
	m.chatMsgs = append(m.chatMsgs, agent.Message{Role: "assistant", Content: msg.Content})
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
