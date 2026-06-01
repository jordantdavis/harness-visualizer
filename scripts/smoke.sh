#!/usr/bin/env bash
# smoke.sh — end-to-end plumbing smoke test for hv.
#
# Validates the capture path without a real Claude Code session:
#   build → daemon start → hook invocation → event lands in JSONL.
#
# Usage:
#   ./scripts/smoke.sh
#
# Environment:
#   HV_DATA_DIR is set to a fresh temp dir for the run and cleaned up on exit.
#   No real session data is touched.
#
# What this does NOT cover (must be verified manually):
#   - Plugin installed in Claude Code.
#   - Claude Code actually invokes the hook command.
#   - Events arriving from a live CC session.
#   See the MANUAL VERIFICATION section at the bottom of this file.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN="$REPO_ROOT/plugin/bin/hv"
DATA_DIR="$(mktemp -d)"
DAEMON_PID=""

# Cleanup on exit: kill any daemon we spawned, remove temp dir.
cleanup() {
  if [[ -n "$DAEMON_PID" ]]; then
    kill "$DAEMON_PID" 2>/dev/null || true
  fi
  rm -rf "$DATA_DIR"
}
trap cleanup EXIT

pass() { printf '\033[32mPASS\033[0m  %s\n' "$1"; }
fail() { printf '\033[31mFAIL\033[0m  %s\n' "$1"; exit 1; }

# ---------------------------------------------------------------------------
# Step 1 — Build
# ---------------------------------------------------------------------------
printf '==> building plugin/bin/hv\n'
go build -o "$BIN" "$REPO_ROOT/cmd/hv" || fail "go build failed"
pass "build"

# ---------------------------------------------------------------------------
# Step 2 — Start daemon explicitly on a random port in the temp data dir.
#           An explicit start isolates the test from any daemon already
#           running on the default port (7842) from a live Claude Code session.
# ---------------------------------------------------------------------------
printf '==> starting daemon (foreground, port 0)\n'
export HV_DATA_DIR="$DATA_DIR"

# Spawn daemon in background; capture its PID for cleanup.
"$BIN" daemon --foreground --port 0 >"$DATA_DIR/daemon.log" 2>&1 &
DAEMON_PID=$!

# Wait for the daemon to write its port file (up to 3 s).
PORT_FILE="$DATA_DIR/daemon.port"
for i in $(seq 1 30); do
  if [[ -s "$PORT_FILE" ]]; then
    break
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || fail "daemon did not write port file within 3s"
DAEMON_PORT="$(cat "$PORT_FILE")"
pass "daemon started on port $DAEMON_PORT"

# ---------------------------------------------------------------------------
# Step 3 — First hook invocation (PreToolUse)
# ---------------------------------------------------------------------------
printf '==> sending hook payload 1 (PreToolUse)\n'

printf '{"hook_event_name":"PreToolUse","session_id":"smoke-test","tool_name":"Bash","cwd":"/tmp"}' \
  | "$BIN" hook

sleep 0.3
pass "hook 1 (PreToolUse posted)"

# ---------------------------------------------------------------------------
# Step 4 — Second hook invocation (PostToolUse)
# ---------------------------------------------------------------------------
printf '==> sending hook payload 2 (event should land in JSONL)\n'

printf '{"hook_event_name":"PostToolUse","session_id":"smoke-test","tool_name":"Bash","cwd":"/tmp"}' \
  | "$BIN" hook

sleep 0.3
pass "hook 2 (PostToolUse posted)"

# ---------------------------------------------------------------------------
# Step 4b — Third hook invocation: PostToolUseFailure (Tier 1 hook)
# ---------------------------------------------------------------------------
printf '==> sending hook payload 3 (PostToolUseFailure)\n'

printf '{"hook_event_name":"PostToolUseFailure","session_id":"smoke-test","tool_name":"Bash","cwd":"/tmp","tool_use_id":"smoke-uid","error":"smoke failure"}' \
  | "$BIN" hook

sleep 0.3
pass "hook 3 (PostToolUseFailure posted)"

# ---------------------------------------------------------------------------
# Step 5 — Assert JSONL exists and contains the events
# ---------------------------------------------------------------------------
SESSION_FILE="$DATA_DIR/sessions/smoke-test.jsonl"

[[ -f "$SESSION_FILE" ]] || fail "session file not found: $SESSION_FILE"
pass "session file exists"

grep -q '"session_id":"smoke-test"' "$SESSION_FILE" \
  || fail "session_id not found in JSONL"
pass "session_id present in JSONL"

grep -q '"hook_event":"PostToolUse"' "$SESSION_FILE" \
  || fail "hook_event PostToolUse not found in JSONL"
pass "hook_event captured in JSONL"

grep -q '"tool_name":"Bash"' "$SESSION_FILE" \
  || fail "tool_name not found in JSONL"
pass "tool_name captured in JSONL"

grep -q '"hook_event":"PostToolUseFailure"' "$SESSION_FILE" \
  || fail "hook_event PostToolUseFailure not found in JSONL"
pass "PostToolUseFailure captured in JSONL"

# ---------------------------------------------------------------------------
# Done
# ---------------------------------------------------------------------------
printf '\n\033[32m==> smoke test PASSED\033[0m\n\n'

cat <<'MANUAL'
MANUAL VERIFICATION (cannot be automated here)
-----------------------------------------------
1. Build and install the plugin (see README Install section).
2. Open a new Claude Code session in any project.
3. Trigger at least one tool use (e.g. ask Claude to run `ls`).
4. In another terminal:
     hv tui
   You should see the session appear with PreToolUse / PostToolUse events.
5. Or check the JSONL directly:
     ls "${HV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/hv}/sessions/"
     cat <session-id>.jsonl | jq .
MANUAL
