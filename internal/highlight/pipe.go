package highlight

import (
	"strings"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// PipeRenderer colours a pipe-delimited line in place: the leading timestamp
// is dimmed and the level field is coloured. All other text, including the
// original spacing around separators, is left untouched.
func PipeRenderer(msg logmsg.LogMessage) string {
	ls := getLogfmtStyles()
	fields := strings.Split(msg.Raw, "|")
	fields[0] = stylePrettyTime.Render(fields[0])
	if msg.Level != "" {
		for i := 1; i < len(fields); i++ {
			f := strings.TrimSpace(fields[i])
			if f != "" && logmsg.NormalizeLevel(f) == msg.Level {
				fields[i] = strings.Replace(fields[i], f, prettyLevelStyle(ls, msg.Level).Render(f), 1)
				break
			}
		}
	}
	return strings.Join(fields, "|")
}

// PrettyPipeRenderer renders a parsed pipe-delimited message in humanlog-style
// layout (time level message key=val …).
func PrettyPipeRenderer(msg logmsg.LogMessage) string {
	return renderPrettyFields(msg)
}
