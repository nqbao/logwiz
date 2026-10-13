package tui

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/nqbao/logwiz/internal/config"
)

func key(s string) tea.KeyMsg {
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestMemoryAddReloadsAndStaysOpen(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true

	m2, _ := m.dispatchCommand(":memory add 502s on /checkout during deploy are expected")
	if !m2.cmdActive {
		t.Fatal("palette should stay open after :memory add")
	}
	if !strings.HasPrefix(m2.cmdInfo, "remembered:") {
		t.Errorf("cmdInfo = %q", m2.cmdInfo)
	}
	if m2.cfg.Memory != "- 502s on /checkout during deploy are expected" {
		t.Errorf("cfg.Memory = %q", m2.cfg.Memory)
	}
	if !strings.Contains(m2.cfg.EffectiveSystemPrompt(), "/checkout") {
		t.Error("memory not in system prompt after add")
	}
}

func TestMemoryNoArgsShowsUsage(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m2, _ := m.dispatchCommand(":memory")
	if !strings.Contains(m2.cmdInfo, "memory is empty") || !strings.Contains(m2.cmdInfo, ":memory add") {
		t.Errorf("cmdInfo = %q", m2.cmdInfo)
	}
	m2, _ = m.dispatchCommand(":memory bogus")
	if m2.cmdErr == "" {
		t.Error("expected usage error for unknown subcommand")
	}
}

func TestMemoryClearConfirm(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m, _ = m.dispatchCommand(":memory add one")

	// "n" cancels and keeps the memory.
	m, _ = m.dispatchCommand(":memory clear")
	if m.cmdConfirm == nil || !strings.Contains(m.cmdInfo, "(y/n)") {
		t.Fatalf("expected confirm prompt, got %q", m.cmdInfo)
	}
	mi, _ := m.Update(key("n"))
	m = mi.(Model)
	if m.cmdInfo != "cancelled" || m.cfg.Memory == "" {
		t.Fatalf("after n: info=%q memory=%q", m.cmdInfo, m.cfg.Memory)
	}
	if mem, _ := config.LoadMemory(); mem == "" {
		t.Fatal("file cleared despite cancel")
	}

	// "y" clears.
	m, _ = m.dispatchCommand(":memory clear")
	mi, _ = m.Update(key("y"))
	m = mi.(Model)
	if m.cmdInfo != "memory cleared" || m.cfg.Memory != "" || m.cmdConfirm != nil {
		t.Fatalf("after y: info=%q memory=%q", m.cmdInfo, m.cfg.Memory)
	}
	if mem, _ := config.LoadMemory(); mem != "" {
		t.Fatalf("file not cleared: %q", mem)
	}
	if got := m.cmdInput.Value(); got != "" {
		t.Errorf("confirm key leaked into input: %q", got)
	}
}

func TestMemoryClearWhenEmpty(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m2, _ := m.dispatchCommand(":memory clear")
	if m2.cmdConfirm != nil || m2.cmdInfo != "memory is already empty" {
		t.Errorf("info=%q confirm=%v", m2.cmdInfo, m2.cmdConfirm)
	}
}

func TestMemoryEditReturnsExecAndReloads(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	t.Setenv("VISUAL", "true")
	m2, cmd := m.dispatchCommand(":memory edit")
	if cmd == nil {
		t.Fatal("expected an exec command")
	}
	if m2.cmdActive {
		t.Error("palette should close for :memory edit")
	}

	// Simulate the editor having written the file.
	if err := config.AppendMemory("from editor"); err != nil {
		t.Fatal(err)
	}
	mi, _ := m2.Update(memoryEditedMsg{})
	m3 := mi.(Model)
	if m3.cfg.Memory != "- from editor" || m3.yankNotice != "memory reloaded" {
		t.Errorf("memory=%q notice=%q", m3.cfg.Memory, m3.yankNotice)
	}
}
