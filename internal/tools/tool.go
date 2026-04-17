// Package tools provides the buffer-query tools available to the chat agent.
package tools

import (
	"encoding/json"
	"fmt"

	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/chat"
)

// MaxToolLines is the maximum number of lines any single tool call may return.
const MaxToolLines = 500

// Tool is implemented by each buffer-query tool.
type Tool interface {
	Spec() chat.ToolSpec
	Execute(args json.RawMessage, buf *buffer.Buffer) (string, error)
}

// Registry holds the set of tools available to the agent.
type Registry struct {
	tools map[string]Tool
}

// NewDefault returns a Registry with the standard tools.
func NewDefault() *Registry {
	r := &Registry{tools: make(map[string]Tool)}
	for _, t := range []Tool{readTool{}, tailTool{}, searchTool{}, countTool{}} {
		r.tools[t.Spec().Name] = t
	}
	return r
}

// Specs returns the tool specifications for all registered tools.
func (r *Registry) Specs() []chat.ToolSpec {
	// Return in a stable order.
	names := []string{"read", "tail", "search_regex", "count_regex"}
	out := make([]chat.ToolSpec, 0, len(names))
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			out = append(out, t.Spec())
		}
	}
	return out
}

// Run executes the named tool with the given arguments against buf.
// Unknown tool names return an "error: ..." string so the agent can recover.
func (r *Registry) Run(name string, args json.RawMessage, buf *buffer.Buffer) string {
	t, ok := r.tools[name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", name)
	}
	result, err := t.Execute(args, buf)
	if err != nil {
		return "error: " + err.Error()
	}
	return result
}
