package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/config"
)

type cmdHandler func(m Model, args string) (Model, tea.Cmd, error)

var cmdRegistry = map[string]cmdHandler{
	"chat":   cmdChat,
	"search": cmdSearch,
	"grep":   cmdGrep,
	"clear":  cmdClear,
	"help":   cmdHelp,
	"config": cmdConfig,
	"quit":   cmdQuit,
}

// infoErr is returned by handlers that want to keep the palette open and
// display a dim informational line instead of a red error.
type infoErr struct{ msg string }

func (e infoErr) Error() string { return e.msg }

// dispatchCommand parses raw (the value of cmdInput), resolves the handler,
// and runs it. On success the palette is closed and layout is recomputed.
// On error cmdErr is set and the palette stays open.
// Input must be prefixed with : or / to be treated as a command; unprefixed
// input is reserved for future natural-language handling.
func (m Model) dispatchCommand(raw string) (Model, tea.Cmd) {
	raw = strings.TrimSpace(raw)

	if raw == "" {
		m.cmdActive = false
		m.cmdErr = ""
		m.cmdInfo = ""
		m.cmdInput.Blur()
		m = m.onResize(m.width, m.height)
		return m, nil
	}

	if raw[0] != ':' && raw[0] != '/' {
		// Agent command: route through LLM tool-caller.
		if m.chatClient == nil {
			m.cmdErr = "agent commands require API key (set OPENAI_API_KEY or --api-key)"
			m.cmdInfo = ""
			return m, nil
		}
		if m.agentCmdContext == "" {
			m.agentCmdContext = buildAgentCmdContext(m)
		}
		m.cmdInfo = "thinking…"
		m.cmdErr = ""
		return startAgentCmd(m, raw)
	}
	raw = strings.TrimSpace(raw[1:])

	name, args, _ := strings.Cut(raw, " ")
	name = strings.ToLower(strings.TrimSpace(name))

	if name == "" {
		m.cmdActive = false
		m.cmdErr = ""
		m.cmdInput.Blur()
		m = m.onResize(m.width, m.height)
		return m, nil
	}

	handler, ok := cmdRegistry[name]
	if !ok {
		m.cmdErr = "unknown command: " + name
		return m, nil
	}

	newM, cmd, err := handler(m, strings.TrimSpace(args))
	if err != nil {
		var ie infoErr
		if errors.As(err, &ie) {
			// infoErr keeps the palette open with a dim info line.
			// State changes from the handler are preserved (e.g. :config
			// saves the new value then reports it).
			newM.cmdInfo = ie.msg
			newM.cmdErr = ""
			return newM, cmd
		}
		m.cmdErr = err.Error()
		m.cmdInfo = ""
		return m, nil
	}

	// Close the palette and recompute layout.
	newM.cmdActive = false
	newM.cmdErr = ""
	newM.cmdInfo = ""
	newM.cmdInput.Blur()
	newM = newM.onResize(newM.width, newM.height)
	return newM, cmd
}

func cmdChat(m Model, args string) (Model, tea.Cmd, error) {
	m.sidebarOpen = true
	m.focus = focusChat
	if args != "" {
		m.input.SetValue(args)
	}
	cmd := m.input.Focus()
	return m, cmd, nil
}

func cmdSearch(m Model, args string) (Model, tea.Cmd, error) {
	if args == "" {
		return m, nil, fmt.Errorf("usage: search <regex>")
	}
	re, err := regexp.Compile(args)
	if err != nil {
		return m, nil, err
	}
	runSearch(&m, re)
	return m, nil, nil
}

// cmdGrep implements :grep [flags] [pattern].
// Supported flags (combinable, e.g. -iv):
//
//	-v   exclude mode (hide matching lines)
//	-i   case-insensitive match
//	-F   treat pattern as a fixed string (no regex)
//
// Rules are ANDed: each :grep call narrows the visible set further.
// :grep with no pattern reports the active rule count (esc clears all).
func cmdGrep(m Model, args string) (Model, tea.Cmd, error) {
	mode, caseInsensitive, fixedString, pattern, err := parseGrepFlags(args)
	if err != nil {
		return m, nil, err
	}
	if pattern == "" {
		n := len(m.filterRules)
		if n == 0 {
			return m, nil, infoErr{"no active filter"}
		}
		return m, nil, infoErr{fmt.Sprintf("%d active filter(s) — esc to clear all", n)}
	}

	reStr := pattern
	if fixedString {
		reStr = regexp.QuoteMeta(pattern)
	}
	if caseInsensitive {
		reStr = "(?i)" + reStr
	}
	re, err := regexp.Compile(reStr)
	if err != nil {
		return m, nil, err
	}
	addFilter(&m, re, mode)
	return m, nil, nil
}

// parseGrepFlags parses leading flag tokens from args and returns the
// resolved options plus the remaining pattern string.
// Flags may be combined (-iv) or separate (-i -v). Unknown flag chars
// produce an error.
func parseGrepFlags(args string) (mode filterMode, caseInsensitive, fixedString bool, pattern string, err error) {
	mode = filterInclude
	tokens := strings.Fields(args)
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		if tok == "--" {
			i++ // end of flags; rest is pattern
			break
		}
		if !strings.HasPrefix(tok, "-") || len(tok) == 1 {
			break
		}
		for _, ch := range tok[1:] {
			switch ch {
			case 'v':
				mode = filterExclude
			case 'i':
				caseInsensitive = true
			case 'F':
				fixedString = true
			default:
				err = fmt.Errorf("unknown flag -%c (supported: -v -i -F)", ch)
				return
			}
		}
		i++
	}
	pattern = strings.Join(tokens[i:], " ")
	return
}

func cmdConfig(m Model, args string) (Model, tea.Cmd, error) {
	key, value, hasValue := strings.Cut(args, " ")
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)

	if key == "" {
		return m, nil, fmt.Errorf("usage: config <key> [value]  keys: %s", strings.Join(config.ConfigurableKeys, ", "))
	}

	if !hasValue {
		display, ok := configGetDisplay(m.cfg, key)
		if !ok {
			return m, nil, fmt.Errorf("unknown config key %q (valid: %s)", key, strings.Join(config.ConfigurableKeys, ", "))
		}
		return m, nil, infoErr{key + ": " + display}
	}

	if err := config.ApplyKey(&m.cfg, key, value); err != nil {
		return m, nil, err
	}
	if m.chatClient != nil {
		m.chatClient.SetModel(m.cfg.Model)
		m.chatClient.SetThinkingBudget(m.cfg.ThinkingBudget)
		m.chatClient.SetReasoningEffort(m.cfg.ReasoningEffort)
	}
	if err := config.SaveKey(key, value); err != nil {
		return m, nil, fmt.Errorf("applied in memory but could not persist: %w", err)
	}
	display, _ := configGetDisplay(m.cfg, key)
	return m, nil, infoErr{"saved " + key + ": " + display}
}

func configGetDisplay(cfg config.Config, key string) (string, bool) {
	switch key {
	case "model":
		return cfg.Model, true
	case "api-key":
		if len(cfg.APIKey) > 4 {
			return "***" + cfg.APIKey[len(cfg.APIKey)-4:], true
		}
		return cfg.APIKey, true
	case "base-url":
		return cfg.BaseURL, true
	case "sidebar-width":
		return strconv.Itoa(cfg.SidebarWidth), true
	case "context-lines":
		return strconv.Itoa(cfg.ContextLines), true
	case "system-prompt":
		return cfg.SystemPrompt, true
	case "thinking-budget":
		return strconv.Itoa(cfg.ThinkingBudget), true
	case "reasoning-effort":
		return cfg.ReasoningEffort, true
	}
	return "", false
}

func cmdClear(m Model, _ string) (Model, tea.Cmd, error) {
	return clearChat(m), nil, nil
}

func cmdHelp(m Model, _ string) (Model, tea.Cmd, error) {
	m.help = true
	return m, nil, nil
}

func cmdQuit(m Model, _ string) (Model, tea.Cmd, error) {
	m.Cancel()
	return m, tea.Quit, nil
}

// clearChat resets conversation state. Used by both the keybinding and
// the :clear command so the logic stays in one place.
func clearChat(m Model) Model {
	m.chatMsgs = nil
	m.chatErr = ""
	m.turns = nil
	m.turnIdxByMsg = make(map[int]int)
	m.activeTurn = -1
	m.sessionSample = nil
	m.agentCmdContext = ""
	m.chatAgent = nil
	m.cmdAgent = nil
	return m
}
