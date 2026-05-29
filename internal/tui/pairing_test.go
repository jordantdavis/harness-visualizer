package tui

import (
	"encoding/json"
	"testing"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// --- helpers ----------------------------------------------------------------

func makeEv(id string, seq int64, hook, tool, session string, capturedAt time.Time, raw string) *event.Event {
	return &event.Event{
		ID:         id,
		Seq:        seq,
		HookEvent:  hook,
		ToolName:   tool,
		SessionID:  session,
		CapturedAt: capturedAt,
		Raw:        json.RawMessage(raw),
	}
}

var (
	t0 = time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	t1 = t0.Add(1 * time.Second)
	t2 = t0.Add(2 * time.Second)
	t3 = t0.Add(3 * time.Second)
	t4 = t0.Add(4 * time.Second)
)

// --- tool_use_id pairing ----------------------------------------------------

func TestPairByToolUseID(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0,
		`{"tool_use_id":"uid-1"}`)
	post := makeEv("e2", 2, "PostToolUse", "Bash", "s1", t1,
		`{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)

	rows := buildDisplayRows([]*event.Event{pre, post})

	if len(rows) != 1 {
		t.Fatalf("expected 1 folded row, got %d", len(rows))
	}
	dr := rows[0]
	if !dr.IsPair {
		t.Error("row should be a paired op")
	}
	if dr.Pre != pre {
		t.Error("Pre should point to pre event")
	}
	if dr.Post != post {
		t.Error("Post should point to post event")
	}
}

func TestPairDurationFromCapturedAt(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0,
		`{"tool_use_id":"uid-1"}`)
	post := makeEv("e2", 2, "PostToolUse", "Bash", "s1", t2,
		`{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].Duration != 2*time.Second {
		t.Errorf("duration = %v, want 2s", rows[0].Duration)
	}
}

// --- heuristic fallback pairing (same tool, next unclaimed Post) -----------

func TestPairHeuristicSameTool(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{}`)
	post := makeEv("e2", 2, "PostToolUse", "Bash", "s1", t1,
		`{"tool_response":{"exit_code":0}}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("expected 1 paired row, got %d", len(rows))
	}
	if !rows[0].IsPair {
		t.Error("heuristic: row should be a pair")
	}
}

func TestHeuristicDoesNotPairDifferentTool(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{}`)
	post := makeEv("e2", 2, "PostToolUse", "Read", "s1", t1, `{}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	// Neither should pair: pre is unmatched, post is unmatched.
	if len(rows) != 2 {
		t.Fatalf("expected 2 standalone rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.IsPair {
			t.Error("mismatched tool names should not pair")
		}
	}
}

func TestHeuristicDoesNotClaimAlreadyPairedPost(t *testing.T) {
	// Two pre events, two post events, each pair should bind independently.
	pre1 := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{"tool_use_id":"uid-1"}`)
	pre2 := makeEv("e2", 2, "PreToolUse", "Bash", "s1", t1, `{"tool_use_id":"uid-2"}`)
	post1 := makeEv("e3", 3, "PostToolUse", "Bash", "s1", t2, `{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)
	post2 := makeEv("e4", 4, "PostToolUse", "Bash", "s1", t3, `{"tool_use_id":"uid-2","tool_response":{"exit_code":0}}`)

	rows := buildDisplayRows([]*event.Event{pre1, pre2, post1, post2})
	if len(rows) != 2 {
		t.Fatalf("expected 2 paired rows, got %d", len(rows))
	}
	for _, r := range rows {
		if !r.IsPair {
			t.Error("each pre should pair with its post")
		}
	}
	if rows[0].Pre != pre1 || rows[0].Post != post1 {
		t.Error("first pair should be pre1+post1")
	}
	if rows[1].Pre != pre2 || rows[1].Post != post2 {
		t.Error("second pair should be pre2+post2")
	}
}

// --- standalone events -------------------------------------------------------

func TestStandaloneLifecycleEvents(t *testing.T) {
	start := makeEv("e1", 1, "SessionStart", "", "s1", t0, `{}`)
	prompt := makeEv("e2", 2, "UserPromptSubmit", "", "s1", t1, `{}`)
	end := makeEv("e3", 3, "SessionEnd", "", "s1", t2, `{}`)

	rows := buildDisplayRows([]*event.Event{start, prompt, end})
	if len(rows) != 3 {
		t.Fatalf("expected 3 standalone rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.IsPair {
			t.Errorf("lifecycle event %q should not be a pair", r.Pre.HookEvent)
		}
	}
}

func TestUnpairedPreRendersStandalone(t *testing.T) {
	// A Pre with no matching Post (still running) renders as standalone.
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{}`)

	rows := buildDisplayRows([]*event.Event{pre})
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0].IsPair {
		t.Error("unpaired Pre should be standalone (running)")
	}
	if rows[0].Pre != pre {
		t.Error("standalone Pre should reference the event")
	}
}

// --- chronological ordering -------------------------------------------------

func TestDisplayRowsChronologicalOrder(t *testing.T) {
	prompt := makeEv("e1", 1, "UserPromptSubmit", "", "s1", t0, `{}`)
	pre := makeEv("e2", 2, "PreToolUse", "Bash", "s1", t1, `{"tool_use_id":"uid-1"}`)
	post := makeEv("e3", 3, "PostToolUse", "Bash", "s1", t2, `{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)
	notif := makeEv("e4", 4, "Notification", "", "s1", t3, `{}`)

	rows := buildDisplayRows([]*event.Event{prompt, pre, post, notif})
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (prompt, pair, notif), got %d", len(rows))
	}
	if rows[0].Pre != prompt {
		t.Errorf("row[0] should be prompt, got hook=%q", rows[0].Pre.HookEvent)
	}
	if !rows[1].IsPair {
		t.Error("row[1] should be the paired op")
	}
	if rows[2].Pre != notif {
		t.Errorf("row[2] should be notif, got hook=%q", rows[2].Pre.HookEvent)
	}
}

// --- displayRow helpers -----------------------------------------------------

func TestDisplayRowStatus(t *testing.T) {
	// Paired row derives status from Post.
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{"tool_use_id":"uid-1"}`)
	post := makeEv("e2", 2, "PostToolUse", "Bash", "s1", t1,
		`{"tool_use_id":"uid-1","tool_response":{"exit_code":1}}`)
	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("want 1 row, got %d", len(rows))
	}
	if rows[0].EffectiveStatus() != statusError {
		t.Errorf("paired row with exit_code=1 should be statusError, got %v", rows[0].EffectiveStatus())
	}
}

func TestDisplayRowStatusRunningWhenNoPost(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{}`)
	rows := buildDisplayRows([]*event.Event{pre})
	if rows[0].EffectiveStatus() != statusRunning {
		t.Errorf("unpaired Pre should be statusRunning, got %v", rows[0].EffectiveStatus())
	}
}

func TestExtractToolUseIDTopLevel(t *testing.T) {
	raw := json.RawMessage(`{"tool_use_id":"abc-123"}`)
	if got := extractToolUseID(raw); got != "abc-123" {
		t.Errorf("top-level tool_use_id = %q, want abc-123", got)
	}
}

func TestExtractToolUseIDAbsent(t *testing.T) {
	raw := json.RawMessage(`{"something_else":"x"}`)
	if got := extractToolUseID(raw); got != "" {
		t.Errorf("absent tool_use_id should return \"\", got %q", got)
	}
}

func TestExtractToolUseIDMalformed(t *testing.T) {
	raw := json.RawMessage(`not json`)
	if got := extractToolUseID(raw); got != "" {
		t.Errorf("malformed JSON should return \"\", got %q", got)
	}
}
