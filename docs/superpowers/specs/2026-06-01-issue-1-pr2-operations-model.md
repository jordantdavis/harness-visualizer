# Issue #1 PR 2 — Operations Model Extension

**Status:** Approved
**Source:** Issue #1, "PR 2 — operations model" section
**Predecessor:** PR 1 (commit `754d0c1`) registered all Tier 1/2/3 hooks; capture path is
already hook-agnostic.

## Goal

Extend the harness-agnostic operations model so three new Pre/Post hook pairs produce
paired `Operation` rows alongside today's `PreToolUse`/`PostToolUse`:

| Pre               | Post(s)                                      |
|-------------------|----------------------------------------------|
| `PreToolUse`      | `PostToolUse` **or** `PostToolUseFailure`    |
| `SubagentStart`   | `SubagentStop`                               |
| `PreCompact`      | `PostCompact`                                |

And introduce a per-hook decoder package so `Raw` parsing is isolated to one place
(mirroring how `internal/source/claudecode` isolates transcript schema).

## Non-Goals (deferred to PR 3)

- TUI glyph/label polish for the new operation kinds (`internal/tui/format.go`).
- Web SPA recognition of new event types.
- New lanes for Permission*, Instructions*, Config*, Cwd*, Task*.

## Design

### `Operation.Kind`

`Operation` gains a `Kind` discriminator:

```go
type Operation struct {
    Kind      string        `json:"kind"`           // "tool" | "subagent" | "compact"
    ID        string        `json:"id"`             // tool_use_id | subagent_id | compact_id
    Tool      string        `json:"tool,omitempty"` // empty for non-tool kinds
    Status    Status        `json:"status"`
    StartedAt time.Time     `json:"started_at"`
    Duration  time.Duration `json:"duration"`
    Target    string        `json:"target"`         // tool target | subagent type | compact trigger
    Seq       int64         `json:"seq"`
}
```

`Kind` defaults to `"tool"` in JSON so existing web SPA consumers see no behavioral
change for tool ops.

### Pairing engine

`BuildOperations` becomes one engine parameterised by kind. For each kind it:

1. Collects all matching Post events into per-ID and per-kind index maps.
2. Walks Pres in order, attempting:
   - **Stable ID match** — `Pre.id == Post.id` (per-kind ID extractor).
   - **Heuristic fallback** — first unclaimed Post of the **same kind** with a later Seq.
3. Unclaimed Pres are returned as `StatusRunning`.

Tool pairs additionally consider both `PostToolUse` and `PostToolUseFailure` as terminal
posts. The failure variant is preferred when both are present for the same `tool_use_id`
(which would be a daemon bug but worth handling deterministically).

Heuristics never cross kinds — a `SubagentStop` cannot accidentally close a
`PreCompact`.

### Status derivation

`DeriveStatus` gains:

- `PostToolUseFailure` → `StatusError` (unconditional; today's `exit_code` path on
  `PostToolUse` is unchanged).
- `SubagentStop` → `StatusError` if `Raw` has a non-empty `error` field, otherwise
  `StatusSuccess`.
- `PostCompact` → `StatusSuccess` (no failure variant in Tier 1/2).

`StatusRunning` and `StatusNeutral` semantics are unchanged.

### Decoder package: `internal/source/claudecode/hooks`

New package containing **only** defensive `Raw` decoders. Each function takes
`json.RawMessage` and returns a string (or struct) with zero-value on any error or
absent field. Initial surface:

```go
func ToolUseID(raw json.RawMessage) string
func SubagentID(raw json.RawMessage) string
func SubagentTarget(raw json.RawMessage) string    // subagent_type or description
func CompactID(raw json.RawMessage) string
func CompactTarget(raw json.RawMessage) string     // trigger / reason
func PostToolUseFailureMessage(raw json.RawMessage) string
func SubagentHasError(raw json.RawMessage) bool
```

The current duplicated `toolUseID` (model) and `extractToolUseID` (tui) helpers are
replaced by `hooks.ToolUseID`.

### Target extraction

`internal/model/target.go` `ExtractTarget` branches on `event.HookEvent`:

- `PreToolUse` / `PostToolUse` / `PostToolUseFailure` — existing tool target logic.
- `SubagentStart` / `SubagentStop` — `hooks.SubagentTarget(ev.Raw)`.
- `PreCompact` / `PostCompact` — `hooks.CompactTarget(ev.Raw)`.

### TUI pairing parity

`internal/tui/pairing.go` mirrors the operation-layer rewrite. `displayRow.IsPair` and
`EffectiveStatus` work identically for all three pair types. The unknown-event fallback
(`default:` branch) still picks up everything else as standalone.

## Field-name risk

The exact `Raw` field names for subagent and compact IDs/targets are speculative
(Claude Code docs catalog the hooks but not always the payload shape). The decoder
package is the **single chokepoint** to update when real payloads arrive. Until then:

- Pairing falls back to the per-kind heuristic, so missing IDs still produce paired
  rows (worst case: misordered pairs under interleaved spans, which is acceptable
  for v1).
- Tests use representative payloads documented inline; when the upstream schema is
  confirmed, only `hooks/*.go` needs updating.

## Test plan

- `internal/model/operation_test.go` — for each new kind:
  - ID-based pairing.
  - Heuristic fallback pairing.
  - Unpaired Pre → `StatusRunning`.
  - Cross-kind heuristic isolation.
- `internal/model/status_test.go` — `PostToolUseFailure` → error; subagent error/success;
  compact success.
- `internal/model/target_test.go` — subagent and compact target extraction.
- `internal/source/claudecode/hooks/hooks_test.go` — golden decoder tests for each
  helper, including empty/malformed Raw.
- `internal/tui/pairing_test.go` — parity for new pair types.
- `scripts/smoke.sh` — one additional `PostToolUseFailure` payload.

## Out of scope / future work

- PR 3 will introduce new lanes (Permission*, Instructions*, Task*) and TUI/web
  rendering polish.
- A shared event-type registry between TUI and SPA is noted in the issue as worth
  exploring in PR 3.
