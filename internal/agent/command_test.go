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
	want := []string{"grep", "clear_grep", "jump", "set_model", "clear_chat", "quit", "reject"}
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

func TestCmdInspectToolDefs_OnlyFindAndCount(t *testing.T) {
	defs := CmdInspectToolDefs()
	if len(defs) != 2 {
		t.Fatalf("CmdInspectToolDefs len = %d, want 2", len(defs))
	}
	names := map[string]bool{defs[0].Name: true, defs[1].Name: true}
	if !names["find"] || !names["count"] {
		t.Errorf("CmdInspectToolDefs = %v, want [find count]", []string{defs[0].Name, defs[1].Name})
	}
}

func TestCmdCommitToolDef_IsPickTool(t *testing.T) {
	def := CmdCommitToolDef()
	if def.Name != "pick_tool" {
		t.Errorf("CmdCommitToolDef name = %q, want pick_tool", def.Name)
	}
}

func TestBuildCmdPrompt_StructureAndContent(t *testing.T) {
	msgs := BuildCmdPrompt("filter errors", "")
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
	msgs := BuildCmdPrompt("test", "")
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
	ctx := "Log sample:\n```\nfoo bar\n```"
	msgs := BuildCmdPrompt("q", ctx)
	if !strings.Contains(msgs[0].Content, "Log sample") {
		t.Error("system prompt should include the provided log context")
	}
}

func TestBuildCmdPrompt_EmptyContextOmitted(t *testing.T) {
	msgs := BuildCmdPrompt("q", "")
	// Should still be well-formed with no trailing whitespace issues.
	if msgs[0].Content == "" {
		t.Error("system prompt should not be empty")
	}
}
