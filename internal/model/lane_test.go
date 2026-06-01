package model

import (
	"encoding/json"
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestGistExtractors(t *testing.T) {
	cases := []struct {
		name string
		hook string
		raw  string
		want string
	}{
		{"permission with tool_input", "PermissionRequest",
			`{"tool_name":"Bash","tool_input":{"command":"npm test"}}`,
			"Bash: npm test"},
		{"permission denied with reason", "PermissionDenied",
			`{"tool_name":"Edit","tool_input":{"file_path":"foo.go"},"reason":"rule X"}`,
			"Edit: foo.go (denied: rule X)"},
		{"permission missing fields", "PermissionRequest", `{}`, ""},

		{"instructions with memory_type", "InstructionsLoaded",
			`{"path":"CLAUDE.md","memory_type":"project"}`,
			"CLAUDE.md (project)"},
		{"instructions path only", "InstructionsLoaded",
			`{"path":"AGENTS.md"}`, "AGENTS.md"},
		{"instructions missing", "InstructionsLoaded", `{}`, ""},

		{"config change with old/new", "ConfigChange",
			`{"key":"model","old_value":"opus-4-7","new_value":"sonnet-4-6"}`,
			"model = sonnet-4-6 (was opus-4-7)"},
		{"config change new only", "ConfigChange",
			`{"key":"model","new_value":"sonnet-4-6"}`,
			"model = sonnet-4-6"},
		{"config missing", "ConfigChange", `{}`, ""},

		{"cwd changed", "CwdChanged",
			`{"old_cwd":"/a","new_cwd":"/a/web"}`, "→ /a/web"},
		{"cwd missing", "CwdChanged", `{}`, ""},

		{"task created", "TaskCreated",
			`{"task_id":"42","subject":"Run baseline tests"}`,
			"Create #42: Run baseline tests"},
		{"task completed", "TaskCompleted",
			`{"task_id":"42","subject":"Run baseline tests","status":"completed"}`,
			"Done #42: Run baseline tests"},
		{"task missing", "TaskCreated", `{}`, ""},

		{"expansion", "UserPromptExpansion",
			`{"original":"/loop 5m","expanded":"run baseline tests"}`,
			"/loop 5m → run baseline tests"},
		{"expansion expanded only", "UserPromptExpansion",
			`{"expanded":"hello world"}`, "hello world"},
		{"expansion missing", "UserPromptExpansion", `{}`, ""},

		{"message display", "MessageDisplay",
			`{"text":"Hello, how can I help?"}`,
			"Hello, how can I help?"},
		{"message missing", "MessageDisplay", `{}`, ""},

		{"worktree remove", "WorktreeRemove",
			`{"name":"feature-x","path":"/w/feature-x"}`,
			"removed: feature-x"},
		{"worktree no name", "WorktreeRemove",
			`{"path":"/w/feature-x"}`, "removed: /w/feature-x"},
		{"worktree missing", "WorktreeRemove", `{}`, ""},

		{"stop failure", "StopFailure",
			`{"error_type":"rate_limit","message":"slow down"}`,
			"rate_limit: slow down"},
		{"stop failure type only", "StopFailure",
			`{"error_type":"api_error"}`, "api_error"},
		{"stop failure missing", "StopFailure", `{}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev := &event.Event{HookEvent: tc.hook, Raw: json.RawMessage(tc.raw)}
			got := laneGist(ev)
			if got != tc.want {
				t.Errorf("laneGist = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestBuildLaneEvents(t *testing.T) {
	t0 := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	events := []*event.Event{
		{ID: "a", HookEvent: "PreToolUse", CapturedAt: t0, Seq: 1,
			Raw: json.RawMessage(`{"tool_name":"Bash"}`)},
		{ID: "b", HookEvent: "PermissionRequest", CapturedAt: t0.Add(time.Second), Seq: 2,
			Raw: json.RawMessage(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)},
		{ID: "c", HookEvent: "MessageDisplay", CapturedAt: t0.Add(2 * time.Second), Seq: 3,
			Raw: json.RawMessage(`{"text":"hi"}`)},
	}

	got := BuildLaneEvents(events)

	if len(got) != 2 {
		t.Fatalf("len = %d, want 2 (PreToolUse should be filtered out)", len(got))
	}
	if got[0].HookEvent != "PermissionRequest" {
		t.Errorf("[0].HookEvent = %q, want PermissionRequest", got[0].HookEvent)
	}
	if got[0].Lane != "permission" {
		t.Errorf("[0].Lane = %q, want permission", got[0].Lane)
	}
	if got[0].Severity != "warn" {
		t.Errorf("[0].Severity = %q, want warn", got[0].Severity)
	}
	if got[0].Gist != "Bash: ls" {
		t.Errorf("[0].Gist = %q, want \"Bash: ls\"", got[0].Gist)
	}
	if !got[0].At.Equal(t0.Add(time.Second)) {
		t.Errorf("[0].At = %v, want %v", got[0].At, t0.Add(time.Second))
	}
	if got[0].Seq != 2 {
		t.Errorf("[0].Seq = %d, want 2", got[0].Seq)
	}
}
