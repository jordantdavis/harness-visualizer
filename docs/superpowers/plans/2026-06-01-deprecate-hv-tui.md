# Deprecate `hv tui` Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the `hv tui` subcommand and its entire `internal/tui/` package, leaving `hv serve` (the web UI) as the only viewer.

**Architecture:** Pure deletion. Strip the `internal/tui` import + switch case from `cmd/hv/main.go`, delete the package directory, run `go mod tidy` to drop the bubbletea/lipgloss dependency tree, and update `README.md` + `CLAUDE.md` to remove TUI references. No replacement subcommand. Issue [#9](https://github.com/jordantdavis/harness-visualizer/issues/9).

**Tech Stack:** Go 1.26, `go mod tidy`, plain markdown edits. No new code.

**Spec:** `docs/superpowers/specs/2026-06-01-deprecate-hv-tui-design.md`

---

## File Inventory

**Delete (entire files):**
- `internal/tui/client.go`, `client_test.go`
- `internal/tui/filter.go`, `filter_test.go`
- `internal/tui/format.go`, `format_test.go`
- `internal/tui/inspect.go`, `inspect_test.go`
- `internal/tui/layout.go`, `layout_test.go`
- `internal/tui/live.go`, `live_test.go`, `live_view_test.go`
- `internal/tui/model.go`, `model_test.go`
- `internal/tui/pairing.go`, `pairing_test.go`
- `internal/tui/phase7_test.go`, `phase8_test.go`
- `internal/tui/plain.go`, `plain_test.go`
- `internal/tui/run.go`
- `internal/tui/stream.go`, `stream_test.go`
- `internal/tui/theme.go`, `theme_test.go`
- `internal/tui/testmain_test.go`
- `internal/tui/view.go`
- `internal/tui/testdata/` (entire subdirectory)
- (Final: `internal/tui/` directory itself is removed)

**Modify:**
- `cmd/hv/main.go` — remove the `internal/tui` import (line 20), the `case "tui":` switch arm, the `hv tui` line in the `usage()` block, and the `hv tui` bullet in the top-of-file package doc comment. Update the "three roles" phrasing in that comment.
- `README.md` — line 15 (`hv tui` bullet under "How it works") and lines 85-90 (`hv tui` usage example). Replace with `hv serve` equivalents.
- `CLAUDE.md` — line 35 heading ("four roles" → "three roles"), lines 49-51 (`hv tui` bullet — delete), line 61 data-flow diagram (`TUI / Web UI` → `Web UI`), line 76 (rephrase the "TUI and web client share" sentence).
- `go.mod`, `go.sum` — `go mod tidy` will drop bubbletea, lipgloss, and the entire indirect dependency tree they pulled in.

---

## Task 1: Delete the `internal/tui/` package

**Files:**
- Delete: entire `internal/tui/` directory and contents

**Rationale:** Removing first makes the import in `cmd/hv/main.go` break, which gives us a fast `go build` signal that we caught every reference. After step 1 the tree is broken; step 2 fixes it.

- [ ] **Step 1: Delete the directory**

```bash
git rm -r internal/tui
```

- [ ] **Step 2: Verify the build is broken in exactly the expected way**

```bash
go build ./cmd/hv
```

Expected: build fails with an error like `cmd/hv/main.go:20:2: package jordandavis.dev/harness-visualizer/internal/tui is not in std`. If you see any *other* compilation errors from other packages, stop — something else depended on `internal/tui` and the spec missed it. Investigate before continuing.

- [ ] **Step 3: Verify no other Go file imports `internal/tui`**

```bash
grep -rn "internal/tui" --include="*.go" .
```

Expected: only `cmd/hv/main.go:20` shows up. If anything else shows, stop and report.

- [ ] **Step 4: Stage the deletion (no commit yet — we want one atomic commit per logical change, and the deletion + main.go fix go together)**

The `git rm -r` in Step 1 already staged the deletions. Leave them staged.

---

## Task 2: Update `cmd/hv/main.go`

**Files:**
- Modify: `cmd/hv/main.go`

- [ ] **Step 1: Replace the file contents**

Read the current file first to be sure you're starting from the expected state, then overwrite with:

```go
// Command hv is the Claude Code Harness Visualizer. The single binary plays
// three roles, selected by subcommand:
//
//	hv hook     forward one hook payload (stdin) to the daemon; also the
//	              bare/default invocation Claude Code calls per hook
//	hv daemon   long-running HTTP server + SSE hub (--foreground for dev)
//	hv serve    ensure the daemon is up and open the web UI in a browser
//
// Each role lives in its own package exposing Run(args []string) int; main is
// pure dispatch so the hook critical path stays tiny.
package main

import (
	"fmt"
	"os"

	"jordandavis.dev/harness-visualizer/internal/client"
	"jordandavis.dev/harness-visualizer/internal/daemon"
	"jordandavis.dev/harness-visualizer/internal/serve"
)

func main() {
	// Bare invocation (no subcommand) is the hook forwarder: Claude Code
	// runs `hv` per hook, and the hook path must never fail.
	if len(os.Args) < 2 {
		os.Exit(client.Run(nil))
	}

	cmd, rest := os.Args[1], os.Args[2:]
	switch cmd {
	case "hook":
		os.Exit(client.Run(rest))
	case "daemon":
		os.Exit(daemon.Run(rest))
	case "serve":
		os.Exit(serve.Run(rest))
	case "-h", "--help", "help":
		usage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "hv: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `hv — Claude Code Harness Visualizer

usage:
  hv hook       forward a hook payload from stdin to the daemon (default)
  hv daemon     run the capture daemon (--foreground, --port)
  hv serve      ensure the daemon is up and open the web UI in a browser
`)
}
```

Changes from the original (for reviewer clarity):
- Top doc comment: "three roles" (was "three" but listed four); removed the `hv tui` line.
- Removed `"jordandavis.dev/harness-visualizer/internal/tui"` import.
- Removed `case "tui": os.Exit(tui.Run(rest))` from the switch.
- Removed `  hv tui        open the terminal viewer` line from `usage()`.

- [ ] **Step 2: Build**

```bash
go build ./cmd/hv
```

Expected: clean build, no errors.

- [ ] **Step 3: Smoke-test the new help output and unknown-command behavior**

```bash
./hv help
```

Expected output (exactly):
```
hv — Claude Code Harness Visualizer

usage:
  hv hook       forward a hook payload from stdin to the daemon (default)
  hv daemon     run the capture daemon (--foreground, --port)
  hv serve      ensure the daemon is up and open the web UI in a browser
```

```bash
./hv tui; echo "exit=$?"
```

Expected:
```
hv: unknown command "tui"

hv — Claude Code Harness Visualizer
... (usage block) ...
exit=2
```

- [ ] **Step 4: Run all Go tests**

```bash
go test ./...
```

Expected: PASS across every package. No `internal/tui` package shows up in the test output (because it's gone). Nothing else broke.

- [ ] **Step 5: Commit (deletion + main.go fix together)**

```bash
git add cmd/hv/main.go
git status   # sanity-check: should show deletions under internal/tui/ + modified cmd/hv/main.go
git commit -m "$(cat <<'EOF'
refactor: remove hv tui subcommand and internal/tui package (#9)

The web UI (hv serve) is the primary viewer. Maintaining two viewers
over the same event stream meant every new hook type, lane, or
operation field had to be rendered twice. Drop the bubbletea TUI
entirely; users get hv serve.

- delete internal/tui/ (~8,700 LOC)
- drop tui subcommand dispatch + import in cmd/hv/main.go
- update usage and package doc to reflect three roles, not four

go.mod / docs updates follow in subsequent commits.
EOF
)"
```

Cleanup any local `hv` binary so it's not staged accidentally: `rm -f hv`.

---

## Task 3: Tidy Go modules

**Files:**
- Modify: `go.mod`, `go.sum`

- [ ] **Step 1: Run go mod tidy**

```bash
go mod tidy
```

Expected: no errors. `go.mod` should lose `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, and `github.com/aymanbagabas/go-osc52/v2` from the direct requires block; `go.sum` and the indirect block should shrink substantially (charmbracelet/x/*, clipperhouse/*, erikgeiser/coninput, lucasb-eyer/go-colorful, mattn/go-isatty, mattn/go-localereader, mattn/go-runewidth, muesli/*, rivo/uniseg, xo/terminfo, golang.org/x/sys, golang.org/x/text).

- [ ] **Step 2: Verify the build still works after tidy**

```bash
go build ./cmd/hv
```

Expected: clean build.

- [ ] **Step 3: Verify tests still pass after tidy**

```bash
go test ./...
```

Expected: PASS across every package.

- [ ] **Step 4: Inspect the go.mod diff**

```bash
git diff go.mod
```

Sanity-check: only removals (no additions). Direct requires block should now contain only Go-stdlib-adjacent deps actually used by non-TUI code (likely none — the project's non-TUI packages are pure stdlib + internal packages). If `go.mod` shows additions, something is wrong — stop and investigate.

- [ ] **Step 5: Commit**

```bash
git add go.mod go.sum
git commit -m "$(cat <<'EOF'
chore: go mod tidy after dropping internal/tui (#9)

Drops bubbletea, lipgloss, go-osc52, and their transitive dependency
tree. No remaining package imports them.
EOF
)"
```

---

## Task 4: Update `README.md`

**Files:**
- Modify: `README.md` (lines 15 and 85-90 per pre-task `grep`)

- [ ] **Step 1: Update the "How it works" bullet (line 15)**

Find:
```
- `hv tui` — terminal viewer (master-detail: sessions → events → inspector)
```

Replace with:
```
- `hv serve` — opens the web UI in a browser (ensures the daemon is up first)
```

- [ ] **Step 2: Update the Usage section (lines 85-90)**

Find:
```
After install, just use Claude Code normally. The daemon starts itself on the first hook
event. To watch events live:

```bash
hv tui
```
```

Replace with:
```
After install, just use Claude Code normally. The daemon starts itself on the first hook
event. To watch events live:

```bash
hv serve
```
```

(Only the command on the fenced line changes; the surrounding prose stays.)

- [ ] **Step 3: Confirm no TUI mentions remain**

```bash
grep -n "tui\|TUI" README.md
```

Expected: no matches.

- [ ] **Step 4: Commit**

```bash
git add README.md
git commit -m "$(cat <<'EOF'
docs(readme): replace hv tui mentions with hv serve (#9)
EOF
)"
```

---

## Task 5: Update `CLAUDE.md`

**Files:**
- Modify: `CLAUDE.md`

`CLAUDE.md` has four TUI touchpoints (per pre-task `grep`):
1. Line 35 — heading: `## Architecture: one binary, four roles` → `three roles`
2. Lines 49-51 — the `hv tui` bullet — delete entirely
3. Line 61 — data-flow ASCII: `TUI / Web UI` → `Web UI`
4. Line 76 — sentence: "so the TUI and web client share rendering decisions" → drop the TUI half

- [ ] **Step 1: Change the architecture heading (line 35)**

Find:
```
## Architecture: one binary, four roles
```

Replace with:
```
## Architecture: one binary, three roles
```

- [ ] **Step 2: Delete the `hv tui` bullet (lines 49-51)**

Find (three lines forming one bullet):
```
- **`hv tui`** (`internal/tui`) — bubbletea terminal viewer (master-detail: sessions →
  events → inspector). `--plain` gives line-per-event output (screen-reader/pipe safe);
  `NO_COLOR` and `--no-animation` are honored.
```

Delete those three lines entirely. The `- **`hv daemon`** …` bullet above and `- **`hv serve`** …` bullet below should sit adjacent with no gap.

- [ ] **Step 3: Update the data-flow diagram (line 61)**

Find:
```
                                                  TUI / Web UI <──── GET /api/... + SSE
```

Replace with:
```
                                                       Web UI <──── GET /api/... + SSE
```

(Adjust the leading whitespace so `Web UI` aligns roughly where `TUI / Web UI` was — keep the arrow visually pointing at the same spot. Exact column count: the original starts `TUI` at column 51; the new line should start `Web UI` at column 56 so the arrow lines up. If alignment looks off after the edit, eyeball it and adjust.)

- [ ] **Step 4: Rephrase the `hooks.go` comment (line 76)**

Find:
```
  metadata for standalone lane events (glyph, label, lane, severity) lives in
  `internal/event/hooks.go` so the TUI and web client share rendering decisions.
```

Replace with:
```
  metadata for standalone lane events (glyph, label, lane, severity) lives in
  `internal/event/hooks.go`, consumed by the web client to render each event.
```

- [ ] **Step 5: Confirm no TUI mentions remain**

```bash
grep -n "tui\|TUI" CLAUDE.md
```

Expected: no matches.

- [ ] **Step 6: Commit**

```bash
git add CLAUDE.md
git commit -m "$(cat <<'EOF'
docs(claude.md): drop TUI references; architecture now three roles (#9)
EOF
)"
```

---

## Task 6: End-to-end verification

**Files:** none (verification only)

- [ ] **Step 1: Full Go build + tests**

```bash
go build ./cmd/hv
go test ./...
```

Expected: clean build, all tests PASS, no `internal/tui` in the test output.

- [ ] **Step 2: Full project build (frontend + binary)**

```bash
make build
```

Expected: succeeds. Produces `./hv`. (If the Lit frontend isn't already built in this worktree, this will run Vite. That's fine.)

- [ ] **Step 3: Smoke script**

```bash
./scripts/smoke.sh
```

Expected: passes end-to-end (fires test hook payloads through the binary, asserts events land in JSONL). Uses an isolated temp dir; safe.

- [ ] **Step 4: Manual subcommand sanity**

```bash
./hv help                      # lists hook, daemon, serve — no tui
./hv tui 2>&1; echo "exit=$?"  # prints unknown-command error + usage; exits 2
```

- [ ] **Step 5: Final repo-wide TUI sweep**

```bash
grep -rn "internal/tui\|hv tui\| TUI\|^TUI" \
  --include="*.go" --include="*.md" --include="*.json" --include="*.sh" \
  .
```

Expected: zero matches. (If any remain — e.g. an `hv tui` reference in `scripts/`, `plugin/`, or another doc the spec missed — stop, surface them, decide whether to clean up in this PR or note as follow-up.)

- [ ] **Step 6: No commit for this task** — verification only.

---

## Task 7: Open the PR

**Files:** none

- [ ] **Step 1: Push the branch**

```bash
git push -u origin issue-9-deprecate-tui
```

- [ ] **Step 2: Open the PR with `gh`**

```bash
gh pr create --title "Deprecate hv tui: remove subcommand and internal/tui package (#9)" --body "$(cat <<'EOF'
## Summary
- Deletes `internal/tui/` (~8,700 LOC) and the `hv tui` subcommand
- Drops bubbletea / lipgloss / charmbracelet & friends from `go.mod` via `go mod tidy`
- Updates `README.md` and `CLAUDE.md` to reflect three roles (hook / daemon / serve), web UI as the sole viewer
- No replacement for `--plain`; users can `tail -f` the JSONL or use the web UI

Closes #9. Per spec `docs/superpowers/specs/2026-06-01-deprecate-hv-tui-design.md`.

## Test plan
- [ ] `go build ./cmd/hv` clean
- [ ] `go test ./...` passes
- [ ] `make build` produces a working binary
- [ ] `./scripts/smoke.sh` passes
- [ ] `hv help` lists only hook / daemon / serve
- [ ] `hv tui` prints "unknown command" + usage and exits 2
EOF
)"
```

- [ ] **Step 3: Report the PR URL.**
