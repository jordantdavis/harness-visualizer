// internal/model/status.go

// Package model holds the harness-agnostic domain types and derivation logic
// for the visualizer: tool operations, diffs, and the interleaved timeline.
// It is the single source of truth shared by the HTTP API and (eventually) the
// TUI. It depends only on internal/event and the standard library.
package model

import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// Status is the derived lifecycle/result state of an operation, as a stable
// JSON string. Glyph/colour rendering is a presentation concern and lives in
// the client, not here.
type Status string

const (
	StatusRunning Status = "running" // PreToolUse without a paired Post
	StatusSuccess Status = "success" // tool exited 0
	StatusError   Status = "error"   // tool exited non-0
	StatusNeutral Status = "neutral" // unknown / not pass-fail
)

// DeriveStatus inspects a single event to derive its lifecycle status.
//   PreToolUse                       → StatusRunning
//   PostToolUse                      → from tool_response.exit_code
//   PostToolUseFailure               → StatusError
//   SubagentStop                     → StatusError if .error present, else StatusSuccess
//   PostCompact                      → StatusSuccess
//   anything else                    → StatusNeutral
//
// Parse failures fall through to StatusNeutral.
func DeriveStatus(ev *event.Event) Status {
	switch ev.HookEvent {
	case "PreToolUse":
		return StatusRunning
	case "PostToolUse":
		return postStatus(ev.Raw)
	case "PostToolUseFailure":
		return StatusError
	case "SubagentStop":
		if hooks.SubagentHasError(ev.Raw) {
			return StatusError
		}
		return StatusSuccess
	case "PostCompact":
		return StatusSuccess
	default:
		return StatusNeutral
	}
}

// postStatus reads exit_code from tool_response in raw. 0 -> success, non-0 ->
// error, absent/malformed -> neutral.
func postStatus(raw json.RawMessage) Status {
	if len(raw) == 0 {
		return StatusNeutral
	}
	var w struct {
		ToolResponse struct {
			ExitCode *int `json:"exit_code"`
		} `json:"tool_response"`
	}
	if err := json.Unmarshal(raw, &w); err != nil || w.ToolResponse.ExitCode == nil {
		return StatusNeutral
	}
	if *w.ToolResponse.ExitCode == 0 {
		return StatusSuccess
	}
	return StatusError
}
