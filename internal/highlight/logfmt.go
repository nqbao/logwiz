package highlight

import (
	"io"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/go-logfmt/logfmt"
	"github.com/muesli/termenv"
)

// lfr forces TrueColor so ANSI codes appear whether or not stdout is a TTY.
// SetColorProfile must be used; NewRenderer ignores termenv.WithProfile for
// the lipgloss color profile (it uses EnvColorProfile lazily instead).
var lfr = func() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return r
}()

var (
	// Keys and = are intentionally subdued so they don't compete with JSON/monokai.
	lfKey = lfr.NewStyle().Foreground(lipgloss.Color("240")) // dim gray
	lfEq  = lfr.NewStyle().Foreground(lipgloss.Color("238")) // dimmer gray

	// Only level severity and numeric values get distinct color.
	lfNum   = lfr.NewStyle().Foreground(lipgloss.Color("179")) // soft yellow — matches monokai numbers
	lfError = lfr.NewStyle().Foreground(lipgloss.Color("203")).Bold(true)
	lfWarn  = lfr.NewStyle().Foreground(lipgloss.Color("214")).Bold(true)
	lfInfo  = lfr.NewStyle().Foreground(lipgloss.Color("111"))
	lfDebug = lfr.NewStyle().Faint(true)
)

// LogfmtRenderer colourises a logfmt line. Keys and = are dim so the values
// stand out; level severity is colour-coded; numeric values are soft yellow.
// All other values are rendered in the default foreground colour. Falls back
// to raw text when the line has fewer than two key=value pairs or cannot be
// decoded.
func LogfmtRenderer(text string) string {
	if strings.Count(text, "=") < 2 {
		return text
	}
	dec := logfmt.NewDecoder(strings.NewReader(text))
	if !dec.ScanRecord() {
		return text
	}
	var sb strings.Builder
	first := true
	for dec.ScanKeyval() {
		if !first {
			sb.WriteByte(' ')
		}
		first = false
		key := string(dec.Key())
		val := string(dec.Value())
		sb.WriteString(lfKey.Render(key))
		sb.WriteString(lfEq.Render("="))
		sb.WriteString(lfRenderValue(key, val))
	}
	if dec.Err() != nil {
		return text
	}
	return sb.String()
}

func lfRenderValue(key, val string) string {
	switch strings.ToLower(key) {
	case "level", "lvl", "severity":
		switch strings.ToLower(val) {
		case "error", "err", "fatal":
			return lfError.Render(val)
		case "warn", "warning":
			return lfWarn.Render(val)
		case "info":
			return lfInfo.Render(val)
		case "debug", "trace":
			return lfDebug.Render(val)
		}
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return lfNum.Render(val)
	}
	return val // default fg — no lipgloss overhead for the common case
}
