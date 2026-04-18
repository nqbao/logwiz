package tui

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/chat"
)

// AIToolCallMsg carries the LLM's chosen tool back to the update loop.
type AIToolCallMsg struct {
	Name string
	Args json.RawMessage
}

// AIErrMsg carries an AI command failure.
type AIErrMsg struct{ Err error }

// aiToolDefs returns the static tool schema for AI command routing.
func aiToolDefs() []chat.ToolSpec {
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

// buildAIContext returns a sampled-lines block used in the AI system prompt.
// The caller is responsible for caching the result on m.aiContext.
func buildAIContext(m Model) string {
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

// buildAIPrompt constructs the message list for an AI command routing call.
func buildAIPrompt(question, logContext string) []chat.Message {
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

// dispatchAICommand fires an async LLM call and returns AIToolCallMsg or AIErrMsg.
func dispatchAICommand(m Model, raw string) tea.Cmd {
	ctx := m.ctx
	client := m.chatClient
	tools := aiToolDefs()
	msgs := buildAIPrompt(raw, m.aiContext)
	return func() tea.Msg {
		tc, err := client.CompleteWithTools(ctx, msgs, tools)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return AIErrMsg{Err: err}
		}
		return AIToolCallMsg{Name: tc.Name, Args: json.RawMessage(tc.Arguments)}
	}
}

// dispatchAIToolCall routes the LLM's chosen tool to the appropriate handler.
func dispatchAIToolCall(m Model, msg AIToolCallMsg) (Model, tea.Cmd) {
	switch msg.Name {
	case "grep":
		var args struct {
			Pattern string `json:"pattern"`
			Mode    string `json:"mode"`
			Flags   string `json:"flags"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return aiCmdErr(m, "AI command error: "+err.Error())
		}
		reStr := args.Pattern
		if args.Flags == "i" {
			reStr = "(?i)" + reStr
		}
		re, err := regexp.Compile(reStr)
		if err != nil {
			return aiCmdErr(m, "invalid regex: "+err.Error())
		}
		mode := filterInclude
		if args.Mode == "exclude" {
			mode = filterExclude
		}
		addFilter(&m, re, mode)
		return aiClosePalette(m), nil

	case "clear_grep":
		m.clearFilter()
		return aiClosePalette(m), nil

	case "search":
		var args struct {
			Pattern string `json:"pattern"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return aiCmdErr(m, "AI command error: "+err.Error())
		}
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return aiCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		return aiClosePalette(m), nil

	case "jump":
		var args struct {
			Pattern  string `json:"pattern"`
			Position string `json:"position"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return aiCmdErr(m, "AI command error: "+err.Error())
		}
		re, err := regexp.Compile(args.Pattern)
		if err != nil {
			return aiCmdErr(m, "invalid regex: "+err.Error())
		}
		runSearch(&m, re)
		if len(m.searchMatches) > 0 && args.Position == "last" {
			m.searchCursor = len(m.searchMatches) - 1
			m.logs.JumpTo(m.searchMatches[m.searchCursor])
		}
		return aiClosePalette(m), nil

	case "set_model":
		var args struct {
			Model string `json:"model"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return aiCmdErr(m, "AI command error: "+err.Error())
		}
		newM, cmd, err := cmdConfig(m, "model "+args.Model)
		if err != nil {
			return aiCmdErr(m, err.Error())
		}
		return aiClosePalette(newM), cmd

	case "clear_chat":
		m = clearChat(m)
		return aiClosePalette(m), nil

	case "answer":
		var args struct {
			Question string `json:"question"`
		}
		if err := json.Unmarshal(msg.Args, &args); err != nil {
			return aiCmdErr(m, "AI command error: "+err.Error())
		}
		m.cmdActive = false
		m.cmdErr = ""
		m.cmdInfo = ""
		m.aiLoading = false
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
			return aiCmdErr(m, "not a recognized command")
		}
		return aiCmdErr(m, args.Reason)

	default:
		return aiCmdErr(m, fmt.Sprintf("unknown AI tool: %s", msg.Name))
	}
}

// aiCmdErr sets a command error and resets AI loading state without closing
// the palette so the user can see the message.
func aiCmdErr(m Model, msg string) (Model, tea.Cmd) {
	m.cmdErr = msg
	m.cmdInfo = ""
	m.aiLoading = false
	return m, nil
}

// aiClosePalette closes the command palette after a successful AI tool dispatch.
func aiClosePalette(m Model) Model {
	m.cmdActive = false
	m.cmdErr = ""
	m.cmdInfo = ""
	m.aiLoading = false
	m.cmdInput.Blur()
	m = m.onResize(m.width, m.height)
	return m
}
