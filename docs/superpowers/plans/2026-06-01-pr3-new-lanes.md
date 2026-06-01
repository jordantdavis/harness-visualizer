# PR 3: New Lanes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Render 11 non-pairable Claude Code hook events as distinct, registry-driven lane rows in the TUI and web UI, with a shared `internal/event/hooks.go` registry exposed via `/api/hooks` so the two clients can't drift on glyphs/labels. Also bumps the web inspector pane from 380px → 520px.

**Architecture:** Introduce a small typed registry of hook metadata (`HookMeta{Name,Glyph,Label,Lane,Severity}`) in Go. Extend the domain `TimelineItem` union to a third kind (`event`) carrying a `LaneEvent` with a one-line `Gist` derived per-hook from `Raw`. The daemon serves the registry at `GET /api/hooks` and includes lane events in `/api/sessions/{id}/timeline`. The TUI extends its existing per-hook switches in `internal/tui/format.go` (no layout change). The web client fetches the registry once, dispatches a third `<hv-lane-event-row>` from `timeline.ts`, and widens the inspector grid column.

**Tech Stack:** Go 1.x (`internal/event`, `internal/model`, `internal/daemon`, `internal/tui`), Lit + TypeScript + Vite (`web/`), vitest (web tests), `go test` (Go tests).

**Spec:** `docs/superpowers/specs/2026-06-01-pr3-new-lanes-design.md`

**Scope of new events (11 total):**
- `PermissionRequest`, `PermissionDenied` → lane `permission`
- `InstructionsLoaded` → lane `instructions`
- `ConfigChange` → lane `config`
- `CwdChanged` → lane `cwd`
- `TaskCreated`, `TaskCompleted` → lane `task`
- `UserPromptExpansion` → lane `expansion`
- `MessageDisplay` → lane `message`
- `WorktreeRemove` → lane `worktree`
- `StopFailure` → lane `stop_failure`

---

## Task 1: Hook registry skeleton

**Files:**
- Create: `internal/event/hooks.go`
- Test: `internal/event/hooks_test.go`

- [ ] **Step 1: Write the failing test**

Append to a new file `internal/event/hooks_test.go`:

```go
package event

import "testing"

func TestHooksRegistryAllNamesPresent(t *testing.T) {
	want := []string{
		"PermissionRequest", "PermissionDenied",
		"InstructionsLoaded",
		"ConfigChange",
		"CwdChanged",
		"TaskCreated", "TaskCompleted",
		"UserPromptExpansion",
		"MessageDisplay",
		"WorktreeRemove",
		"StopFailure",
	}
	got := map[string]bool{}
	for _, h := range Hooks {
		got[h.Name] = true
	}
	for _, n := range want {
		if !got[n] {
			t.Errorf("registry missing hook %q", n)
		}
	}
	if len(Hooks) != len(want) {
		t.Errorf("registry length = %d, want %d", len(Hooks), len(want))
	}
}

func TestHooksRegistryEntriesComplete(t *testing.T) {
	validSeverity := map[string]bool{"info": true, "warn": true, "error": true, "dim": true}
	validLane := map[Lane]bool{
		LanePermission: true, LaneInstructions: true, LaneConfig: true,
		LaneCwd: true, LaneTask: true, LaneExpansion: true,
		LaneMessage: true, LaneWorktree: true, LaneStopFailure: true,
	}
	for _, h := range Hooks {
		if h.Glyph == "" {
			t.Errorf("%s: empty glyph", h.Name)
		}
		if h.Label == "" || len(h.Label) > 16 {
			t.Errorf("%s: bad label %q (len=%d)", h.Name, h.Label, len(h.Label))
		}
		if !validLane[h.Lane] {
			t.Errorf("%s: bad lane %q", h.Name, h.Lane)
		}
		if !validSeverity[h.Severity] {
			t.Errorf("%s: bad severity %q", h.Name, h.Severity)
		}
	}
}

func TestLookup(t *testing.T) {
	if _, ok := Lookup("PermissionRequest"); !ok {
		t.Error("Lookup(PermissionRequest) should succeed")
	}
	if _, ok := Lookup("PreToolUse"); ok {
		t.Error("Lookup(PreToolUse) should fail — paired hooks are not in the registry")
	}
	if _, ok := Lookup(""); ok {
		t.Error("Lookup(empty) should fail")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/event/ -run 'TestHooks|TestLookup' -v`
Expected: compile failure (`Lane`, `Hooks`, `Lookup`, lane constants undefined).

- [ ] **Step 3: Implement the registry**

Create `internal/event/hooks.go`:

```go
// Package event — hook registry: typed metadata for the non-pairable hook
// events that get rendered as standalone lane rows in the TUI / web UI.
// Paired hooks (PreToolUse/PostToolUse, SubagentStart/SubagentStop,
// PreCompact/PostCompact, etc.) are deliberately NOT in this registry —
// they flow through the Operation model.
package event

// Lane groups related hook events for client-side categorization.
type Lane string

const (
	LanePermission   Lane = "permission"
	LaneInstructions Lane = "instructions"
	LaneConfig       Lane = "config"
	LaneCwd          Lane = "cwd"
	LaneTask         Lane = "task"
	LaneExpansion    Lane = "expansion"
	LaneMessage      Lane = "message"
	LaneWorktree     Lane = "worktree"
	LaneStopFailure  Lane = "stop_failure"
)

// HookMeta is the per-hook rendering metadata both clients share.
type HookMeta struct {
	Name     string `json:"name"`
	Glyph    string `json:"glyph"`    // single display cell
	Label    string `json:"label"`    // ≤16 chars
	Lane     Lane   `json:"lane"`
	Severity string `json:"severity"` // info | warn | error | dim
}

// Hooks is the canonical list of non-pairable hooks recognized by hv.
var Hooks = []HookMeta{
	{"PermissionRequest", "🔒", "Permission", LanePermission, "warn"},
	{"PermissionDenied", "🚫", "PermDenied", LanePermission, "error"},
	{"InstructionsLoaded", "📄", "Instructions", LaneInstructions, "info"},
	{"ConfigChange", "⚙", "Config", LaneConfig, "info"},
	{"CwdChanged", "📁", "Cwd", LaneCwd, "info"},
	{"TaskCreated", "☐", "TaskCreate", LaneTask, "info"},
	{"TaskCompleted", "☑", "TaskDone", LaneTask, "info"},
	{"UserPromptExpansion", "🪄", "Expansion", LaneExpansion, "info"},
	{"MessageDisplay", "💬", "Message", LaneMessage, "dim"},
	{"WorktreeRemove", "🪵", "WorktreeRm", LaneWorktree, "info"},
	{"StopFailure", "⚠", "StopFailure", LaneStopFailure, "error"},
}

// Lookup returns the HookMeta for a hook event name. The second result is
// false when name is not in the registry — clients should fall back to a
// generic rendering.
func Lookup(name string) (HookMeta, bool) {
	for _, h := range Hooks {
		if h.Name == name {
			return h, true
		}
	}
	return HookMeta{}, false
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/event/ -v`
Expected: all tests PASS, including pre-existing `event_test.go`.

- [ ] **Step 5: Commit**

```bash
git add internal/event/hooks.go internal/event/hooks_test.go
git commit -m "feat(event): add shared hook registry for non-pairable lane events"
```

---

## Task 2: Daemon `/api/hooks` endpoint

**Files:**
- Modify: `internal/daemon/daemon.go` (route registration)
- Modify: `internal/daemon/api.go` (handler)
- Test: `internal/daemon/api_test.go`

- [ ] **Step 1: Inspect existing route registration to mirror its pattern**

Read `internal/daemon/daemon.go` and find where `/api/sessions` is registered (look for `mux.HandleFunc` or `http.HandleFunc`). The new `/api/hooks` route follows the same pattern.

- [ ] **Step 2: Write the failing test**

Append to `internal/daemon/api_test.go`:

```go
func TestHandleAPIHooks(t *testing.T) {
	srv := newTestServer(t) // existing test helper
	req := httptest.NewRequest(http.MethodGet, "/api/hooks", nil)
	w := httptest.NewRecorder()
	srv.handleAPIHooks(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if got := w.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
	var got []event.HookMeta
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got) != len(event.Hooks) {
		t.Errorf("got %d entries, want %d", len(got), len(event.Hooks))
	}
	if got[0].Name == "" || got[0].Glyph == "" {
		t.Errorf("first entry incomplete: %+v", got[0])
	}
}

func TestHandleAPIHooksMethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks", nil)
	w := httptest.NewRecorder()
	srv.handleAPIHooks(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}
```

If `newTestServer` does not exist, look for the pattern used by other `handleAPI*` tests in `api_test.go` and follow it. If existing tests construct `&Server{...}` directly, do the same.

If imports are missing, add: `"encoding/json"`, `"net/http"`, `"net/http/httptest"`, `"jordandavis.dev/harness-visualizer/internal/event"`.

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestHandleAPIHooks -v`
Expected: compile failure (`handleAPIHooks` undefined).

- [ ] **Step 4: Add the handler**

Append to `internal/daemon/api.go`:

```go
// handleAPIHooks: GET /api/hooks — returns the shared hook metadata
// registry (event.Hooks) so the web client renders lane events with the
// same glyphs and labels the TUI uses.
func (s *Server) handleAPIHooks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, event.Hooks)
}
```

`event` is already imported in `api.go` (line 9). No new imports needed.

- [ ] **Step 5: Wire the route**

In `internal/daemon/daemon.go`, find where `/api/sessions` is registered (e.g. `mux.HandleFunc("/api/sessions", s.handleAPISessions)`). Add directly below:

```go
mux.HandleFunc("/api/hooks", s.handleAPIHooks)
```

Use the exact same registration style as the surrounding lines (likely `mux.HandleFunc` on the http mux being built; copy whatever name is in use).

- [ ] **Step 6: Run all daemon tests**

Run: `go test ./internal/daemon/ -v`
Expected: PASS (both new tests + all existing).

- [ ] **Step 7: Commit**

```bash
git add internal/daemon/api.go internal/daemon/daemon.go internal/daemon/api_test.go
git commit -m "feat(daemon): expose hook registry at GET /api/hooks"
```

---

## Task 3: Extend `TimelineItem` to a 3-way union

**Files:**
- Modify: `internal/model/timeline.go` (lines 9-52)
- Test: `internal/model/timeline_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/model/timeline_test.go`:

```go
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
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/model/ -run TestMergeTimelineInterleavesLaneEvents -v`
Expected: compile failure (`LaneEvent` undefined, `MergeTimeline` takes 2 args not 3).

- [ ] **Step 3: Add `LaneEvent` type and extend `TimelineItem`**

Replace lines 20-28 of `internal/model/timeline.go` with:

```go
// LaneEvent is one standalone (non-pairable) hook event rendered as its own
// row in the unified timeline. The Gist is a one-line summary derived from
// Raw by per-hook extractors in lane.go; Raw is preserved for inspector
// drill-down. Severity mirrors the registry entry for client convenience.
type LaneEvent struct {
	ID        string          `json:"id"`
	HookEvent string          `json:"hook_event"`
	Lane      string          `json:"lane"`
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
```

Add `"encoding/json"` to the imports at the top of the file (next to `"sort"` and `"time"`).

- [ ] **Step 4: Extend `MergeTimeline` to accept lane events**

Replace the existing `MergeTimeline` function (lines 35-52) with:

```go
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
```

- [ ] **Step 5: Update existing `MergeTimeline` call sites**

Find call sites: `grep -rn "MergeTimeline" --include='*.go'`. Update each to pass `nil` as the third arg.

Expected call site: `internal/daemon/api.go:61`:
```go
items := model.MergeTimeline(ops, turns)
```
Change to:
```go
items := model.MergeTimeline(ops, turns, nil)
```

(This becomes a real value in Task 5.)

If grep surfaces other call sites (e.g., in tests), apply the same `, nil` to each.

- [ ] **Step 6: Run all model + daemon tests**

Run: `go test ./internal/model/ ./internal/daemon/ -v`
Expected: PASS, including the new test and all existing.

- [ ] **Step 7: Commit**

```bash
git add internal/model/timeline.go internal/model/timeline_test.go internal/daemon/api.go
git commit -m "feat(model): extend TimelineItem to 3-way union (operation|turn|event)"
```

---

## Task 4: Per-hook gist extractors + `BuildLaneEvents`

**Files:**
- Create: `internal/model/lane.go`
- Test: `internal/model/lane_test.go`

These extractors read structured fields out of `Raw`. Field names are best-guesses from the Claude Code hooks docs — every extractor returns `""` on missing/malformed input so a wrong guess can't break the timeline. Risk explicitly accepted in the spec.

- [ ] **Step 1: Write the failing test**

Create `internal/model/lane_test.go`:

```go
package model

import (
	"encoding/json"
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestGistExtractors(t *testing.T) {
	cases := []struct {
		name string
		hook string
		raw  string
		want string
	}{
		{"permission with tool_input", "PermissionRequest",
			`{"tool_name":"Bash","tool_input":{"command":"npm test"}}`,
			"Bash: npm test"},
		{"permission denied with reason", "PermissionDenied",
			`{"tool_name":"Edit","tool_input":{"file_path":"foo.go"},"reason":"rule X"}`,
			"Edit: foo.go (denied: rule X)"},
		{"permission missing fields", "PermissionRequest", `{}`, ""},

		{"instructions with memory_type", "InstructionsLoaded",
			`{"path":"CLAUDE.md","memory_type":"project"}`,
			"CLAUDE.md (project)"},
		{"instructions path only", "InstructionsLoaded",
			`{"path":"AGENTS.md"}`, "AGENTS.md"},
		{"instructions missing", "InstructionsLoaded", `{}`, ""},

		{"config change with old/new", "ConfigChange",
			`{"key":"model","old_value":"opus-4-7","new_value":"sonnet-4-6"}`,
			"model = sonnet-4-6 (was opus-4-7)"},
		{"config change new only", "ConfigChange",
			`{"key":"model","new_value":"sonnet-4-6"}`,
			"model = sonnet-4-6"},
		{"config missing", "ConfigChange", `{}`, ""},

		{"cwd changed", "CwdChanged",
			`{"old_cwd":"/a","new_cwd":"/a/web"}`, "→ /a/web"},
		{"cwd missing", "CwdChanged", `{}`, ""},

		{"task created", "TaskCreated",
			`{"task_id":"42","subject":"Run baseline tests"}`,
			"Create #42: Run baseline tests"},
		{"task completed", "TaskCompleted",
			`{"task_id":"42","subject":"Run baseline tests","status":"completed"}`,
			"Done #42: Run baseline tests"},
		{"task missing", "TaskCreated", `{}`, ""},

		{"expansion", "UserPromptExpansion",
			`{"original":"/loop 5m","expanded":"run baseline tests"}`,
			"/loop 5m → run baseline tests"},
		{"expansion expanded only", "UserPromptExpansion",
			`{"expanded":"hello world"}`, "hello world"},
		{"expansion missing", "UserPromptExpansion", `{}`, ""},

		{"message display", "MessageDisplay",
			`{"text":"Hello, how can I help?"}`,
			"Hello, how can I help?"},
		{"message missing", "MessageDisplay", `{}`, ""},

		{"worktree remove", "WorktreeRemove",
			`{"name":"feature-x","path":"/w/feature-x"}`,
			"removed: feature-x"},
		{"worktree no name", "WorktreeRemove",
			`{"path":"/w/feature-x"}`, "removed: /w/feature-x"},
		{"worktree missing", "WorktreeRemove", `{}`, ""},

		{"stop failure", "StopFailure",
			`{"error_type":"rate_limit","message":"slow down"}`,
			"rate_limit: slow down"},
		{"stop failure type only", "StopFailure",
			`{"error_type":"api_error"}`, "api_error"},
		{"stop failure missing", "StopFailure", `{}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(tc.raw)}
			got := laneGist(ev)
			if got != tc.want {
				t.Errorf("laneGist = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildLaneEvents(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []*event.Event{
		{ID: "a", HookEvent: "PreToolUse", CapturedAt: t0, Seq: 1,
			Raw: json.RawMessage(`{"tool_name":"Bash"}`)},
		{ID: "b", HookEvent: "PermissionRequest", CapturedAt: t0.Add(time.Second), Seq: 2,
			Raw: json.RawMessage(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)},
		{ID: "c", HookEvent: "MessageDisplay", CapturedAt: t0.Add(2 * time.Second), Seq: 3,
			Raw: json.RawMessage(`{"text":"hi"}`)},
	}

	got := BuildLaneEvents(events)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (PreToolUse should be filtered out)", len(got))
	}
	if got[0].HookEvent != "PermissionRequest" {
		t.Errorf("[0].HookEvent = %q, want PermissionRequest", got[0].HookEvent)
	}
	if got[0].Lane != "permission" {
		t.Errorf("[0].Lane = %q, want permission", got[0].Lane)
	}
	if got[0].Severity != "warn" {
		t.Errorf("[0].Severity = %q, want warn", got[0].Severity)
	}
	if got[0].Gist != "Bash: ls" {
		t.Errorf("[0].Gist = %q, want \"Bash: ls\"", got[0].Gist)
	}
	if !got[0].At.Equal(t0.Add(time.Second)) {
		t.Errorf("[0].At = %v, want %v", got[0].At, t0.Add(time.Second))
	}
	if got[0].Seq != 2 {
		t.Errorf("[0].Seq = %d, want 2", got[0].Seq)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/model/ -run 'TestGistExtractors|TestBuildLaneEvents' -v`
Expected: compile failure (`laneGist`, `BuildLaneEvents` undefined).

- [ ] **Step 3: Implement `internal/model/lane.go`**

Create `internal/model/lane.go`:

```go
// internal/model/lane.go — per-hook gist extractors and the BuildLaneEvents
// reducer. Each extractor reads structured fields out of Raw defensively and
// returns "" rather than erroring on missing/malformed input, so a wrong
// field-name guess can't break the timeline. Field names match the Claude
// Code hooks documentation as of 2026-06.
package model

import (
	"encoding/json"
	"fmt"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// BuildLaneEvents reduces a stream of events to standalone lane events,
// filtering out anything not in the shared registry (event.Hooks).
func BuildLaneEvents(events []*event.Event) []LaneEvent {
	out := make([]LaneEvent, 0, len(events))
	for _, ev := range events {
		meta, ok := event.Lookup(ev.HookEvent)
		if !ok {
			continue
		}
		out = append(out, LaneEvent{
			ID:        ev.ID,
			HookEvent: ev.HookEvent,
			Lane:      string(meta.Lane),
			Gist:      laneGist(ev),
			Severity:  meta.Severity,
			Raw:       ev.Raw,
			At:        ev.CapturedAt,
			Seq:       ev.Seq,
		})
	}
	return out
}

// laneGist dispatches to a per-hook extractor. Returns "" when no extractor
// matches or the extractor cannot find usable fields — clients fall back to
// the hook name.
func laneGist(ev *event.Event) string {
	if ev == nil || len(ev.Raw) == 0 {
		return ""
	}
	switch ev.HookEvent {
	case "PermissionRequest", "PermissionDenied":
		return gistPermission(ev)
	case "InstructionsLoaded":
		return gistInstructions(ev.Raw)
	case "ConfigChange":
		return gistConfig(ev.Raw)
	case "CwdChanged":
		return gistCwd(ev.Raw)
	case "TaskCreated":
		return gistTask(ev.Raw, "Create")
	case "TaskCompleted":
		return gistTask(ev.Raw, "Done")
	case "UserPromptExpansion":
		return gistExpansion(ev.Raw)
	case "MessageDisplay":
		return gistMessage(ev.Raw)
	case "WorktreeRemove":
		return gistWorktree(ev.Raw)
	case "StopFailure":
		return gistStopFailure(ev.Raw)
	}
	return ""
}

func gistPermission(ev *event.Event) string {
	var f struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
		Reason    string          `json:"reason"`
	}
	if json.Unmarshal(ev.Raw, &f) != nil {
		return ""
	}
	tgt := firstStringField(f.ToolInput)
	parts := []string{}
	if f.ToolName != "" {
		head := f.ToolName
		if tgt != "" {
			head = head + ": " + tgt
		}
		parts = append(parts, head)
	} else if tgt != "" {
		parts = append(parts, tgt)
	}
	if ev.HookEvent == "PermissionDenied" && f.Reason != "" {
		parts = append(parts, "(denied: "+f.Reason+")")
	}
	return strings.Join(parts, " ")
}

func gistInstructions(raw json.RawMessage) string {
	var f struct {
		Path       string `json:"path"`
		MemoryType string `json:"memory_type"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Path == "" {
		return ""
	}
	if f.MemoryType != "" {
		return f.Path + " (" + f.MemoryType + ")"
	}
	return f.Path
}

func gistConfig(raw json.RawMessage) string {
	var f struct {
		Key      string `json:"key"`
		OldValue string `json:"old_value"`
		NewValue string `json:"new_value"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Key == "" || f.NewValue == "" {
		return ""
	}
	if f.OldValue != "" {
		return fmt.Sprintf("%s = %s (was %s)", f.Key, f.NewValue, f.OldValue)
	}
	return fmt.Sprintf("%s = %s", f.Key, f.NewValue)
}

func gistCwd(raw json.RawMessage) string {
	var f struct {
		NewCwd string `json:"new_cwd"`
	}
	if json.Unmarshal(raw, &f) != nil || f.NewCwd == "" {
		return ""
	}
	return "→ " + f.NewCwd
}

func gistTask(raw json.RawMessage, verb string) string {
	var f struct {
		TaskID  string `json:"task_id"`
		Subject string `json:"subject"`
	}
	if json.Unmarshal(raw, &f) != nil || f.TaskID == "" || f.Subject == "" {
		return ""
	}
	return fmt.Sprintf("%s #%s: %s", verb, f.TaskID, f.Subject)
}

func gistExpansion(raw json.RawMessage) string {
	var f struct {
		Original string `json:"original"`
		Expanded string `json:"expanded"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Expanded == "" {
		return ""
	}
	if f.Original != "" {
		return f.Original + " → " + f.Expanded
	}
	return f.Expanded
}

func gistMessage(raw json.RawMessage) string {
	var f struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	return f.Text
}

func gistWorktree(raw json.RawMessage) string {
	var f struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	if f.Name != "" {
		return "removed: " + f.Name
	}
	if f.Path != "" {
		return "removed: " + f.Path
	}
	return ""
}

func gistStopFailure(raw json.RawMessage) string {
	var f struct {
		ErrorType string `json:"error_type"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(raw, &f) != nil || f.ErrorType == "" {
		return ""
	}
	if f.Message != "" {
		return f.ErrorType + ": " + f.Message
	}
	return f.ErrorType
}

// firstStringField returns the first non-empty string value found in a JSON
// object (used to pull a useful target out of tool_input for permission rows
// without needing per-tool knowledge). Returns "" on parse failure or empty
// object. Field iteration order is deterministic via key sort.
func firstStringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Bias toward common useful field names first.
	preferred := []string{"command", "file_path", "path", "url"}
	for _, k := range preferred {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				if k == "command" {
					return strings.SplitN(s, "\n", 2)[0]
				}
				return s
			}
		}
	}
	// Fallback: lexicographically first non-empty string.
	sortStrings(keys)
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j-1] > s[j]; j-- {
			s[j-1], s[j] = s[j], s[j-1]
		}
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/model/ -v`
Expected: PASS, including all existing tests.

- [ ] **Step 5: Commit**

```bash
git add internal/model/lane.go internal/model/lane_test.go
git commit -m "feat(model): add per-hook gist extractors and BuildLaneEvents"
```

---

## Task 5: Daemon includes lane events in `/api/sessions/{id}/timeline`

**Files:**
- Modify: `internal/daemon/api.go:53-66` (`handleAPITimeline`)
- Test: `internal/daemon/api_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/daemon/api_test.go`:

```go
func TestHandleAPITimelineIncludesLaneEvents(t *testing.T) {
	srv := newTestServer(t)
	// Write a session with one PreToolUse (paired-style, ignored) plus one
	// PermissionRequest (lane event). Use the same store helper other tests
	// in this file use; if none exists, reuse the pattern from the closest
	// neighbor test that posts events via the store.
	sid := "s-lanes"
	writeTestEvent(t, srv, sid, "PreToolUse",
		`{"tool_name":"Bash","tool_input":{"command":"ls"},"tool_use_id":"u1"}`)
	writeTestEvent(t, srv, sid, "PermissionRequest",
		`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)

	req := httptest.NewRequest(http.MethodGet, "/api/sessions/"+sid+"/timeline", nil)
	w := httptest.NewRecorder()
	srv.handleAPISession(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var items []model.TimelineItem
	if err := json.NewDecoder(w.Body).Decode(&items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	var sawEvent bool
	for _, it := range items {
		if it.Kind == "event" && it.Event != nil && it.Event.HookEvent == "PermissionRequest" {
			sawEvent = true
			if it.Event.Lane != "permission" {
				t.Errorf("event.Lane = %q, want permission", it.Event.Lane)
			}
			if it.Event.Gist == "" {
				t.Errorf("event.Gist empty")
			}
		}
	}
	if !sawEvent {
		t.Errorf("no lane event in timeline; got %+v", items)
	}
}
```

`writeTestEvent` and `newTestServer` should already exist in `api_test.go`. If they don't, use whatever helper the existing `TestHandleAPITimeline*` tests use and follow that exact pattern (do not invent new helpers).

- [ ] **Step 2: Run to verify it fails**

Run: `go test ./internal/daemon/ -run TestHandleAPITimelineIncludesLaneEvents -v`
Expected: FAIL — lane event not produced.

- [ ] **Step 3: Update `handleAPITimeline`**

Change `internal/daemon/api.go:53-66`. Replace:

```go
func (s *Server) handleAPITimeline(w http.ResponseWriter, r *http.Request, id string) {
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ops := model.BuildOperations(events)
	turns, _ := claudecode.ReadConversation(transcriptPathFromEvents(events))
	items := model.MergeTimeline(ops, turns, nil)
	if items == nil {
		items = []model.TimelineItem{}
	}
	writeJSON(w, items)
}
```

With:

```go
func (s *Server) handleAPITimeline(w http.ResponseWriter, r *http.Request, id string) {
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ops := model.BuildOperations(events)
	turns, _ := claudecode.ReadConversation(transcriptPathFromEvents(events))
	laneEvents := model.BuildLaneEvents(events)
	items := model.MergeTimeline(ops, turns, laneEvents)
	if items == nil {
		items = []model.TimelineItem{}
	}
	writeJSON(w, items)
}
```

- [ ] **Step 4: Run all daemon tests**

Run: `go test ./internal/daemon/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/api.go internal/daemon/api_test.go
git commit -m "feat(daemon): include lane events in timeline API"
```

---

## Task 6: TUI per-hook switch extensions

The TUI consumes raw `*event.Event`s, not `TimelineItem`. So the only change is teaching `internal/tui/format.go`'s existing switches about the new hook names: status (from registry severity), lane gist (from the model extractors), and lifecycle dim (for `MessageDisplay`).

**Files:**
- Modify: `internal/tui/format.go:79-100` (`deriveStatus`)
- Modify: `internal/tui/format.go:129-151` (`targetGist`)
- Modify: `internal/tui/format.go:430-439` (`isLifecycleHook`)
- Test: `internal/tui/format_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/tui/format_test.go`:

```go
func TestDeriveStatusLaneEvents(t *testing.T) {
	cases := []struct {
		hook string
		want eventStatus
	}{
		{"PermissionRequest", statusNeutral}, // warn → neutral (no warn status today)
		{"PermissionDenied", statusError},
		{"InstructionsLoaded", statusNeutral},
		{"StopFailure", statusError},
		{"MessageDisplay", statusNeutral},
		{"UnknownNewHook", statusNeutral},
	}
	for _, tc := range cases {
		t.Run(tc.hook, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(`{}`)}
			if got := deriveStatus(ev); got != tc.want {
				t.Errorf("deriveStatus(%s) = %v, want %v", tc.hook, got, tc.want)
			}
		})
	}
}

func TestTargetGistLaneEvents(t *testing.T) {
	cases := []struct {
		hook string
		raw  string
		want string
	}{
		{"CwdChanged", `{"new_cwd":"/a/web"}`, "→ /a/web"},
		{"PermissionRequest",
			`{"tool_name":"Bash","tool_input":{"command":"npm test"}}`,
			"Bash: npm test"},
		{"InstructionsLoaded", `{"path":"CLAUDE.md","memory_type":"project"}`,
			"CLAUDE.md (project)"},
	}
	for _, tc := range cases {
		t.Run(tc.hook, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(tc.raw)}
			got := targetGist(ev)
			if got != tc.want {
				t.Errorf("targetGist(%s) = %q, want %q", tc.hook, got, tc.want)
			}
		})
	}
}

func TestIsLifecycleHookIncludesMessageDisplay(t *testing.T) {
	if !isLifecycleHook("MessageDisplay") {
		t.Error("MessageDisplay should be lifecycle (dim)")
	}
	if isLifecycleHook("PermissionRequest") {
		t.Error("PermissionRequest should NOT be lifecycle (gets warn glyph)")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `go test ./internal/tui/ -run 'TestDeriveStatusLaneEvents|TestTargetGistLaneEvents|TestIsLifecycleHookIncludesMessageDisplay' -v`
Expected: FAIL — `CwdChanged` etc. fall through to `statusNeutral`/`""` already, but `PermissionDenied`/`StopFailure` won't be `statusError`, and `MessageDisplay` is not lifecycle.

- [ ] **Step 3: Extend `deriveStatus`**

In `internal/tui/format.go`, replace the switch at lines 83-99 with:

```go
	switch ev.HookEvent {
	case "PreToolUse":
		return statusRunning
	case "PostToolUse":
		return derivePostStatus(ev.Raw)
	case "PostToolUseFailure":
		return statusError
	case "SubagentStop":
		if hooks.SubagentHasError(ev.Raw) {
			return statusError
		}
		return statusOK
	case "PostCompact":
		return statusOK
	case "PermissionDenied", "StopFailure":
		return statusError
	default:
		return statusNeutral
	}
```

- [ ] **Step 4: Extend `targetGist`**

In `internal/tui/format.go`, replace the switch in `targetGist` (lines 142-149) with:

```go
	var gist string
	switch ev.HookEvent {
	case "PreToolUse", "PostToolUse":
		gist = toolInputGist(ev.ToolName, fields.ToolInput)
	case "UserPromptSubmit":
		gist = fields.Prompt
	case "Notification":
		gist = fields.Notification
	default:
		// Lane events: delegate to the model extractor so TUI and web share
		// the same one-liners.
		if _, ok := event.Lookup(ev.HookEvent); ok {
			gist = model.LaneGistForTUI(ev)
		}
	}
	return clip(gist, 40)
```

Add to the imports at the top of `format.go`:
- `"jordandavis.dev/harness-visualizer/internal/event"` (verify whether already imported — if so, skip)
- `"jordandavis.dev/harness-visualizer/internal/model"`

- [ ] **Step 5: Add the TUI-facing wrapper in the model package**

The TUI shouldn't reach into the unexported `laneGist`. Add a thin exported wrapper at the bottom of `internal/model/lane.go`:

```go
// LaneGistForTUI returns the per-hook one-line summary for an event. It is
// exported so the TUI's targetGist can reuse the same extractors the web
// timeline does, keeping the one-liners in sync across clients.
func LaneGistForTUI(ev *event.Event) string {
	return laneGist(ev)
}
```

- [ ] **Step 6: Extend `isLifecycleHook`**

Replace `internal/tui/format.go` lines 430-439 with:

```go
func isLifecycleHook(hook string) bool {
	switch hook {
	case "SessionStart", "UserPromptSubmit", "Notification", "Stop",
		"SessionEnd", "MessageDisplay":
		return true
	}
	return false
}
```

- [ ] **Step 7: Run TUI tests**

Run: `go test ./internal/tui/ ./internal/model/ -v`
Expected: PASS for all new and existing tests.

- [ ] **Step 8: Commit**

```bash
git add internal/tui/format.go internal/tui/format_test.go internal/model/lane.go
git commit -m "feat(tui): style lane events via shared hook registry + gist extractors"
```

---

## Task 7: Web mirror types

**Files:**
- Modify: `web/src/api/types.ts`

- [ ] **Step 1: Update the file**

Replace the body of `web/src/api/types.ts` with:

```ts
export interface SessionInfo {
  id: string
  event_count: number
  last_seq: number
  mod_time: string
  cwd?: string
  title?: string
}

export type Status = 'running' | 'success' | 'error' | 'neutral'
export type Severity = 'info' | 'warn' | 'error' | 'dim'

export interface Operation {
  id: string
  tool: string
  status: Status
  started_at: string
  duration: number // nanoseconds (Go time.Duration)
  target: string
  seq: number
}

export interface Turn {
  role: 'user' | 'assistant'
  text: string
  thinking?: string
  tool_refs?: string[]
  at: string
}

export interface LaneEvent {
  id: string
  hook_event: string
  lane: string
  gist: string
  severity: Severity
  raw?: unknown
  at: string
  seq: number
}

export interface TimelineItem {
  kind: 'operation' | 'turn' | 'event'
  at: string
  seq: number
  op?: Operation
  turn?: Turn
  event?: LaneEvent
}

export interface DiffOp {
  kind: 'context' | 'del' | 'add'
  text: string
}

export interface OperationDetail {
  id: string
  tool: string
  detail_kind: 'diff' | 'output' | 'generic'
  file_path?: string
  diff?: DiffOp[]
  command?: string
  output?: string
  exit_code?: number
  raw_pre?: unknown
  raw_post?: unknown
}

export interface HookMeta {
  name: string
  glyph: string
  label: string
  lane: string
  severity: Severity
}
```

- [ ] **Step 2: Type-check**

Run: `cd web && npx tsc --noEmit`
Expected: PASS.

- [ ] **Step 3: Run web tests** (they should still pass — nothing consumes the new types yet)

Run: `cd web && npm run test`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add web/src/api/types.ts
git commit -m "feat(web): mirror LaneEvent + HookMeta types from Go"
```

---

## Task 8: Web hook registry fetcher

**Files:**
- Create: `web/src/api/hooks.ts`
- Test: `web/src/api/hooks.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/api/hooks.test.ts`:

```ts
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { loadHooks, lookupHook, _resetHookCache } from './hooks'

describe('hooks registry', () => {
  beforeEach(() => {
    _resetHookCache()
    vi.restoreAllMocks()
  })

  it('fetches and caches hooks on first call', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
      ],
    })
    vi.stubGlobal('fetch', fetchMock)

    await loadHooks()
    await loadHooks() // second call should not refetch

    expect(fetchMock).toHaveBeenCalledTimes(1)
    expect(fetchMock).toHaveBeenCalledWith('/api/hooks')
  })

  it('looks up a known hook', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
      ],
    }))

    await loadHooks()
    const meta = lookupHook('PermissionRequest')
    expect(meta?.glyph).toBe('🔒')
    expect(meta?.label).toBe('Permission')
  })

  it('returns a generic fallback for unknown hooks', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: true, json: async () => [] }))

    await loadHooks()
    const meta = lookupHook('SomeFutureHook')
    expect(meta.name).toBe('SomeFutureHook')
    expect(meta.glyph).toBe('·')
    expect(meta.lane).toBe('unknown')
  })

  it('survives fetch failure (registry stays empty, fallback works)', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({ ok: false, status: 500 }))

    await loadHooks() // should not throw
    const meta = lookupHook('PermissionRequest')
    expect(meta.glyph).toBe('·') // fallback because registry empty
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/api/hooks.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `web/src/api/hooks.ts`**

```ts
import type { HookMeta, Severity } from './types'

let cache: Map<string, HookMeta> | null = null
let inflight: Promise<void> | null = null

const FALLBACK_SEVERITY: Severity = 'info'

/** Internal: clear the cache. Tests only. */
export function _resetHookCache(): void {
  cache = null
  inflight = null
}

/** Fetch the registry once and cache it. Subsequent calls are no-ops. On
 *  fetch failure, cache is set to empty Map so lookups still work via the
 *  generic fallback. */
export async function loadHooks(): Promise<void> {
  if (cache) return
  if (inflight) return inflight
  inflight = (async () => {
    try {
      const res = await fetch('/api/hooks')
      if (!res.ok) {
        cache = new Map()
        return
      }
      const list = (await res.json()) as HookMeta[]
      cache = new Map(list.map((h) => [h.name, h]))
    } catch {
      cache = new Map()
    } finally {
      inflight = null
    }
  })()
  return inflight
}

/** Lookup metadata for a hook event name. Returns a generic fallback when
 *  the hook is unknown, so unrecognized future hooks still render a row. */
export function lookupHook(name: string): HookMeta {
  const found = cache?.get(name)
  if (found) return found
  return {
    name,
    glyph: '·',
    label: name,
    lane: 'unknown',
    severity: FALLBACK_SEVERITY,
  }
}
```

- [ ] **Step 4: Run web tests**

Run: `cd web && npm run test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/api/hooks.ts web/src/api/hooks.test.ts
git commit -m "feat(web): add /api/hooks fetcher with fallback for unknown hooks"
```

---

## Task 9: `<hv-lane-event-row>` component

**Files:**
- Create: `web/src/components/lane-event-row.ts`
- Test: `web/src/components/lane-event-row.test.ts`

- [ ] **Step 1: Write the failing test**

Create `web/src/components/lane-event-row.test.ts`:

```ts
import { describe, expect, it, beforeEach, vi } from 'vitest'
import { loadHooks, _resetHookCache } from '../api/hooks'
import type { LaneEvent } from '../api/types'
import './lane-event-row'

describe('hv-lane-event-row', () => {
  beforeEach(async () => {
    _resetHookCache()
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue({
      ok: true,
      json: async () => [
        { name: 'PermissionRequest', glyph: '🔒', label: 'Permission', lane: 'permission', severity: 'warn' },
        { name: 'MessageDisplay', glyph: '💬', label: 'Message', lane: 'message', severity: 'dim' },
      ],
    }))
    await loadHooks()
  })

  it('renders glyph + label + gist for a known hook', async () => {
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e1', hook_event: 'PermissionRequest', lane: 'permission',
      gist: 'Bash: ls', severity: 'warn', at: '2026-06-01T12:00:00Z', seq: 1,
    } satisfies LaneEvent
    document.body.appendChild(el)
    await el.updateComplete

    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('🔒')
    expect(text).toContain('Permission')
    expect(text).toContain('Bash: ls')
    el.remove()
  })

  it('falls back to hook name as gist when gist is empty', async () => {
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e2', hook_event: 'PermissionRequest', lane: 'permission',
      gist: '', severity: 'warn', at: '2026-06-01T12:00:00Z', seq: 2,
    }
    document.body.appendChild(el)
    await el.updateComplete

    const text = el.shadowRoot!.textContent ?? ''
    expect(text).toContain('PermissionRequest')
    el.remove()
  })

  it('applies dim class for severity=dim', async () => {
    const el = document.createElement('hv-lane-event-row') as any
    el.event = {
      id: 'e3', hook_event: 'MessageDisplay', lane: 'message',
      gist: 'hello', severity: 'dim', at: '2026-06-01T12:00:00Z', seq: 3,
    }
    document.body.appendChild(el)
    await el.updateComplete

    const host = el.shadowRoot!.querySelector('.row')
    expect(host?.className).toContain('sev-dim')
    el.remove()
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/components/lane-event-row.test.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement the component**

Create `web/src/components/lane-event-row.ts`:

```ts
import { css, html, LitElement } from 'lit'
import { customElement, property } from 'lit/decorators.js'
import type { LaneEvent } from '../api/types'
import { lookupHook } from '../api/hooks'

/** Single timeline lane-event row: glyph + label + gist. Glyph/label come
 *  from the shared hook registry (fetched by api/hooks.ts) so the TUI and
 *  web client cannot drift on rendering. */
@customElement('hv-lane-event-row')
export class LaneEventRow extends LitElement {
  @property({ attribute: false }) event!: LaneEvent

  static styles = css`
    :host { display: block; }
    .row {
      padding: 1px 10px 1px 8px;
      white-space: pre;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    .glyph { display: inline-block; width: 2ch; margin-right: 1ch; }
    .label { color: var(--accent); }
    .gist { color: var(--fg); margin-left: 1ch; }
    .sev-dim { color: var(--fg-faint); }
    .sev-dim .label, .sev-dim .gist { color: var(--fg-faint); }
    .sev-warn .glyph { color: var(--yellow); }
    .sev-error .glyph, .sev-error .label { color: var(--red); }
  `

  render() {
    const ev = this.event
    const meta = lookupHook(ev.hook_event)
    const gist = ev.gist || ev.hook_event
    const sev = ev.severity || meta.severity || 'info'
    return html`<div class="row sev-${sev}"
      ><span class="glyph">${meta.glyph}</span
      ><span class="label">${meta.label}</span
      ><span class="gist">${gist}</span
    ></div>`
  }
}

declare global {
  interface HTMLElementTagNameMap {
    'hv-lane-event-row': LaneEventRow
  }
}
```

- [ ] **Step 4: Run web tests**

Run: `cd web && npm run test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/lane-event-row.ts web/src/components/lane-event-row.test.ts
git commit -m "feat(web): add hv-lane-event-row component"
```

---

## Task 10: Timeline third arm + app preloads registry

**Files:**
- Modify: `web/src/components/timeline.ts`
- Modify: `web/src/components/app.ts`

- [ ] **Step 1: Wire the third arm in `timeline.ts`**

Replace the body of the `render()` method in `web/src/components/timeline.ts`:

```ts
  render() {
    if (!this.items.length) return html`<div class="empty">No activity yet</div>`
    return html`<div class="items">
      ${this.items.map((it) => {
        if (it.kind === 'operation' && it.op) {
          const op = it.op
          return html`<hv-op-row
            .op=${op}
            ?selected=${op.id !== '' && op.id === this.selectedOpId}
            @click=${() => op.id && this.pickOp(op.id)}
          ></hv-op-row>`
        }
        if (it.kind === 'turn' && it.turn) return html`<hv-turn-row .turn=${it.turn}></hv-turn-row>`
        if (it.kind === 'event' && it.event) return html`<hv-lane-event-row .event=${it.event}></hv-lane-event-row>`
        return ''
      })}
    </div>`
  }
```

Add the import at the top of `timeline.ts`:

```ts
import './lane-event-row'
```

- [ ] **Step 2: Preload the registry on app mount**

In `web/src/components/app.ts`, find `connectedCallback` (or add one). The app needs to call `loadHooks()` before the first render so `lookupHook` returns real metadata. Add this import at the top:

```ts
import { loadHooks } from '../api/hooks'
```

In the class body, add (or extend) `connectedCallback`:

```ts
  connectedCallback(): void {
    super.connectedCallback()
    void loadHooks()
  }
```

If `connectedCallback` already exists, add the `void loadHooks()` line at the bottom — do not re-implement the rest.

- [ ] **Step 3: Run web tests**

Run: `cd web && npm run test`
Expected: PASS (existing tests do not assert on lane events; we will add the inspector-width test in the next task).

- [ ] **Step 4: Manual sanity-check by build**

Run: `cd web && npm run build`
Expected: build succeeds; no TS errors.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/timeline.ts web/src/components/app.ts
git commit -m "feat(web): render lane events in timeline; preload hook registry on mount"
```

---

## Task 11: Widen web inspector pane

**Files:**
- Modify: `web/src/components/app.ts:31` (grid-template-columns)
- Test: `web/src/components/app.test.ts`

- [ ] **Step 1: Write the failing test**

Append to `web/src/components/app.test.ts`:

```ts
it('uses widened inspector column (520px)', async () => {
  const el = document.createElement('hv-app') as any
  document.body.appendChild(el)
  await el.updateComplete

  // The styles are static; assert against the constructor's static styles.
  const css = (el.constructor as typeof HTMLElement & { styles: unknown }).styles
  const cssText = Array.isArray(css) ? css.map((c) => String(c)).join('\n') : String(css)
  expect(cssText).toContain('520px')
  // And does NOT still contain the old 380px width for the inspector.
  expect(cssText).not.toMatch(/240px 1fr 380px/)

  el.remove()
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd web && npx vitest run src/components/app.test.ts`
Expected: FAIL — CSS still says 380px.

- [ ] **Step 3: Update the grid template**

Change `web/src/components/app.ts:31`:

```ts
      grid-template-columns: 240px 1fr 380px;
```

to:

```ts
      grid-template-columns: 240px 1fr 520px;
```

- [ ] **Step 4: Run web tests**

Run: `cd web && npm run test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add web/src/components/app.ts web/src/components/app.test.ts
git commit -m "feat(web): widen inspector pane from 380px to 520px"
```

---

## Task 12: Smoke-test payloads for all 11 new hooks

**Files:**
- Modify: `scripts/smoke.sh`

- [ ] **Step 1: Read the existing smoke script**

Read `scripts/smoke.sh` to understand the pattern it uses to fire a payload (likely `echo '<json>' | hv hook` or similar). Mirror that pattern exactly — do not invent a new helper.

- [ ] **Step 2: Add one sample payload per new hook**

After the existing payload-firing block in `scripts/smoke.sh`, append a section that fires one payload for each of the 11 new hooks. Use the same `hook_event_name`/`session_id` boilerplate already in the script. Example for the first one (use the script's existing helper, not a new echo):

```bash
fire_event '{"hook_event_name":"PermissionRequest","session_id":"'"$SID"'","tool_name":"Bash","tool_input":{"command":"echo hi"}}'
fire_event '{"hook_event_name":"PermissionDenied","session_id":"'"$SID"'","tool_name":"Edit","tool_input":{"file_path":"foo.go"},"reason":"sandbox"}'
fire_event '{"hook_event_name":"InstructionsLoaded","session_id":"'"$SID"'","path":"CLAUDE.md","memory_type":"project"}'
fire_event '{"hook_event_name":"ConfigChange","session_id":"'"$SID"'","key":"model","old_value":"opus","new_value":"sonnet"}'
fire_event '{"hook_event_name":"CwdChanged","session_id":"'"$SID"'","old_cwd":"/a","new_cwd":"/a/web"}'
fire_event '{"hook_event_name":"TaskCreated","session_id":"'"$SID"'","task_id":"42","subject":"Run baseline"}'
fire_event '{"hook_event_name":"TaskCompleted","session_id":"'"$SID"'","task_id":"42","subject":"Run baseline","status":"completed"}'
fire_event '{"hook_event_name":"UserPromptExpansion","session_id":"'"$SID"'","original":"/loop","expanded":"run baseline"}'
fire_event '{"hook_event_name":"MessageDisplay","session_id":"'"$SID"'","text":"hello"}'
fire_event '{"hook_event_name":"WorktreeRemove","session_id":"'"$SID"'","name":"feature-x","path":"/w/feature-x"}'
fire_event '{"hook_event_name":"StopFailure","session_id":"'"$SID"'","error_type":"rate_limit","message":"slow down"}'
```

Replace `fire_event` with whatever helper the script actually uses (could be a function, could be inline `hv hook`). Replace `$SID` with whatever session-id variable the script uses.

- [ ] **Step 3: Extend the assertion block (if any)**

If `smoke.sh` checks that specific `hook_event_name` strings landed in the JSONL after firing, add the 11 new names to that check. If it just asserts file non-empty, no change needed.

- [ ] **Step 4: Run the smoke script**

Run: `./scripts/smoke.sh`
Expected: PASS (script exits 0).

- [ ] **Step 5: Commit**

```bash
git add scripts/smoke.sh
git commit -m "test(smoke): fire one payload for each PR 3 lane event"
```

---

## Task 13: Update CLAUDE.md to mention the third timeline kind

**Files:**
- Modify: `CLAUDE.md`

- [ ] **Step 1: Read the existing model section**

Read `CLAUDE.md`. Find the paragraph describing `internal/model` and `MergeTimeline` (the existing one says "interleaves operations with conversation Turns into one chronological TimelineItem list").

- [ ] **Step 2: Update it**

Replace the bullet about `internal/model` with one that mentions the third kind. Keep the existing tone; do not pad. Example replacement:

> - **`internal/model`** — harness-agnostic domain types and the timeline logic. `BuildOperations` pairs `PreToolUse`/`PostToolUse` into `Operation`s. `BuildLaneEvents` reduces standalone (non-pairable) hooks like `PermissionRequest`, `InstructionsLoaded`, `CwdChanged`, etc. into `LaneEvent`s, with per-hook gist extractors that read `Raw` defensively. `MergeTimeline` interleaves operations, conversation `Turn`s, and lane events into one chronological `TimelineItem` list. **Operations are authoritative; turns and lane events enrich.** Sort key is `At`, ties broken by `Seq`.

Also append one short sentence to the `internal/event` bullet noting the registry:

> Hook metadata for standalone lane events (glyph, label, severity) is centralized in `internal/event/hooks.go` so the TUI and web client share rendering decisions.

- [ ] **Step 3: Update the HTTP surface section**

In the daemon HTTP surface section, append `/api/hooks` to the JSON API list:

> JSON API for the web UI: `/api/sessions`, `/api/sessions/{id}/timeline?after=`, `/api/sessions/{id}/operations/{opID}`, `/api/hooks`. `/` serves the embedded SPA.

- [ ] **Step 4: Verify build still works**

Run: `make test`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add CLAUDE.md
git commit -m "docs(claude.md): document lane events and /api/hooks"
```

---

## Final verification

- [ ] **Step 1: Full Go test suite**

Run: `make test`
Expected: PASS.

- [ ] **Step 2: Full web test suite**

Run: `make test-web`
Expected: PASS.

- [ ] **Step 3: Full build**

Run: `make build`
Expected: succeeds; produces `hv` binary.

- [ ] **Step 4: Smoke**

Run: `./scripts/smoke.sh`
Expected: PASS.

- [ ] **Step 5: Open PR**

```bash
git push -u origin worktree-issue-1-pr3-new-lanes
gh pr create --title "feat: new lanes — render Permission/Instructions/Config/Cwd/Task events (issue #1 PR 3)" --body "$(cat <<'EOF'
## Summary
- Adds shared hook registry (`internal/event/hooks.go`) and exposes it at `GET /api/hooks` so TUI and web can't drift on glyphs/labels for the 11 non-pairable hook events.
- Extends `TimelineItem` to a 3-way union (operation | turn | event) with a new `LaneEvent` carrying a per-hook `Gist` derived defensively from `Raw`.
- TUI: `format.go` switches recognize the new hooks; `MessageDisplay` is treated as lifecycle (dim); `PermissionDenied` / `StopFailure` get error glyph.
- Web: new `<hv-lane-event-row>` component, registry fetcher with unknown-hook fallback, inspector pane widened 380px → 520px.
- Smoke script fires one sample payload for each of the 11 new events.

Spec: `docs/superpowers/specs/2026-06-01-pr3-new-lanes-design.md`
Plan: `docs/superpowers/plans/2026-06-01-pr3-new-lanes.md`

## Test plan
- [ ] `make test` (Go)
- [ ] `make test-web` (vitest)
- [ ] `make build` (full build, embedded UI)
- [ ] `./scripts/smoke.sh` (end-to-end capture)
- [ ] Manual: start daemon, fire a few PR 3 payloads via the smoke helper, open `http://127.0.0.1:7842/`, confirm permission/instructions/cwd rows render with correct glyphs and the inspector column is visibly wider.
EOF
)"
```
