// Package tools provides the buffer-query tools available to the chat agent.
package tools

import (
	"encoding/json"
	"fmt"

	"github.com/nqbao/logwiz/internal/agent"
	"github.com/nqbao/logwiz/internal/buffer"
	"github.com/nqbao/logwiz/internal/redact"
)

// MaxToolLines is the maximum number of lines any single tool call may return.
const MaxToolLines = 500

// Tool is implemented by each buffer-query tool.
type Tool interface {
	Spec() agent.ToolSpec
	Execute(args json.RawMessage, buf *buffer.Buffer) (string, error)
}

// Registry holds the set of tools available to the agent.
type Registry struct {
	tools   map[string]Tool
	redactor *redact.Redactor
}

// NewDefault returns a Registry with the standard tools.
// r may be nil to disable redaction.
func NewDefault(r *redact.Redactor) *Registry {
	reg := &Registry{tools: make(map[string]Tool), redactor: r}
	for _, t := range []Tool{readTool{}, tailTool{}, searchTool{}, countTool{}} {
		reg.tools[t.Spec().Name] = t
	}
	return reg
}

// Specs returns the tool specifications for all registered tools.
func (r *Registry) Specs() []agent.ToolSpec {
	// Return in a stable order.
	names := []string{"read", "tail", "search_regex", "count_regex"}
	out := make([]agent.ToolSpec, 0, len(names))
	for _, name := range names {
		if t, ok := r.tools[name]; ok {
			out = append(out, t.Spec())
		}
	}
	return out
}

// Run executes the named tool with the given arguments against buf.
// The result is redacted before being returned so no sensitive data
// reaches the LLM. Unknown tool names return an "error: ..." string.
func (r *Registry) Run(name string, args json.RawMessage, buf *buffer.Buffer) string {
	t, ok := r.tools[name]
	if !ok {
		return fmt.Sprintf("error: unknown tool %q", name)
	}
	result, err := t.Execute(args, buf)
	if err != nil {
		return "error: " + err.Error()
	}
	return r.redactor.Redact(result)
}
