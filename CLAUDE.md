# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

`hv` is a local-only, single-user **Claude Code harness visualizer**: it captures every
Claude Code hook event and lets you see what the harness actually does — which hooks fire,
in what order, with what data, and how long tools take. **Config is intent; the captured
event stream is reality.** Security model: the daemon binds `127.0.0.1` only — no auth, no
TLS, no cloud. Do not add network exposure, auth, or remote features without reconsidering
this premise.

## Commands

```bash
make build        # full build: Lit frontend (web/) -> internal/web/dist, then embed into `hv`
make go-build     # Go binary only (embeds whatever is currently in internal/web/dist)
make web          # rebuild frontend only; restores internal/web/dist/.gitkeep after Vite wipes it
make test         # Go tests (fast, no Node) — `go test ./...`
make test-web     # frontend unit tests (vitest)
make clean        # remove binary + dist, recreate dist/.gitkeep

make ci           # reproduce the full CI pipeline locally (ci-go + ci-web)
make ci-go        # Go gate: fmt-check, vet, test, go-build
make ci-web       # web gate: web-deps, web-build, web-test
make fmt          # gofmt -w all tracked Go files (fmt-check is the CI gate)

go test ./internal/model/...                 # one package
go test ./internal/daemon/ -run TestName     # one test
cd web && npm run test                       # vitest directly (npx vitest run path/to.test.ts for one file)

./scripts/smoke.sh   # end-to-end: build, fire test hook payloads, assert events land in JSONL (isolated temp dir)
```

A fresh checkout can `go build ./cmd/hv` before the frontend is built — `internal/web/dist`
ships a tracked `.gitkeep` so `go:embed` always has something, and the binary serves a "run
`make build`" notice instead of the app until the real bundle is embedded.

## Architecture: one binary, three roles

`cmd/hv/main.go` is pure subcommand dispatch. Each role is a package exposing `Run(args []string) int`:

- **`hv hook`** (`internal/client`) — the hook forwarder. Claude Code runs this **per hook
  event** (also the bare `hv` invocation). It reads the hook payload from stdin, enriches it
  into an `event.Event`, and POSTs to the daemon. **Hard constraints: always exits 0, bounded
  to ~100ms total, swallows all errors, writes nothing to stdout.** The hook path must never
  block or break Claude Code. If the POST hits connection-refused, it auto-spawns the daemon
  once. Treat this critical path as sacred — keep it tiny and panic-safe.
- **`hv daemon`** (`internal/daemon`) — long-running HTTP capture server + SSE hub. Receives
  events via POST, persists via a per-session writer goroutine (`internal/store`), and fans
  out live to SSE subscribers (`hub`). Auto-spawned by the first hook; users don't normally
  start it (`--foreground` / `--port` for dev). The `Server` value is the testable unit.
- **`hv serve`** (`internal/serve`) — thin launcher: ensures the daemon is up, then opens the
  browser at the daemon's embedded web UI. It is **not** a second server — the daemon serves
  the UI at `/`.

### Data flow

```
Claude Code hook ──stdin──> hv hook ──POST /events──> daemon ──> store (JSONL) ──> SSE /stream
                                                                          │
                                                          Web UI <──── GET /api/... + SSE
```

Events are stored as JSONL, one file per session, at
`$HV_DATA_DIR` || `$XDG_DATA_HOME/hv` || `~/.local/share/hv` under `sessions/{session_id}.jsonl`.
Runtime files (port file, pidfile, daemon log) live under `$XDG_RUNTIME_DIR/hv` or fall back
to the data dir. Path resolution is centralized in `internal/paths`. Default port: **7842**.

### Key domain packages

- **`internal/event`** — the canonical `Event` envelope: a few fields `hv` owns (`ID`,
  `CapturedAt`, `Seq`, `HookEvent`, `SessionID`, `ToolName`…) **plus `Raw`, the entire
  original hook payload kept verbatim**. Parsing is deliberately lenient (absent fields →
  zero values, not errors) so new upstream Claude Code fields never break capture. Hook
  metadata for standalone lane events (glyph, label, lane, severity) lives in
  `internal/event/hooks.go`, consumed by the web client to render each event.
- **`internal/model`** — harness-agnostic domain types and the timeline logic.
  `BuildOperations` pairs `PreToolUse`/`PostToolUse` into `Operation`s (keyed by `tool_use_id`,
  so live upserts replace a running row in place). `BuildLaneEvents` reduces standalone
  (non-pairable) hooks like `PermissionRequest`, `InstructionsLoaded`, `CwdChanged`, etc. into
  `LaneEvent`s, with per-hook gist extractors that read `Raw` defensively. `MergeTimeline`
  interleaves operations, conversation `Turn`s, and lane events into one chronological
  `TimelineItem` list. **Operations are authoritative; turns and lane events enrich.** Sort
  key is `At`, ties broken by `Seq`.
- **`internal/source/claudecode`** — the **only** package that knows Claude Code's on-disk
  transcript schema. It adapts transcripts into the harness-agnostic `model.Turn`. Everything
  upstream consumes `model.Turn`. Parsing is defensive: a missing/unreadable/foreign file
  yields no turns and no error, so the timeline degrades gracefully to operations-only. Add
  new harness sources here as sibling packages — don't leak schema knowledge upward.
- **`internal/web`** — `go:embed all:dist` of the built Lit SPA, served with SPA fallback.
  Embeds `internal/web/dist` (not top-level `web/dist`) because `go:embed` can't traverse
  upward out of the source file's directory.

### Daemon HTTP surface

Legacy/raw: `/healthz`, `/events` (POST), `/sessions`, `/sessions/{id}`, `/stream` (SSE).
JSON API for the web UI: `/api/sessions`, `/api/sessions/{id}/timeline?after=`,
`/api/sessions/{id}/operations/{opID}`, `/api/hooks`. `/` serves the embedded SPA.

### Frontend (`web/`)

Lit + TypeScript + Vite SPA. Components in `web/src/components`, API client + SSE in
`web/src/api`. Built output goes to `internal/web/dist` for embedding. Vite's `emptyOutDir`
wipes the tracked `.gitkeep`, so `make web` / the npm `postbuild` step recreates it to keep
`git status` clean.

## Conventions

- **Always exit 0 / degrade gracefully**: the hook path never fails, and transcript/timeline
  parsing tolerates missing or foreign data rather than erroring. Preserve this.
- Each role package exposes a `Run(args) int`; `main` stays pure dispatch.
- Heavy state is injected (store dir, daemon spawn fn, browser-open fn) so packages are
  testable against temp dirs and fakes — follow this when extending.
- Tests live beside code (`*_test.go`, `*.test.ts`) and are the norm; mirror existing test
  style when adding behavior.

## Plugin packaging

The repo root ships `.claude-plugin/marketplace.json` and `plugin/` is the installable
Claude Code plugin (`plugin/hooks/hooks.json` registers `hv hook` for all hook events, all
`async: true`). The plugin's binary is `plugin/bin/hv` (gitignored — built via
`go build -o plugin/bin/hv ./cmd/hv`). See README.md for the `claude plugin marketplace add`
/ `claude plugin install` flow.
