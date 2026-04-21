package highlight

import (
	"io"
	"sync"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/styles"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

// ActiveStyle is the chroma style used by both JSONRenderer and LogfmtRenderer.
// Defaults to monokai. Change it with SetActiveStyle; both renderers will use
// the new style on the next uncached line.
var activeStyle = styles.Get("monokai")

var activeStyleMu sync.RWMutex

// SetActiveStyle replaces the active chroma style. Call Highlighter.Clear()
// afterwards so cached renderings are invalidated.
func SetActiveStyle(s *chroma.Style) {
	activeStyleMu.Lock()
	defer activeStyleMu.Unlock()
	activeStyle = s
	invalidateLogfmtStyles()
}

// GetActiveStyle returns the current chroma style.
func GetActiveStyle() *chroma.Style {
	activeStyleMu.RLock()
	defer activeStyleMu.RUnlock()
	return activeStyle
}

// lfr is the shared lipgloss renderer with a forced TrueColor profile so ANSI
// codes appear whether or not stdout is a TTY. SetColorProfile must be used;
// NewRenderer ignores termenv.WithProfile (it uses EnvColorProfile lazily).
var lfr = func() *lipgloss.Renderer {
	r := lipgloss.NewRenderer(io.Discard)
	r.SetColorProfile(termenv.TrueColor)
	return r
}()

// chromaToLipgloss converts a chroma StyleEntry into a lipgloss Style, mapping
// foreground colour, bold, and italic. Background is intentionally ignored —
// the TUI controls the background.
func chromaToLipgloss(entry chroma.StyleEntry) lipgloss.Style {
	s := lfr.NewStyle()
	if entry.Colour.IsSet() {
		s = s.Foreground(lipgloss.Color(entry.Colour.String()))
	}
	if entry.Bold == chroma.Yes {
		s = s.Bold(true)
	}
	if entry.Italic == chroma.Yes {
		s = s.Italic(true)
	}
	return s
}

// styleFor returns the lipgloss Style for ttype from the active chroma style.
func styleFor(ttype chroma.TokenType) lipgloss.Style {
	activeStyleMu.RLock()
	st := activeStyle
	activeStyleMu.RUnlock()
	if st == nil {
		return lfr.NewStyle()
	}
	return chromaToLipgloss(st.Get(ttype))
}
