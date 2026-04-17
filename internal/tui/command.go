package tui

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

type cmdHandler func(m Model, args string) (Model, tea.Cmd, error)

var cmdRegistry = map[string]cmdHandler{
	"chat":   cmdChat,
	"search": cmdSearch,
	"clear":  cmdClear,
	"help":   cmdHelp,
	"model":  cmdModel,
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
		m.cmdErr = "prefix commands with : or /"
		m.cmdInfo = ""
		return m, nil
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
			m.cmdInfo = ie.msg
			m.cmdErr = ""
		} else {
			m.cmdErr = err.Error()
			m.cmdInfo = ""
		}
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

func cmdModel(m Model, args string) (Model, tea.Cmd, error) {
	if args == "" {
		return m, nil, infoErr{"current model: " + m.cfg.Model}
	}
	m.cfg.Model = args
	if m.chatClient != nil {
		m.chatClient.SetModel(args)
	}
	return m, nil, nil
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
	return m
}
