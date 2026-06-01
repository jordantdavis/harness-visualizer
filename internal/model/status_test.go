// internal/model/status_test.go
package model

import (
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func TestDeriveStatus(t *testing.T) {
	cases := []struct {
		name string
		ev   *event.Event
		want Status
	}{
		{"pre is running", &event.Event{HookEvent: "PreToolUse"}, StatusRunning},
		{"post exit 0 is success", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{"exit_code":0}}`)}, StatusSuccess},
		{"post exit 1 is error", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{"exit_code":2}}`)}, StatusError},
		{"post without exit code is neutral", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`{"tool_response":{}}`)}, StatusNeutral},
		{"lifecycle is neutral", &event.Event{HookEvent: "SessionStart"}, StatusNeutral},
		{"malformed raw is neutral", &event.Event{HookEvent: "PostToolUse", Raw: []byte(`not json`)}, StatusNeutral},
		{"post failure is error", &event.Event{HookEvent: "PostToolUseFailure"}, StatusError},
		{"post failure ignores raw", &event.Event{HookEvent: "PostToolUseFailure", Raw: []byte(`{"tool_response":{"exit_code":0}}`)}, StatusError},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DeriveStatus(c.ev); got != c.want {
				t.Fatalf("DeriveStatus = %q, want %q", got, c.want)
			}
		})
	}
}
