package tui

import "testing"

func TestLayoutSidebarClosed(t *testing.T) {
	l := computeLayout(100, 30, false, 50)
	if l.logsWidth != 100 || l.sidebarWidth != 0 {
		t.Errorf("logs=%d sidebar=%d", l.logsWidth, l.sidebarWidth)
	}
	if l.paneHeight != 29 {
		t.Errorf("paneHeight = %d, want 29", l.paneHeight)
	}
}

func TestLayoutSidebarOpenRespectsPref(t *testing.T) {
	l := computeLayout(120, 40, true, 50)
	if l.sidebarWidth != 50 {
		t.Errorf("sidebarWidth = %d, want 50", l.sidebarWidth)
	}
	if l.logsWidth != 120-50-1 {
		t.Errorf("logsWidth = %d, want %d", l.logsWidth, 120-50-1)
	}
}

func TestLayoutSidebarCappedAtHalf(t *testing.T) {
	l := computeLayout(60, 20, true, 100)
	if l.sidebarWidth > 30 {
		t.Errorf("sidebar = %d exceeds half of 60", l.sidebarWidth)
	}
}

func TestLayoutTinyTerminalDoesNotUnderflow(t *testing.T) {
	l := computeLayout(20, 5, true, 50)
	if l.logsWidth < 10 {
		t.Errorf("logsWidth = %d, want >= 10", l.logsWidth)
	}
	if l.sidebarWidth < 10 {
		t.Errorf("sidebarWidth = %d, want >= 10", l.sidebarWidth)
	}
}

func TestLayoutZeroHeightClamped(t *testing.T) {
	l := computeLayout(80, 0, false, 50)
	if l.paneHeight < 1 {
		t.Errorf("paneHeight = %d, want >= 1", l.paneHeight)
	}
}
