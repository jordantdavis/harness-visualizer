# Issue #1 PR 2 — Operations Model Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the harness-agnostic operations model so `PostToolUseFailure`, `SubagentStart`/`SubagentStop`, and `PreCompact`/`PostCompact` produce paired `Operation` rows with correct status. Introduce a per-hook decoder package so all `Raw`-payload parsing lives in one place.

**Architecture:** Add a `Kind` discriminator (`"tool" | "subagent" | "compact"`) to `Operation`. Rewrite `BuildOperations` as one engine that handles three Pre/Post pairs per kind (ID-based first, heuristic fallback per kind only). Mirror the change in the TUI's `buildDisplayRows`. Move every `Raw`-JSON lookup into the new `internal/source/claudecode/hooks` package.

**Tech Stack:** Go 1.x, standard library only, test files `*_test.go` co-located with code.

---

## Spec reference

`docs/superpowers/specs/2026-06-01-issue-1-pr2-operations-model.md`

## File map

**Create:**
- `internal/source/claudecode/hooks/hooks.go` — defensive decoders for Raw payloads
- `internal/source/claudecode/hooks/hooks_test.go`

**Modify:**
- `internal/model/operation.go` — add `Kind`, rewrite `BuildOperations` for three kinds
- `internal/model/operation_test.go` — add tests for new kinds + failure pairing
- `internal/model/status.go` — extend `DeriveStatus` for new hooks
- `internal/model/status_test.go` — add cases for new status branches
- `internal/model/target.go` — branch `ExtractTarget` on subagent/compact hooks
- `internal/model/target_test.go` — add subagent/compact target tests
- `internal/tui/pairing.go` — parity rewrite + replace `extractToolUseID` with `hooks.ToolUseID`
- `internal/tui/pairing_test.go` — add tests for new kinds
- `internal/tui/format.go` — extend `deriveStatus` for new post hooks (mirrors model)
- `internal/tui/format_test.go` — add cases for new status branches
- `scripts/smoke.sh` — add one `PostToolUseFailure` payload

## Conventions reminder

- **Always degrade gracefully.** Decoders return zero values on absent/malformed; pairing produces a still-running row when no Post matches.
- **TDD strictly.** Write the failing test, run it, see it fail, write the minimum code, see it pass, commit.
- **Frequent commits.** Each task ends with one commit. Use Conventional Commit prefixes (`feat:`, `refactor:`, `test:`).
- **Run `go test ./...` and `make test-web` only when relevant** (no web changes in this PR; vitest skip is fine).
- **Don't rename existing functions or change exported signatures** beyond what each task specifies. The web SPA reads `/api/sessions/{id}/timeline` — adding fields is fine; removing/renaming is not.

---

## Task 1 — `internal/source/claudecode/hooks` decoder package

**Files:**
- Create: `internal/source/claudecode/hooks/hooks.go`
- Create: `internal/source/claudecode/hooks/hooks_test.go`

This package owns every `Raw` JSON lookup we need for PR 2. It depends only on `encoding/json`. Field-name choices for subagent and compact are **speculative**; the package is the single chokepoint to fix when real payloads arrive.

- [ ] **Step 1: Write the failing test file**

Create `internal/source/claudecode/hooks/hooks_test.go`:

```go
// internal/source/claudecode/hooks/hooks_test.go
package hooks

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestToolUseID(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"present", raw(`{"tool_use_id":"abc"}`), "abc"},
		{"absent", raw(`{"other":1}`), ""},
		{"empty raw", raw(``), ""},
		{"malformed", raw(`not json`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ToolUseID(c.raw); got != c.want {
				t.Fatalf("ToolUseID = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubagentID(t *testing.T) {
	if got := SubagentID(raw(`{"subagent_id":"sa-1"}`)); got != "sa-1" {
		t.Fatalf("SubagentID = %q, want sa-1", got)
	}
	if got := SubagentID(raw(`{}`)); got != "" {
		t.Fatalf("SubagentID absent = %q, want \"\"", got)
	}
	if got := SubagentID(raw(`not json`)); got != "" {
		t.Fatalf("SubagentID malformed = %q, want \"\"", got)
	}
}

func TestSubagentTarget(t *testing.T) {
	// Prefer subagent_type, fall back to description, else "".
	if got := SubagentTarget(raw(`{"subagent_type":"engineer"}`)); got != "engineer" {
		t.Fatalf("subagent_type = %q, want engineer", got)
	}
	if got := SubagentTarget(raw(`{"description":"do a thing"}`)); got != "do a thing" {
		t.Fatalf("description fallback = %q, want \"do a thing\"", got)
	}
	if got := SubagentTarget(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestSubagentHasError(t *testing.T) {
	if !SubagentHasError(raw(`{"error":"boom"}`)) {
		t.Fatal("error field present should return true")
	}
	if !SubagentHasError(raw(`{"error":{"message":"x"}}`)) {
		t.Fatal("non-empty error object should return true")
	}
	if SubagentHasError(raw(`{"error":""}`)) {
		t.Fatal("empty error string should return false")
	}
	if SubagentHasError(raw(`{}`)) {
		t.Fatal("absent error should return false")
	}
	if SubagentHasError(raw(`not json`)) {
		t.Fatal("malformed should return false")
	}
}

func TestCompactID(t *testing.T) {
	if got := CompactID(raw(`{"compact_id":"c-1"}`)); got != "c-1" {
		t.Fatalf("CompactID = %q, want c-1", got)
	}
	if got := CompactID(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestCompactTarget(t *testing.T) {
	// Prefer trigger, fall back to reason, else "".
	if got := CompactTarget(raw(`{"trigger":"auto"}`)); got != "auto" {
		t.Fatalf("trigger = %q, want auto", got)
	}
	if got := CompactTarget(raw(`{"reason":"manual"}`)); got != "manual" {
		t.Fatalf("reason fallback = %q, want manual", got)
	}
	if got := CompactTarget(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestPostToolUseFailureMessage(t *testing.T) {
	if got := PostToolUseFailureMessage(raw(`{"error":"boom"}`)); got != "boom" {
		t.Fatalf("error string = %q, want boom", got)
	}
	if got := PostToolUseFailureMessage(raw(`{"error":{"message":"oops"}}`)); got != "oops" {
		t.Fatalf("error.message = %q, want oops", got)
	}
	if got := PostToolUseFailureMessage(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}
```

- [ ] **Step 2: Run tests, verify they fail**

```
go test ./internal/source/claudecode/hooks/
```

Expected: build fails (`undefined: ToolUseID`, etc.).

- [ ] **Step 3: Write the minimal package**

Create `internal/source/claudecode/hooks/hooks.go`:

```go
// Package hooks adapts Claude Code's per-hook Raw payloads into harness-agnostic
// scalars. It is the ONLY place in hv that knows hook payload field names.
// All functions are defensive: a missing field, malformed JSON, or empty input
// yields a zero value rather than an error.
package hooks

import "encoding/json"

// ToolUseID returns the top-level "tool_use_id" from a PreToolUse/PostToolUse/
// PostToolUseFailure Raw payload, or "" if absent or malformed.
func ToolUseID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["tool_use_id"])
	})
}

// SubagentID returns the top-level "subagent_id" from a SubagentStart/SubagentStop
// Raw payload, or "" if absent.
func SubagentID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["subagent_id"])
	})
}

// SubagentTarget returns a human-readable label for a subagent operation,
// preferring "subagent_type" and falling back to "description".
func SubagentTarget(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		if s := unquote(m["subagent_type"]); s != "" {
			return s
		}
		return unquote(m["description"])
	})
}

// SubagentHasError reports whether the SubagentStop payload signals an error.
// True when "error" is a non-empty string, or an object/array of any shape.
func SubagentHasError(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	v, ok := m["error"]
	if !ok || len(v) == 0 {
		return false
	}
	// Distinguish "" / null from any other JSON value.
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s != ""
	}
	// Non-string (object/array/number/bool) — treat any value as an error
	// except JSON null.
	return string(v) != "null"
}

// CompactID returns the top-level "compact_id" from a PreCompact/PostCompact
// Raw payload, or "" if absent.
func CompactID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["compact_id"])
	})
}

// CompactTarget returns a human-readable label for a compact operation,
// preferring "trigger" and falling back to "reason".
func CompactTarget(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		if s := unquote(m["trigger"]); s != "" {
			return s
		}
		return unquote(m["reason"])
	})
}

// PostToolUseFailureMessage returns a short failure description from a
// PostToolUseFailure payload. Tries "error" as a string first, then
// "error.message".
func PostToolUseFailureMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if s := unquote(m["error"]); s != "" {
		return s
	}
	if e, ok := m["error"]; ok && len(e) > 0 {
		var inner struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e, &inner) == nil {
			return inner.Message
		}
	}
	return ""
}

// decodeString runs f over a top-level JSON object decoded from raw.
// Returns "" if raw is empty or not a JSON object.
func decodeString(raw json.RawMessage, f func(map[string]json.RawMessage) string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return f(m)
}

// unquote unmarshals a JSON RawMessage as a string. Returns "" on any error.
func unquote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}
```

- [ ] **Step 4: Run tests, verify they pass**

```
go test ./internal/source/claudecode/hooks/
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/source/claudecode/hooks/
git commit -m "feat: per-hook decoder package for Raw payload fields"
```

---

## Task 2 — `Operation.Kind` discriminator (additive only)

Adds `Kind` to `Operation` with JSON tag and a "tool" default at construction. Behavior is unchanged this task: only the field is new and only existing tool ops are produced.

**Files:**
- Modify: `internal/model/operation.go:16-24` (Operation struct), `internal/model/operation.go:58-65` (op construction in `BuildOperations`)
- Modify: `internal/model/operation_test.go` (extend existing tests)

- [ ] **Step 1: Write the failing test**

Add to `internal/model/operation_test.go` (at the bottom of the file):

```go
func TestBuildOperations_DefaultKindIsTool(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Edit", `{"tool_use_id":"a"}`, t0),
		ev(2, "PostToolUse", "Edit", `{"tool_use_id":"a","tool_response":{"exit_code":0}}`, t0.Add(100*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	if ops[0].Kind != "tool" {
		t.Fatalf("Kind = %q, want \"tool\"", ops[0].Kind)
	}
}
```

- [ ] **Step 2: Run the test, verify it fails**

```
go test ./internal/model/ -run TestBuildOperations_DefaultKindIsTool
```

Expected: FAIL (`Kind` field unknown).

- [ ] **Step 3: Add the `Kind` field and set it in `BuildOperations`**

In `internal/model/operation.go`, replace the `Operation` struct:

```go
type Operation struct {
	Kind      string        `json:"kind"`           // "tool" | "subagent" | "compact"
	ID        string        `json:"id"`             // tool_use_id | subagent_id | compact_id
	Tool      string        `json:"tool,omitempty"` // empty for non-tool kinds
	Status    Status        `json:"status"`
	StartedAt time.Time     `json:"started_at"`
	Duration  time.Duration `json:"duration"`
	Target    string        `json:"target"`
	Seq       int64         `json:"seq"`
}
```

In `BuildOperations`, inside the `for _, e := range events` loop that constructs `op`, add `Kind: "tool"`:

```go
op := Operation{
	Kind:      "tool",
	ID:        toolUseID(e.Raw),
	Tool:      e.ToolName,
	Status:    StatusRunning,
	StartedAt: e.CapturedAt,
	Target:    ExtractTarget(e),
	Seq:       e.Seq,
}
```

- [ ] **Step 4: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS (existing tests still pass; new test passes).

- [ ] **Step 5: Commit**

```bash
git add internal/model/operation.go internal/model/operation_test.go
git commit -m "feat(model): add Kind discriminator to Operation (default \"tool\")"
```

---

## Task 3 — Switch model decoders to `hooks` package

Replace the duplicated `toolUseID` helper in `internal/model/operation.go` with `hooks.ToolUseID`. Pure refactor — no behavior change.

**Files:**
- Modify: `internal/model/operation.go:93-106` (delete the local `toolUseID` func), `internal/model/operation.go:45,59` (call sites), top-of-file import block

- [ ] **Step 1: Confirm existing tests are passing baseline**

```
go test ./internal/model/...
```

Expected: PASS.

- [ ] **Step 2: Replace the helper and call sites**

In `internal/model/operation.go`:

1. Add to the import block:

```go
import (
	"sort"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)
```

2. Delete the existing `toolUseID` function (lines 93–106).

3. Replace both call sites — change `toolUseID(e.Raw)` to `hooks.ToolUseID(e.Raw)` in:
   - the Post-scan loop where `postByID[id] = idx` is assigned
   - the Pre-walk loop where `op.ID` is set

- [ ] **Step 3: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS (no behavior change).

- [ ] **Step 4: Commit**

```bash
git add internal/model/operation.go
git commit -m "refactor(model): use hooks.ToolUseID instead of local helper"
```

---

## Task 4 — `DeriveStatus` for `PostToolUseFailure`

Failure post is unconditional `StatusError`. No payload inspection needed; the hook event name itself is the signal.

**Files:**
- Modify: `internal/model/status.go:30-39` (DeriveStatus switch)
- Modify: `internal/model/status_test.go` (add cases)

- [ ] **Step 1: Write the failing test**

Add a new case to the `cases` table in `internal/model/status_test.go`:

```go
{"post failure is error", &event.Event{HookEvent: "PostToolUseFailure"}, StatusError},
{"post failure ignores raw", &event.Event{HookEvent: "PostToolUseFailure", Raw: []byte(`{"tool_response":{"exit_code":0}}`)}, StatusError},
```

- [ ] **Step 2: Run, verify failure**

```
go test ./internal/model/ -run TestDeriveStatus
```

Expected: FAIL (returns StatusNeutral via the default arm).

- [ ] **Step 3: Add the case to `DeriveStatus`**

In `internal/model/status.go`, extend the switch:

```go
func DeriveStatus(ev *event.Event) Status {
	switch ev.HookEvent {
	case "PreToolUse":
		return StatusRunning
	case "PostToolUse":
		return postStatus(ev.Raw)
	case "PostToolUseFailure":
		return StatusError
	default:
		return StatusNeutral
	}
}
```

- [ ] **Step 4: Run, verify pass**

```
go test ./internal/model/ -run TestDeriveStatus
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/status.go internal/model/status_test.go
git commit -m "feat(model): derive StatusError from PostToolUseFailure"
```

---

## Task 5 — Tool-kind pairing accepts `PostToolUseFailure`

`PreToolUse` should pair with `PostToolUseFailure` when there's no `PostToolUse`. Status follows from `DeriveStatus(post)` which Task 4 set up.

**Files:**
- Modify: `internal/model/operation.go:39-51` (post-scan), `internal/model/operation.go:54-90` (pre-walk)
- Modify: `internal/model/operation_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/model/operation_test.go`:

```go
func TestBuildOperations_PairsToolUseWithFailurePost(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"f1","tool_input":{"command":"false"}}`, t0),
		ev(2, "PostToolUseFailure", "Bash", `{"tool_use_id":"f1","error":"boom"}`, t0.Add(50*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "tool" || op.Status != StatusError {
		t.Fatalf("kind=%q status=%q, want tool/error", op.Kind, op.Status)
	}
	if op.Duration != 50*time.Millisecond {
		t.Fatalf("duration = %v, want 50ms", op.Duration)
	}
}

func TestBuildOperations_HeuristicFailurePost(t *testing.T) {
	// No tool_use_id on either; heuristic still pairs same-tool PostToolUseFailure.
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Read", `{"tool_input":{"file_path":"x"}}`, t0),
		ev(2, "PostToolUseFailure", "Read", `{"error":"nope"}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusError {
		t.Fatalf("want one error op, got %+v", ops)
	}
}
```

- [ ] **Step 2: Run, verify failure**

```
go test ./internal/model/ -run TestBuildOperations_PairsToolUseWithFailurePost
go test ./internal/model/ -run TestBuildOperations_HeuristicFailurePost
```

Expected: both FAIL — Failure posts aren't collected for pairing yet.

- [ ] **Step 3: Extend the post-scan in `BuildOperations`**

In `internal/model/operation.go`, replace the post-scan condition. Change:

```go
for _, e := range events {
	if e.HookEvent != "PostToolUse" {
		continue
	}
	...
}
```

to:

```go
for _, e := range events {
	if e.HookEvent != "PostToolUse" && e.HookEvent != "PostToolUseFailure" {
		continue
	}
	...
}
```

The pairing inside the Pre walk is unchanged: it already looks up by `op.ID` and by `e.ToolName`, both of which match Failure posts the same way.

- [ ] **Step 4: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS (new tests + existing ones unaffected).

- [ ] **Step 5: Commit**

```bash
git add internal/model/operation.go internal/model/operation_test.go
git commit -m "feat(model): pair PreToolUse with PostToolUseFailure"
```

---

## Task 6 — Generalise `BuildOperations` to three kinds

This is the meatiest task. Introduce a per-kind pair table and rewrite `BuildOperations` to iterate it. Same algorithm three times: collect posts → walk pres → pair by ID, then heuristic.

**Files:**
- Modify: `internal/model/operation.go` (rewrite `BuildOperations`)
- Modify: `internal/model/operation_test.go` (new test cases)

- [ ] **Step 1: Write failing tests**

Append to `internal/model/operation_test.go`:

```go
func TestBuildOperations_SubagentPairByID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-1","subagent_type":"engineer"}`, t0),
		ev(2, "SubagentStop", "", `{"subagent_id":"sa-1"}`, t0.Add(2*time.Second)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "subagent" {
		t.Fatalf("Kind = %q, want subagent", op.Kind)
	}
	if op.ID != "sa-1" {
		t.Fatalf("ID = %q, want sa-1", op.ID)
	}
	if op.Status != StatusSuccess {
		t.Fatalf("Status = %q, want success", op.Status)
	}
	if op.Duration != 2*time.Second {
		t.Fatalf("Duration = %v, want 2s", op.Duration)
	}
}

func TestBuildOperations_SubagentStopWithError(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-2"}`, t0),
		ev(2, "SubagentStop", "", `{"subagent_id":"sa-2","error":"timeout"}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusError {
		t.Fatalf("want one error op, got %+v", ops)
	}
}

func TestBuildOperations_SubagentUnpairedIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{"subagent_id":"sa-3"}`, t0),
	})
	if len(ops) != 1 || ops[0].Kind != "subagent" || ops[0].Status != StatusRunning {
		t.Fatalf("want one running subagent op, got %+v", ops)
	}
}

func TestBuildOperations_SubagentHeuristicWithoutID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SubagentStart", "", `{}`, t0),
		ev(2, "SubagentStop", "", `{}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 {
		t.Fatalf("want one paired subagent op, got %d", len(ops))
	}
	if ops[0].Duration != time.Second {
		t.Fatalf("Duration = %v, want 1s", ops[0].Duration)
	}
}

func TestBuildOperations_CompactPairByID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{"compact_id":"c-1","trigger":"auto"}`, t0),
		ev(2, "PostCompact", "", `{"compact_id":"c-1"}`, t0.Add(500*time.Millisecond)),
	})
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.Kind != "compact" || op.ID != "c-1" || op.Status != StatusSuccess {
		t.Fatalf("unexpected op: %+v", op)
	}
}

func TestBuildOperations_CompactUnpairedIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{"compact_id":"c-2"}`, t0),
	})
	if len(ops) != 1 || ops[0].Kind != "compact" || ops[0].Status != StatusRunning {
		t.Fatalf("want one running compact op, got %+v", ops)
	}
}

func TestBuildOperations_CrossKindHeuristicIsolation(t *testing.T) {
	// A SubagentStop must never close a PreCompact (or vice versa), even
	// without IDs and even when Seq order would allow it.
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreCompact", "", `{}`, t0),
		ev(2, "SubagentStop", "", `{}`, t0.Add(time.Second)),
	})
	// Result: one running compact + one standalone subagent stop is NOT an
	// op (no matching Start). So we expect exactly one op (the running compact).
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1 running compact", len(ops))
	}
	if ops[0].Kind != "compact" || ops[0].Status != StatusRunning {
		t.Fatalf("unexpected op: %+v", ops[0])
	}
}

func TestBuildOperations_MixedKindsChronological(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"t1"}`, t0),
		ev(2, "SubagentStart", "", `{"subagent_id":"sa"}`, t0.Add(time.Second)),
		ev(3, "PostToolUse", "Bash", `{"tool_use_id":"t1","tool_response":{"exit_code":0}}`, t0.Add(2*time.Second)),
		ev(4, "PreCompact", "", `{"compact_id":"c"}`, t0.Add(3*time.Second)),
		ev(5, "SubagentStop", "", `{"subagent_id":"sa"}`, t0.Add(4*time.Second)),
		ev(6, "PostCompact", "", `{"compact_id":"c"}`, t0.Add(5*time.Second)),
	})
	if len(ops) != 3 {
		t.Fatalf("got %d ops, want 3 (tool, subagent, compact)", len(ops))
	}
	want := []string{"tool", "subagent", "compact"}
	for i, w := range want {
		if ops[i].Kind != w {
			t.Errorf("ops[%d].Kind = %q, want %q", i, ops[i].Kind, w)
		}
	}
}
```

- [ ] **Step 2: Run, verify failure**

```
go test ./internal/model/ -run TestBuildOperations
```

Expected: new tests FAIL (non-tool pre events are ignored today).

- [ ] **Step 3: Rewrite `BuildOperations`**

Replace the entire `BuildOperations` function in `internal/model/operation.go` with this generalised version. The helper closures keep the body short and let each kind's pair table drive identical logic.

```go
// pairSpec describes one Pre/Post hook pair handled by BuildOperations.
type pairSpec struct {
	kind   string
	pre    string
	posts  []string // accepted post hook events (first listed wins on ID ties)
	id     func(raw json.RawMessage) string
	target func(ev *event.Event) string
	// toolName: for the tool kind we key the heuristic by ev.ToolName; for
	// non-tool kinds the heuristic is keyed by the constant kind name (since
	// ToolName is empty on subagent/compact events).
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
// and returns the resulting operations in chronological (Seq) order.
// Pairing prefers a stable ID match, then falls back to the first unclaimed
// Post of the same kind (and same tool, when applicable) with a later Seq.
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

func buildPairs(events []*event.Event, sp pairSpec) []Operation {
	type slot struct {
		ev      *event.Event
		claimed bool
	}
	postSet := map[string]bool{}
	for _, p := range sp.posts {
		postSet[p] = true
	}

	postByID := map[string]int{}
	postByKey := map[string][]int{}
	var posts []slot

	for _, e := range events {
		if !postSet[e.HookEvent] {
			continue
		}
		idx := len(posts)
		posts = append(posts, slot{ev: e})
		if id := sp.id(e.Raw); id != "" {
			// First wins for ID — preserves "PostToolUse beats PostToolUseFailure
			// on the same tool_use_id" since PostToolUse appears first in sp.posts
			// but both are scanned in event order; we prefer the existing entry.
			if _, exists := postByID[id]; !exists {
				postByID[id] = idx
			}
		}
		postByKey[sp.toolKey(e)] = append(postByKey[sp.toolKey(e)], idx)
	}

	var ops []Operation
	for _, e := range events {
		if e.HookEvent != sp.pre {
			continue
		}
		op := Operation{
			Kind:      sp.kind,
			ID:        sp.id(e.Raw),
			Tool:      e.ToolName, // empty for non-tool kinds
			Status:    StatusRunning,
			StartedAt: e.CapturedAt,
			Target:    sp.target(e),
			Seq:       e.Seq,
		}
		var post *event.Event
		if op.ID != "" {
			if idx, ok := postByID[op.ID]; ok && !posts[idx].claimed {
				posts[idx].claimed = true
				post = posts[idx].ev
			}
		}
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
```

Notes for the engineer:
- The local `toolUseID` helper deleted in Task 3 is now fully unused. If you still see it in the file, delete it (the import block already has `hooks`).
- The function signature of `BuildOperations` is unchanged.

- [ ] **Step 4: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS (every test including the new mixed-kind, isolation, and heuristic tests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/operation.go internal/model/operation_test.go
git commit -m "feat(model): generalise BuildOperations to handle tool, subagent, compact"
```

---

## Task 7 — `DeriveStatus` for subagent / compact

Now wire status for the non-tool kinds.

**Files:**
- Modify: `internal/model/status.go`
- Modify: `internal/model/status_test.go`

- [ ] **Step 1: Write failing tests**

Add to the `cases` table in `internal/model/status_test.go`:

```go
{"subagent stop without error is success", &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{}`)}, StatusSuccess},
{"subagent stop with error is error", &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{"error":"boom"}`)}, StatusError},
{"subagent stop with empty error is success", &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{"error":""}`)}, StatusSuccess},
{"post compact is success", &event.Event{HookEvent: "PostCompact", Raw: []byte(`{}`)}, StatusSuccess},
```

- [ ] **Step 2: Run, verify failure**

```
go test ./internal/model/ -run TestDeriveStatus
```

Expected: FAIL (all four hit the default StatusNeutral arm).

- [ ] **Step 3: Extend `DeriveStatus`**

Edit `internal/model/status.go`. Update imports to include the hooks package, then extend the switch:

```go
import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// DeriveStatus inspects a single event to derive its lifecycle status.
//   PreToolUse                       → StatusRunning
//   PostToolUse                      → from tool_response.exit_code
//   PostToolUseFailure               → StatusError
//   SubagentStop                     → StatusError if .error present, else StatusSuccess
//   PostCompact                      → StatusSuccess
//   anything else                    → StatusNeutral
// Parse failures fall through to StatusNeutral.
func DeriveStatus(ev *event.Event) Status {
	switch ev.HookEvent {
	case "PreToolUse":
		return StatusRunning
	case "PostToolUse":
		return postStatus(ev.Raw)
	case "PostToolUseFailure":
		return StatusError
	case "SubagentStop":
		if hooks.SubagentHasError(ev.Raw) {
			return StatusError
		}
		return StatusSuccess
	case "PostCompact":
		return StatusSuccess
	default:
		return StatusNeutral
	}
}
```

- [ ] **Step 4: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS. (`TestBuildOperations_SubagentStopWithError` and `TestBuildOperations_SubagentPairByID` now actually validate status too, so this confirms Task 6's tests stayed correct.)

- [ ] **Step 5: Commit**

```bash
git add internal/model/status.go internal/model/status_test.go
git commit -m "feat(model): derive status for SubagentStop and PostCompact"
```

---

## Task 8 — `ExtractTarget` for subagent / compact

Make non-tool operations carry a useful Target so the UI has something to show.

**Files:**
- Modify: `internal/model/target.go`
- Modify: `internal/model/target_test.go`

- [ ] **Step 1: Read existing test file for style**

```
cat internal/model/target_test.go
```

Read the existing test patterns; mirror them.

- [ ] **Step 2: Write failing tests**

Append to `internal/model/target_test.go`:

```go
func TestExtractTarget_SubagentType(t *testing.T) {
	ev := &event.Event{
		HookEvent: "SubagentStart",
		Raw:       []byte(`{"subagent_type":"engineer"}`),
	}
	if got := ExtractTarget(ev); got != "engineer" {
		t.Fatalf("ExtractTarget = %q, want engineer", got)
	}
}

func TestExtractTarget_SubagentDescriptionFallback(t *testing.T) {
	ev := &event.Event{
		HookEvent: "SubagentStop",
		Raw:       []byte(`{"description":"do a thing"}`),
	}
	if got := ExtractTarget(ev); got != "do a thing" {
		t.Fatalf("ExtractTarget = %q, want \"do a thing\"", got)
	}
}

func TestExtractTarget_CompactTrigger(t *testing.T) {
	ev := &event.Event{
		HookEvent: "PreCompact",
		Raw:       []byte(`{"trigger":"auto"}`),
	}
	if got := ExtractTarget(ev); got != "auto" {
		t.Fatalf("ExtractTarget = %q, want auto", got)
	}
}

func TestExtractTarget_CompactReasonFallback(t *testing.T) {
	ev := &event.Event{
		HookEvent: "PostCompact",
		Raw:       []byte(`{"reason":"manual"}`),
	}
	if got := ExtractTarget(ev); got != "manual" {
		t.Fatalf("ExtractTarget = %q, want manual", got)
	}
}

func TestExtractTarget_PostToolUseFailure(t *testing.T) {
	ev := &event.Event{
		HookEvent: "PostToolUseFailure",
		ToolName:  "Bash",
		Raw:       []byte(`{"tool_input":{"command":"false"}}`),
	}
	// PostToolUseFailure carries the same tool_input shape as PostToolUse;
	// target should be the command gist.
	if got := ExtractTarget(ev); got != "false" {
		t.Fatalf("ExtractTarget = %q, want \"false\"", got)
	}
}
```

- [ ] **Step 3: Run, verify failure**

```
go test ./internal/model/ -run TestExtractTarget
```

Expected: all five new tests FAIL.

- [ ] **Step 4: Extend `ExtractTarget`**

Edit `internal/model/target.go`. Add import for `hooks` and extend the switch:

```go
package model

import (
	"encoding/json"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

func ExtractTarget(ev *event.Event) string {
	if len(ev.Raw) == 0 {
		return ""
	}
	switch ev.HookEvent {
	case "PreToolUse", "PostToolUse", "PostToolUseFailure":
		var fields struct {
			ToolInput json.RawMessage `json:"tool_input"`
		}
		if err := json.Unmarshal(ev.Raw, &fields); err != nil {
			return ""
		}
		return toolInputGist(ev.ToolName, fields.ToolInput)
	case "SubagentStart", "SubagentStop":
		return hooks.SubagentTarget(ev.Raw)
	case "PreCompact", "PostCompact":
		return hooks.CompactTarget(ev.Raw)
	case "UserPromptSubmit":
		var fields struct {
			Prompt string `json:"prompt"`
		}
		if err := json.Unmarshal(ev.Raw, &fields); err != nil {
			return ""
		}
		return strings.TrimSpace(fields.Prompt)
	case "Notification":
		var fields struct {
			Notification string `json:"notification"`
		}
		if err := json.Unmarshal(ev.Raw, &fields); err != nil {
			return ""
		}
		return strings.TrimSpace(fields.Notification)
	}
	return ""
}
```

(`toolInputGist` is unchanged. The original monolithic `fields` struct was split per branch to keep each unmarshal targeted; this is functionally identical for existing callers.)

- [ ] **Step 5: Run all model tests, verify they pass**

```
go test ./internal/model/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/model/target.go internal/model/target_test.go
git commit -m "feat(model): extract Target for subagent and compact operations"
```

---

## Task 9 — TUI `deriveStatus` parity for new hook posts

The TUI has its own `eventStatus` enum (separate from `model.Status`) and its own `deriveStatus`. Mirror the model-layer additions.

**Files:**
- Modify: `internal/tui/format.go:76-115` (`deriveStatus` and `derivePostStatus`)
- Modify: `internal/tui/format_test.go`

- [ ] **Step 1: Read context to confirm shape**

```
sed -n '70,120p' internal/tui/format.go
```

You should see `deriveStatus(ev *event.Event) eventStatus` switching on `ev.HookEvent`.

- [ ] **Step 2: Write failing tests**

Append to `internal/tui/format_test.go`:

```go
func TestDeriveStatusPostToolUseFailure(t *testing.T) {
	ev := &event.Event{HookEvent: "PostToolUseFailure"}
	if got := deriveStatus(ev); got != statusError {
		t.Errorf("PostToolUseFailure = %v, want statusError", got)
	}
}

func TestDeriveStatusSubagentStop(t *testing.T) {
	ok := &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{}`)}
	if got := deriveStatus(ok); got != statusOK {
		t.Errorf("SubagentStop ok = %v, want statusOK", got)
	}
	err := &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{"error":"boom"}`)}
	if got := deriveStatus(err); got != statusError {
		t.Errorf("SubagentStop err = %v, want statusError", got)
	}
}

func TestDeriveStatusPostCompact(t *testing.T) {
	ev := &event.Event{HookEvent: "PostCompact", Raw: []byte(`{}`)}
	if got := deriveStatus(ev); got != statusOK {
		t.Errorf("PostCompact = %v, want statusOK", got)
	}
}
```

- [ ] **Step 3: Run, verify failure**

```
go test ./internal/tui/ -run TestDeriveStatus
```

Expected: new tests FAIL (return statusNeutral via default).

- [ ] **Step 4: Extend `deriveStatus`**

Edit `internal/tui/format.go`. Add the hooks import and update the function (it currently lives around lines 76–94):

```go
import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
	// ...other existing imports unchanged
)

func deriveStatus(ev *event.Event) eventStatus {
	if ev == nil {
		return statusNeutral
	}
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
	default:
		return statusNeutral
	}
}
```

Leave `derivePostStatus` exactly as-is (it still handles `PostToolUse`'s `exit_code` shape).

- [ ] **Step 5: Run all TUI tests, verify they pass**

```
go test ./internal/tui/...
```

Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/tui/format.go internal/tui/format_test.go
git commit -m "feat(tui): derive status for PostToolUseFailure, SubagentStop, PostCompact"
```

---

## Task 10 — TUI pairing parity for new kinds

Generalise `buildDisplayRows` the same way Task 6 generalised `BuildOperations`. Also swap `extractToolUseID` for `hooks.ToolUseID` (and delete the local helper + its tests, which now duplicate hooks_test.go).

**Files:**
- Modify: `internal/tui/pairing.go`
- Modify: `internal/tui/pairing_test.go`

- [ ] **Step 1: Write failing tests**

Append to `internal/tui/pairing_test.go`:

```go
func TestPairToolUseWithFailure(t *testing.T) {
	pre := makeEv("e1", 1, "PreToolUse", "Bash", "s1", t0, `{"tool_use_id":"u1"}`)
	post := makeEv("e2", 2, "PostToolUseFailure", "Bash", "s1", t1, `{"tool_use_id":"u1","error":"boom"}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("want 1 paired row, got %d", len(rows))
	}
	if !rows[0].IsPair {
		t.Fatal("row should pair Pre with PostToolUseFailure")
	}
	if rows[0].EffectiveStatus() != statusError {
		t.Errorf("EffectiveStatus = %v, want statusError", rows[0].EffectiveStatus())
	}
}

func TestPairSubagent(t *testing.T) {
	pre := makeEv("e1", 1, "SubagentStart", "", "s1", t0, `{"subagent_id":"sa-1"}`)
	post := makeEv("e2", 2, "SubagentStop", "", "s1", t1, `{"subagent_id":"sa-1"}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("want 1 paired row, got %d", len(rows))
	}
	if !rows[0].IsPair {
		t.Fatal("subagent row should pair")
	}
	if rows[0].EffectiveStatus() != statusOK {
		t.Errorf("EffectiveStatus = %v, want statusOK", rows[0].EffectiveStatus())
	}
}

func TestPairCompact(t *testing.T) {
	pre := makeEv("e1", 1, "PreCompact", "", "s1", t0, `{"compact_id":"c-1"}`)
	post := makeEv("e2", 2, "PostCompact", "", "s1", t1, `{"compact_id":"c-1"}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 1 {
		t.Fatalf("want 1 paired row, got %d", len(rows))
	}
	if !rows[0].IsPair {
		t.Fatal("compact row should pair")
	}
}

func TestPairCrossKindIsolation(t *testing.T) {
	// SubagentStop must not close a PreCompact even without IDs.
	pre := makeEv("e1", 1, "PreCompact", "", "s1", t0, `{}`)
	post := makeEv("e2", 2, "SubagentStop", "", "s1", t1, `{}`)

	rows := buildDisplayRows([]*event.Event{pre, post})
	if len(rows) != 2 {
		t.Fatalf("want 2 standalone rows, got %d", len(rows))
	}
	for _, r := range rows {
		if r.IsPair {
			t.Error("cross-kind pairing must not happen")
		}
	}
}
```

Also delete the now-obsolete TUI-local helper tests `TestExtractToolUseIDTopLevel`, `TestExtractToolUseIDAbsent`, `TestExtractToolUseIDMalformed` from `pairing_test.go` — their coverage moves to `internal/source/claudecode/hooks/hooks_test.go` (Task 1) and the helper itself goes away below.

- [ ] **Step 2: Run, verify the new tests fail**

```
go test ./internal/tui/ -run "TestPairToolUseWithFailure|TestPairSubagent|TestPairCompact|TestPairCrossKindIsolation"
```

Expected: FAIL.

- [ ] **Step 3: Rewrite `pairing.go`**

Replace `internal/tui/pairing.go` with the generalised version. Drop the local `extractToolUseID` (its tests were removed in Step 1).

```go
// Package tui — pairing.go
//
// Pairs Pre/Post events for tool, subagent, and compact kinds and renders them
// as folded display rows for the events pane. Pairing is pure: given a sorted
// event slice it returns a slice of displayRows in chronological order.
//
// For each kind:
//   1. Stable ID match (per-kind id extractor).
//   2. Heuristic: first unclaimed Post of the same kind (same ToolName for the
//      tool kind) with a later Seq.
// Cross-kind pairing never happens.
package tui

import (
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

type displayRow struct {
	Pre      *event.Event
	Post     *event.Event
	IsPair   bool
	Duration time.Duration
}

func (dr displayRow) EffectiveStatus() eventStatus {
	if dr.IsPair {
		if dr.Post != nil {
			return deriveStatus(dr.Post)
		}
		return statusRunning
	}
	return deriveStatus(dr.Pre)
}

type rowAnchor struct {
	seq int64
	row displayRow
}

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
		key:   func(ev *event.Event) string { return "tool:" + ev.ToolName },
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
func buildDisplayRows(events []*event.Event) []displayRow {
	specs := []pairSpec{toolSpec(), subagentSpec(), compactSpec()}

	// Collect post slots per spec. A single global "claimed" flag would still
	// work given cross-kind isolation, but per-spec slots keeps it obvious.
	type slot struct {
		ev      *event.Event
		claimed bool
	}
	allClaimed := map[*event.Event]bool{}
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
			k := sp.key(ev)
			postByKey[i][k] = append(postByKey[i][k], idx)
		}
	}

	var anchors []rowAnchor

	for _, ev := range events {
		// Find which spec (if any) treats this event as a Pre.
		specIdx := -1
		for i, sp := range specs {
			if ev.HookEvent == sp.pre {
				specIdx = i
				break
			}
		}
		switch {
		case specIdx >= 0:
			sp := specs[specIdx]
			row := displayRow{Pre: ev}
			preID := sp.id(ev.Raw)
			var post *event.Event
			if preID != "" {
				if idx, ok := postByID[specIdx][preID]; ok && !specPosts[specIdx][idx].claimed {
					specPosts[specIdx][idx].claimed = true
					allClaimed[specPosts[specIdx][idx].ev] = true
					post = specPosts[specIdx][idx].ev
				}
			}
			if post == nil {
				k := sp.key(ev)
				for _, idx := range postByKey[specIdx][k] {
					s := &specPosts[specIdx][idx]
					if !s.claimed && s.ev.Seq > ev.Seq {
						s.claimed = true
						allClaimed[s.ev] = true
						post = s.ev
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

		default:
			// Posts are consumed during their Pre's processing; unclaimed Posts
			// are emitted as standalone rows below. Skip them in this branch.
			if isAnyPost(specs, ev.HookEvent) {
				continue
			}
			anchors = append(anchors, rowAnchor{seq: ev.Seq, row: displayRow{Pre: ev}})
		}
	}

	// Emit unclaimed posts as standalone rows.
	for i := range specs {
		for _, s := range specPosts[i] {
			if !s.claimed && !allClaimed[s.ev] {
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

func isAnyPost(specs []pairSpec, hook string) bool {
	for _, sp := range specs {
		if sp.posts[hook] {
			return true
		}
	}
	return false
}

func sortRowAnchors(a []rowAnchor) {
	for i := 1; i < len(a); i++ {
		for j := i; j > 0 && a[j].seq < a[j-1].seq; j-- {
			a[j], a[j-1] = a[j-1], a[j]
		}
	}
}
```

Notes:
- The old `extractToolUseID` is gone — everything routes through `hooks.ToolUseID` via the spec.
- If any other file in `internal/tui/` referenced `extractToolUseID` directly, rg before deleting: `grep -rn extractToolUseID internal/tui/`. Migrate any survivors to `hooks.ToolUseID`.

- [ ] **Step 4: Run all TUI tests, verify they pass**

```
go test ./internal/tui/...
```

Expected: PASS. The deletions of `TestExtractToolUseID*` are fine because the helper is gone.

- [ ] **Step 5: Commit**

```bash
git add internal/tui/pairing.go internal/tui/pairing_test.go
git commit -m "feat(tui): generalise display-row pairing to tool/subagent/compact"
```

---

## Task 11 — Smoke test coverage for `PostToolUseFailure`

The end-to-end smoke script gets one additional payload so CI exercises the new pairing path.

**Files:**
- Modify: `scripts/smoke.sh`

- [ ] **Step 1: Add a third hook invocation**

After Step 4 of `smoke.sh` (the `PostToolUse` payload), add a `PostToolUseFailure` payload. Insert these lines between the existing Step 4 block and Step 5:

```bash
# ---------------------------------------------------------------------------
# Step 4b — Third hook invocation: PostToolUseFailure (Tier 1 hook)
# ---------------------------------------------------------------------------
printf '==> sending hook payload 3 (PostToolUseFailure)\n'

printf '{"hook_event_name":"PostToolUseFailure","session_id":"smoke-test","tool_name":"Bash","cwd":"/tmp","tool_use_id":"smoke-uid","error":"smoke failure"}' \
  | "$BIN" hook

sleep 0.3
pass "hook 3 (PostToolUseFailure posted)"
```

Then extend the assertion block (Step 5) with one more grep:

```bash
grep -q '"hook_event":"PostToolUseFailure"' "$SESSION_FILE" \
  || fail "hook_event PostToolUseFailure not found in JSONL"
pass "PostToolUseFailure captured in JSONL"
```

- [ ] **Step 2: Run the smoke test**

```
./scripts/smoke.sh
```

Expected: all PASS lines, including the new ones. The daemon will already be embedded with the new operations code at this point so end-to-end the failure payload should also become a paired error op — but the smoke script only checks JSONL capture, not pairing, which is fine.

- [ ] **Step 3: Commit**

```bash
git add scripts/smoke.sh
git commit -m "test(smoke): add PostToolUseFailure capture assertion"
```

---

## Task 12 — Final verification

End-to-end gate before opening the PR.

- [ ] **Step 1: Full Go test suite**

```
go test ./...
```

Expected: PASS across all packages, including any tests that link in `model` or `tui`.

- [ ] **Step 2: Full build**

```
make build
```

Expected: build success. The frontend rebuild is unaffected since this PR makes no web changes, but `make build` exercises the embed step and is the closest CI proxy.

- [ ] **Step 3: Smoke test**

```
./scripts/smoke.sh
```

Expected: green.

- [ ] **Step 4: Diff and reflect**

```
git log --oneline main..HEAD
git diff --stat main..HEAD
```

Sanity check: every commit is conventional, every file in the diff is one of the files this plan named. If anything else changed, justify or revert.

- [ ] **Step 5: No commit here**

This is a verification-only task. Don't create an empty commit. Report status (`result: PR 2 implemented, N commits ahead of main, all tests green`).

---

## Self-review summary

- **Spec coverage:** Tasks 1 (decoder), 2 (Kind), 3 (refactor), 4–6 (PostToolUseFailure pairing + status + general engine), 7 (subagent/compact status), 8 (target), 9–10 (TUI parity), 11 (smoke), 12 (verify). Every spec section maps to at least one task.
- **Placeholders:** none — every code step has the full code; every command has expected output.
- **Type consistency:** `Operation.Kind` (string) is introduced in Task 2 and used unchanged in Tasks 6/8; `hooks.ToolUseID`, `hooks.SubagentID`, `hooks.SubagentTarget`, `hooks.SubagentHasError`, `hooks.CompactID`, `hooks.CompactTarget`, `hooks.PostToolUseFailureMessage` are defined in Task 1 and used by exact name in Tasks 3, 6, 7, 8, 9, 10.
- **Out-of-scope confirmed:** no `internal/tui/format.go` glyph/label changes, no web SPA changes, no new lifecycle-row types (Permission*/Instructions*/Task*) — all deferred to PR 3 per the spec.
