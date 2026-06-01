# hv — Claude Code Harness Visualizer

A local-only, single-user tool for seeing what your Claude Code harness actually does:
which hooks fire, in what order, with what data, and how long tools take. Config is intent;
the captured event stream is reality. This is a reality-viewer.

**Security model:** binds `127.0.0.1` only. No auth, no TLS, no cloud.

## How it works

One binary (`hv`), three roles:

- `hv hook` — hook forwarder; Claude Code runs this per hook event (reads stdin, POSTs to daemon, exits 0 always, <100ms)
- `hv daemon` — HTTP capture server; auto-spawned by the first hook, you don't normally start it
- `hv tui` — terminal viewer (master-detail: sessions → events → inspector)

Events land as JSONL under `$XDG_DATA_HOME/hv/sessions/{session_id}.jsonl`
(override with `HV_DATA_DIR`). Runtime files (port, pid, daemon log) live under
`$XDG_RUNTIME_DIR/hv/` or fall back to the data dir. Default daemon port: **7842**.

## Install

### 1. Build the binary

```bash
git clone <this-repo> ~/workspace/harness-visualizer
cd ~/workspace/harness-visualizer
go build -o plugin/bin/hv ./cmd/hv
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
claude plugin install hv@harness-visualizer --scope user
```

**Verify:**

```bash
claude plugin list
# Should show: hv@harness-visualizer  enabled
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
claude plugin uninstall hv

# Remove the local marketplace registration
claude plugin marketplace remove harness-visualizer

# Remove data (optional — your captured session history)
rm -rf "${HV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/hv}"
```

## Usage

After install, just use Claude Code normally. The daemon starts itself on the first hook
event. To watch events live:

```bash
hv tui
```

Or inspect session files directly:

```bash
ls "${HV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/hv}/sessions/"
cat <session-id>.jsonl | jq .
```

Debug hook forwarding (verbose stderr):

```bash
HV_DEBUG=1 hv hook < /dev/stdin
```

Run daemon in the foreground (dev/debug mode):

```bash
hv daemon --foreground
```

Delete all captured session JSONL files (prompts for confirmation; use `--yes` to skip):

```bash
hv sessions clear
```
