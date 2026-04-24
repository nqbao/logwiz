package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// JSONRenderer colourises a JSON line using the active chroma style (see
// SetActiveStyle) and truecolour ANSI escape sequences. Falls back to the
// raw text on any error.
func JSONRenderer(msg logmsg.LogMessage) string {
	lexer := lexers.Get("json")
	if lexer == nil {
		return msg.Raw
	}
	iter, err := lexer.Tokenise(nil, msg.Raw)
	if err != nil {
		return msg.Raw
	}
	var sb strings.Builder
	if err := formatters.TTY16m.Format(&sb, GetActiveStyle(), iter); err != nil {
		return msg.Raw
	}
	return sb.String()
}

// PrettyJSONRenderer renders a parsed JSON log message in humanlog-style
// layout (time level message key=val …), coloured via the active chroma
// style. Falls back to JSONRenderer when the message has no parsed
// canonical fields (malformed JSON, top-level array).
func PrettyJSONRenderer(msg logmsg.LogMessage) string {
	if msg.Time == "" && msg.Level == "" && msg.Message == "" && msg.Extra.Len() == 0 {
		return JSONRenderer(msg)
	}
	return renderPrettyFields(msg)
}
