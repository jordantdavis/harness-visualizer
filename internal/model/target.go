// internal/model/target.go
package model

import (
	"encoding/json"
	"strings"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// ExtractTarget returns a short, human-readable description of what an event
// targeted: a file path, a command's first line, or a prompt. Returns "" when
// nothing useful is found. Unlike the TUI, it does not truncate — the client
// elides for display.
func ExtractTarget(ev *event.Event) string {
	if len(ev.Raw) == 0 {
		return ""
	}
	var fields struct {
		ToolInput    json.RawMessage `json:"tool_input"`
		Prompt       string          `json:"prompt"`
		Notification string          `json:"notification"`
	}
	if err := json.Unmarshal(ev.Raw, &fields); err != nil {
		return ""
	}
	switch ev.HookEvent {
	case "PreToolUse", "PostToolUse":
		return toolInputGist(ev.ToolName, fields.ToolInput)
	case "UserPromptSubmit":
		return strings.TrimSpace(fields.Prompt)
	case "Notification":
		return strings.TrimSpace(fields.Notification)
	}
	return ""
}

// toolInputGist extracts a short string from tool_input for known tools.
func toolInputGist(tool string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	switch tool {
	case "Bash":
		var inp struct {
			Command string `json:"command"`
		}
		if err := json.Unmarshal(raw, &inp); err == nil && inp.Command != "" {
			return strings.SplitN(inp.Command, "\n", 2)[0]
		}
	case "Read", "Write", "Edit", "MultiEdit":
		var inp struct {
			FilePath string `json:"file_path"`
			Path     string `json:"path"`
		}
		if err := json.Unmarshal(raw, &inp); err == nil {
			if inp.FilePath != "" {
				return inp.FilePath
			}
			return inp.Path
		}
	default:
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err == nil {
			for _, v := range m {
				if s, ok := v.(string); ok && s != "" {
					return s
				}
			}
		}
	}
	return ""
}
