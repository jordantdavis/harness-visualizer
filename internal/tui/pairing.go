// Package tui — pairing.go
//
// Pairs Pre/Post events for tool, subagent, and compact kinds and renders them
// as folded display rows for the events pane. Pairing is pure: given a sorted
// event slice it returns a slice of displayRows in chronological order.
//
// For each kind:
//  1. Stable ID match (per-kind id extractor).
//  2. Heuristic: first unclaimed Post of the same kind (same ToolName for the
//     tool kind) with a later Seq.
//
// Cross-kind pairing never happens.
package tui

import (
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// displayRow is one row in the events pane. It is either a folded paired op
// (IsPair == true, both Pre and Post set) or a standalone event (IsPair ==
// false, only Pre set — Post is nil).
type displayRow struct {
	// Pre is the primary event. For a paired op this is the pre-event (e.g.
	// PreToolUse, SubagentStart, PreCompact); for a standalone row it is the
	// single event (lifecycle, unpaired Pre, etc.).
	Pre *event.Event

	// Post is the post-event for a paired op; nil for standalone rows.
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

// pairSpec describes one pairable kind (tool, subagent, compact).
//
//   - pre:   the hook event name for the pre-event.
//   - posts: the set of hook event names that close the pre.
//   - id:    extracts the stable pairing ID from a Raw payload; returns "" if absent.
//   - key:   secondary grouping key used for the heuristic fallback (same-kind
//     bucket). For tool it encodes ToolName; for subagent/compact it is a constant.
type pairSpec struct {
	pre   string
	posts map[string]bool
	id    func(raw []byte) string
	key   func(ev *event.Event) string
}

func toolSpec() pairSpec {
	return pairSpec{
		pre:   "PreToolUse",
		posts: map[string]bool{"PostToolUse": true, "PostToolUseFailure": true},
		id:    func(raw []byte) string { return hooks.ToolUseID(raw) },
		// key returns "" when ToolName is absent — a "" key disables the
		// heuristic fallback (matching by tool name requires a non-empty name).
		key: func(ev *event.Event) string { return ev.ToolName },
	}
}

func subagentSpec() pairSpec {
	return pairSpec{
		pre:   "SubagentStart",
		posts: map[string]bool{"SubagentStop": true},
		id:    func(raw []byte) string { return hooks.SubagentID(raw) },
		key:   func(*event.Event) string { return "subagent" },
	}
}

func compactSpec() pairSpec {
	return pairSpec{
		pre:   "PreCompact",
		posts: map[string]bool{"PostCompact": true},
		id:    func(raw []byte) string { return hooks.CompactID(raw) },
		key:   func(*event.Event) string { return "compact" },
	}
}

// buildDisplayRows pairs events and returns them as display rows in
// chronological (Seq) order. Input must already be sorted ascending by Seq.
//
// Pre-events without a matching Post are emitted as standalone (still-running)
// rows. Unclaimed Post events are emitted as standalone rows anchored at their
// own Seq. All other events (lifecycle, unknown) are standalone.
func buildDisplayRows(events []*event.Event) []displayRow {
	specs := []pairSpec{toolSpec(), subagentSpec(), compactSpec()}

	// Pre-scan: index all Post events into per-spec slot tables.
	type slot struct {
		ev      *event.Event
		claimed bool
	}
	specPosts := make([][]slot, len(specs))
	postByID := make([]map[string]int, len(specs))
	postByKey := make([]map[string][]int, len(specs))

	for i, sp := range specs {
		postByID[i] = map[string]int{}
		postByKey[i] = map[string][]int{}
		for _, ev := range events {
			if !sp.posts[ev.HookEvent] {
				continue
			}
			idx := len(specPosts[i])
			specPosts[i] = append(specPosts[i], slot{ev: ev})
			if id := sp.id(ev.Raw); id != "" {
				if _, exists := postByID[i][id]; !exists {
					postByID[i][id] = idx
				}
			}
			if k := sp.key(ev); k != "" {
				postByKey[i][k] = append(postByKey[i][k], idx)
			}
		}
	}

	var anchors []rowAnchor

	// Forward pass: emit a row for each pre-event (paired or standalone) and
	// for each lifecycle/unknown event.
	for _, ev := range events {
		specIdx := -1
		for i, sp := range specs {
			if ev.HookEvent == sp.pre {
				specIdx = i
				break
			}
		}

		switch {
		case specIdx >= 0:
			// Pre-event: attempt to claim a matching Post.
			sp := specs[specIdx]
			row := displayRow{Pre: ev}
			preID := sp.id(ev.Raw)
			var post *event.Event

			// Prefer stable ID match.
			if preID != "" {
				if idx, ok := postByID[specIdx][preID]; ok && !specPosts[specIdx][idx].claimed {
					specPosts[specIdx][idx].claimed = true
					post = specPosts[specIdx][idx].ev
				}
			}
			// Heuristic fallback: first unclaimed same-key Post with higher Seq.
			// A blank key (e.g. PreToolUse with no ToolName) disables this path
			// to avoid spurious pairings between unrelated events.
			if post == nil {
				if k := sp.key(ev); k != "" {
					for _, idx := range postByKey[specIdx][k] {
						s := &specPosts[specIdx][idx]
						if !s.claimed && s.ev.Seq > ev.Seq {
							s.claimed = true
							post = s.ev
							break
						}
					}
				}
			}
			if post != nil {
				row.Post = post
				row.IsPair = true
				row.Duration = post.CapturedAt.Sub(ev.CapturedAt)
			}
			anchors = append(anchors, rowAnchor{seq: ev.Seq, row: row})

		default:
			// Post events are consumed during pre processing; skip here.
			// Lifecycle and unknown events are standalone.
			if isAnyPost(specs, ev.HookEvent) {
				continue
			}
			anchors = append(anchors, rowAnchor{seq: ev.Seq, row: displayRow{Pre: ev}})
		}
	}

	// Add unclaimed Post events as standalone rows.
	for i := range specs {
		for _, s := range specPosts[i] {
			if !s.claimed {
				anchors = append(anchors, rowAnchor{seq: s.ev.Seq, row: displayRow{Pre: s.ev}})
			}
		}
	}

	sortRowAnchors(anchors)
	out := make([]displayRow, len(anchors))
	for i, a := range anchors {
		out[i] = a.row
	}
	return out
}

// isAnyPost reports whether hook is a Post event name for any known spec.
func isAnyPost(specs []pairSpec, hook string) bool {
	for _, sp := range specs {
		if sp.posts[hook] {
			return true
		}
	}
	return false
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
