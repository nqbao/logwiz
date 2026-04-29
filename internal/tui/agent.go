package tui

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/redact"
)

// Type aliases so update.go and tests don't need to import agent directly.
type Agent = agent.Agent
type AgentStepMsg = agent.StepMsg
type AgentDoneMsg = agent.DoneMsg
type AgentErrMsg = agent.ErrMsg

// cmdSampleSize caps the number of log lines captured for the agent system prompt.
const cmdSampleSize = 10

// captureCmdSample picks up to cmdSampleSize lines spread evenly across the buffer
// and returns deep copies so the result is safe to retain past ring-buffer eviction.
// Returns nil if the buffer is empty.
func captureCmdSample(buf *buffer.Buffer) []logmsg.LogMessage {
	n := buf.Len()
	if n == 0 {
		return nil
	}
	step := n / cmdSampleSize
	if step < 1 {
		step = 1
	}
	sample := make([]logmsg.LogMessage, 0, cmdSampleSize)
	for i := 0; i < n && len(sample) < cmdSampleSize; i += step {
		sample = append(sample, buf.Line(i).Copy())
	}
	return sample
}

// formatCmdSample renders the captured sample into the format-awareness block
// included in the agent system prompt. Redaction is applied at render time.
func formatCmdSample(sample []logmsg.LogMessage, r *redact.Redactor) string {
	if len(sample) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Log format samples (NOT comprehensive, NOT necessarily recent — use inspection tools for actual content):\n```\n")
	for _, lm := range sample {
		b.WriteString(r.Redact(lm.Raw))
		b.WriteByte('\n')
	}
	b.WriteString("```")
	return b.String()
}

// startAgentCmd creates a short-lived cmd agent and fires its first Step.
// Prior palette Q&A (m.cmdHistory) is threaded in for conversational follow-ups.
// The log sample is captured once on first use and reused for the run so the
// system-prompt prefix stays stable across turns (helps prompt caching).
func startAgentCmd(m Model, raw string) (Model, tea.Cmd) {
	if m.cmdSample == nil {
		m.cmdSample = captureCmdSample(m.buf)
	}
	ctx := formatCmdSample(m.cmdSample, m.redactor)
	msgs := agent.BuildCmdPrompt(raw, ctx, m.cmdHistory)
	m.cmdPending = raw
	m.cmdAgent = agent.NewCmdAgent(m.chatClient, m.ctx, msgs)
	m.cmdToolBudget = MaxCmdToolTurns
	return m, tea.Batch(m.cmdAgent.Step(), spinnerCmd())
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
	case "find", "count", "tail", "read":
		slog.Debug("cmd agent tool", "tool", call.Name, "args", call.Arguments)
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
	slog.Debug("cmd agent tool", "tool", call.Name, "args", call.Arguments)
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
		return agentKeepPalette(m, fmt.Sprintf("%s: %s", verb, a.Pattern), false), nil

	case "clear_grep":
		m.clearFilter()
		return agentKeepPalette(m, "filters cleared", false), nil

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
		return agentKeepPalette(m, fmt.Sprintf("jump: %s", a.Pattern), false), nil

	case "highlight":
		reStr := a.Pattern
		if a.Flags == "i" {
			reStr = "(?i)" + reStr
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return agentCmdErr(m, "invalid regex: "+err.Error())
		}
		addHL(&m, re)
		return agentKeepPalette(m, fmt.Sprintf("highlight: %s", a.Pattern), false), nil

	case "clear_highlight":
		clearHL(&m)
		return agentKeepPalette(m, "highlights cleared", false), nil

	case "set_model":
		newM, cmd, err := cmdConfig(m, "model "+a.Model)
		if err != nil {
			return agentCmdErr(m, err.Error())
		}
		return agentKeepPalette(newM, "model: "+a.Model, false), cmd

	case "clear_chat":
		m = clearChat(m)
		return agentKeepPalette(m, "chat cleared", false), nil

	case "quit":
		m.Cancel()
		return m, tea.Quit

	case "answer":
		return agentKeepPalette(m, a.Reason, true), nil

	case "reject":
		return agentKeepPalette(m, a.Reason, true), nil

	default:
		return agentCmdErr(m, fmt.Sprintf("unknown pick_tool action: %s", a.Action))
	}
}

// runCmdInspection executes a read-only inspection call (find, count, tail, read) against the buffer.
// Returns the full tool result string for the LLM and a short label for cmdInfo.
func runCmdInspection(m Model, call agent.ToolCall) (result, info string) {
	switch call.Name {
	case "tail":
		return runCmdTail(m, call)
	case "read":
		return runCmdRead(m, call)
	default:
		return runCmdFindCount(m, call)
	}
}

func runCmdTail(m Model, call agent.ToolCall) (result, info string) {
	var a struct {
		N int `json:"n"`
	}
	if err := json.Unmarshal(json.RawMessage(call.Arguments), &a); err != nil {
		return "error: " + err.Error(), "error"
	}
	if a.N < 1 {
		a.N = 1
	}
	if a.N > 200 {
		a.N = 200
	}
	n := m.buf.Len()
	start := n - a.N
	if start < 0 {
		start = 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "tail: %d of %d lines\nlines:\n", n-start, n)
	for i := start; i < n; i++ {
		fmt.Fprintf(&b, "  [line %d] %s\n", i+1, m.buf.Line(i).Raw)
	}
	return b.String(), fmt.Sprintf("reading last %d lines", a.N)
}

func runCmdRead(m Model, call agent.ToolCall) (result, info string) {
	var a struct {
		Start int `json:"start"`
		End   int `json:"end"`
	}
	if err := json.Unmarshal(json.RawMessage(call.Arguments), &a); err != nil {
		return "error: " + err.Error(), "error"
	}
	if a.Start < 1 || a.End < 1 || a.Start > a.End {
		return "error: invalid range (start and end must be ≥1 and start ≤ end)", "error"
	}
	n := m.buf.Len()
	s := a.Start - 1 // convert to 0-based
	e := a.End - 1
	if s >= n {
		return fmt.Sprintf("error: start %d is beyond buffer length %d", a.Start, n), "error"
	}
	if e >= n {
		e = n - 1
	}
	if e-s+1 > 200 {
		e = s + 199
	}
	var b strings.Builder
	fmt.Fprintf(&b, "read: lines %d-%d of %d\nlines:\n", a.Start, e+1, n)
	for i := s; i <= e; i++ {
		fmt.Fprintf(&b, "  [line %d] %s\n", i+1, m.buf.Line(i).Raw)
	}
	return b.String(), fmt.Sprintf("reading lines %d-%d", a.Start, e+1)
}

func runCmdFindCount(m Model, call agent.ToolCall) (result, info string) {
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
	m.cmdAnswer = ""
	return m, nil
}

// agentKeepPalette keeps the palette open after a successful agent command,
// clears the input, shows an informational summary, and records the Q&A pair
// in cmdHistory for conversational follow-ups.
// When isAnswer is true, the info text is shown in readable style (for longer
// natural language responses); otherwise it's dim.
func agentKeepPalette(m Model, info string, isAnswer bool) Model {
	if isAnswer {
		m.cmdAnswer = info
	} else {
		m.cmdInfo = info
	}
	m.cmdErr = ""
	if m.cmdPending != "" {
		m.cmdHistory = append(m.cmdHistory,
			agent.Message{Role: "user", Content: m.cmdPending},
			agent.Message{Role: "assistant", Content: info},
		)
		// Trim oldest pairs to stay within the configured limit.
		max := m.cfg.CmdHistoryTurns * 2
		if len(m.cmdHistory) > max {
			m.cmdHistory = m.cmdHistory[len(m.cmdHistory)-max:]
		}
		m.cmdPending = ""
	}
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
