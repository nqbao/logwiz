package tui

// layout describes the rectangular split between the logs pane and
// the (optional) chat sidebar. Heights exclude the status bar.
type layout struct {
	logsWidth    int
	sidebarWidth int
	paneHeight   int // height of logs / sidebar body
}

// compute returns a layout for the given window size and sidebar state.
// A minimum width is enforced so very narrow terminals stay readable;
// extreme edge cases just render whatever fits.
func computeLayout(width, height int, sidebarOpen bool, sidebarPref int) layout {
	// Status bar occupies 1 row.
	paneH := height - 1
	if paneH < 1 {
		paneH = 1
	}

	if !sidebarOpen {
		return layout{logsWidth: width, sidebarWidth: 0, paneHeight: paneH}
	}

	// Sidebar takes min(pref, width/2), but at least 20 cols when the
	// terminal allows it.
	sb := sidebarPref
	if sb > width/2 {
		sb = width / 2
	}
	if sb < 20 && width >= 40 {
		sb = 20
	}
	if sb >= width {
		sb = width - 10
	}
	if sb < 10 {
		sb = 10
	}

	logsW := width - sb - 1 // 1 col for the vertical divider
	if logsW < 10 {
		logsW = 10
	}
	return layout{logsWidth: logsW, sidebarWidth: sb, paneHeight: paneH}
}
