# Web UI Pivot — Design Spec

**Date:** 2026-05-31
**Status:** Approved (brainstorm), pending implementation plan
**Author:** Jordan Davis (with Claude)

## Summary

Pivot the Claude Code Harness Visualizer (`cchv`) from its terminal TUI to a
web-based UI built with [Lit](https://lit.dev/), served from the existing Go
binary. The web app is a **dumb client**: all operation/diff/conversation
derivation happens **server-side** in a new `internal/model` package (extracted
from `internal/tui`). The TUI and web UI coexist as peer clients of the same
HTTP/SSE API until the web UI reaches parity.

v1 is a **thin vertical slice** that proves the whole pipe end-to-end, then
layers features on as fast-follows.

## Decisions (locked in brainstorm)

| Decision | Choice |
|---|---|
| v1 scope | Thin vertical slice (smallest end-to-end loop), not parity-first |
| Primary layout | Three-pane: Sessions │ Timeline │ Inspector |
| Input model (v1) | Mouse/click-driven; keyboard navigation is a fast-follow |
| Conversation transcript | Interleaved into a single unified timeline (Approach 1) |
| Timeline authority | Hook events primary; transcript enriches; degrades gracefully |
| Visual direction | Terminal-native (monospace, dark, ANSI palette, glyphs, dense) |
| Derivation | Server-side in `internal/model`; browser never derives |
| Harness coupling | Claude Code parsing quarantined behind `internal/source/claudecode` |
| Network | Localhost-only (`127.0.0.1`), no auth, no TLS, no CORS |
| Frontend stack | Lit + Vite, `go:embed`'d into the daemon binary |
| Live streaming | Model A (raw events over existing `/stream`) + nudge-refetch |
| Model B (server-pushed derived deltas) | Noted as a future optimization; NOT built |

## Guiding constraints (from ROADMAP)

- Single-user, local, personal. Binding to `127.0.0.1` **is** the security model.
- Capture is the contract: hook events are authoritative; the daemon owns ordering.
- A Go channel is the queue; JSONL files are the database. No broker, no SQLite.
- YAGNI ruthlessly.

---

## A. System architecture

One binary, unchanged shape. We extend the daemon; we do not add a process.

```
cchv hook     (default)  forward hook payload                      [unchanged]
cchv daemon              HTTP + SSE + now also serves the web app  [extended]
cchv tui                 terminal viewer                           [unchanged, coexists]
cchv serve               ensure daemon is up, open browser to UI   [new, thin launcher]
```

- The daemon already binds `127.0.0.1`, owns the SSE hub, and reads the store.
- Mount the embedded Lit bundle at `/`; keep data routes under `/api/`; leave
  `/stream`, `/events`, `/healthz` as-is.
- `cchv serve` is a ~40-line launcher: ensure the daemon is running (reuse the
  hook CLI's detach logic in `internal/client`), read the port file, open the
  browser at `http://127.0.0.1:<port>/`. It is a launcher, not a second server.
- TUI and web are peer clients of the same API. No flag-day cutover; the TUI is
  the reference implementation until web reaches parity.

## B. `internal/model` + `internal/source` extraction (keystone)

Derivation currently lives inside `internal/tui` (`pairing.go`, `format.go`,
`inspect.go` — ~1660 lines). Lift the **domain logic** out so the TUI, the web
client, and the HTTP layer share one source of truth.

```
internal/model/        # harness-agnostic domain types + derivation (pure, tested)
  operation.go         #   Operation (was displayRow + EffectiveStatus + Duration)
  pairing.go           #   buildOperations()  (moved from tui/pairing.go)
  status.go            #   deriveStatus, gist/target extraction (from format.go)
  diff.go              #   structured line-diff + payload shaping (see note below)
  conversation.go      #   Turn{Role, Text, Thinking, ToolRefs, Timestamp}
  timeline.go          #   MergeTimeline(ops, turns) -> []TimelineItem
internal/source/       # harness-specific parsing quarantined here
  claudecode/          #   transcript reader -> []model.Turn (ONLY CC-aware code)
```

`internal/tui` becomes a **renderer** of `model.*` types: its view code stays,
its derivation logic moves. Do this refactor **first**, keeping the TUI's
existing tests green, before any web code exists. Highest-leverage, lowest-risk
step; de-risks everything downstream.

**Diff responsibility split (clarification).** The TUI today *fakes* a diff for
`Edit` (full `old_string` / `new_string` blocks). v1 upgrades this: the server
computes a **structured line diff** (added / removed / context hunks with line
numbers) from `old_string`/`new_string` in `internal/model/diff.go` — this is
new logic, not a straight move. The browser is responsible only for **rendering
and syntax-highlighting** that structured diff (highlighting is a client
concern, lazy-loaded grammars). Status/target/gist derivation *is* a straight
move from `format.go`/`inspect.go`.

## C. Unified-timeline data model (Approach 1)

```go
// model — the wire contract for the timeline
type TimelineItem struct {
    Kind string    `json:"kind"`           // "operation" | "turn"
    At   time.Time `json:"at"`             // ordering key
    Seq  int64     `json:"seq"`            // hook Seq when Kind=="operation"
    Op   *Operation `json:"op,omitempty"`
    Turn *Turn      `json:"turn,omitempty"`
}

type Operation struct {
    ID        string        `json:"id"`        // tool_use_id — stable upsert key
    Tool      string        `json:"tool"`      // Edit, Bash, Read…
    Status    string        `json:"status"`    // running | success | error
    StartedAt time.Time     `json:"started_at"`
    Duration  time.Duration `json:"duration"`  // 0 while running
    Target    string        `json:"target"`    // file path / command gist
    // detail (diff, input, response, raw) lazy-loaded via /operations/{id}
}

type Turn struct {
    Role     string    `json:"role"`      // user | assistant
    Text     string    `json:"text"`
    Thinking string    `json:"thinking,omitempty"`
    ToolRefs []string  `json:"tool_refs,omitempty"` // tool_use_ids issued in this turn
    At       time.Time `json:"at"`
}
```

**Reconciliation rules (server-side, `MergeTimeline`):**

1. Operations are authoritative — built from hook events, keyed by `tool_use_id`.
2. Turns come from the transcript adapter; a turn's `tool_use` entries are
   **dropped** (the matching Operation already represents them, matched by
   `tool_use_id`).
3. Merge-sort by `At`; ties broken by `Seq` so operations stay chronologically
   stable.
4. **Graceful degradation:** no transcript → operations-only list, full tool
   timeline intact.

**Clock alignment risk:** hook `CapturedAt` (CLI wall-clock) and transcript
timestamps are different clocks. Sub-second ordering is approximate; `Seq`
breaks ties for operations. Acceptable for v1. If interleaving order looks wrong
in practice, a fast-follow can anchor turns relative to their `ToolRefs`
operations rather than by raw timestamp.

## D. HTTP API + streaming contract

REST + JSON, no versioning, all data routes under `/api/` (the SPA catch-all at
`/` must not collide). Operation *list* is lightweight; heavy detail is lazy.

| Route | Returns |
|---|---|
| `GET /api/sessions` | `[]SessionInfo` *(exists today)* |
| `GET /api/sessions/{id}/timeline?after={seq}&limit={n}` | `[]TimelineItem` — merged ops + turns, seq-cursor paginated |
| `GET /api/sessions/{id}/operations/{opID}` | full operation detail (diff, input, response, raw) |
| `GET /stream?session={id}` | SSE — raw events, **unchanged** (Model A) |
| `GET /healthz` | liveness *(exists)* |

Notes:
- `POST /events` (hook ingest) stays where it is — separate concern from the
  browser read API. The TUI's `HTTPClient` base paths move to `/api/` in the
  same change (only other consumer).
- **Pagination** is seq-cursor (`?after={seq}&limit={n}`), not offset. The
  store's `Read` must accept a `limit` and stop scanning early so a large
  session does not fully serialize on first paint.

**Streaming — Model A + nudge-refetch:**

- Browser opens `EventSource('/stream?session=…')`.
- Each raw SSE frame is a **nudge** ("something changed past seq N"). The client
  debounces (~150ms) a `GET …/timeline?after={lastSeq}` to pull the
  **server-derived** delta, then upserts `TimelineItem`s keyed by
  `Operation.ID` (running row → resolved row, in place).
- The browser never pairs or derives.
- Reconnect honors `Last-Event-ID` → catch-up read from the store, then resume
  live. (The SSE handler already emits `id: {seq}` per frame.)
- **Model B** (server pushes derived operation deltas, removing the refetch
  round-trip) is a future optimization; the wire contract above does not depend
  on it.

## E. Lit component tree (v1 — mouse-first)

```
<cchv-app>                 router + stores (no keyboard controller yet)
├─ <cchv-top-bar>          daemon status · live indicator · error count
├─ <cchv-session-list>     left pane — GET /api/sessions, live dots
├─ <cchv-timeline>         center pane — unified TimelineItem stream
│  ├─ <cchv-op-row>          tool operation (glyph · tool · target · status · dur)
│  └─ <cchv-turn-row>        conversation turn (you / assistant prose)
└─ <cchv-inspector>        right pane — selected operation detail
   ├─ <cchv-diff-view>       syntax-highlighted line diff (Edit/Write)
   ├─ <cchv-code-view>       Read/Bash output
   └─ <cchv-raw-view>        pretty-printed raw JSON (escape hatch)
```

State lives in a few lightweight reactive stores (not a heavy framework):

- `SessionStore` — `GET /api/sessions`, selected session, live-recency dots.
- `TimelineStore` — owns the `after`-cursor fetch and the SSE-nudge merge;
  holds `TimelineItem`s keyed by `Operation.ID` for in-place upserts.
- `StreamController` — wraps `EventSource`, reconnect with backoff, parses
  `id:`/`data:` frames.
- `UiStore` — selected session/operation, pane widths (persisted later).

Components are presentational; flow is unidirectional (mirrors Bubble Tea's
`Msg → Update → View`). Virtualize the timeline list (render only visible rows).
Syntax highlighting: **lightweight + lazy-loaded grammars** (Shiki deferred for
bundle budget).

**Visual direction:** terminal-native. Monospace for all data; square corners;
the dark ANSI palette from `docs/tui-mockup.html`; status shown by glyph +
position + color (never color alone — keep `✔ ✘ ▶ ·`). Selection = left accent
bar + subtle band + reserved gutter (zero-reflow), carried from the TUI.

## F. Build & embed toolchain

```
web/                     Lit + Vite source (src/, package.json, vite.config.ts)
web/dist/                Vite build output — gitignored, embedded at compile time
internal/web/embed.go    //go:embed dist  (+ a //go:build dev stub serving empty FS)
```

- **Makefile is the source of truth:** `build: web` (npm build → `web/dist`)
  then `go build`, so `go build` never embeds a stale `dist`. Do **not** use
  `go:generate` for the frontend build (easy to forget; `go build` won't trigger
  it).
- Commit `web/dist/.gitkeep` (or use the `dev` build-tag stub) so a fresh
  checkout `go build`s before anyone runs npm.
- **Dev:** `vite dev` (HMR, e.g. `:5173`) + `cchv daemon --foreground`. Vite
  **proxies** `/api` and `/stream` to the daemon → browser stays same-origin →
  **no CORS** anywhere.
- **Prod:** `go build` ships one self-contained binary, zero runtime deps.
- **CI:** a fast Go job (vet/test, no Node) plus a release job running
  `make build` (Node + Go) that uploads the embedded binary.

## G. v1 scope line

**In v1 (the thin slice):**

- Session list (left pane)
- Live interleaved timeline (operations + conversation transcript)
- Inspector with: real syntax-highlighted diff (Edit/Write), Read/Bash output,
  raw JSON escape hatch
- `cchv serve` launcher
- Embedded production build (`go build` → self-contained binary)
- Mouse/click-driven interaction

**Fast-follows (explicitly deferred — NOT built in v1):**

- Keyboard navigation (and the global keyboard controller)
- Filter (`/`), error-hop, operation folding toggle, yank/copy menu
- Light theme / adaptive theming, density toggle, reduced-motion
- Error-minimap on the scrollbar, deep links, command palette
- Model B streaming (server-pushed derived deltas)
- Full E2E test suite

## H. Testing strategy

- `internal/model` and `internal/source/claudecode`: table-driven Go unit tests.
  The extraction must be **behavior-identical** to the current TUI derivation —
  the existing TUI tests are the regression net during the move.
- API handlers: `httptest` coverage (timeline pagination, operation detail,
  graceful-degradation when transcript is absent).
- Frontend: a lightweight component smoke test in v1; full E2E deferred.

## I. Risks & mitigations

| Risk | Mitigation |
|---|---|
| Derivation duplicated in two languages | Server-side derivation only; `internal/model` is the single source of truth. Highest-priority decision. |
| Large first-paint payloads | Lazy operation detail + seq-cursor pagination + `limit` on store reads + list virtualization. |
| SSE dropped frames (hub drops to slow subscribers) | Treat SSE as a nudge; reconcile via `after={lastSeq}` refetch + `Last-Event-ID` catch-up. Don't trust every frame. |
| Clock skew between hook events and transcript | `Seq` tie-break for operations; fast-follow can anchor turns to `ToolRefs`. |
| Binary size from embedded assets | Lit app is small; avoid heavy deps (lazy-load highlighter grammars); optional CI size budget. |
| CORS / SSE cross-origin in dev | Vite proxy keeps browser same-origin; never enable CORS. |
| Accidental non-localhost bind | Keep `127.0.0.1`-only assumption explicit in code/docs. |

## J. Implementation sequencing (for the plan)

1. Refactor: extract derivation from `internal/tui` → `internal/model`; TUI
   consumes it; existing tests stay green. (No UI change.)
2. Add `internal/source/claudecode` transcript reader → `[]model.Turn`; add
   `MergeTimeline`.
3. Add `/api/...` endpoints (timeline, operation detail); point the TUI's
   `HTTPClient` at `/api/`.
4. Scaffold `web/` (Lit + Vite, dev proxy); render session list + timeline +
   inspector against the live API. No embedding yet.
5. Add `internal/web` embed + SPA handler; add `cchv serve`; wire `make build`.
6. Reach parity over time via fast-follows; retire the TUI only then.

## Open questions (non-blocking)

- Is the entire hook payload always present in `Event.Raw`, or can large bodies
  be truncated by the hook CLI? If truncated, diffs/outputs may need a derived
  endpoint rather than client-side parsing. (Confirm during step 3.)
- Realistic max events/session — affects whether the `/timeline` full-array
  fetch needs hard range limits beyond `limit`. Measure during step 4.
