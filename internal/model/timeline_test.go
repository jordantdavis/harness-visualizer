// internal/model/timeline_test.go
package model

import (
	"testing"
	"time"
)

func TestMergeTimeline_OrdersByTimeThenSeq(t *testing.T) {
	base := time.Unix(2000, 0)
	ops := []Operation{
		{ID: "a", Tool: "Edit", Seq: 2, StartedAt: base.Add(1 * time.Second)},
	}
	turns := []Turn{
		{Role: "user", Text: "do the thing", At: base},                   // before the op
		{Role: "assistant", Text: "done", At: base.Add(2 * time.Second)}, // after the op
	}
	items := MergeTimeline(ops, turns, nil)
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3", len(items))
	}
	if items[0].Kind != "turn" || items[0].Turn.Role != "user" {
		t.Fatalf("item0 = %+v, want user turn", items[0])
	}
	if items[1].Kind != "operation" || items[1].Op.ID != "a" {
		t.Fatalf("item1 = %+v, want op a", items[1])
	}
	if items[2].Kind != "turn" || items[2].Turn.Role != "assistant" {
		t.Fatalf("item2 = %+v, want assistant turn", items[2])
	}
}

func TestMergeTimeline_NoTurnsDegradesToOps(t *testing.T) {
	items := MergeTimeline([]Operation{{ID: "a", Seq: 1}}, nil, nil)
	if len(items) != 1 || items[0].Kind != "operation" {
		t.Fatalf("want ops-only timeline, got %+v", items)
	}
}

func TestMergeTimelineInterleavesLaneEvents(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	ops := []Operation{
		{ID: "op1", StartedAt: t0.Add(1 * time.Second), Seq: 1},
		{ID: "op2", StartedAt: t0.Add(3 * time.Second), Seq: 3},
	}
	turns := []Turn{{Role: "user", Text: "hi", At: t0}}
	events := []LaneEvent{
		{ID: "e1", HookEvent: "PermissionRequest", Lane: "permission",
			Gist: "Bash", Severity: "warn", At: t0.Add(2 * time.Second), Seq: 2},
	}

	items := MergeTimeline(ops, turns, events)

	if len(items) != 4 {
		t.Fatalf("len = %d, want 4", len(items))
	}
	kinds := []string{items[0].Kind, items[1].Kind, items[2].Kind, items[3].Kind}
	want := []string{"turn", "operation", "event", "operation"}
	for i := range kinds {
		if kinds[i] != want[i] {
			t.Errorf("kinds[%d] = %q, want %q (got order: %v)", i, kinds[i], want[i], kinds)
		}
	}
	if items[2].Event == nil || items[2].Event.ID != "e1" {
		t.Errorf("event slot wrong: %+v", items[2])
	}
}
