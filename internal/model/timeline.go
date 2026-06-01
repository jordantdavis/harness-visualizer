// internal/model/timeline.go
package model

import (
	"encoding/json"
	"sort"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// Turn is one conversation turn from the harness transcript. tool_use blocks
// are NOT turns — they are represented by Operations and referenced here via
// ToolRefs (their tool_use ids).
type Turn struct {
	Role     string    `json:"role"`               // user | assistant
	Text     string    `json:"text"`               // rendered prose
	Thinking string    `json:"thinking,omitempty"` // assistant reasoning, if present
	ToolRefs []string  `json:"tool_refs,omitempty"`
	At       time.Time `json:"at"`
}

// LaneEvent is one standalone (non-pairable) hook event rendered as its own
// row in the unified timeline. The Gist is a one-line summary derived from
// Raw by per-hook extractors in lane.go; Raw is preserved for inspector
// drill-down. Severity mirrors the registry entry for client convenience.
type LaneEvent struct {
	ID        string          `json:"id"`
	HookEvent string          `json:"hook_event"`
	Lane      event.Lane      `json:"lane"`
	Gist      string          `json:"gist"`
	Severity  string          `json:"severity"`
	Raw       json.RawMessage `json:"raw,omitempty"`
	At        time.Time       `json:"at"`
	Seq       int64           `json:"seq"`
}

// TimelineItem is one row in the unified, interleaved timeline. Exactly one of
// Op / Turn / Event is set, selected by Kind.
type TimelineItem struct {
	Kind  string     `json:"kind"` // "operation" | "turn" | "event"
	At    time.Time  `json:"at"`
	Seq   int64      `json:"seq"`
	Op    *Operation `json:"op,omitempty"`
	Turn  *Turn      `json:"turn,omitempty"`
	Event *LaneEvent `json:"event,omitempty"`
}

// MergeTimeline merges operations, conversation turns, and standalone lane
// events into one chronological list. Operations are authoritative; turns and
// lane events enrich. Sort key is At; ties are broken by Seq so rows stay
// stable relative to each other. Any of the input slices may be empty.
func MergeTimeline(ops []Operation, turns []Turn, events []LaneEvent) []TimelineItem {
	items := make([]TimelineItem, 0, len(ops)+len(turns)+len(events))
	for i := range ops {
		op := ops[i]
		items = append(items, TimelineItem{Kind: "operation", At: op.StartedAt, Seq: op.Seq, Op: &op})
	}
	for i := range turns {
		tn := turns[i]
		items = append(items, TimelineItem{Kind: "turn", At: tn.At, Turn: &tn})
	}
	for i := range events {
		ev := events[i]
		items = append(items, TimelineItem{Kind: "event", At: ev.At, Seq: ev.Seq, Event: &ev})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].At.Equal(items[j].At) {
			return items[i].Seq < items[j].Seq
		}
		return items[i].At.Before(items[j].At)
	})
	return items
}
