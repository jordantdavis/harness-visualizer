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
printf '==> starting daemon (hv daemon start, port 0)\n'
export HV_DATA_DIR="$DATA_DIR"

# Spawn daemon in background; capture its PID for cleanup.
"$BIN" daemon start --port 0 >"$DATA_DIR/daemon.log" 2>&1 &
DAEMON_PID=$!

# Wait for the daemon to write its port file (up to 3 s).
PORT_FILE="$DATA_DIR/daemon.port"
PID_FILE="$DATA_DIR/daemon.pid"
for i in $(seq 1 30); do
  if [[ -s "$PORT_FILE" ]]; then
    break
  fi
  sleep 0.1
done
[[ -s "$PORT_FILE" ]] || fail "daemon did not write port file within 3s"
DAEMON_PORT="$(cat "$PORT_FILE")"
pass "daemon started on port $DAEMON_PORT"

# hv daemon status should report healthy (exit 0).
if "$BIN" daemon status >/dev/null 2>&1; then
  pass "daemon status reports running (exit 0)"
else
  fail "daemon status exited non-zero while daemon is up"
fi

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
# Step 4c — PR 3 lane-event hooks (one sample payload each)
# ---------------------------------------------------------------------------
printf '==> sending lane-event hook payloads (10 events)\n'

printf '{"hook_event_name":"PermissionRequest","session_id":"smoke-test","tool_name":"Bash","tool_input":{"command":"echo hi"}}' \
  | "$BIN" hook

printf '{"hook_event_name":"PermissionDenied","session_id":"smoke-test","tool_name":"Edit","tool_input":{"file_path":"foo.go"},"reason":"sandbox"}' \
  | "$BIN" hook

printf '{"hook_event_name":"InstructionsLoaded","session_id":"smoke-test","path":"CLAUDE.md","memory_type":"project"}' \
  | "$BIN" hook

printf '{"hook_event_name":"ConfigChange","session_id":"smoke-test","key":"model","old_value":"opus","new_value":"sonnet"}' \
  | "$BIN" hook

printf '{"hook_event_name":"CwdChanged","session_id":"smoke-test","old_cwd":"/a","new_cwd":"/a/web"}' \
  | "$BIN" hook

printf '{"hook_event_name":"TaskCreated","session_id":"smoke-test","task_id":"42","subject":"Run baseline"}' \
  | "$BIN" hook

printf '{"hook_event_name":"TaskCompleted","session_id":"smoke-test","task_id":"42","subject":"Run baseline","status":"completed"}' \
  | "$BIN" hook

printf '{"hook_event_name":"UserPromptExpansion","session_id":"smoke-test","original":"/loop","expanded":"run baseline"}' \
  | "$BIN" hook

printf '{"hook_event_name":"WorktreeRemove","session_id":"smoke-test","name":"feature-x","path":"/w/feature-x"}' \
  | "$BIN" hook

printf '{"hook_event_name":"StopFailure","session_id":"smoke-test","error_type":"rate_limit","message":"slow down"}' \
  | "$BIN" hook

sleep 0.5
pass "hooks 4-13 (10 lane-event payloads posted)"

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

for hook in \
  PermissionRequest PermissionDenied InstructionsLoaded ConfigChange CwdChanged \
  TaskCreated TaskCompleted UserPromptExpansion WorktreeRemove StopFailure
do
  grep -q "\"hook_event\":\"$hook\"" "$SESSION_FILE" \
    || fail "lane event $hook not found in JSONL"
  pass "lane event $hook captured in JSONL"
done

# ---------------------------------------------------------------------------
# Step 5b — DELETE /api/sessions/{id} (web UI delete, backed by the daemon)
# ---------------------------------------------------------------------------
printf '==> testing: DELETE /api/sessions/{id}\n'

# Capture a second, independent session we can delete without disturbing the
# primary smoke-test session that later steps assert on.
printf '{"hook_event_name":"SessionStart","session_id":"smoke-del","cwd":"/tmp"}' \
  | "$BIN" hook
sleep 0.3

DEL_FILE="$DATA_DIR/sessions/smoke-del.jsonl"
[[ -f "$DEL_FILE" ]] || fail "second session file not created: $DEL_FILE"
pass "second session (smoke-del) captured"

API="http://127.0.0.1:$DAEMON_PORT/api/sessions"

# DELETE the second session; expect 204 and the file gone.
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$API/smoke-del")"
[[ "$CODE" == "204" ]] || fail "DELETE smoke-del returned $CODE (want 204)"
[[ ! -f "$DEL_FILE" ]] || fail "smoke-del.jsonl still present after DELETE"
[[ -f "$SESSION_FILE" ]] || fail "DELETE smoke-del also removed the primary session file"
pass "DELETE removed smoke-del and left smoke-test intact"

# Deleting a now-missing id is idempotent (still 204, never 500).
CODE="$(curl -s -o /dev/null -w '%{http_code}' -X DELETE "$API/smoke-del")"
[[ "$CODE" == "204" ]] || fail "idempotent DELETE returned $CODE (want 204)"
pass "DELETE of a missing session is idempotent (204)"

# ---------------------------------------------------------------------------
# Step 6 — sessions clear
# ---------------------------------------------------------------------------
printf '==> testing: hv sessions clear\n'

# Confirm the session JSONL from earlier steps exists.
[[ -f "$SESSION_FILE" ]] || fail "session file missing before sessions clear test"
pass "session file present before clear"

# Place a decoy non-jsonl file that must survive deletion.
DECOY="$DATA_DIR/sessions/keep.txt"
printf 'do not delete me\n' > "$DECOY"

# dry-run: lists the file, deletes nothing.
DRY_OUT="$("$BIN" sessions clear --dry-run)"
echo "$DRY_OUT" | grep -q "smoke-test.jsonl" \
  || fail "dry-run output did not list smoke-test.jsonl"
[[ -f "$SESSION_FILE" ]] || fail "dry-run deleted the session file (it should not)"
pass "sessions clear --dry-run lists file without deleting"

# --yes: deletes the JSONL, leaves the decoy.
"$BIN" sessions clear --yes
EXIT_CODE=$?
[[ $EXIT_CODE -eq 0 ]] || fail "sessions clear --yes exited $EXIT_CODE (want 0)"
[[ ! -f "$SESSION_FILE" ]] || fail "session file still exists after sessions clear --yes"
[[ -f "$DECOY" ]] || fail "keep.txt was deleted by sessions clear (it should not be)"
pass "sessions clear --yes removed JSONL and left non-jsonl decoy intact"

# ---------------------------------------------------------------------------
# Step 7 — daemon stop
# ---------------------------------------------------------------------------
printf '==> testing: hv daemon stop\n'

"$BIN" daemon stop
STOP_CODE=$?
[[ $STOP_CODE -eq 0 ]] || fail "daemon stop exited $STOP_CODE (want 0)"
DAEMON_PID="" # stopped cleanly; nothing left for cleanup to kill

# Runtime files must be gone after a clean stop.
[[ ! -f "$PID_FILE" ]] || fail "daemon.pid still exists after stop"
[[ ! -f "$PORT_FILE" ]] || fail "daemon.port still exists after stop"
pass "daemon stop removed pid/port files"

# status must now report stopped (non-zero exit).
if "$BIN" daemon status >/dev/null 2>&1; then
  fail "daemon status reported running after stop"
fi
pass "daemon status reports stopped (non-zero exit)"

# A second stop must hard-refuse (exit 1).
if "$BIN" daemon stop >/dev/null 2>&1; then
  fail "daemon stop on a dead daemon should exit non-zero"
fi
pass "daemon stop hard-refuses when nothing is running"

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
     hv serve
   You should see the session appear with PreToolUse / PostToolUse events.
5. Or check the JSONL directly:
     ls "${HV_DATA_DIR:-${XDG_DATA_HOME:-$HOME/.local/share}/hv}/sessions/"
     cat <session-id>.jsonl | jq .
MANUAL
