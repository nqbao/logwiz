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
// Context is rebuilt fresh each invocation.
func startAgentCmd(m Model, raw string) (Model, tea.Cmd) {
	ctx := buildAgentCmdContext(m)
	msgs := agent.BuildCmdPrompt(raw, ctx)
	m.cmdAgent = agent.NewCmdAgent(m.chatClient, m.ctx, msgs)
	m.cmdToolBudget = MaxCmdToolTurns
	return m, m.cmdAgent.Step()
}

// executeCmdTool routes the agent's tool call to the appropriate TUI handler.
// Inspection calls (find, count) feed a result back and keep the loop alive.
// pick_tool commits an action and closes the loop.
func executeCmdTool(m Model, msg AgentStepMsg) (Model, tea.Cmd) {
	if len(msg.Calls) == 0 {
		m.cmdAgent = nil
		return agentCmdErr(m, "agent returned no tool call")
	}
	call := msg.Calls[0]

	// Inspection tools: read-only, feed result back, keep looping.
	switch call.Name {
	case "find", "count":
		result, info := runCmdInspection(m, call)
		toolMsg := agent.Message{
			Role:       "tool",
			ToolCallID: call.ID,
			Name:       call.Name,
			Content:    result,
		}
		m.cmdToolBudget--
		if m.cmdToolBudget <= 0 {
			msg.Agent.Tools = []agent.ToolSpec{agent.CmdCommitToolDef()}
		}
		m.cmdInfo = info
		m.cmdErr = ""
		msg.Agent.FeedResults(msg.Asst, []agent.Message{toolMsg})
		return m, msg.Agent.Step()
	}

	// Commit tool: dispatch action and close the loop.
	m.cmdAgent = nil
	if call.Name != "pick_tool" {
		return agentCmdErr(m, fmt.Sprintf("unknown agent tool: %s", call.Name))
	}

	var a struct {
		Action   string `json:"action"`
		Pattern  string `json:"pattern"`
		Mode     string `json:"mode"`
		Flags    string `json:"flags"`
		Position string `json:"position"`
		Model    string `json:"model"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal(json.RawMessage(call.Arguments), &a); err != nil {
		return agentCmdErr(m, "agent command error: "+err.Error())
	}

	switch a.Action {
	case "grep":
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
		verb := "filter"
		if a.Mode == "exclude" {
			verb = "exclude"
		}
		return agentKeepPalette(m, fmt.Sprintf("%s: %q", verb, a.Pattern)), nil

	case "clear_grep":
		m.clearFilter()
		return agentKeepPalette(m, "filters cleared"), nil

	case "jump":
		reStr := a.Pattern
		if a.Flags == "i" {
			reStr = "(?i)" + reStr
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		if len(m.searchMatches) > 0 && a.Position == "last" {
			m.searchCursor = len(m.searchMatches) - 1
			m.logs.JumpTo(m.searchMatches[m.searchCursor])
		}
		return agentKeepPalette(m, fmt.Sprintf("jump: %q", a.Pattern)), nil

	case "set_model":
		newM, cmd, err := cmdConfig(m, "model "+a.Model)
		if err != nil {
			return agentCmdErr(m, err.Error())
		}
		return agentKeepPalette(newM, "model: "+a.Model), cmd

	case "clear_chat":
		m = clearChat(m)
		return agentKeepPalette(m, "chat cleared"), nil

	case "quit":
		m.Cancel()
		return m, tea.Quit

	case "reject":
		return agentKeepPalette(m, a.Reason), nil

	default:
		return agentCmdErr(m, fmt.Sprintf("unknown pick_tool action: %s", a.Action))
	}
}

// runCmdInspection executes a read-only find or count call against the buffer.
// Returns the full tool result string for the LLM and a short label for cmdInfo.
func runCmdInspection(m Model, call agent.ToolCall) (result, info string) {
	var a struct {
		Pattern string `json:"pattern"`
		Flags   string `json:"flags"`
	}
	if err := json.Unmarshal(json.RawMessage(call.Arguments), &a); err != nil {
		return "error: " + err.Error(), "error"
	}
	reStr := a.Pattern
	if a.Flags == "i" {
		reStr = "(?i)" + reStr
	}
	re, err := regexp.Compile(reStr)
	if err != nil {
		return "error: invalid pattern: " + err.Error(), "error"
	}
	n := m.buf.Len()

	if call.Name == "count" {
		cnt := 0
		for i := 0; i < n; i++ {
			if re.MatchString(m.buf.Line(i).Raw) {
				cnt++
			}
		}
		return fmt.Sprintf("count: %d of %d lines", cnt, n),
			fmt.Sprintf("checking… %d matches", cnt)
	}

	// find: collect count, first/last line, up to 10 samples.
	const maxSamples = 10
	cnt := 0
	firstLine := -1
	lastLine := -1
	var samples []string
	for i := 0; i < n; i++ {
		line := m.buf.Line(i)
		if re.MatchString(line.Raw) {
			cnt++
			if firstLine == -1 {
				firstLine = i + 1 // 1-based
			}
			lastLine = i + 1 // 1-based
			if len(samples) < maxSamples {
				samples = append(samples, fmt.Sprintf("[line %d] %s", i+1, line.Raw))
			}
		}
	}
	info = fmt.Sprintf("checking… %d matches", cnt)
	if cnt == 0 {
		result = fmt.Sprintf("count: 0 of %d lines — no matches", n)
		return
	}
	var b strings.Builder
	fmt.Fprintf(&b, "count: %d of %d lines\nfirst_line: %d\nlast_line: %d\nsamples:\n", cnt, n, firstLine, lastLine)
	for _, s := range samples {
		b.WriteString("  ")
		b.WriteString(s)
		b.WriteByte('\n')
	}
	result = b.String()
	return
}

// agentCmdErr shows an error in the palette without closing it.
func agentCmdErr(m Model, msg string) (Model, tea.Cmd) {
	m.cmdErr = msg
	m.cmdInfo = ""
	return m, nil
}

// agentKeepPalette keeps the palette open after a successful agent command,
// clears the input, and shows an informational summary.
func agentKeepPalette(m Model, info string) Model {
	m.cmdInfo = info
	m.cmdErr = ""
	m.cmdInput.SetValue("")
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
