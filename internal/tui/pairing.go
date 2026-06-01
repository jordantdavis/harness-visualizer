// Package tui — pairing.go
//
// Pairs PreToolUse events with their PostToolUse counterparts to produce
// display rows for the folded op view. Pairing is pure (no I/O, no state):
// given a sorted event slice it returns a slice of displayRows in chronological
// order that can be fed directly to the events pane renderer.
//
// Pairing strategy (in priority order):
//  1. Stable ID: both Pre and Post carry the same "tool_use_id" in their Raw
//     payload (looked up at the top level). This is the preferred, exact match.
//  2. Heuristic fallback: the first unclaimed PostToolUse with the same ToolName
//     that appears after the PreToolUse in Seq order.
//
// Unpaired Pres (still running) and standalone lifecycle events (SessionStart,
// SessionEnd, UserPromptSubmit, Notification, Stop, …) each become a single
// standalone displayRow.
package tui

import (
	"encoding/json"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// displayRow is one row in the events pane. It is either a folded paired op
// (IsPair == true, both Pre and Post set) or a standalone event (IsPair ==
// false, only Pre set — Post is nil).
type displayRow struct {
	// Pre is the primary event. For a paired op this is the PreToolUse; for a
	// standalone row it is the single event (lifecycle, unpaired Pre, etc.).
	Pre *event.Event

	// Post is the PostToolUse event for a paired op; nil for standalone rows.
	Post *event.Event

	// IsPair is true when Pre and Post are matched.
	IsPair bool

	// Duration is Post.CapturedAt − Pre.CapturedAt for paired ops; 0 otherwise.
	Duration time.Duration
}

// EffectiveStatus derives the display status for the row:
//   - Paired with Post: derived from Post (exit code / error flag).
//   - Paired without Post (still running): statusRunning.
//   - Standalone: deriveStatus(Pre).
func (dr displayRow) EffectiveStatus() eventStatus {
	if dr.IsPair {
		if dr.Post != nil {
			return deriveStatus(dr.Post)
		}
		return statusRunning
	}
	return deriveStatus(dr.Pre)
}

// rowAnchor pairs a display row with its Seq for sorting.
type rowAnchor struct {
	seq int64
	row displayRow
}

// buildDisplayRows pairs events and returns them as display rows in
// chronological (Seq) order. Input must already be sorted ascending by Seq.
//
// Events outside the pairable set (not PreToolUse or PostToolUse) are always
// standalone. PreToolUse events without a matching Post are standalone (running).
func buildDisplayRows(events []*event.Event) []displayRow {
	// Phase 1: collect all PostToolUse events into indexed slots for pairing.
	//
	// We pre-scan Posts so that Pres can claim them in a single forward pass,
	// regardless of whether the matching Post appears later in the stream.

	type slot struct {
		ev      *event.Event
		claimed bool
	}

	// postByID: tool_use_id → slot index into postSlots
	postByID := make(map[string]int)
	// postByTool: ToolName → ordered slot indices (for heuristic)
	postByTool := make(map[string][]int)
	var postSlots []slot

	for _, ev := range events {
		if ev.HookEvent != "PostToolUse" {
			continue
		}
		idx := len(postSlots)
		postSlots = append(postSlots, slot{ev: ev})
		if id := extractToolUseID(ev.Raw); id != "" {
			postByID[id] = idx
		}
		if ev.ToolName != "" {
			postByTool[ev.ToolName] = append(postByTool[ev.ToolName], idx)
		}
	}

	// Phase 2: walk events in order, pairing Pres greedily.
	//
	// The anchor position of a paired row is the Pre's Seq so the row sorts
	// to where the op started.

	var anchors []rowAnchor

	for _, ev := range events {
		switch ev.HookEvent {
		case "PreToolUse":
			row := displayRow{Pre: ev}
			// Attempt pairing: prefer stable ID, then heuristic.
			preID := extractToolUseID(ev.Raw)
			var post *event.Event

			if preID != "" {
				if idx, ok := postByID[preID]; ok && !postSlots[idx].claimed {
					postSlots[idx].claimed = true
					post = postSlots[idx].ev
				}
			}
			if post == nil && ev.ToolName != "" {
				// Heuristic: first unclaimed Post of same tool after this Pre.
				for _, idx := range postByTool[ev.ToolName] {
					if !postSlots[idx].claimed && postSlots[idx].ev.Seq > ev.Seq {
						postSlots[idx].claimed = true
						post = postSlots[idx].ev
						break
					}
				}
			}

			if post != nil {
				row.Post = post
				row.IsPair = true
				row.Duration = post.CapturedAt.Sub(ev.CapturedAt)
			}
			anchors = append(anchors, rowAnchor{seq: ev.Seq, row: row})

		case "PostToolUse":
			// Posts are consumed during Pre processing. Unclaimed Posts are
			// added as standalone rows in Phase 3 below.

		default:
			// Lifecycle / other events are always standalone.
			anchors = append(anchors, rowAnchor{seq: ev.Seq, row: displayRow{Pre: ev}})
		}
	}

	// Phase 3: add unclaimed Post events as standalone rows.
	for _, s := range postSlots {
		if !s.claimed {
			anchors = append(anchors, rowAnchor{seq: s.ev.Seq, row: displayRow{Pre: s.ev}})
		}
	}

	// Phase 4: sort anchors by Seq to restore chronological order.
	sortRowAnchors(anchors)

	out := make([]displayRow, len(anchors))
	for i, a := range anchors {
		out[i] = a.row
	}
	return out
}

// extractToolUseID looks for "tool_use_id" at the top level of raw JSON.
// Returns "" on any error or absence.
func extractToolUseID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var wrapper struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return ""
	}
	return wrapper.ToolUseID
}

// sortRowAnchors sorts a slice of rowAnchor by Seq ascending (insertion sort —
// inputs are usually nearly sorted so this is fast enough in practice).
func sortRowAnchors(a []rowAnchor) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].seq < a[j-1].seq; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
