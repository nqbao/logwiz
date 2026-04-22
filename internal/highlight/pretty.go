package highlight

import (
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// stylePrettyTime is the dim colour used for the timestamp column in pretty mode.
var stylePrettyTime = lfr.NewStyle().Foreground(lipgloss.Color("244"))

// renderPrettyFields converts a Fields struct into a single-line coloured string:
//
//	HH:MM:SS.mmm LEVEL message key=val key=val …
//
// Colours are derived from the active chroma style via getLogfmtStyles.
func renderPrettyFields(f logmsg.Fields) string {
	ls := getLogfmtStyles()
	var parts []string

	if f.Time != "" {
		parts = append(parts, stylePrettyTime.Render(f.Time))
	}
	if f.Level != "" {
		parts = append(parts, prettyLevelStyle(ls, f.Level).Render(f.Level))
	}
	if f.Message != "" {
		parts = append(parts, f.Message)
	}
	for _, kv := range f.Rest {
		parts = append(parts, ls.key.Render(kv.Key)+ls.eq.Render("=")+ls.str.Render(kv.Value))
	}

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
