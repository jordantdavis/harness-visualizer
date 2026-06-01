# Deprecate `hv tui` — Design

Issue: [#9](https://github.com/jordantdavis/harness-visualizer/issues/9)

## Motivation

`hv` ships two viewers over the same event stream:

- `hv tui` — bubbletea master-detail viewer (~8,700 LOC across `internal/tui/`)
- `hv serve` — opens the daemon's embedded Lit/Vite web UI at `/`

The web UI is now the primary surface: it's where new rendering work lands
(lanes, operations inspector, live updates over SSE), it's easier to iterate on,
and it doesn't carry the terminal-specific complexity (layout math, themes, key
bindings, screen-reader fallbacks). Maintaining two viewers means every new hook
type, lane, or operation field needs to be rendered twice. Collapsing to one
consumer lets `internal/event/hooks.go` (shared render metadata) simplify too.

## Resolved Open Questions

The issue lists three open questions. Resolved before design:

1. **`--plain` line-per-event mode** → **Drop it entirely.** Users can `tail -f`
   the JSONL files directly, or use the web UI. No `hv tail` subcommand.
2. **Phasing** → **Single PR.** `hv` is a local-only, single-user tool
   (per `CLAUDE.md`); there are no external users to warn. No soft-deprecation
   step.
3. **Explicit user warning** → **No.** Same reason.

## Scope

### Delete

- Entire `internal/tui/` package — all `.go` files (model, view, inspect,
  filter, theme, layout, stream, live, pairing, format, plain, run, client) and
  their `_test.go` siblings, plus `testdata/`. ~8,666 LOC.
- `tui` case in the `switch` block of `cmd/hv/main.go`.
- The `internal/tui` import line in `cmd/hv/main.go`.
- The `hv tui` line in `cmd/hv/main.go`'s `usage()` help text.
- The `hv tui` bullet in `cmd/hv/main.go`'s top-of-file package doc comment
  (which currently calls out "three roles" but enumerates four — after removal
  it genuinely is three).
- Unused dependencies from `go.mod` / `go.sum`. Today the direct deps are
  `github.com/aymanbagabas/go-osc52/v2`, `github.com/charmbracelet/bubbletea`,
  and `github.com/charmbracelet/lipgloss`; the indirect block pulls in
  `charmbracelet/colorprofile`, `charmbracelet/x/*`, `clipperhouse/*`,
  `erikgeiser/coninput`, `lucasb-eyer/go-colorful`, `mattn/go-*`, `muesli/*`,
  `rivo/uniseg`, `xo/terminfo`, `golang.org/x/sys`, `golang.org/x/text`. The
  authoritative cleanup is whatever `go mod tidy` removes after the
  `internal/tui` import is gone.

### Update

- **`README.md`** — replace `hv tui` mentions under "How it works" and "Usage"
  with `hv serve` equivalents. Drop the "watch events live: `hv tui`" snippet;
  point at `hv serve` instead.
- **`CLAUDE.md`** — change the `## Architecture: one binary, four roles`
  heading to `three roles`. Remove the `hv tui` bullet from that list. Sweep
  for any other TUI mentions and remove or rephrase.

### Keep untouched

- `internal/event/hooks.go` — shared render metadata. The TUI was one
  consumer; the web UI is the other. After this PR the "shared" framing is
  inaccurate, but simplifying single-consumer code is out of scope (separate
  follow-up if desired). Scope discipline.
- Default bare `hv` invocation behavior (hook forwarder via `internal/client`).
- All other packages (`daemon`, `serve`, `client`, `store`, `event`, `model`,
  `source/claudecode`, `web`, `paths`).
- `.claude-plugin/` and `plugin/hooks/hooks.json` — plugin packaging is
  independent of the viewer.

## Behavior After Removal

`hv tui` (and any other unknown subcommand) hits the existing `default` branch
in `main.go`'s switch:

```
hv: unknown command "tui"

hv — Claude Code Harness Visualizer
usage:
  hv hook       forward a hook payload from stdin to the daemon (default)
  hv daemon     run the capture daemon (--foreground, --port)
  hv serve      ensure the daemon is up and open the web UI in a browser
```

Exit code 2. No special-case "did you mean `hv serve`?" hint — the usage block
already lists `hv serve`, so the generic error path conveys the same
information without a one-off code path.

## Verification

- `go build ./cmd/hv` succeeds.
- `go mod tidy` produces a clean `go.mod` / `go.sum` (no stray deps, no
  missing deps). Diff should show only removals of bubbletea/lipgloss/etc.
- `go test ./...` passes. TUI tests are deleted with the package; no other
  package imports `internal/tui`, so nothing else should break.
- `./scripts/smoke.sh` still passes end-to-end (it exercises hook → daemon →
  JSONL and never touched the TUI).
- `make build` produces a working binary.
- `hv serve` still opens the web UI.
- `hv tui` prints `unknown command "tui"`, prints usage (which now lists three
  commands), and exits 2.

## Non-goals

- Refactoring `internal/event/hooks.go` to drop its "shared" framing.
- Adding a `hv tail` subcommand or any other replacement for `--plain`.
- Touching the web UI, daemon, or hook forwarder.
- Plugin / marketplace changes.
