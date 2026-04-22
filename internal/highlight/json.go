package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"

	"github.com/nqbao/logwiz/internal/logmsg"
)

// JSONRenderer colourises a JSON string using the active chroma style (see
// SetActiveStyle) and truecolour ANSI escape sequences. Falls back to the raw
// text on any error.
func JSONRenderer(text string) string {
	lexer := lexers.Get("json")
	if lexer == nil {
		return text
	}
	iter, err := lexer.Tokenise(nil, text)
	if err != nil {
		return text
	}
	var sb strings.Builder
	if err := formatters.TTY16m.Format(&sb, GetActiveStyle(), iter); err != nil {
		return text
	}
	return sb.String()
}

// PrettyJSONRenderer parses raw as a JSON object and renders it in
// humanlog-style layout (time level message key=val …), coloured via the
// active chroma style. Falls back to JSONRenderer on parse errors.
func PrettyJSONRenderer(raw string) string {
	f, ok := logmsg.ExtractFromJSON(raw)
	if !ok {
		return JSONRenderer(raw)
	}
	return renderPrettyFields(f)
}
