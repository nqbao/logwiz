package highlight

import (
	"strings"

	"github.com/alecthomas/chroma/v2/formatters"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/alecthomas/chroma/v2/styles"
)

// JSONRenderer colourises a JSON string using chroma's monokai theme and
// truecolour ANSI escape sequences. Falls back to the raw text on any error.
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
	if err := formatters.TTY16m.Format(&sb, styles.Get("monokai"), iter); err != nil {
		return text
	}
	return sb.String()
}
