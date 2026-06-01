package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// fixedNow is a deterministic timestamp used across plain-mode tests.
var fixedNow = time.Date(2026, 5, 28, 10, 0, 1, 0, time.UTC)

// plainEvent builds a minimal event for plain-mode tests.
func plainEvent(hook, tool string, raw string) *event.Event {
	return &event.Event{
		ID:         "test-id",
		Seq:        1,
		HookEvent:  hook,
		SessionID:  "sess-plain",
		ToolName:   tool,
		CapturedAt: fixedNow,
		Raw:        json.RawMessage(raw),
	}
}

// TestPlainLineOK asserts exact output for a PostToolUse success.
func TestPlainLineOK(t *testing.T) {
	ev := plainEvent("PostToolUse", "Bash",
		`{"tool_response":{"exit_code":0},"tool_input":{"command":"ls"}}`)
	got := plainLine(ev)
	want := "10:00:01 [OK]  PostToolUse     Bash         ls"
	if got != want {
		t.Errorf("plainLine OK:\n got  %q\n want %q", got, want)
	}
}

// TestPlainLineErr asserts exact output for a PostToolUse failure.
func TestPlainLineErr(t *testing.T) {
	ev := plainEvent("PostToolUse", "Bash",
		`{"tool_response":{"exit_code":1},"tool_input":{"command":"bad cmd"}}`)
	got := plainLine(ev)
	want := "10:00:01 [ERR] PostToolUse     Bash         bad cmd"
	if got != want {
		t.Errorf("plainLine ERR:\n got  %q\n want %q", got, want)
	}
}

// TestPlainLineRun asserts exact output for a PreToolUse (running).
func TestPlainLineRun(t *testing.T) {
	ev := plainEvent("PreToolUse", "Bash",
		`{"tool_input":{"command":"git status"}}`)
	got := plainLine(ev)
	want := "10:00:01 [RUN] PreToolUse      Bash         git status"
	if got != want {
		t.Errorf("plainLine RUN:\n got  %q\n want %q", got, want)
	}
}

// TestPlainLineNeutral asserts neutral lifecycle events use [-- ] (5-char tag).
// Trailing whitespace is stripped; hook is always present.
func TestPlainLineNeutral(t *testing.T) {
	ev := plainEvent("SessionStart", "", `{}`)
	got := plainLine(ev)
	// Trailing spaces are trimmed; verify time + tag + hook present.
	want := "10:00:01 [-- ] SessionStart"
	if !strings.HasPrefix(got, want) {
		t.Errorf("plainLine neutral:\n got  %q\n want prefix %q", got, want)
	}
}

// TestPlainLineNoTool asserts empty tool name renders as blank (no column noise).
func TestPlainLineNoTool(t *testing.T) {
	ev := plainEvent("UserPromptSubmit", "", `{"prompt":"hello world"}`)
	got := plainLine(ev)
	if strings.Contains(got, "  ") && !strings.HasPrefix(got, "10:00:01") {
		t.Errorf("plainLine UserPromptSubmit should start with time, got %q", got)
	}
	// Must not contain ANSI escapes.
	if strings.Contains(got, "\x1b[") {
		t.Errorf("plainLine must not contain ANSI escapes, got %q", got)
	}
}

// TestPlainLineNoANSI confirms no color escape sequences in any status.
func TestPlainLineNoANSI(t *testing.T) {
	cases := []*event.Event{
		plainEvent("PostToolUse", "Bash", `{"tool_response":{"exit_code":0}}`),
		plainEvent("PostToolUse", "Bash", `{"tool_response":{"exit_code":1}}`),
		plainEvent("PreToolUse", "Read", `{}`),
		plainEvent("SessionStart", "", `{}`),
	}
	for _, ev := range cases {
		got := plainLine(ev)
		if strings.Contains(got, "\x1b") {
			t.Errorf("plainLine must not contain ANSI escapes for %q, got %q",
				ev.HookEvent, got)
		}
	}
}

// TestPlainLineMalformedRaw confirms no panic and a deterministic fallback.
func TestPlainLineMalformedRaw(t *testing.T) {
	ev := plainEvent("PostToolUse", "Bash", `not json`)
	got := plainLine(ev)
	// Must not panic; must contain time and hook.
	if !strings.Contains(got, "10:00:01") {
		t.Errorf("malformed raw: expected time in output, got %q", got)
	}
	if !strings.Contains(got, "PostToolUse") {
		t.Errorf("malformed raw: expected hook in output, got %q", got)
	}
}

// TestRunPlainHistoricalEvents verifies runPlain prints the session header and
// all historical events from the auto-selected session, then returns when the
// context is cancelled.
func TestRunPlainHistoricalEvents(t *testing.T) {
	evs := []*event.Event{
		{
			ID: "e1", Seq: 1, HookEvent: "PreToolUse", SessionID: "sess-1",
			ToolName: "Bash", CapturedAt: fixedNow,
			Raw: json.RawMessage(`{"tool_input":{"command":"ls"}}`),
		},
		{
			ID: "e2", Seq: 2, HookEvent: "PostToolUse", SessionID: "sess-1",
			ToolName: "Bash", CapturedAt: fixedNow,
			Raw: json.RawMessage(`{"tool_response":{"exit_code":0},"tool_input":{"command":"ls"}}`),
		},
	}
	fake := &FakeClient{
		Sessions_: []store.SessionInfo{{ID: "sess-1", EventCount: 2}},
		Events_:   map[string][]*event.Event{"sess-1": evs},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel so runPlain exits after history without blocking on stream

	var buf bytes.Buffer
	if err := runPlain(ctx, fake, &buf); err != nil {
		t.Fatalf("runPlain error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "# session: sess-1") {
		t.Errorf("expected session header in plain output, got:\n%s", out)
	}
	if !strings.Contains(out, "[RUN]") {
		t.Errorf("expected [RUN] tag for PreToolUse, got:\n%s", out)
	}
	if !strings.Contains(out, "[OK]") {
		t.Errorf("expected [OK] tag for PostToolUse exit 0, got:\n%s", out)
	}
}

// TestRunPlainNoSessions verifies runPlain emits a helpful message when no
// sessions exist, without error.
func TestRunPlainNoSessions(t *testing.T) {
	fake := &FakeClient{
		Sessions_: nil,
		Events_:   map[string][]*event.Event{},
	}
	ctx := context.Background()
	var buf bytes.Buffer
	if err := runPlain(ctx, fake, &buf); err != nil {
		t.Fatalf("runPlain no-sessions error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "no sessions") {
		t.Errorf("expected hint message for no-sessions case, got:\n%s", out)
	}
}
