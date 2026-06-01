# PR 3: New Lanes — Render Permission/Instructions/Config/Cwd/Task events

**Issue:** [#1 — Expand Claude Code hooks support](https://github.com/jordantdavis/harness-visualizer/issues/1)
**Branch:** `worktree-issue-1-pr3-new-lanes`
**Status:** Spec
**Date:** 2026-06-01

## Context

PR 1 registered ~30 Claude Code hooks for capture. PR 2 added operation-model pairing for `PostToolUseFailure`, `PreCompact`/`PostCompact`, and `SubagentStart`/`SubagentStop`. PR 3 (this spec) makes the *standalone* (non-pairable) new event types visible in the TUI and web UI as distinct row types — what the issue calls "new lanes."

Today both clients render two timeline kinds: `Operation` (paired Pre/Post tool calls) and `Turn` (conversation turns from the on-disk transcript). New lifecycle events fall through to a generic dim row in the TUI and have no representation at all in the web UI. PR 3 adds a third kind, `LaneEvent`, alongside a shared hook registry so the two clients can't drift on glyphs/labels.

## Scope

**In scope — 11 hook events across 9 lanes:**

| Lane | Hook(s) | Glyph (proposed) |
|---|---|---|
| permission | `PermissionRequest`, `PermissionDenied` | 🔒 |
| instructions | `InstructionsLoaded` | 📄 |
| config | `ConfigChange` | ⚙ |
| cwd | `CwdChanged` | 📁 |
| task | `TaskCreated`, `TaskCompleted` | ☐ / ☑ |
| expansion | `UserPromptExpansion` | 🪄 |
| message | `MessageDisplay` | 💬 |
| worktree | `WorktreeRemove` | 🪵 |
| stop_failure | `StopFailure` | ⚠ |

**Out of scope (explicit YAGNI):**

- Collapsible groups, sidebar lane summary, lane-based filtering.
- Resizable inspector splitter / persisted widths.
- Tier 3 hooks (`Elicitation`*, `TeammateIdle`, `FileChanged`, `Setup`, `WorktreeCreate`). Registry can grow without schema change.

## UX decisions (resolved)

1. **Row shape:** Separate row kind, inline in timeline (no collapsing). Inspector pane handles drill-down via the existing `Raw` view.
2. **Drift control:** Introduce a small shared registry now; web fetches it via `/api/hooks`.
3. **Scope:** 5 named lanes + Tier 1/2 orphans (UserPromptExpansion, StopFailure, MessageDisplay, WorktreeRemove).
4. **Inspector width:** Bump web inspector from 380px → 520px, still fixed.

## Architecture

### 1. Shared hook registry — `internal/event/hooks.go` (new)

```go
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

type HookMeta struct {
    Name     string `json:"name"`
    Glyph    string `json:"glyph"`    // unicode, one cell
    Label    string `json:"label"`    // ≤16 chars
    Lane     Lane   `json:"lane"`
    Severity string `json:"severity"` // "info" | "warn" | "error" | "dim"
}

var Hooks = []HookMeta{ /* 11 entries */ }
func Lookup(hookName string) (HookMeta, bool)
```

The 11 listed events get entries; every other hook name falls through to a generic `(_, false)` from `Lookup` and clients render with a generic glyph + the raw hook name. Existing paired/operation hooks (`PreToolUse`, `PostToolUse`, etc.) are deliberately **not** in the registry — they already have render paths through `Operation`.

### 2. Domain model — extend `TimelineItem` to 3-way union

`internal/model/timeline.go`:

```go
type LaneEvent struct {
    ID        string
    HookEvent string          // "PermissionRequest"
    Lane      event.Lane      // "permission"
    Gist      string          // one-line summary from per-hook extractor
    Severity  string          // mirrored from registry for client convenience
    Raw       json.RawMessage // for inspector
    At        time.Time
    Seq       int64
}

type TimelineItem struct {
    Kind  string // "operation" | "turn" | "event"
    At    time.Time
    Seq   int64
    Op    *Operation
    Turn  *Turn
    Event *LaneEvent
}
```

`web/src/api/types.ts` mirrors this exactly. The kind discriminator stays a plain string for parsing simplicity.

### 3. Per-hook gist extractors — `internal/model/lane.go` (new)

One pure function per event; all defensive (zero-value tolerant, missing fields → `""`):

```go
func gistPermission(raw json.RawMessage) string  // "Bash: npm test" or "Edit: foo.go (denied: rule)"
func gistInstructions(raw) string                // "CLAUDE.md (project)" — memory_type + path
func gistConfig(raw) string                      // "model = claude-sonnet-4-6 (was claude-opus-4-7)"
func gistCwd(raw) string                         // "→ web/"
func gistTask(raw) string                        // "Create #42: Run baseline tests"
func gistExpansion(raw) string                   // "/loop 5m → run baseline tests…"
func gistMessage(raw) string                     // truncated assistant text
func gistWorktree(raw) string                    // "removed: feature-x"
func gistStopFailure(raw) string                 // "API error: rate limited"
```

`BuildLaneEvents([]event.Event) []LaneEvent` walks the event stream, filters to hooks present in the registry, runs the appropriate extractor, and returns lane events. `MergeTimeline` is extended to append lane events into the existing chronological sort (key `At`, ties by `Seq`).

**Field names are best-guesses from the Claude Code hooks docs.** Each extractor degrades to `""` (row still renders with hook name) so a wrong guess can't break the timeline. `scripts/smoke.sh` payloads validate the field names empirically; fixes are localized to one extractor.

### 4. TUI render — minimal change

- `internal/tui/format.go`: new switch arm for lane events looking up `event.Lookup(ev.HookEvent)`, printing `glyph + label + gist`, dimmed by `severity == "dim"`.
- `internal/tui/view.go`: handle `TimelineItem.Kind == "event"` by delegating to the new format function.
- **No column-width or layout changes in the TUI.** (User asked for inspector width; that's a web change.)
- Inspector pane already shows `Raw` — no new code needed for drill-down.

### 5. Web render — two small components + width bump

- **`web/src/api/hooks.ts`** (new): one-time fetch of `/api/hooks` on mount, cached `Map<string, HookMeta>`. Fallback to a generic `HookMeta` for unknown names (forward-compat for future hooks).
- **`web/src/components/lane-event-row.ts`** (new): analogous to `op-row.ts`. Reads glyph/label/severity from the cached registry; falls back to hook name + generic glyph if unknown.
- **`web/src/components/timeline.ts`**: third arm `if (it.kind === 'event' && it.event) return html\`<hv-lane-event-row …/>\``.
- **`web/src/components/app.ts:31`**: `grid-template-columns: 240px 1fr 380px;` → `240px 1fr 520px;`. That's the entire inspector-width change.

### 6. Daemon API — one new endpoint

`GET /api/hooks` returns `event.Hooks` as JSON. Pure read, no params, no auth (loopback-only per security model). Wired in `internal/daemon/router.go` (or wherever the existing `/api/...` routes live).

## Testing

- **`internal/event/hooks_test.go`**: all 11 names present; every entry has glyph + label + lane + severity; lane values are valid `Lane` constants.
- **`internal/model/lane_test.go`**: table-driven per extractor — sample `Raw` → expected gist, missing-fields → `""`.
- **`internal/model/timeline_test.go`**: extend to verify a `LaneEvent` interleaves correctly between operations and turns by `At` (with `Seq` as tiebreak).
- **`internal/tui/format_test.go`** / **`view_test.go`**: lane events render with correct glyph, label, and dim styling for severity `"dim"`.
- **`internal/daemon/`**: `httptest` for `/api/hooks` returns the registry verbatim.
- **`web/src/components/lane-event-row.test.ts`**: glyph/label/severity dispatch including unknown-hook fallback.
- **`web/src/components/app.test.ts`**: assert the new 520px column width (smoke check against the CSS rule).
- **`scripts/smoke.sh`**: one sample payload for each of the 11 new event types; assert capture lands in JSONL and `BuildLaneEvents` produces a non-empty gist (or empty + hook-name fallback).

## Risks (accepted)

- **`MessageDisplay` may be chatty.** Severity `"dim"` keeps it visually quiet; disk volume already accepted per the issue.
- **`Raw` field names are best-guesses.** Each extractor degrades gracefully to `""`; smoke payloads validate field names; fixes are localized to one extractor at a time.

## Implementation order

1. Registry (`internal/event/hooks.go`) + tests.
2. `LaneEvent` type + `TimelineItem` extension + `MergeTimeline` update + tests.
3. Gist extractors + `BuildLaneEvents` + tests.
4. `/api/hooks` endpoint + test.
5. TUI render (format.go, view.go) + tests.
6. Web mirror types, hook-registry fetcher, `lane-event-row` component, timeline dispatch.
7. Web inspector width bump (one CSS rule).
8. Smoke script payloads for the 11 events.
9. README / CLAUDE.md update to reflect the third timeline kind.

Each step is independently testable and lands its own tests in the same commit.
