// internal/model/operation.go
package model

import (
	"encoding/json"
	"sort"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// Operation is one paired Pre/Post event: tool, subagent, or compact. It is
// keyed by ID so live upserts replace a running row in place. Heavy detail
// (diff/input/response/raw) is fetched separately via BuildOperationDetail.
type Operation struct {
	Kind      string        `json:"kind"`           // "tool" | "subagent" | "compact"
	ID        string        `json:"id"`             // stable id, or "" when absent
	Tool      string        `json:"tool,omitempty"` // Edit, Bash, Read…; empty for non-tool kinds
	Status    Status        `json:"status"`         // running | success | error | neutral
	StartedAt time.Time     `json:"started_at"`     // Pre.CapturedAt
	Duration  time.Duration `json:"duration"`       // nanoseconds (marshals as int64); 0 while running
	Target    string        `json:"target"`         // file path / command gist
	Seq       int64         `json:"seq"`            // Pre.Seq — the chronological anchor
}

// pairSpec describes one Pre/Post hook pair handled by BuildOperations.
type pairSpec struct {
	kind    string
	pre     string
	posts   []string // accepted post hook events
	id      func(raw json.RawMessage) string
	target  func(ev *event.Event) string
	toolKey func(ev *event.Event) string
}

func toolPairSpec() pairSpec {
	return pairSpec{
		kind:    "tool",
		pre:     "PreToolUse",
		posts:   []string{"PostToolUse", "PostToolUseFailure"},
		id:      hooks.ToolUseID,
		target:  ExtractTarget,
		toolKey: func(ev *event.Event) string { return ev.ToolName },
	}
}

func subagentPairSpec() pairSpec {
	return pairSpec{
		kind:    "subagent",
		pre:     "SubagentStart",
		posts:   []string{"SubagentStop"},
		id:      hooks.SubagentID,
		target:  ExtractTarget,
		toolKey: func(*event.Event) string { return "subagent" },
	}
}

func compactPairSpec() pairSpec {
	return pairSpec{
		kind:    "compact",
		pre:     "PreCompact",
		posts:   []string{"PostCompact"},
		id:      hooks.CompactID,
		target:  ExtractTarget,
		toolKey: func(*event.Event) string { return "compact" },
	}
}

// BuildOperations pairs Pre/Post events for tool, subagent, and compact kinds
// and returns the resulting operations in chronological (Seq) order. Input need
// not be sorted. Pairing prefers a stable ID match, then falls back to the first
// unclaimed Post of the same kind (and same tool, when applicable) with a later
// Seq. Heuristics never cross kinds — a SubagentStop cannot close a PreCompact.
// Non-paired events (e.g., SessionStart) are ignored.
func BuildOperations(events []*event.Event) []Operation {
	specs := []pairSpec{toolPairSpec(), subagentPairSpec(), compactPairSpec()}
	var ops []Operation
	for _, sp := range specs {
		ops = append(ops, buildPairs(events, sp)...)
	}
	sort.SliceStable(ops, func(i, j int) bool { return ops[i].Seq < ops[j].Seq })
	return ops
}

// buildPairs runs one Pre/Post pairing pass for a single pairSpec.
func buildPairs(events []*event.Event, sp pairSpec) []Operation {
	type slot struct {
		ev      *event.Event
		claimed bool
	}

	postSet := make(map[string]bool, len(sp.posts))
	for _, p := range sp.posts {
		postSet[p] = true
	}

	// Index all matching Post events before scanning Pres.
	var posts []slot
	postByID := map[string]int{}
	postByKey := map[string][]int{}

	for _, e := range events {
		if !postSet[e.HookEvent] {
			continue
		}
		idx := len(posts)
		posts = append(posts, slot{ev: e})
		if id := sp.id(e.Raw); id != "" {
			if _, exists := postByID[id]; !exists {
				postByID[id] = idx
			}
		}
		key := sp.toolKey(e)
		postByKey[key] = append(postByKey[key], idx)
	}

	var ops []Operation
	for _, e := range events {
		if e.HookEvent != sp.pre {
			continue
		}
		op := Operation{
			Kind:      sp.kind,
			ID:        sp.id(e.Raw),
			Tool:      e.ToolName,
			Status:    StatusRunning,
			StartedAt: e.CapturedAt,
			Target:    sp.target(e),
			Seq:       e.Seq,
		}

		var post *event.Event

		// 1. Stable ID match.
		if op.ID != "" {
			if idx, ok := postByID[op.ID]; ok && !posts[idx].claimed {
				posts[idx].claimed = true
				post = posts[idx].ev
			}
		}

		// 2. Heuristic: first unclaimed Post of same kind (same toolKey) with
		//    a later Seq. Heuristics are scoped to this spec's postByKey, so
		//    cross-kind matches are structurally impossible.
		if post == nil {
			key := sp.toolKey(e)
			for _, idx := range postByKey[key] {
				if !posts[idx].claimed && posts[idx].ev.Seq > e.Seq {
					posts[idx].claimed = true
					post = posts[idx].ev
					break
				}
			}
		}

		if post != nil {
			op.Status = DeriveStatus(post)
			op.Duration = post.CapturedAt.Sub(e.CapturedAt)
		}
		ops = append(ops, op)
	}
	return ops
}
