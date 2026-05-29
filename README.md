# cchv — Claude Code Harness Visualizer

A local-only, single-user tool for seeing what your Claude Code harness actually does:
which hooks fire, in what order, with what data, and how long tools take. Config is intent;
the captured event stream is reality. This is a reality-viewer.

**Security model:** binds `127.0.0.1` only. No auth, no TLS, no cloud.

## How it works

One binary (`cchv`), three roles:

- `cchv hook` — hook forwarder; Claude Code runs this per hook event (reads stdin, POSTs to daemon, exits 0 always, <100ms)
- `cchv daemon` — HTTP capture server; auto-spawned by the first hook, you don't normally start it
- `cchv tui` — terminal viewer (master-detail: sessions → events → inspector)

Events land as JSONL under `$XDG_DATA_HOME/cchv/sessions/{session_id}.jsonl`
(override with `CCHV_DATA_DIR`). Runtime files (port, pid, daemon log) live under
`$XDG_RUNTIME_DIR/cchv/` or fall back to the data dir. Default daemon port: **7842**.

## Install

### 1. Build the binary

```bash
git clone <this-repo> ~/workspace/cc-harness-visualizer
cd ~/workspace/cc-harness-visualizer
go build -o plugin/bin/cchv ./cmd/cchv
```

### 2. Register as a local Claude Code plugin

The `plugin/` directory is a Claude Code plugin, and the repo root ships a
`.claude-plugin/marketplace.json` so the `claude plugin` CLI can discover it — no manual
file creation needed.

**Add this repo as a local marketplace and install the plugin:**

```bash
# From repo root — adds this directory as a local-scoped marketplace
claude plugin marketplace add "$(pwd)" --scope local

# Install the plugin (user scope = available in all projects)
claude plugin install cchv@cc-harness-visualizer --scope user
```

**Verify:**

```bash
claude plugin list
# Should show: cchv@cc-harness-visualizer  enabled
```

> **verify:** The exact `claude plugin marketplace add <local-path>` behavior was
> confirmed by inspecting the CLI error output, which requires a
> `.claude-plugin/marketplace.json` at the path root. The commands above match that
> requirement and match the structure of existing personal plugin marketplaces on this
> machine. The `--scope local` on marketplace add (not install) limits the marketplace
> registration to this project; use `--scope user` to make it available globally.

### 3. Smoke test (optional but recommended)

```bash
./scripts/smoke.sh
```

Builds the binary, fires test hook payloads, and confirms events land in JSONL.
Uses an isolated temp directory — does not touch real session data.

## Uninstall

```bash
# Remove the plugin
claude plugin uninstall cchv

# Remove the local marketplace registration
claude plugin marketplace remove cc-harness-visualizer

# Remove data (optional — your captured session history)
rm -rf "${CCHV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/cchv}"
```

## Usage

After install, just use Claude Code normally. The daemon starts itself on the first hook
event. To watch events live:

```bash
cchv tui
```

Or inspect session files directly:

```bash
ls "${CCHV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/cchv}/sessions/"
cat <session-id>.jsonl | jq .
```

Debug hook forwarding (verbose stderr):

```bash
CCHV_DEBUG=1 cchv hook < /dev/stdin
```

Run daemon in the foreground (dev/debug mode):

```bash
cchv daemon --foreground
```
