package tui

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/config"
)

const memoryUsage = "usage: memory [add <text> | edit | clear]"

// memoryEditedMsg is delivered when the $EDITOR process launched by
// :memory edit exits.
type memoryEditedMsg struct{ err error }

// paletteConfirm is a pending y/n question shown in the command palette.
// While set, the next key answers it: y runs run, anything else cancels.
type paletteConfirm struct {
	run func(m Model) (Model, string)
}

// cmdMemory implements :memory [add <text> | edit | clear].
func cmdMemory(m Model, args string) (Model, tea.Cmd, error) {
	sub, rest, _ := strings.Cut(args, " ")
	rest = strings.TrimSpace(rest)
	switch strings.ToLower(sub) {
	case "", "help":
		return m, nil, infoErr{memorySummary(m.cfg.Memory) + " — :memory add <text> | edit | clear"}
	case "add":
		if rest == "" {
			return m, nil, fmt.Errorf("usage: memory add <text>")
		}
		if err := config.AppendMemory(rest); err != nil {
			return m, nil, err
		}
		if err := reloadMemory(&m); err != nil {
			return m, nil, err
		}
		return m, nil, infoErr{"remembered: " + rest}
	case "edit":
		cmd, err := editMemoryCmd()
		if err != nil {
			return m, nil, err
		}
		return m, cmd, nil
	case "clear":
		if m.cfg.Memory == "" {
			return m, nil, infoErr{"memory is already empty"}
		}
		m.cmdConfirm = &paletteConfirm{run: func(m Model) (Model, string) {
			if err := config.ClearMemory(); err != nil {
				return m, "clear failed: " + err.Error()
			}
			m.cfg.Memory = ""
			return m, "memory cleared"
		}}
		return m, nil, infoErr{"delete " + memorySummary(m.cfg.Memory) + "? (y/n)"}
	default:
		return m, nil, fmt.Errorf(memoryUsage)
	}
}

// memorySummary describes the memory file for one-line palette messages.
func memorySummary(mem string) string {
	if mem == "" {
		return "memory is empty"
	}
	n := 0
	for _, line := range strings.Split(mem, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return fmt.Sprintf("memory: %d line(s) in ~/.logwiz/memory.md", n)
}

// reloadMemory re-reads the memory file into m.cfg so the next agent
// request picks it up.
func reloadMemory(m *Model) error {
	mem, err := config.LoadMemory()
	if err != nil {
		return err
	}
	m.cfg.Memory = mem
	return nil
}

// editMemoryCmd suspends the TUI and opens the memory file in $VISUAL,
// $EDITOR, or vi.
func editMemoryCmd() (tea.Cmd, error) {
	path, err := config.EnsureMemoryFile()
	if err != nil {
		return nil, err
	}
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		editor = "vi"
	}
	argv := append(strings.Fields(editor), path)
	c := exec.Command(argv[0], argv[1:]...)
	return tea.ExecProcess(c, func(err error) tea.Msg { return memoryEditedMsg{err: err} }), nil
}

// onMemoryEdited reloads memory after the editor exits and flashes the
// outcome in the status bar.
func (m Model) onMemoryEdited(msg memoryEditedMsg) (Model, tea.Cmd) {
	switch {
	case msg.err != nil:
		m.yankNotice = "editor failed: " + msg.err.Error()
	default:
		if err := reloadMemory(&m); err != nil {
			m.yankNotice = err.Error()
		} else {
			m.yankNotice = "memory reloaded"
		}
	}
	return m, tea.Tick(3*time.Second, func(time.Time) tea.Msg { return yankNoticeClearMsg{} })
}
