// Package highlight maps a LogMessage.Format to a terminal colour renderer
// and caches the rendered output keyed by MsgID.
package highlight

import "github.com/nqbao/logwiz/internal/logmsg"

// SpanRenderer colourises a parsed log message and returns the ANSI-escaped
// string. Renderers may read msg.Raw for token-level passes (chroma, logfmt)
// or the canonical fields (Time/Level/Message/Extra) for pretty layouts.
type SpanRenderer func(msg logmsg.LogMessage) string

// Highlighter dispatches to the registered SpanRenderer for a message's
// Format and caches results by MsgID. The cache must be pruned via Evict
// when the buffer evicts a message, so it does not grow unboundedly.
type Highlighter struct {
	handlers map[string]SpanRenderer
	cache    map[logmsg.MsgID]string
}

// New returns a ready-to-use Highlighter with no handlers registered.
func New() *Highlighter {
	return &Highlighter{
		handlers: make(map[string]SpanRenderer),
		cache:    make(map[logmsg.MsgID]string),
	}
}

// Register associates a SpanRenderer with a format name (e.g. "json").
func (h *Highlighter) Register(format string, fn SpanRenderer) {
	h.handlers[format] = fn
}

// maxHighlightLen is the maximum byte length a line may have before syntax
// highlighting is skipped. Chroma tokenising 50k–170k of JSON is very slow;
// lines this long are typically opaque blobs that don't benefit from colour.
const maxHighlightLen = 10240

// Display returns the colourised representation of msg, computing and caching
// it on the first call and returning the cached value on subsequent calls.
// If no handler is registered for msg.Format, or the line exceeds
// maxHighlightLen bytes, msg.Raw is returned unchanged.
func (h *Highlighter) Display(msg logmsg.LogMessage) string {
	if cached, ok := h.cache[msg.ID]; ok {
		return cached
	}
	var result string
	if msg.Format != "" {
		if fn, ok := h.handlers[msg.Format]; ok {
			if len(msg.Raw) > maxHighlightLen {
				truncated := msg
				truncated.Raw = msg.Raw[:maxHighlightLen]
				result = fn(truncated) + msg.Raw[maxHighlightLen:]
			} else {
				result = fn(msg)
			}
		} else {
			result = msg.Raw
		}
	} else {
		result = msg.Raw
	}
	h.cache[msg.ID] = result
	return result
}

// Evict removes the cached rendering for id. Call this when the buffer
// evicts a message so the cache does not grow past buffer capacity.
func (h *Highlighter) Evict(id logmsg.MsgID) {
	delete(h.cache, id)
}

// Clear drops all cached renderings. Use when handlers or theme change.
func (h *Highlighter) Clear() {
	h.cache = make(map[logmsg.MsgID]string)
}
