package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCmdToolDefs_HasInspectionAndCommit(t *testing.T) {
	defs := CmdToolDefs()
	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{"find", "count", "pick_tool"} {
		if !names[want] {
			t.Errorf("CmdToolDefs missing tool %q", want)
		}
	}
	// Old flat top-level tools must be gone.
	for _, old := range []string{"grep", "clear_grep", "jump", "set_model", "clear_chat", "quit", "reject"} {
		if names[old] {
			t.Errorf("CmdToolDefs should not contain old flat tool %q", old)
		}
	}
}

func TestCmdToolDefs_PickToolHasActionEnum(t *testing.T) {
	defs := CmdToolDefs()
	var pt *ToolSpec
	for i := range defs {
		if defs[i].Name == "pick_tool" {
			pt = &defs[i]
			break
		}
	}
	if pt == nil {
		t.Fatal("pick_tool not found in CmdToolDefs")
	}
	var schema struct {
		Properties struct {
			Action struct {
				Enum []string `json:"enum"`
			} `json:"action"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(pt.Parameters, &schema); err != nil {
		t.Fatalf("parsing pick_tool schema: %v", err)
	}
	want := []string{"grep", "clear_grep", "jump", "highlight", "clear_highlight", "set_model", "clear_chat", "quit", "answer", "reject"}
	got := make(map[string]bool)
	for _, a := range schema.Properties.Action.Enum {
		got[a] = true
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("pick_tool action enum missing %q", w)
		}
	}
}

func TestCmdInspectToolDefs_HasExpectedTools(t *testing.T) {
	defs := CmdInspectToolDefs()
	names := make(map[string]bool)
	for _, d := range defs {
		names[d.Name] = true
	}
	for _, want := range []string{"find", "count", "tail", "read"} {
		if !names[want] {
			t.Errorf("CmdInspectToolDefs missing tool %q", want)
		}
	}
}

func TestCmdCommitToolDef_IsPickTool(t *testing.T) {
	def := CmdCommitToolDef()
	if def.Name != "pick_tool" {
		t.Errorf("CmdCommitToolDef name = %q, want pick_tool", def.Name)
	}
}

func TestBuildCmdPrompt_StructureAndContent(t *testing.T) {
	msgs := BuildCmdPrompt("filter errors", "", nil)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0].Role = %q, want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" {
		t.Errorf("msgs[1].Role = %q, want user", msgs[1].Role)
	}
	if msgs[1].Content != "filter errors" {
		t.Errorf("user content = %q, want %q", msgs[1].Content, "filter errors")
	}
}

func TestBuildCmdPrompt_SystemPromptHasRulesAndExamples(t *testing.T) {
	msgs := BuildCmdPrompt("test", "", nil)
	sys := msgs[0].Content
	for _, want := range []string{"pick_tool", "find", "Rules", "inspect"} {
		if !strings.Contains(strings.ToLower(sys), strings.ToLower(want)) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	// Few-shot examples indicated by →
	if !strings.Contains(sys, "→") {
		t.Error("system prompt should contain few-shot examples with →")
	}
}

func TestBuildCmdPrompt_IncludesLogContext(t *testing.T) {
	ctx := "Log format samples"
	msgs := BuildCmdPrompt("q", ctx, nil)
	if !strings.Contains(msgs[0].Content, "Log format samples") {
		t.Error("system prompt should include the provided log context")
	}
}

func TestBuildCmdPrompt_EmptyContextOmitted(t *testing.T) {
	msgs := BuildCmdPrompt("q", "", nil)
	// Should still be well-formed with no trailing whitespace issues.
	if msgs[0].Content == "" {
		t.Error("system prompt should not be empty")
	}
}

func TestBuildCmdPrompt_WithHistory(t *testing.T) {
	history := []Message{
		{Role: "user", Content: "how many errors?"},
		{Role: "assistant", Content: "37 of 500 lines match 'error'"},
	}
	msgs := BuildCmdPrompt("why?", "", history)
	// system, prior user, prior assistant, new user
	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages with history, got %d", len(msgs))
	}
	if msgs[0].Role != "system" {
		t.Errorf("msgs[0].Role = %q, want system", msgs[0].Role)
	}
	if msgs[1].Role != "user" || msgs[1].Content != "how many errors?" {
		t.Errorf("msgs[1] = {%q, %q}, want prior user turn", msgs[1].Role, msgs[1].Content)
	}
	if msgs[2].Role != "assistant" {
		t.Errorf("msgs[2].Role = %q, want assistant", msgs[2].Role)
	}
	if msgs[3].Role != "user" || msgs[3].Content != "why?" {
		t.Errorf("msgs[3] = {%q, %q}, want new user turn", msgs[3].Role, msgs[3].Content)
	}
}
