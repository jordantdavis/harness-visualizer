// internal/model/operation_test.go
package model

import (
	"testing"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

func ev(seq int64, hook, tool, raw string, at time.Time) *event.Event {
	return &event.Event{Seq: seq, HookEvent: hook, ToolName: tool, Raw: []byte(raw), CapturedAt: at}
}

func TestBuildOperations_PairsByToolUseID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	events := []*event.Event{
		ev(1, "PreToolUse", "Edit", `{"tool_use_id":"a","tool_input":{"file_path":"x.go"}}`, t0),
		ev(2, "PostToolUse", "Edit", `{"tool_use_id":"a","tool_response":{"exit_code":0}}`, t0.Add(200*time.Millisecond)),
	}
	ops := BuildOperations(events)
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.ID != "a" || op.Tool != "Edit" || op.Status != StatusSuccess {
		t.Fatalf("unexpected op: %+v", op)
	}
	if op.Target != "x.go" {
		t.Fatalf("target = %q, want x.go", op.Target)
	}
	if op.Duration != 200*time.Millisecond {
		t.Fatalf("duration = %v, want 200ms", op.Duration)
	}
	if op.Seq != 1 {
		t.Fatalf("seq = %d, want 1 (Pre anchor)", op.Seq)
	}
}

func TestBuildOperations_UnpairedPreIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"b","tool_input":{"command":"sleep 9"}}`, t0),
	})
	if len(ops) != 1 || ops[0].Status != StatusRunning {
		t.Fatalf("want one running op, got %+v", ops)
	}
	if ops[0].Duration != 0 {
		t.Fatalf("running op duration = %v, want 0", ops[0].Duration)
	}
}

func TestBuildOperations_HeuristicFallbackByTool(t *testing.T) {
	t0 := time.Unix(1000, 0)
	// No tool_use_id on either; pair by same tool + Post.Seq > Pre.Seq.
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Read", `{"tool_input":{"file_path":"a"}}`, t0),
		ev(2, "PostToolUse", "Read", `{"tool_response":{"exit_code":0}}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusSuccess {
		t.Fatalf("want one success op, got %+v", ops)
	}
}

func TestBuildOperations_DropsNonToolEvents(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SessionStart", "", `{}`, t0),
		ev(2, "UserPromptSubmit", "", `{"prompt":"hi"}`, t0),
	})
	if len(ops) != 0 {
		t.Fatalf("non-tool events should not become operations, got %+v", ops)
	}
}
