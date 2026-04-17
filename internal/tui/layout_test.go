package tui

import "testing"

func TestLayoutSidebarClosed(t *testing.T) {
	l := computeLayout(100, 30, 0, false, 50)
	if l.logsWidth != 100 || l.sidebarWidth != 0 {
		t.Errorf("logs=%d sidebar=%d", l.logsWidth, l.sidebarWidth)
	}
	if l.paneHeight != 29 {
		t.Errorf("paneHeight = %d, want 29", l.paneHeight)
	}
}

func TestLayoutSidebarOpenRespectsPref(t *testing.T) {
	l := computeLayout(120, 40, 0, true, 50)
	if l.sidebarWidth != 50 {
		t.Errorf("sidebarWidth = %d, want 50", l.sidebarWidth)
	}
	if l.logsWidth != 120-50-1 {
		t.Errorf("logsWidth = %d, want %d", l.logsWidth, 120-50-1)
	}
}

func TestLayoutSidebarCappedAtHalf(t *testing.T) {
	l := computeLayout(60, 20, 0, true, 100)
	if l.sidebarWidth > 30 {
		t.Errorf("sidebar = %d exceeds half of 60", l.sidebarWidth)
	}
}

func TestLayoutTinyTerminalDoesNotUnderflow(t *testing.T) {
	l := computeLayout(20, 5, 0, true, 50)
	if l.logsWidth < 10 {
		t.Errorf("logsWidth = %d, want >= 10", l.logsWidth)
	}
	if l.sidebarWidth < 10 {
		t.Errorf("sidebarWidth = %d, want >= 10", l.sidebarWidth)
	}
}

func TestLayoutZeroHeightClamped(t *testing.T) {
	l := computeLayout(80, 0, 0, false, 50)
	if l.paneHeight < 1 {
		t.Errorf("paneHeight = %d, want >= 1", l.paneHeight)
	}
}

func TestLayoutCmdPaletteSubtractsRows(t *testing.T) {
	// Without palette: paneHeight = 30 - 1 = 29
	l0 := computeLayout(80, 30, 0, false, 50)
	// With palette (3 rows): paneHeight = 30 - 1 - 3 = 26
	l3 := computeLayout(80, 30, 3, false, 50)
	if l3.paneHeight != l0.paneHeight-3 {
		t.Errorf("cmdRows=3: paneHeight=%d, want %d", l3.paneHeight, l0.paneHeight-3)
	}
	// With palette + error (4 rows): paneHeight = 30 - 1 - 4 = 25
	l4 := computeLayout(80, 30, 4, false, 50)
	if l4.paneHeight != l0.paneHeight-4 {
		t.Errorf("cmdRows=4: paneHeight=%d, want %d", l4.paneHeight, l0.paneHeight-4)
	}
}
