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
		{Role: "user", Text: "do the thing", At: base},                    // before the op
		{Role: "assistant", Text: "done", At: base.Add(2 * time.Second)},  // after the op
	}
	items := MergeTimeline(ops, turns)
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
	items := MergeTimeline([]Operation{{ID: "a", Seq: 1}}, nil)
	if len(items) != 1 || items[0].Kind != "operation" {
		t.Fatalf("want ops-only timeline, got %+v", items)
	}
}
