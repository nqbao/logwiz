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
		parts = append(parts, stylePrettyTime.Render(logmsg.NormalizeTime(m.Time)))
	}
	if m.Level != "" {
		parts = append(parts, prettyLevelStyle(ls, m.Level).Render(m.Level))
	}
	if m.Message != "" {
		parts = append(parts, m.Message)
	}
	m.Extra.Range(func(k, v string) bool {
		parts = append(parts, ls.key.Render(k)+ls.eq.Render("=")+ls.str.Render(renderAttrValue(v)))
		return true
	})

	return strings.Join(parts, " ")
}

// renderAttrValue returns the display form of an attribute value. JSON
// objects/arrays are self-delimiting and rendered as-is; everything else
// goes through QuoteIfNeeded.
func renderAttrValue(v string) string {
	if len(v) >= 2 && ((v[0] == '{' && v[len(v)-1] == '}') || (v[0] == '[' && v[len(v)-1] == ']')) {
		return v
	}
	return logmsg.QuoteIfNeeded(v)
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
