// internal/model/target_test.go
package model

import (
	"testing"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

func TestExtractTarget(t *testing.T) {
	cases := []struct {
		name string
		ev   *event.Event
		want string
	}{
		{
			"bash first line only",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Bash", Raw: []byte(`{"tool_input":{"command":"go test ./...\n# second line"}}`)},
			"go test ./...",
		},
		{
			"edit uses file_path",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Edit", Raw: []byte(`{"tool_input":{"file_path":"internal/model/diff.go"}}`)},
			"internal/model/diff.go",
		},
		{
			"read falls back to path",
			&event.Event{HookEvent: "PreToolUse", ToolName: "Read", Raw: []byte(`{"tool_input":{"path":"/tmp/x"}}`)},
			"/tmp/x",
		},
		{
			"user prompt",
			&event.Event{HookEvent: "UserPromptSubmit", Raw: []byte(`{"prompt":"add the api"}`)},
			"add the api",
		},
		{"empty raw", &event.Event{HookEvent: "PreToolUse", ToolName: "Bash"}, ""},
		{"malformed raw", &event.Event{HookEvent: "PreToolUse", ToolName: "Bash", Raw: []byte(`nope`)}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ExtractTarget(c.ev); got != c.want {
				t.Fatalf("ExtractTarget = %q, want %q", got, c.want)
			}
		})
	}
}
