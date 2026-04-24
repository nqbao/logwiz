package highlight

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// stylePrettyTime is the dim colour used for the timestamp column in pretty mode.
var stylePrettyTime = lfr.NewStyle().Foreground(lipgloss.Color("244"))

// renderPrettyFields renders a parsed LogMessage as a single-line coloured string:
//
//	HH:MM:SS.mmm LEVEL message key=val key=val …
//
// Canonical fields come first; remaining attributes are iterated in sorted
// key order for stable rendering across similar lines.
func renderPrettyFields(m logmsg.LogMessage) string {
	ls := getLogfmtStyles()
	var parts []string

	if m.Time != "" {
		parts = append(parts, stylePrettyTime.Render(m.Time))
	}
	if m.Level != "" {
		level := logmsg.NormalizeLevel(m.Level)
		parts = append(parts, prettyLevelStyle(ls, level).Render(level))
	}
	if m.Message != "" {
		parts = append(parts, m.Message)
	}
	m.Attrs.Range(func(k, v string) bool {
		parts = append(parts, ls.key.Render(k)+ls.eq.Render("=")+ls.str.Render(logmsg.QuoteIfNeeded(v)))
		return true
	})

	return strings.Join(parts, " ")
}

// prettyLevelStyle selects the logfmt level colour matching the 5-char level token.
func prettyLevelStyle(ls logfmtStyles, level string) lipgloss.Style {
	switch strings.TrimSpace(level) {
	case "ERROR", "FATAL":
		return ls.lvlError
	case "WARN":
		return ls.lvlWarn
	case "INFO":
		return ls.lvlInfo
	case "DEBUG", "TRACE":
		return ls.lvlDebug
	default:
		return ls.str
	}
}
