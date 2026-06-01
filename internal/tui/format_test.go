package tui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// --- statusGlyph ---

func TestStatusGlyphShapeDiffers(t *testing.T) {
	// Shapes must differ between OK and Error so color is not required.
	ok := statusGlyph(statusOK, false)
	err := statusGlyph(statusError, false)
	if ok == err {
		t.Errorf("OK and Error glyphs must differ, both are %q", ok)
	}
	okNC := statusGlyph(statusOK, true)
	errNC := statusGlyph(statusError, true)
	if okNC == errNC {
		t.Errorf("noColor: OK and Error glyphs must differ, both are %q", okNC)
	}
}

func TestStatusGlyphWidth(t *testing.T) {
	// All glyphs must render the same rune-count so columns align.
	statuses := []eventStatus{statusOK, statusError, statusRunning, statusNeutral}
	for _, noColor := range []bool{false, true} {
		var first int
		for _, s := range statuses {
			g := statusGlyph(s, noColor)
			w := utf8.RuneCountInString(g)
			if first == 0 {
				first = w
			} else if w != first {
				t.Errorf("noColor=%v: glyph for %d has width %d, want %d", noColor, s, w, first)
			}
		}
	}
}

// --- deriveStatus ---

func TestDeriveStatusPreToolUse(t *testing.T) {
	ev := &event.Event{HookEvent: "PreToolUse"}
	if got := deriveStatus(ev); got != statusRunning {
		t.Errorf("PreToolUse = %v, want statusRunning", got)
	}
}

func TestDeriveStatusPostToolUseExitZero(t *testing.T) {
	raw := json.RawMessage(`{"tool_response":{"exit_code":0}}`)
	ev := &event.Event{HookEvent: "PostToolUse", Raw: raw}
	if got := deriveStatus(ev); got != statusOK {
		t.Errorf("PostToolUse exit 0 = %v, want statusOK", got)
	}
}

func TestDeriveStatusPostToolUseExitNonZero(t *testing.T) {
	raw := json.RawMessage(`{"tool_response":{"exit_code":1}}`)
	ev := &event.Event{HookEvent: "PostToolUse", Raw: raw}
	if got := deriveStatus(ev); got != statusError {
		t.Errorf("PostToolUse exit 1 = %v, want statusError", got)
	}
}

func TestDeriveStatusPostToolUseNoExitCode(t *testing.T) {
	raw := json.RawMessage(`{"tool_response":{}}`)
	ev := &event.Event{HookEvent: "PostToolUse", Raw: raw}
	if got := deriveStatus(ev); got != statusNeutral {
		t.Errorf("PostToolUse no exit code = %v, want statusNeutral", got)
	}
}

func TestDeriveStatusMalformedRaw(t *testing.T) {
	ev := &event.Event{HookEvent: "PostToolUse", Raw: json.RawMessage(`not json`)}
	if got := deriveStatus(ev); got != statusNeutral {
		t.Errorf("malformed raw = %v, want statusNeutral (never crash)", got)
	}
}

func TestDeriveStatusLifecycleEvents(t *testing.T) {
	for _, hook := range []string{"Stop", "SessionStart", "UserPromptSubmit", "Notification"} {
		ev := &event.Event{HookEvent: hook}
		if got := deriveStatus(ev); got != statusNeutral {
			t.Errorf("lifecycle %q = %v, want statusNeutral", hook, got)
		}
	}
}

// --- targetGist ---

func TestTargetGistBashCommand(t *testing.T) {
	ev := &event.Event{
		HookEvent: "PreToolUse",
		ToolName:  "Bash",
		Raw:       json.RawMessage(`{"tool_input":{"command":"git status\ngit log"}}`),
	}
	got := targetGist(ev)
	if got != "git status" {
		t.Errorf("targetGist bash = %q, want %q", got, "git status")
	}
}

func TestTargetGistClipsLongGist(t *testing.T) {
	long := strings.Repeat("a", 60)
	ev := &event.Event{
		HookEvent: "UserPromptSubmit",
		Raw:       json.RawMessage(`{"prompt":"` + long + `"}`),
	}
	got := targetGist(ev)
	if utf8.RuneCountInString(got) > 40 {
		t.Errorf("targetGist too long: %d runes", utf8.RuneCountInString(got))
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clipped gist should end with ellipsis, got %q", got)
	}
}

func TestTargetGistEmptyRaw(t *testing.T) {
	ev := &event.Event{HookEvent: "Stop"}
	if got := targetGist(ev); got != "" {
		t.Errorf("empty raw = %q, want empty string", got)
	}
}

func TestTargetGistMalformedRaw(t *testing.T) {
	ev := &event.Event{HookEvent: "PreToolUse", ToolName: "Bash", Raw: json.RawMessage(`{bad`)}
	// Must not panic; returns "".
	got := targetGist(ev)
	_ = got
}

// --- buildEventRow / renderEventRow ---

func TestBuildEventRowNeverPanics(t *testing.T) {
	evs := []*event.Event{
		{}, // zero value
		{HookEvent: "PreToolUse", ToolName: "Bash", Raw: json.RawMessage(`{}`)},
		{HookEvent: "PostToolUse", Raw: json.RawMessage(`not json`)},
	}
	for _, ev := range evs {
		row := buildEventRow(ev)
		_ = renderEventRow(row, 80, false, false)
		_ = renderEventRow(row, 80, true, true)
		_ = renderEventRow(row, 30, false, false) // very narrow
	}
}

func TestBuildEventRowTimeFormat(t *testing.T) {
	ts, _ := time.Parse(time.RFC3339, "2026-05-28T10:00:01Z")
	ev := &event.Event{CapturedAt: ts, HookEvent: "Stop"}
	row := buildEventRow(ev)
	// Time should be HH:MM:SS (8 chars)
	if len(row.Time) != 8 {
		t.Errorf("time field length = %d, want 8, got %q", len(row.Time), row.Time)
	}
}

// --- clip ---

func TestClipEmpty(t *testing.T) {
	if got := clip("", 10); got != "" {
		t.Errorf("clip empty = %q, want empty", got)
	}
}

func TestClipExact(t *testing.T) {
	if got := clip("hello", 5); got != "hello" {
		t.Errorf("clip exact = %q, want hello", got)
	}
}

func TestClipLong(t *testing.T) {
	got := clip("hello world", 8)
	if utf8.RuneCountInString(got) > 8 {
		t.Errorf("clip: result too long: %q", got)
	}
	if !strings.HasSuffix(got, "…") {
		t.Errorf("clip: should end with ellipsis: %q", got)
	}
}

// --- padRight ---

func TestPadRightPads(t *testing.T) {
	got := padRight("hi", 5)
	if got != "hi   " {
		t.Errorf("padRight = %q, want %q", got, "hi   ")
	}
}

func TestPadRightClips(t *testing.T) {
	got := padRight("hello world", 5)
	if got != "hello" {
		t.Errorf("padRight clips: %q, want %q", got, "hello")
	}
}

// --- sessionLabel ---

func TestSessionLabel(t *testing.T) {
	label := sessionLabel("abc-123", 5, 30)
	if !strings.Contains(label, "abc-123") {
		t.Errorf("sessionLabel missing id: %q", label)
	}
	if !strings.Contains(label, "(5)") {
		t.Errorf("sessionLabel missing count: %q", label)
	}
}

func TestSessionLabelClipsLong(t *testing.T) {
	long := strings.Repeat("x", 50)
	label := sessionLabel(long, 3, 20)
	if utf8.RuneCountInString(label) > 20 {
		t.Errorf("sessionLabel too long: %d runes, %q", utf8.RuneCountInString(label), label)
	}
}

// --- formatDuration ---

func TestFormatDurationZero(t *testing.T) {
	if got := formatDuration(0); got != "" {
		t.Errorf("formatDuration(0) = %q, want empty", got)
	}
}

func TestFormatDurationMillis(t *testing.T) {
	got := formatDuration(250_000_000) // 250ms
	if got != "250ms" {
		t.Errorf("formatDuration(250ms) = %q, want 250ms", got)
	}
}

func TestDeriveStatusPostToolUseFailure(t *testing.T) {
	ev := &event.Event{HookEvent: "PostToolUseFailure"}
	if got := deriveStatus(ev); got != statusError {
		t.Errorf("PostToolUseFailure = %v, want statusError", got)
	}
}

func TestDeriveStatusSubagentStop(t *testing.T) {
	ok := &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{}`)}
	if got := deriveStatus(ok); got != statusOK {
		t.Errorf("SubagentStop ok = %v, want statusOK", got)
	}
	err := &event.Event{HookEvent: "SubagentStop", Raw: []byte(`{"error":"boom"}`)}
	if got := deriveStatus(err); got != statusError {
		t.Errorf("SubagentStop err = %v, want statusError", got)
	}
}

func TestDeriveStatusPostCompact(t *testing.T) {
	ev := &event.Event{HookEvent: "PostCompact", Raw: []byte(`{}`)}
	if got := deriveStatus(ev); got != statusOK {
		t.Errorf("PostCompact = %v, want statusOK", got)
	}
}

func TestDeriveStatusLaneEvents(t *testing.T) {
	cases := []struct {
		hook string
		want eventStatus
	}{
		{"PermissionRequest", statusNeutral}, // warn → neutral (no warn status today)
		{"PermissionDenied", statusError},
		{"InstructionsLoaded", statusNeutral},
		{"StopFailure", statusError},
		{"MessageDisplay", statusNeutral},
		{"UnknownNewHook", statusNeutral},
	}
	for _, tc := range cases {
		t.Run(tc.hook, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(`{}`)}
			if got := deriveStatus(ev); got != tc.want {
				t.Errorf("deriveStatus(%s) = %v, want %v", tc.hook, got, tc.want)
			}
		})
	}
}

func TestTargetGistLaneEvents(t *testing.T) {
	cases := []struct {
		hook string
		raw  string
		want string
	}{
		{"CwdChanged", `{"new_cwd":"/a/web"}`, "→ /a/web"},
		{"PermissionRequest",
			`{"tool_name":"Bash","tool_input":{"command":"npm test"}}`,
			"Bash: npm test"},
		{"InstructionsLoaded", `{"path":"CLAUDE.md","memory_type":"project"}`,
			"CLAUDE.md (project)"},
	}
	for _, tc := range cases {
		t.Run(tc.hook, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(tc.raw)}
			got := targetGist(ev)
			if got != tc.want {
				t.Errorf("targetGist(%s) = %q, want %q", tc.hook, got, tc.want)
			}
		})
	}
}

func TestIsLifecycleHookIncludesMessageDisplay(t *testing.T) {
	if !isLifecycleHook("MessageDisplay") {
		t.Error("MessageDisplay should be lifecycle (dim)")
	}
	if isLifecycleHook("PermissionRequest") {
		t.Error("PermissionRequest should NOT be lifecycle (gets warn glyph)")
	}
}
