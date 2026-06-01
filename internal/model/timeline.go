// internal/model/timeline.go
package model

import (
	"sort"
	"time"
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

// TimelineItem is one row in the unified, interleaved timeline. Exactly one of
// Op / Turn is set, selected by Kind.
type TimelineItem struct {
	Kind string     `json:"kind"` // "operation" | "turn"
	At   time.Time  `json:"at"`
	Seq  int64      `json:"seq"`  // hook Seq when Kind=="operation"; 0 for turns
	Op   *Operation `json:"op,omitempty"`
	Turn *Turn      `json:"turn,omitempty"`
}

// MergeTimeline merges operations and conversation turns into one
// chronological list (Approach 1). Operations are authoritative; turns enrich.
// Sort key is At; ties are broken by Seq so operations stay stable relative to
// each other. When turns is empty the result is the operations alone (graceful
// degradation).
func MergeTimeline(ops []Operation, turns []Turn) []TimelineItem {
	items := make([]TimelineItem, 0, len(ops)+len(turns))
	for i := range ops {
		op := ops[i]
		items = append(items, TimelineItem{Kind: "operation", At: op.StartedAt, Seq: op.Seq, Op: &op})
	}
	for i := range turns {
		tn := turns[i]
		items = append(items, TimelineItem{Kind: "turn", At: tn.At, Turn: &tn})
	}
	sort.SliceStable(items, func(i, j int) bool {
		if items[i].At.Equal(items[j].At) {
			return items[i].Seq < items[j].Seq
		}
		return items[i].At.Before(items[j].At)
	})
	return items
}
