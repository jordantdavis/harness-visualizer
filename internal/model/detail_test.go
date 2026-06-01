// internal/model/detail_test.go
package model

import (
	"testing"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

func TestBuildOperationDetail_EditProducesDiff(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Edit",
		Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a\nb","new_string":"a\nB"}}`)}
	post := &event.Event{HookEvent: "PostToolUse", ToolName: "Edit",
		Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)}

	d := BuildOperationDetail(pre, post)
	if d.DetailKind != "diff" {
		t.Fatalf("DetailKind = %q, want diff", d.DetailKind)
	}
	if d.FilePath != "x.go" {
		t.Fatalf("FilePath = %q, want x.go", d.FilePath)
	}
	if len(d.Diff) != 3 { // context "a", del "b", add "B"
		t.Fatalf("diff len = %d, want 3: %+v", len(d.Diff), d.Diff)
	}
	if len(d.RawPre) == 0 || len(d.RawPost) == 0 {
		t.Fatal("raw passthrough must be populated")
	}
}

func TestBuildOperationDetail_BashProducesOutput(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Bash",
		Raw: []byte(`{"tool_input":{"command":"echo hi"}}`)}
	post := &event.Event{HookEvent: "PostToolUse", ToolName: "Bash",
		Raw: []byte(`{"tool_response":{"exit_code":0,"stdout":"hi\n"}}`)}

	d := BuildOperationDetail(pre, post)
	if d.DetailKind != "output" {
		t.Fatalf("DetailKind = %q, want output", d.DetailKind)
	}
	if d.Command != "echo hi" {
		t.Fatalf("Command = %q, want echo hi", d.Command)
	}
	if d.Output != "hi\n" {
		t.Fatalf("Output = %q, want 'hi\\n'", d.Output)
	}
	if d.ExitCode == nil || *d.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", d.ExitCode)
	}
}

func TestBuildOperationDetail_RunningHasNilPost(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Read",
		Raw: []byte(`{"tool_input":{"file_path":"y"}}`)}
	d := BuildOperationDetail(pre, nil)
	if d.DetailKind != "generic" {
		t.Fatalf("DetailKind = %q, want generic", d.DetailKind)
	}
	if len(d.RawPost) != 0 {
		t.Fatal("running op must have empty RawPost")
	}
}
