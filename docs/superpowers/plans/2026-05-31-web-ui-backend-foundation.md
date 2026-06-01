# Web UI Backend Foundation — Implementation Plan (Plan 1 of 2)

## Status

**COMPLETE & smoke-verified (2026-05-31).** All 11 tasks implemented via subagent-driven TDD on branch `feat/web-ui-backend-foundation`. `go build ./...`, `go vet ./...`, and `go test ./...` (all 9 packages) are green. Transcript schema (Task 7 Step 6) was validated against a real `~/.claude/projects/.../*.jsonl` — fields match (`type`, `timestamp` RFC3339, `message.role`/`content`, blocks `text`/`thinking`/`tool_use.id`). End-to-end smoke against a running daemon confirmed `/api/sessions`, `/api/sessions/{id}/timeline` (ops-only graceful degradation), and `/api/sessions/{id}/operations/{id}` (diff detail). A final code review added two follow-ups: a 404 guard for empty opID and a `Duration` wire-unit doc note. Plan 2 (Lit frontend + `go:embed` + `hv serve`) is next.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the server-side domain layer (`internal/model`), the Claude Code conversation adapter (`internal/source/claudecode`), and the `/api` HTTP endpoints that expose a derived, interleaved timeline — so the Lit web UI (Plan 2) can be a dumb client.

**Architecture:** All operation/diff/timeline derivation moves into a new harness-agnostic `internal/model` package. Hook events stay authoritative; conversation turns come from a quarantined `internal/source/claudecode` adapter and are merged in by timestamp (Approach 1, graceful degradation when no transcript). The daemon gains read-only `/api/...` routes that compose store reads + model derivation. The existing `/events` ingest, `/stream` SSE, and the TUI keep working unchanged.

**Tech Stack:** Go 1.x (stdlib only — `encoding/json`, `net/http`, `net/http/httptest`, `time`). No new dependencies.

**Spec:** `docs/superpowers/specs/2026-05-31-web-ui-pivot-design.md`

---

## Scope & sequencing notes (read first)

- **This is Plan 1 of 2.** Plan 1 = backend foundation (this document). Plan 2 = the Lit frontend, `go:embed`, and `hv serve`.
- **`internal/model` is the new single source of truth for derivation.** The TUI currently has its own render-layer derivation (`internal/tui/format.go`, `pairing.go`, `inspect.go`). Plan 1 builds `internal/model` and points the **API** at it; it does **not** rewrite the TUI's rendering. Consolidating the TUI onto `internal/model` (deleting its duplicated pairing/status logic) is a **follow-up refactor plan** — it is a behavior-preserving change of presentation code best done independently with the TUI's own test suite as the net. The temporary overlap is intentional and tracked.
- **v1 timeline is returned whole, not paginated.** `GET /api/.../timeline` accepts `after`/`limit` query params for forward-compatibility but v1 returns the full session timeline (correct pairing requires whole-history context; the client upserts by `Operation.ID`). True seq-cursor pagination is a fast-follow.
- **v1 operations = tool calls only.** `BuildOperations` pairs `PreToolUse`/`PostToolUse`. Non-tool lifecycle events (`SessionStart`, `Stop`, …) are dropped from the timeline for v1; user/assistant narrative comes from transcript **turns**.

## File structure

| File | Responsibility |
|---|---|
| `internal/model/status.go` | `Status` type + `DeriveStatus` / post-status from `tool_response.exit_code` |
| `internal/model/target.go` | `ExtractTarget` — short gist of what an event targeted (path / command / prompt) |
| `internal/model/operation.go` | `Operation` type + `BuildOperations` (Pre/Post pairing → []Operation) |
| `internal/model/diff.go` | `DiffOp` type + `DiffLines` (LCS line diff) |
| `internal/model/detail.go` | `OperationDetail` type + `BuildOperationDetail` (input/response/diff/raw shaping) |
| `internal/model/timeline.go` | `Turn`, `TimelineItem` types + `MergeTimeline` |
| `internal/source/claudecode/conversation.go` | Read a Claude Code transcript JSONL → `[]model.Turn` (the ONLY CC-aware code) |
| `internal/daemon/api.go` | `/api/...` handlers composing store + model + source |
| `internal/daemon/daemon.go` | register `/api/...` routes (modify `NewServer`) |
| `internal/tui/client.go` | repoint `HTTPClient` base paths to `/api/...` (modify) |

All `*_test.go` files live beside their source.

---

### Task 1: `internal/model` — status derivation

**Files:**
- Create: `internal/model/status.go`
- Test: `internal/model/status_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/model/status_test.go
package model

import (
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name string
		ev   *event.Event
		want Status
	}{
		{"pre is running", &event.Event{HookEvent: "PreToolUse"}, StatusRunning},
		{"post exit 0 is success", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{"exit_code":0}}`)}, StatusSuccess},
		{"post exit 1 is error", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{"exit_code":2}}`)}, StatusError},
		{"post without exit code is neutral", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{}}`)}, StatusNeutral},
		{"lifecycle is neutral", &event.Event{HookEvent: "SessionStart"}, StatusNeutral},
		{"malformed raw is neutral", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`not json`)}, StatusNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeriveStatus(c.ev); got != c.want {
				t.Fatalf("DeriveStatus = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestDeriveStatus -v`
Expected: FAIL — `undefined: Status` / `undefined: DeriveStatus`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/model/status.go

// Package model holds the harness-agnostic domain types and derivation logic
// for the visualizer: tool operations, diffs, and the interleaved timeline.
// It is the single source of truth shared by the HTTP API and (eventually) the
// TUI. It depends only on internal/event and the standard library.
package model

import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// Status is the derived lifecycle/result state of an operation, as a stable
// JSON string. Glyph/colour rendering is a presentation concern and lives in
// the client, not here.
type Status string

const (
	StatusRunning Status = "running" // PreToolUse without a paired Post
	StatusSuccess Status = "success" // tool exited 0
	StatusError   Status = "error"   // tool exited non-0
	StatusNeutral Status = "neutral" // unknown / not pass-fail
)

// DeriveStatus inspects a single event. PreToolUse is running; PostToolUse is
// resolved from tool_response.exit_code; everything else is neutral. Any parse
// failure yields StatusNeutral.
func DeriveStatus(ev *event.Event) Status {
	switch ev.HookEvent {
	case "PreToolUse":
		return StatusRunning
	case "PostToolUse":
		return postStatus(ev.Raw)
	default:
		return StatusNeutral
	}
}

// postStatus reads exit_code from tool_response in raw. 0 -> success, non-0 ->
// error, absent/malformed -> neutral.
func postStatus(raw json.RawMessage) Status {
	if len(raw) == 0 {
		return StatusNeutral
	}
	var w struct {
		ToolResponse struct {
			ExitCode *int `json:"exit_code"`
		} `json:"tool_response"`
	}
	if err := json.Unmarshal(raw, &w); err != nil || w.ToolResponse.ExitCode == nil {
		return StatusNeutral
	}
	if *w.ToolResponse.ExitCode == 0 {
		return StatusSuccess
	}
	return StatusError
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestDeriveStatus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/status.go internal/model/status_test.go
git commit -m "feat(model): status derivation from hook events"
```

---

### Task 2: `internal/model` — target gist extraction

**Files:**
- Create: `internal/model/target.go`
- Test: `internal/model/target_test.go`

- [ ] **Step 1: Write the failing test**

```go
// internal/model/target_test.go
package model

import (
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestExtractTarget(t *testing.T) {
	cases := []struct {
		name string
		ev   *event.Event
		want string
	}{
		{
			"bash first line only",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Bash", Raw: []byte(`{"tool_input":{"command":"go test ./...\n# second line"}}`)},
			"go test ./...",
		},
		{
			"edit uses file_path",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Edit", Raw: []byte(`{"tool_input":{"file_path":"internal/model/diff.go"}}`)},
			"internal/model/diff.go",
		},
		{
			"read falls back to path",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Read", Raw: []byte(`{"tool_input":{"path":"/tmp/x"}}`)},
			"/tmp/x",
		},
		{
			"user prompt",
			&event.Event{HookEvent: "UserPromptSubmit", Raw: []byte(`{"prompt":"add the api"}`)},
			"add the api",
		},
		{"empty raw", &event.Event{HookEvent: "PreToolUse", ToolName: "Bash"}, ""},
		{"malformed raw", &event.Event{HookEvent: "PreToolUse", ToolName: "Bash", Raw: []byte(`nope`)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractTarget(c.ev); got != c.want {
				t.Fatalf("ExtractTarget = %q, want %q", got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestExtractTarget -v`
Expected: FAIL — `undefined: ExtractTarget`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/model/target.go
package model

import (
	"encoding/json"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// ExtractTarget returns a short, human-readable description of what an event
// targeted: a file path, a command's first line, or a prompt. Returns "" when
// nothing useful is found. Unlike the TUI, it does not truncate — the client
// elides for display.
func ExtractTarget(ev *event.Event) string {
	if len(ev.Raw) == 0 {
		return ""
	}
	var fields struct {
		ToolInput    json.RawMessage `json:"tool_input"`
		Prompt       string          `json:"prompt"`
		Notification string          `json:"notification"`
	}
	if err := json.Unmarshal(ev.Raw, &fields); err != nil {
		return ""
	}
	switch ev.HookEvent {
	case "PreToolUse", "PostToolUse":
		return toolInputGist(ev.ToolName, fields.ToolInput)
	case "UserPromptSubmit":
		return strings.TrimSpace(fields.Prompt)
	case "Notification":
		return strings.TrimSpace(fields.Notification)
	}
	return ""
}

// toolInputGist extracts a short string from tool_input for known tools.
func toolInputGist(tool string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	switch tool {
	case "Bash":
		var inp struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(raw, &inp); err == nil && inp.Command != "" {
			return strings.SplitN(inp.Command, "\n", 2)[0]
		}
	case "Read", "Write", "Edit", "MultiEdit":
		var inp struct {
			FilePath string `json:"file_path"`
			Path     string `json:"path"`
		}
		if err := json.Unmarshal(raw, &inp); err == nil {
			if inp.FilePath != "" {
				return inp.FilePath
			}
			return inp.Path
		}
	default:
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			for _, v := range m {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestExtractTarget -v`
Expected: PASS.

> Note: the `default` branch iterates a map (non-deterministic order). The test only exercises Bash/Edit/Read/prompt branches, so this is safe. Do not add a test asserting a specific value for the generic branch.

- [ ] **Step 5: Commit**

```bash
git add internal/model/target.go internal/model/target_test.go
git commit -m "feat(model): target gist extraction"
```

---

### Task 3: `internal/model` — Operation + BuildOperations (pairing)

**Files:**
- Create: `internal/model/operation.go`
- Test: `internal/model/operation_test.go`

This ports the pairing strategy from `internal/tui/pairing.go` (stable `tool_use_id` match, heuristic fallback by tool name + seq order) but emits `[]Operation`.

- [ ] **Step 1: Write the failing test**

```go
// internal/model/operation_test.go
package model

import (
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func ev(seq int64, hook, tool, raw string, at time.Time) *event.Event {
	return &event.Event{Seq: seq, HookEvent: hook, ToolName: tool, Raw: []byte(raw), CapturedAt: at}
}

func TestBuildOperations_PairsByToolUseID(t *testing.T) {
	t0 := time.Unix(1000, 0)
	events := []*event.Event{
		ev(1, "PreToolUse", "Edit", `{"tool_use_id":"a","tool_input":{"file_path":"x.go"}}`, t0),
		ev(2, "PostToolUse", "Edit", `{"tool_use_id":"a","tool_response":{"exit_code":0}}`, t0.Add(200*time.Millisecond)),
	}
	ops := BuildOperations(events)
	if len(ops) != 1 {
		t.Fatalf("got %d ops, want 1", len(ops))
	}
	op := ops[0]
	if op.ID != "a" || op.Tool != "Edit" || op.Status != StatusSuccess {
		t.Fatalf("unexpected op: %+v", op)
	}
	if op.Target != "x.go" {
		t.Fatalf("target = %q, want x.go", op.Target)
	}
	if op.Duration != 200*time.Millisecond {
		t.Fatalf("duration = %v, want 200ms", op.Duration)
	}
	if op.Seq != 1 {
		t.Fatalf("seq = %d, want 1 (Pre anchor)", op.Seq)
	}
}

func TestBuildOperations_UnpairedPreIsRunning(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Bash", `{"tool_use_id":"b","tool_input":{"command":"sleep 9"}}`, t0),
	})
	if len(ops) != 1 || ops[0].Status != StatusRunning {
		t.Fatalf("want one running op, got %+v", ops)
	}
	if ops[0].Duration != 0 {
		t.Fatalf("running op duration = %v, want 0", ops[0].Duration)
	}
}

func TestBuildOperations_HeuristicFallbackByTool(t *testing.T) {
	t0 := time.Unix(1000, 0)
	// No tool_use_id on either; pair by same tool + Post.Seq > Pre.Seq.
	ops := BuildOperations([]*event.Event{
		ev(1, "PreToolUse", "Read", `{"tool_input":{"file_path":"a"}}`, t0),
		ev(2, "PostToolUse", "Read", `{"tool_response":{"exit_code":0}}`, t0.Add(time.Second)),
	})
	if len(ops) != 1 || ops[0].Status != StatusSuccess {
		t.Fatalf("want one success op, got %+v", ops)
	}
}

func TestBuildOperations_DropsNonToolEvents(t *testing.T) {
	t0 := time.Unix(1000, 0)
	ops := BuildOperations([]*event.Event{
		ev(1, "SessionStart", "", `{}`, t0),
		ev(2, "UserPromptSubmit", "", `{"prompt":"hi"}`, t0),
	})
	if len(ops) != 0 {
		t.Fatalf("non-tool events should not become operations, got %+v", ops)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestBuildOperations -v`
Expected: FAIL — `undefined: Operation` / `undefined: BuildOperations`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/model/operation.go
package model

import (
	"encoding/json"
	"sort"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// Operation is one tool invocation: a PreToolUse paired with its PostToolUse
// (or a still-running Pre). It is keyed by ID (tool_use_id) so live upserts
// replace a running row in place. Heavy detail (diff/input/response/raw) is
// fetched separately via BuildOperationDetail.
type Operation struct {
	ID        string        `json:"id"`         // tool_use_id, or "" when absent
	Tool      string        `json:"tool"`       // Edit, Bash, Read…
	Status    Status        `json:"status"`     // running | success | error | neutral
	StartedAt time.Time     `json:"started_at"` // Pre.CapturedAt
	Duration  time.Duration `json:"duration"`   // 0 while running
	Target    string        `json:"target"`     // file path / command gist
	Seq       int64         `json:"seq"`        // Pre.Seq — the chronological anchor
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
		if e.HookEvent != "PostToolUse" {
			continue
		}
		idx := len(posts)
		posts = append(posts, slot{ev: e})
		if id := toolUseID(e.Raw); id != "" {
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
			ID:        toolUseID(e.Raw),
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

// toolUseID looks for "tool_use_id" at the top level of raw JSON. Returns ""
// on any error or absence.
func toolUseID(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var w struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if err := json.Unmarshal(raw, &w); err != nil {
		return ""
	}
	return w.ToolUseID
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestBuildOperations -v`
Expected: PASS (all four subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/operation.go internal/model/operation_test.go
git commit -m "feat(model): Operation + BuildOperations pairing"
```

---

### Task 4: `internal/model` — structured line diff

**Files:**
- Create: `internal/model/diff.go`
- Test: `internal/model/diff_test.go`

This is **new** logic (the TUI faked diffs with full before/after blocks). A classic LCS over lines produces `context`/`del`/`add` ops the client renders + highlights.

- [ ] **Step 1: Write the failing test**

```go
// internal/model/diff_test.go
package model

import (
	"reflect"
	"testing"
)

func TestDiffLines(t *testing.T) {
	got := DiffLines("a\nb\nc", "a\nB\nc")
	want := []DiffOp{
		{Kind: "context", Text: "a"},
		{Kind: "del", Text: "b"},
		{Kind: "add", Text: "B"},
		{Kind: "context", Text: "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffLines mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestDiffLines_PureInsertion(t *testing.T) {
	got := DiffLines("a\nc", "a\nb\nc")
	want := []DiffOp{
		{Kind: "context", Text: "a"},
		{Kind: "add", Text: "b"},
		{Kind: "context", Text: "c"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("DiffLines mismatch\ngot:  %+v\nwant: %+v", got, want)
	}
}

func TestDiffLines_Identical(t *testing.T) {
	got := DiffLines("x\ny", "x\ny")
	want := []DiffOp{{Kind: "context", Text: "x"}, {Kind: "context", Text: "y"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestDiffLines -v`
Expected: FAIL — `undefined: DiffOp` / `undefined: DiffLines`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/model/diff.go
package model

import "strings"

// DiffOp is one line in a structured diff. Kind is "context", "del", or "add".
// Deleted lines precede their added counterparts (unified-diff convention).
type DiffOp struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// DiffLines computes a line-level diff between oldStr and newStr using the
// classic longest-common-subsequence algorithm. Both inputs are split on "\n".
// The result is a flat list of context/del/add ops in display order.
func DiffLines(oldStr, newStr string) []DiffOp {
	a := strings.Split(oldStr, "\n")
	b := strings.Split(newStr, "\n")

	// LCS length table: lcs[i][j] = LCS of a[i:] and b[j:].
	n, m := len(a), len(b)
	lcs := make([][]int, n+1)
	for i := range lcs {
		lcs[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var ops []DiffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, DiffOp{Kind: "context", Text: a[i]})
			i++
			j++
		case lcs[i+1][j] >= lcs[i][j+1]:
			ops = append(ops, DiffOp{Kind: "del", Text: a[i]})
			i++
		default:
			ops = append(ops, DiffOp{Kind: "add", Text: b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, DiffOp{Kind: "del", Text: a[i]})
	}
	for ; j < m; j++ {
		ops = append(ops, DiffOp{Kind: "add", Text: b[j]})
	}
	return ops
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestDiffLines -v`
Expected: PASS (all three subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/model/diff.go internal/model/diff_test.go
git commit -m "feat(model): LCS structured line diff"
```

---

### Task 5: `internal/model` — OperationDetail

**Files:**
- Create: `internal/model/detail.go`
- Test: `internal/model/detail_test.go`

`BuildOperationDetail(pre, post)` shapes the heavy per-operation payload: a structured diff for Edit/Write, command+output for Bash, plus the raw Pre/Post passthrough escape hatch.

- [ ] **Step 1: Write the failing test**

```go
// internal/model/detail_test.go
package model

import (
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestBuildOperationDetail_EditProducesDiff(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Edit",
		Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a\nb","new_string":"a\nB"}}`)}
	post := &event.Event{HookEvent: "PostToolUse", ToolName: "Edit",
		Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)}

	d := BuildOperationDetail(pre, post)
	if d.DetailKind != "diff" {
		t.Fatalf("DetailKind = %q, want diff", d.DetailKind)
	}
	if d.FilePath != "x.go" {
		t.Fatalf("FilePath = %q, want x.go", d.FilePath)
	}
	if len(d.Diff) != 3 { // context "a", del "b", add "B"
		t.Fatalf("diff len = %d, want 3: %+v", len(d.Diff), d.Diff)
	}
	if len(d.RawPre) == 0 || len(d.RawPost) == 0 {
		t.Fatal("raw passthrough must be populated")
	}
}

func TestBuildOperationDetail_BashProducesOutput(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Bash",
		Raw: []byte(`{"tool_input":{"command":"echo hi"}}`)}
	post := &event.Event{HookEvent: "PostToolUse", ToolName: "Bash",
		Raw: []byte(`{"tool_response":{"exit_code":0,"stdout":"hi\n"}}`)}

	d := BuildOperationDetail(pre, post)
	if d.DetailKind != "output" {
		t.Fatalf("DetailKind = %q, want output", d.DetailKind)
	}
	if d.Command != "echo hi" {
		t.Fatalf("Command = %q, want echo hi", d.Command)
	}
	if d.Output != "hi\n" {
		t.Fatalf("Output = %q, want 'hi\\n'", d.Output)
	}
	if d.ExitCode == nil || *d.ExitCode != 0 {
		t.Fatalf("ExitCode = %v, want 0", d.ExitCode)
	}
}

func TestBuildOperationDetail_RunningHasNilPost(t *testing.T) {
	pre := &event.Event{HookEvent: "PreToolUse", ToolName: "Read",
		Raw: []byte(`{"tool_input":{"file_path":"y"}}`)}
	d := BuildOperationDetail(pre, nil)
	if d.DetailKind != "generic" {
		t.Fatalf("DetailKind = %q, want generic", d.DetailKind)
	}
	if len(d.RawPost) != 0 {
		t.Fatal("running op must have empty RawPost")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestBuildOperationDetail -v`
Expected: FAIL — `undefined: BuildOperationDetail`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/model/detail.go
package model

import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// OperationDetail is the heavy, lazily-fetched payload for one operation. Only
// one of Diff / (Command,Output) is populated, selected by DetailKind. RawPre /
// RawPost are the verbatim hook payloads — the always-available escape hatch.
type OperationDetail struct {
	ID         string          `json:"id"`
	Tool       string          `json:"tool"`
	DetailKind string          `json:"detail_kind"` // "diff" | "output" | "generic"
	FilePath   string          `json:"file_path,omitempty"`
	Diff       []DiffOp        `json:"diff,omitempty"`
	Command    string          `json:"command,omitempty"`
	Output     string          `json:"output,omitempty"`
	ExitCode   *int            `json:"exit_code,omitempty"`
	RawPre     json.RawMessage `json:"raw_pre,omitempty"`
	RawPost    json.RawMessage `json:"raw_post,omitempty"`
}

// BuildOperationDetail shapes the detail payload from a Pre event and its
// optional Post. post may be nil for a still-running operation.
func BuildOperationDetail(pre, post *event.Event) OperationDetail {
	d := OperationDetail{
		ID:         toolUseID(pre.Raw),
		Tool:       pre.ToolName,
		DetailKind: "generic",
		RawPre:     pre.Raw,
	}
	if post != nil {
		d.RawPost = post.Raw
	}

	switch pre.ToolName {
	case "Edit", "Write", "MultiEdit":
		var in struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if parseToolInput(pre.Raw, &in) && (in.OldString != "" || in.NewString != "") {
			d.DetailKind = "diff"
			d.FilePath = in.FilePath
			d.Diff = DiffLines(in.OldString, in.NewString)
		}
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if parseToolInput(pre.Raw, &in) {
			d.Command = in.Command
		}
		if post != nil {
			var resp struct {
				ToolResponse struct {
					ExitCode *int   `json:"exit_code"`
					Stdout   string `json:"stdout"`
					Stderr   string `json:"stderr"`
				} `json:"tool_response"`
			}
			if json.Unmarshal(post.Raw, &resp) == nil {
				d.ExitCode = resp.ToolResponse.ExitCode
				d.Output = resp.ToolResponse.Stdout + resp.ToolResponse.Stderr
			}
		}
		d.DetailKind = "output"
	}
	return d
}

// parseToolInput unmarshals the tool_input object from a hook payload into v.
func parseToolInput(raw json.RawMessage, v any) bool {
	var w struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(raw, &w) != nil || len(w.ToolInput) == 0 {
		return false
	}
	return json.Unmarshal(w.ToolInput, v) == nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -run TestBuildOperationDetail -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/model/detail.go internal/model/detail_test.go
git commit -m "feat(model): OperationDetail (diff/output/raw)"
```

---

### Task 6: `internal/model` — Turn, TimelineItem, MergeTimeline

**Files:**
- Create: `internal/model/timeline.go`
- Test: `internal/model/timeline_test.go`

- [ ] **Step 1: Write the failing test**

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/model/ -run TestMergeTimeline -v`
Expected: FAIL — `undefined: Turn` / `undefined: MergeTimeline`.

- [ ] **Step 3: Write minimal implementation**

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/model/ -v`
Expected: PASS (whole package green).

- [ ] **Step 5: Commit**

```bash
git add internal/model/timeline.go internal/model/timeline_test.go
git commit -m "feat(model): Turn + TimelineItem + MergeTimeline"
```

---

### Task 7: `internal/source/claudecode` — conversation reader

**Files:**
- Create: `internal/source/claudecode/conversation.go`
- Test: `internal/source/claudecode/conversation_test.go`

Reads a Claude Code transcript JSONL file and returns `[]model.Turn`. This is the **only** Claude-Code-format-aware code. It is defensive: a missing/unreadable/foreign file yields `nil, nil` so the timeline degrades to operations-only. `content` may be a plain string (user) or an array of typed blocks (assistant: `text` / `thinking` / `tool_use`); `tool_use` blocks contribute their id to `ToolRefs` but are NOT emitted as turns.

- [ ] **Step 1: Write the failing test**

```go
// internal/source/claudecode/conversation_test.go
package claudecode

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, lines string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadConversation_UserAndAssistant(t *testing.T) {
	path := writeTranscript(t, ""+
		`{"type":"user","timestamp":"2026-05-31T10:00:00Z","message":{"role":"user","content":"add the api"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-05-31T10:00:05Z","message":{"role":"assistant","content":[`+
		`{"type":"thinking","thinking":"let me start"},`+
		`{"type":"text","text":"I'll start with the model package."},`+
		`{"type":"tool_use","id":"toolu_1","name":"Edit","input":{}}]}}`+"\n")

	turns, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Text != "add the api" {
		t.Fatalf("user turn wrong: %+v", turns[0])
	}
	a := turns[1]
	if a.Role != "assistant" || a.Text != "I'll start with the model package." {
		t.Fatalf("assistant text wrong: %+v", a)
	}
	if a.Thinking != "let me start" {
		t.Fatalf("assistant thinking wrong: %+v", a)
	}
	if len(a.ToolRefs) != 1 || a.ToolRefs[0] != "toolu_1" {
		t.Fatalf("assistant tool_refs wrong: %+v", a.ToolRefs)
	}
	if a.At.IsZero() {
		t.Fatal("assistant timestamp not parsed")
	}
}

func TestReadConversation_MissingFileDegrades(t *testing.T) {
	turns, err := ReadConversation("/no/such/file.jsonl")
	if err != nil || turns != nil {
		t.Fatalf("missing file should yield (nil, nil); got (%v, %v)", turns, err)
	}
}

func TestReadConversation_SkipsMalformedLines(t *testing.T) {
	path := writeTranscript(t, "not json\n"+
		`{"type":"user","timestamp":"2026-05-31T10:00:00Z","message":{"role":"user","content":"hi"}}`+"\n")
	turns, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1 (malformed line skipped)", len(turns))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/source/claudecode/ -v`
Expected: FAIL — `undefined: ReadConversation`.

- [ ] **Step 3: Write minimal implementation**

```go
// internal/source/claudecode/conversation.go

// Package claudecode adapts Claude Code's on-disk transcript format into the
// harness-agnostic model.Turn type. It is the ONLY package that knows the
// Claude Code transcript schema; everything upstream consumes model.Turn. All
// parsing is defensive — a missing, unreadable, or foreign file yields no turns
// and no error, so the timeline degrades gracefully to operations-only.
package claudecode

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"jordandavis.dev/harness-visualizer/internal/model"
)

const (
	scanBufInit = 64 * 1024
	scanBufMax  = 16 * 1024 * 1024
)

// ReadConversation parses path (a Claude Code transcript JSONL) into ordered
// conversation turns. Returns (nil, nil) when the file is absent. Malformed
// lines and unrecognised record types are skipped.
func ReadConversation(path string) ([]model.Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var turns []model.Turn
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.Type != "user" && rec.Type != "assistant" {
			continue
		}
		turn, ok := rec.toTurn()
		if ok {
			turns = append(turns, turn)
		}
	}
	return turns, sc.Err()
}

// record is one transcript line. content is either a JSON string (user) or an
// array of typed blocks (assistant); rawContent defers that decision.
type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role       string          `json:"role"`
		RawContent json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	ID       string `json:"id"`
}

func (r record) toTurn() (model.Turn, bool) {
	t := model.Turn{Role: r.Type}
	if ts, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
		t.At = ts
	}

	// content as a plain string.
	var s string
	if json.Unmarshal(r.Message.RawContent, &s) == nil {
		t.Text = strings.TrimSpace(s)
		return t, t.Text != ""
	}

	// content as an array of blocks.
	var blocks []block
	if json.Unmarshal(r.Message.RawContent, &blocks) != nil {
		return model.Turn{}, false
	}
	var texts, thinks []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		case "thinking":
			if b.Thinking != "" {
				thinks = append(thinks, b.Thinking)
			}
		case "tool_use":
			if b.ID != "" {
				t.ToolRefs = append(t.ToolRefs, b.ID) // referenced, not emitted as a turn
			}
		}
	}
	t.Text = strings.TrimSpace(strings.Join(texts, "\n"))
	t.Thinking = strings.TrimSpace(strings.Join(thinks, "\n"))
	// Keep the turn if it has prose, thinking, or tool references.
	return t, t.Text != "" || t.Thinking != "" || len(t.ToolRefs) > 0
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/source/claudecode/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/source/claudecode/
git commit -m "feat(source/claudecode): conversation transcript reader"
```

- [ ] **Step 6: Validation checkpoint against a REAL transcript**

The schema above matches Claude Code's documented transcript shape, but verify against a real file before trusting it. Find one and dump a couple of lines:

Run:
```bash
ls -t ~/.claude/projects/*/*.jsonl 2>/dev/null | head -1
```
Then inspect the FIRST user line and FIRST assistant line of that file (use your Read tool on the path, or `grep -m1 '"type":"assistant"'`). Confirm the field names used above (`type`, `timestamp`, `message.role`, `message.content`, block `type`/`text`/`thinking`/`id`). If a field differs (e.g. content blocks use `"input"` keyed differently, or timestamps are epoch not RFC3339), adjust `record`/`block` and the test fixture to match, re-run Step 4, and amend the commit. If no transcript exists locally, leave as-is — graceful degradation covers it — and note this checkpoint as deferred.

---

### Task 8: daemon — `/api/sessions` and `/api/sessions/{id}/timeline`

**Files:**
- Create: `internal/daemon/api.go`
- Modify: `internal/daemon/daemon.go:154-159` (route registration in `NewServer`)
- Test: `internal/daemon/api_test.go`

The timeline handler composes: read events (`store.Read(id, 0)`) → `BuildOperations` → find the session's transcript path from the events → `claudecode.ReadConversation` → `MergeTimeline`. v1 returns the full timeline (`after`/`limit` accepted but not yet applied).

- [ ] **Step 1: Write the failing test**

```go
// internal/daemon/api_test.go
package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// newTestServer builds a Server over a temp store and appends the given events.
func newTestServer(t *testing.T, events ...*event.Event) *Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, e := range events {
		if err := st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return NewServer(st)
}

func TestAPISessions_ReturnsArray(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []store.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v", got)
	}
}

func TestAPITimeline_MergesOpsAndTurns(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a","new_string":"b"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PostToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/timeline", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No transcript on disk for s1 -> ops-only, exactly one operation item.
	if len(items) != 1 || items[0]["kind"] != "operation" {
		t.Fatalf("want one operation item, got %+v", items)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestAPI -v`
Expected: FAIL — routes 404 (handlers not registered) so `rec.Code` is 404.

- [ ] **Step 3a: Register the routes** in `internal/daemon/daemon.go`

In `NewServer`, after the existing `s.mux.HandleFunc("/stream", s.handleStream)` line, add:

```go
	s.mux.HandleFunc("/api/sessions", s.handleAPISessions)
	s.mux.HandleFunc("/api/sessions/", s.handleAPISession)
```

- [ ] **Step 3b: Implement the handlers** in the new `internal/daemon/api.go`

```go
// internal/daemon/api.go
package daemon

import (
	"encoding/json"
	"net/http"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/model"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// handleAPISessions: GET /api/sessions — same payload as /sessions, under /api.
func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infos, err := s.st.Sessions()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if infos == nil {
		infos = []store.SessionInfo{}
	}
	writeJSON(w, infos)
}

// handleAPISession routes GET /api/sessions/{id}/timeline and
// GET /api/sessions/{id}/operations/{opID}.
func (s *Server) handleAPISession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id, tail, _ := strings.Cut(rest, "/")
	switch {
	case tail == "timeline":
		s.handleAPITimeline(w, r, id)
	case strings.HasPrefix(tail, "operations/"):
		s.handleAPIOperation(w, r, id, strings.TrimPrefix(tail, "operations/"))
	default:
		http.NotFound(w, r)
	}
}

// handleAPITimeline builds the merged, interleaved timeline for a session.
func (s *Server) handleAPITimeline(w http.ResponseWriter, r *http.Request, id string) {
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ops := model.BuildOperations(events)
	turns, _ := claudecode.ReadConversation(transcriptPathFromEvents(events))
	items := model.MergeTimeline(ops, turns)
	if items == nil {
		items = []model.TimelineItem{}
	}
	writeJSON(w, items)
}

// transcriptPathFromEvents pulls the last non-empty transcript_path from the
// raw payloads. Returns "" when none is present (-> graceful degradation).
func transcriptPathFromEvents(events []*event.Event) string {
	path := ""
	for _, e := range events {
		var w struct {
			TranscriptPath string `json:"transcript_path"`
		}
		if json.Unmarshal(e.Raw, &w) == nil && w.TranscriptPath != "" {
			path = w.TranscriptPath
		}
	}
	return path
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
```

Add the `event` import to `api.go`'s import block:
```go
	"jordandavis.dev/harness-visualizer/internal/event"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestAPI -v`
Expected: PASS. (`handleAPIOperation` is referenced but not yet defined — it is added in Task 9. To keep this task compiling on its own, add a temporary stub at the bottom of `api.go`; Task 9 replaces it.)

Temporary stub (delete in Task 9):
```go
func (s *Server) handleAPIOperation(w http.ResponseWriter, r *http.Request, id, opID string) {
	http.NotFound(w, r)
}
```

- [ ] **Step 5: Run the full daemon + model suites**

Run: `go test ./internal/daemon/ ./internal/model/ ./internal/source/... -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/daemon/api.go internal/daemon/daemon.go internal/daemon/api_test.go
git commit -m "feat(daemon): /api/sessions and /api/sessions/{id}/timeline"
```

---

### Task 9: daemon — `/api/sessions/{id}/operations/{opID}` detail

**Files:**
- Modify: `internal/daemon/api.go` (replace the Task 8 stub)
- Test: `internal/daemon/api_test.go` (add a test)

- [ ] **Step 1: Write the failing test** (append to `internal/daemon/api_test.go`)

```go
func TestAPIOperationDetail_ReturnsDiff(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a\nb","new_string":"a\nB"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PostToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/a", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	var d struct {
		DetailKind string `json:"detail_kind"`
		FilePath   string `json:"file_path"`
		Diff       []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"diff"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.DetailKind != "diff" || d.FilePath != "x.go" || len(d.Diff) == 0 {
		t.Fatalf("unexpected detail: %+v", d)
	}
}

func TestAPIOperationDetail_NotFound(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/missing", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -run TestAPIOperationDetail -v`
Expected: FAIL — the stub returns 404 for the diff case (want 200).

- [ ] **Step 3: Replace the stub** in `internal/daemon/api.go`

```go
// handleAPIOperation returns the heavy detail for one operation, identified by
// its tool_use_id. It re-reads the session and locates the Pre/Post pair.
func (s *Server) handleAPIOperation(w http.ResponseWriter, r *http.Request, id, opID string) {
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var pre, post *event.Event
	for _, e := range events {
		if toolUseIDOf(e.Raw) != opID {
			continue
		}
		switch e.HookEvent {
		case "PreToolUse":
			pre = e
		case "PostToolUse":
			post = e
		}
	}
	if pre == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, model.BuildOperationDetail(pre, post))
}

// toolUseIDOf extracts the top-level tool_use_id from a raw payload.
func toolUseIDOf(raw json.RawMessage) string {
	var w struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if json.Unmarshal(raw, &w) != nil {
		return ""
	}
	return w.ToolUseID
}
```

Delete the temporary stub added in Task 8.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -run TestAPIOperationDetail -v`
Expected: PASS (both subtests).

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/api.go internal/daemon/api_test.go
git commit -m "feat(daemon): /api/sessions/{id}/operations/{opID} detail"
```

---

### Task 10: repoint the TUI HTTP client to `/api/...`

**Files:**
- Modify: `internal/tui/client.go:75-116` (the `Sessions` and `Events` methods)
- Test: `internal/tui/client_test.go` (existing tests should still pass; add one if absent)

The TUI keeps working unchanged from the user's view; it just reads through `/api`. The `/sessions` and `/sessions/{id}/events` routes still exist (untouched), so this is a clean switch with no server-side removal. `/api/sessions/{id}/events` does not exist — keep the TUI's events read on the legacy route OR add an `/api` events alias. Simplest: point `Sessions()` at `/api/sessions` (which returns identical JSON) and leave `Events()` on the legacy `/sessions/{id}/events` route, since the TUI's incremental event read is not part of the new API surface.

- [ ] **Step 1: Update `Sessions()`** in `internal/tui/client.go`

Change the request URL from `/sessions` to `/api/sessions`:

```go
func (c *HTTPClient) Sessions() ([]store.SessionInfo, error) {
	resp, err := c.http.Get(c.base + "/api/sessions")
	if err != nil {
		return nil, fmt.Errorf("GET /api/sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/sessions: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET /api/sessions: read body: %w", err)
	}
	var infos []store.SessionInfo
	if err := json.Unmarshal(body, &infos); err != nil {
		return nil, fmt.Errorf("GET /api/sessions: decode: %w", err)
	}
	return infos, nil
}
```

(Leave `Events()`, `Health()`, and `Stream()` unchanged.)

- [ ] **Step 2: Run the TUI suite**

Run: `go test ./internal/tui/ -v`
Expected: PASS. If a test pins the old `/sessions` path via a stub server, update that stub to register `/api/sessions`. (Search: `grep -rn '"/sessions"' internal/tui/`.)

- [ ] **Step 3: Full build + test**

Run: `go build ./... && go test ./...`
Expected: build OK; all packages PASS.

- [ ] **Step 4: Commit**

```bash
git add internal/tui/client.go
git commit -m "refactor(tui): read sessions via /api/sessions"
```

---

### Task 11: end-to-end smoke against a running daemon

**Files:** none (manual verification + a committed note).

- [ ] **Step 1: Build and run the daemon**

Run: `go build -o /tmp/hv ./cmd/hv && /tmp/hv daemon --port 7842 &`
Expected: prints `hv daemon listening on 127.0.0.1:7842`.

- [ ] **Step 2: Post two fake events (a paired Edit) and read the timeline**

Run:
```bash
curl -s -XPOST localhost:7842/events -d '{"session_id":"smoke","hook_event":"PreToolUse","tool_name":"Edit","raw":{"tool_use_id":"z","tool_input":{"file_path":"x.go","old_string":"a","new_string":"b"}}}'
curl -s -XPOST localhost:7842/events -d '{"session_id":"smoke","hook_event":"PostToolUse","tool_name":"Edit","raw":{"tool_use_id":"z","tool_response":{"exit_code":0}}}'
sleep 1
curl -s localhost:7842/api/sessions | head -c 400; echo
curl -s localhost:7842/api/sessions/smoke/timeline | head -c 600; echo
curl -s localhost:7842/api/sessions/smoke/operations/z | head -c 600; echo
```
Expected: `/api/sessions` lists `smoke`; `/timeline` shows one `"kind":"operation"` item with `"status":"success"`; `/operations/z` shows `"detail_kind":"diff"` with a `diff` array.

> Note: the POST body wraps the hook payload under `raw` because `event.Event` has a `Raw json.RawMessage` field. If the daemon's `/events` decoder expects the bare hook payload instead, post the shape the existing hook client uses (check `internal/client`); the goal is simply to get two paired events into the store.

- [ ] **Step 3: Stop the daemon**

Run: `kill %1` (or the daemon's PID).

- [ ] **Step 4: Commit a short note**

Add a `## Status` line to the spec or a `docs/superpowers/plans/` note recording that Plan 1 is complete and smoke-verified, then:
```bash
git commit -am "docs: Plan 1 (backend foundation) complete and smoke-verified"
```

---

## Self-review checklist (run after implementing all tasks)

- [ ] `go test ./...` is fully green, including the pre-existing TUI and daemon suites.
- [ ] `go build ./...` produces a binary; `hv tui` still works against the daemon.
- [ ] No `model` symbol is referenced before the task that defines it (status → target → operation → diff → detail → timeline → api).
- [ ] The transcript schema was validated against a real file (Task 7 Step 6) OR explicitly noted as deferred.
- [ ] No endpoint leaks `null` instead of `[]` (sessions, timeline both guarded).

## What Plan 2 covers (not this plan)

- `web/` Lit + Vite scaffold; `hv-app` / session-list / timeline / inspector components.
- `EventSource('/stream')` + nudge-refetch wiring; upsert by `Operation.ID`.
- `internal/web` `go:embed` + SPA handler mounted at `/`; `hv serve` launcher.
- Makefile (`build: web`), dev Vite proxy, CI split.

## Deferred to a later refactor plan (tracked, not lost)

- Consolidate the TUI's own derivation (`format.go`, `pairing.go`, `inspect.go`) onto `internal/model`, deleting the duplicated pairing/status logic. Behavior-preserving; gated on the TUI's existing test suite.
- True seq-cursor pagination for `/timeline` (cross-window pairing), plus `limit` on `store.Read`.
- Non-tool lifecycle events in the timeline, if wanted.
