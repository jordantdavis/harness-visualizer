// internal/model/operation_test.go
package model

import (
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
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

func TestBuildOperations_DefaultKindIsTool(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Edit", `{"tool_use_id":"a"}`, t0),
		ev(2, "PostToolUse", "Edit", `{"tool_use_id":"a","tool_response":{"exit_code":0}}`, t0.Add(100*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	if ops[0].Kind != "tool" {
		t.Fatalf("Kind = %q, want \"tool\"", ops[0].Kind)
	}
}

func TestBuildOperations_PairsToolUseWithFailurePost(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"f1","tool_input":{"command":"false"}}`, t0),
		ev(2, "PostToolUseFailure", "Bash", `{"tool_use_id":"f1","error":"boom"}`, t0.Add(50*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "tool" || op.Status != StatusError {
		t.Fatalf("kind=%q status=%q, want tool/error", op.Kind, op.Status)
	}
	if op.Duration != 50*time.Millisecond {
		t.Fatalf("duration = %v, want 50ms", op.Duration)
	}
}

func TestBuildOperations_HeuristicFailurePost(t *testing.T) {
	// No tool_use_id on either; heuristic still pairs same-tool PostToolUseFailure.
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Read", `{"tool_input":{"file_path":"x"}}`, t0),
		ev(2, "PostToolUseFailure", "Read", `{"error":"nope"}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusError {
		t.Fatalf("want one error op, got %+v", ops)
	}
}

func TestBuildOperations_SubagentPairByID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-1","subagent_type":"engineer"}`, t0),
		ev(2, "SubagentStop", "", `{"subagent_id":"sa-1"}`, t0.Add(2*time.Second)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "subagent" {
		t.Fatalf("Kind = %q, want subagent", op.Kind)
	}
	if op.ID != "sa-1" {
		t.Fatalf("ID = %q, want sa-1", op.ID)
	}
	if op.Status != StatusSuccess {
		t.Fatalf("Status = %q, want success", op.Status)
	}
	if op.Duration != 2*time.Second {
		t.Fatalf("Duration = %v, want 2s", op.Duration)
	}
}

func TestBuildOperations_SubagentUnpairedIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-3"}`, t0),
	})
	if len(ops) != 1 || ops[0].Kind != "subagent" || ops[0].Status != StatusRunning {
		t.Fatalf("want one running subagent op, got %+v", ops)
	}
}

func TestBuildOperations_SubagentHeuristicWithoutID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{}`, t0),
		ev(2, "SubagentStop", "", `{}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 {
		t.Fatalf("want one paired subagent op, got %d", len(ops))
	}
	if ops[0].Duration != time.Second {
		t.Fatalf("Duration = %v, want 1s", ops[0].Duration)
	}
}

func TestBuildOperations_CompactPairByID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{"compact_id":"c-1","trigger":"auto"}`, t0),
		ev(2, "PostCompact", "", `{"compact_id":"c-1"}`, t0.Add(500*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "compact" || op.ID != "c-1" {
		t.Fatalf("unexpected op kind/ID: %+v", op)
	}
	if op.Status != StatusSuccess {
		t.Fatalf("Status = %q, want success", op.Status)
	}
}

func TestBuildOperations_CompactUnpairedIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{"compact_id":"c-2"}`, t0),
	})
	if len(ops) != 1 || ops[0].Kind != "compact" || ops[0].Status != StatusRunning {
		t.Fatalf("want one running compact op, got %+v", ops)
	}
}

func TestBuildOperations_CrossKindHeuristicIsolation(t *testing.T) {
	// A SubagentStop must never close a PreCompact (or vice versa), even
	// without IDs and even when Seq order would allow it.
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{}`, t0),
		ev(2, "SubagentStop", "", `{}`, t0.Add(time.Second)),
	})
	// Result: one running compact + a standalone SubagentStop (not a Pre, so not an op).
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1 running compact", len(ops))
	}
	if ops[0].Kind != "compact" || ops[0].Status != StatusRunning {
		t.Fatalf("unexpected op: %+v", ops[0])
	}
}

func TestBuildOperations_SubagentStopWithError(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-2"}`, t0),
		ev(2, "SubagentStop", "", `{"subagent_id":"sa-2","error":"timeout"}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusError {
		t.Fatalf("want one error op, got %+v", ops)
	}
}

func TestBuildOperations_MixedKindsChronological(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"t1"}`, t0),
		ev(2, "SubagentStart", "", `{"subagent_id":"sa"}`, t0.Add(time.Second)),
		ev(3, "PostToolUse", "Bash", `{"tool_use_id":"t1","tool_response":{"exit_code":0}}`, t0.Add(2*time.Second)),
		ev(4, "PreCompact", "", `{"compact_id":"c"}`, t0.Add(3*time.Second)),
		ev(5, "SubagentStop", "", `{"subagent_id":"sa"}`, t0.Add(4*time.Second)),
		ev(6, "PostCompact", "", `{"compact_id":"c"}`, t0.Add(5*time.Second)),
	})
	if len(ops) != 3 {
		t.Fatalf("got %d ops, want 3 (tool, subagent, compact)", len(ops))
	}
	want := []string{"tool", "subagent", "compact"}
	for i, w := range want {
		if ops[i].Kind != w {
			t.Errorf("ops[%d].Kind = %q, want %q", i, ops[i].Kind, w)
		}
	}
}
