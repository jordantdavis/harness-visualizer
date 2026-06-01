// internal/model/operation.go
package model

import (
	"sort"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// Operation is one tool invocation: a PreToolUse paired with its PostToolUse
// (or a still-running Pre). It is keyed by ID (tool_use_id) so live upserts
// replace a running row in place. Heavy detail (diff/input/response/raw) is
// fetched separately via BuildOperationDetail.
type Operation struct {
	Kind      string        `json:"kind"`           // "tool" | "subagent" | "compact"
	ID        string        `json:"id"`             // tool_use_id, or "" when absent
	Tool      string        `json:"tool,omitempty"` // Edit, Bash, Read…; empty for non-tool kinds
	Status    Status        `json:"status"`         // running | success | error | neutral
	StartedAt time.Time     `json:"started_at"`     // Pre.CapturedAt
	Duration  time.Duration `json:"duration"`       // nanoseconds (marshals as int64); 0 while running
	Target    string        `json:"target"`         // file path / command gist
	Seq       int64         `json:"seq"`            // Pre.Seq — the chronological anchor
}

// BuildOperations pairs PreToolUse with PostToolUse events and returns the
// resulting operations in chronological (Seq) order. Input need not be sorted.
// Pairing prefers a stable tool_use_id match, then falls back to the first
// unclaimed Post of the same tool with a later Seq. Non-tool events are ignored.
func BuildOperations(events []*event.Event) []Operation {
	type slot struct {
		ev      *event.Event
		claimed bool
	}
	postByID := map[string]int{}
	postByTool := map[string][]int{}
	var posts []slot

	for _, e := range events {
		if e.HookEvent != "PostToolUse" && e.HookEvent != "PostToolUseFailure" {
			continue
		}
		idx := len(posts)
		posts = append(posts, slot{ev: e})
		if id := hooks.ToolUseID(e.Raw); id != "" {
			postByID[id] = idx
		}
		if e.ToolName != "" {
			postByTool[e.ToolName] = append(postByTool[e.ToolName], idx)
		}
	}

	var ops []Operation
	for _, e := range events {
		if e.HookEvent != "PreToolUse" {
			continue
		}
		op := Operation{
			Kind:      "tool",
			ID:        hooks.ToolUseID(e.Raw),
			Tool:      e.ToolName,
			Status:    StatusRunning,
			StartedAt: e.CapturedAt,
			Target:    ExtractTarget(e),
			Seq:       e.Seq,
		}
		var post *event.Event
		if op.ID != "" {
			if idx, ok := postByID[op.ID]; ok && !posts[idx].claimed {
				posts[idx].claimed = true
				post = posts[idx].ev
			}
		}
		if post == nil && e.ToolName != "" {
			for _, idx := range postByTool[e.ToolName] {
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

	sort.SliceStable(ops, func(i, j int) bool { return ops[i].Seq < ops[j].Seq })
	return ops
}
