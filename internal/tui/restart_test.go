package tui

import (
	"strings"
	"testing"

	"github.com/nqbao/logwiz/internal/logmsg"
	"github.com/nqbao/logwiz/internal/source"
)

type fakeRestarter struct{ n int }

func (f *fakeRestarter) Restart() { f.n++ }

func TestRestartUnavailableWithoutRestarter(t *testing.T) {
	m := newTestModel(t)
	m.cmdActive = true
	m, _ = m.dispatchCommand(":restart")
	if !strings.Contains(m.cmdErr, "only available") {
		t.Fatalf("cmdErr = %q, want unavailable error", m.cmdErr)
	}
}

func TestRestartCallsRestarterAndResetsEOF(t *testing.T) {
	fr := &fakeRestarter{}
	m := newTestModel(t).WithRestarter(fr)
	m.eof = true
	m.chatErr = "source error: exit status 1"
	m.cmdActive = true
	m, _ = m.dispatchCommand(":restart")
	if m.cmdErr != "" || fr.n != 1 {
		t.Fatalf("cmdErr=%q restarts=%d", m.cmdErr, fr.n)
	}
	mi, _ := m.Update(source.RestartingMsg{})
	m = mi.(Model)
	if m.eof || m.chatErr != "" {
		t.Fatalf("eof=%v chatErr=%q after RestartingMsg", m.eof, m.chatErr)
	}
}

func TestRestartClearClearsBufferOnRestarting(t *testing.T) {
	fr := &fakeRestarter{}
	m := newTestModel(t).WithRestarter(fr)
	mi, _ := m.Update(source.NewLinesMsg{Lines: []logmsg.LogMessage{{Raw: "a"}, {Raw: "b"}}})
	m = mi.(Model)
	m.cmdActive = true
	m, _ = m.dispatchCommand(":restart -c")
	if m.buf.Len() != 2 {
		t.Fatalf("buffer cleared too early: len=%d", m.buf.Len())
	}
	mi, _ = m.Update(source.RestartingMsg{})
	m = mi.(Model)
	if m.buf.Len() != 0 || m.restartClear {
		t.Fatalf("len=%d restartClear=%v", m.buf.Len(), m.restartClear)
	}
}
