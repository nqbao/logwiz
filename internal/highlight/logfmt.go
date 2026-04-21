package highlight

import (
	"strconv"
	"strings"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/charmbracelet/lipgloss"
	"github.com/go-logfmt/logfmt"
)

// logfmtStyles holds the lipgloss styles derived from the active chroma theme.
// Rebuilt whenever SetActiveStyle is called.
type logfmtStyles struct {
	key      lipgloss.Style // logfmt key
	eq       lipgloss.Style // '=' separator
	str      lipgloss.Style // string value
	num      lipgloss.Style // numeric value
	lvlError lipgloss.Style
	lvlWarn  lipgloss.Style
	lvlInfo  lipgloss.Style
	lvlDebug lipgloss.Style
}

var (
	lfCachedStyles logfmtStyles
	lfStylesOnce   sync.Once
	lfStylesMu     sync.RWMutex
)

// invalidateLogfmtStyles is called by SetActiveStyle to force a rebuild on
// the next render.
func invalidateLogfmtStyles() {
	lfStylesMu.Lock()
	defer lfStylesMu.Unlock()
	lfStylesOnce = sync.Once{} // reset so Do() fires again
}

func getLogfmtStyles() logfmtStyles {
	lfStylesMu.RLock()
	once := lfStylesOnce
	lfStylesMu.RUnlock()

	once.Do(func() {
		built := buildLogfmtStyles()
		lfStylesMu.Lock()
		lfCachedStyles = built
		lfStylesMu.Unlock()
	})

	lfStylesMu.RLock()
	defer lfStylesMu.RUnlock()
	return lfCachedStyles
}

// buildLogfmtStyles maps chroma token types to logfmt rendering roles.
//
// Token-to-role rationale (holds across most themes):
//
//	NameTag          → keys     (markup tag names: prominent but structural)
//	Comment          → =        (de-emphasised separator)
//	LiteralString    → strings  (string literal colour)
//	LiteralNumber    → numbers  (numeric literal colour)
//	GenericDeleted   → error    (red/removal in diffs → alarming)
//	LiteralString+B  → warn     (warm string colour, bolded for attention)
//	Keyword          → info     (keyword colour — notable but calm)
//	Comment          → debug    (quiet/dim — least important)
func buildLogfmtStyles() logfmtStyles {
	activeStyleMu.RLock()
	st := activeStyle
	activeStyleMu.RUnlock()

	get := func(ttype chroma.TokenType) lipgloss.Style {
		if st == nil {
			return lfr.NewStyle()
		}
		return chromaToLipgloss(st.Get(ttype))
	}

	return logfmtStyles{
		key:      get(chroma.NameTag),
		eq:       get(chroma.Comment),
		str:      get(chroma.LiteralString),
		num:      get(chroma.LiteralNumber),
		lvlError: get(chroma.GenericDeleted).Bold(true),
		lvlWarn:  get(chroma.LiteralString).Bold(true),
		lvlInfo:  get(chroma.Keyword),
		lvlDebug: get(chroma.Comment),
	}
}

// LogfmtRenderer colourises a logfmt line using colours derived from the
// active chroma theme (see SetActiveStyle). Falls back to raw text when the
// line has fewer than two key=value pairs or cannot be decoded.
func LogfmtRenderer(text string) string {
	if strings.Count(text, "=") < 2 {
		return text
	}
	dec := logfmt.NewDecoder(strings.NewReader(text))
	if !dec.ScanRecord() {
		return text
	}
	ls := getLogfmtStyles()
	var sb strings.Builder
	first := true
	for dec.ScanKeyval() {
		if !first {
			sb.WriteByte(' ')
		}
		first = false
		key := string(dec.Key())
		val := string(dec.Value())
		sb.WriteString(ls.key.Render(key))
		sb.WriteString(ls.eq.Render("="))
		sb.WriteString(lfRenderValue(ls, key, val))
	}
	if dec.Err() != nil {
		return text
	}
	return sb.String()
}

func lfRenderValue(ls logfmtStyles, key, val string) string {
	switch strings.ToLower(key) {
	case "level", "lvl", "severity":
		switch strings.ToLower(val) {
		case "error", "err", "fatal":
			return ls.lvlError.Render(val)
		case "warn", "warning":
			return ls.lvlWarn.Render(val)
		case "info":
			return ls.lvlInfo.Render(val)
		case "debug", "trace":
			return ls.lvlDebug.Render(val)
		}
	}
	if _, err := strconv.ParseFloat(val, 64); err == nil {
		return ls.num.Render(val)
	}
	return ls.str.Render(val)
}
