// internal/model/lane.go — per-hook gist extractors and the BuildLaneEvents
// reducer. Each extractor reads structured fields out of Raw defensively and
// returns "" rather than erroring on missing/malformed input, so a wrong
// field-name guess can't break the timeline. Field names match the Claude
// Code hooks documentation as of 2026-06.
package model

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// BuildLaneEvents reduces a stream of events to standalone lane events,
// filtering out anything not in the shared registry (event.Hooks).
func BuildLaneEvents(events []*event.Event) []LaneEvent {
	out := make([]LaneEvent, 0, len(events))
	for _, ev := range events {
		meta, ok := event.Lookup(ev.HookEvent)
		if !ok {
			continue
		}
		out = append(out, LaneEvent{
			ID:        ev.ID,
			HookEvent: ev.HookEvent,
			Lane:      meta.Lane,
			Gist:      laneGist(ev),
			Severity:  meta.Severity,
			Raw:       ev.Raw,
			At:        ev.CapturedAt,
			Seq:       ev.Seq,
		})
	}
	return out
}

// laneGist dispatches to a per-hook extractor. Returns "" when no extractor
// matches or the extractor cannot find usable fields — clients fall back to
// the hook name.
func laneGist(ev *event.Event) string {
	if ev == nil || len(ev.Raw) == 0 {
		return ""
	}
	switch ev.HookEvent {
	case "PermissionRequest", "PermissionDenied":
		return gistPermission(ev)
	case "InstructionsLoaded":
		return gistInstructions(ev.Raw)
	case "ConfigChange":
		return gistConfig(ev.Raw)
	case "CwdChanged":
		return gistCwd(ev.Raw)
	case "TaskCreated":
		return gistTask(ev.Raw, "Create")
	case "TaskCompleted":
		return gistTask(ev.Raw, "Done")
	case "UserPromptExpansion":
		return gistExpansion(ev.Raw)
	case "MessageDisplay":
		return gistMessage(ev.Raw)
	case "WorktreeRemove":
		return gistWorktree(ev.Raw)
	case "StopFailure":
		return gistStopFailure(ev.Raw)
	}
	return ""
}

func gistPermission(ev *event.Event) string {
	var f struct {
		ToolName  string          `json:"tool_name"`
		ToolInput json.RawMessage `json:"tool_input"`
		Reason    string          `json:"reason"`
	}
	if json.Unmarshal(ev.Raw, &f) != nil {
		return ""
	}
	tgt := firstStringField(f.ToolInput)
	parts := []string{}
	if f.ToolName != "" {
		head := f.ToolName
		if tgt != "" {
			head = head + ": " + tgt
		}
		parts = append(parts, head)
	} else if tgt != "" {
		parts = append(parts, tgt)
	}
	if ev.HookEvent == "PermissionDenied" && f.Reason != "" {
		parts = append(parts, "(denied: "+f.Reason+")")
	}
	return strings.Join(parts, " ")
}

func gistInstructions(raw json.RawMessage) string {
	var f struct {
		Path       string `json:"path"`
		MemoryType string `json:"memory_type"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Path == "" {
		return ""
	}
	if f.MemoryType != "" {
		return f.Path + " (" + f.MemoryType + ")"
	}
	return f.Path
}

func gistConfig(raw json.RawMessage) string {
	var f struct {
		Key      string `json:"key"`
		OldValue string `json:"old_value"`
		NewValue string `json:"new_value"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Key == "" || f.NewValue == "" {
		return ""
	}
	if f.OldValue != "" {
		return fmt.Sprintf("%s = %s (was %s)", f.Key, f.NewValue, f.OldValue)
	}
	return fmt.Sprintf("%s = %s", f.Key, f.NewValue)
}

func gistCwd(raw json.RawMessage) string {
	var f struct {
		NewCwd string `json:"new_cwd"`
	}
	if json.Unmarshal(raw, &f) != nil || f.NewCwd == "" {
		return ""
	}
	return "→ " + f.NewCwd
}

func gistTask(raw json.RawMessage, verb string) string {
	var f struct {
		TaskID  string `json:"task_id"`
		Subject string `json:"subject"`
	}
	if json.Unmarshal(raw, &f) != nil || f.TaskID == "" || f.Subject == "" {
		return ""
	}
	return fmt.Sprintf("%s #%s: %s", verb, f.TaskID, f.Subject)
}

func gistExpansion(raw json.RawMessage) string {
	var f struct {
		Original string `json:"original"`
		Expanded string `json:"expanded"`
	}
	if json.Unmarshal(raw, &f) != nil || f.Expanded == "" {
		return ""
	}
	if f.Original != "" {
		return f.Original + " → " + f.Expanded
	}
	return f.Expanded
}

func gistMessage(raw json.RawMessage) string {
	var f struct {
		Text string `json:"text"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	return f.Text
}

func gistWorktree(raw json.RawMessage) string {
	var f struct {
		Name string `json:"name"`
		Path string `json:"path"`
	}
	if json.Unmarshal(raw, &f) != nil {
		return ""
	}
	if f.Name != "" {
		return "removed: " + f.Name
	}
	if f.Path != "" {
		return "removed: " + f.Path
	}
	return ""
}

func gistStopFailure(raw json.RawMessage) string {
	var f struct {
		ErrorType string `json:"error_type"`
		Message   string `json:"message"`
	}
	if json.Unmarshal(raw, &f) != nil || f.ErrorType == "" {
		return ""
	}
	if f.Message != "" {
		return f.ErrorType + ": " + f.Message
	}
	return f.ErrorType
}

// firstStringField returns the first non-empty string value found in a JSON
// object (used to pull a useful target out of tool_input for permission rows
// without needing per-tool knowledge). Returns "" on parse failure or empty
// object. Field iteration order is deterministic via key sort.
func firstStringField(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	// Bias toward common useful field names first.
	preferred := []string{"command", "file_path", "path", "url"}
	for _, k := range preferred {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && s != "" {
				if k == "command" {
					return strings.SplitN(s, "\n", 2)[0]
				}
				return s
			}
		}
	}
	// Fallback: lexicographically first non-empty string.
	sort.Strings(keys)
	for _, k := range keys {
		if s, ok := m[k].(string); ok && s != "" {
			return s
		}
	}
	return ""
}

// LaneGistForTUI returns the per-hook one-line summary for an event. It is
// exported so the TUI's targetGist can reuse the same extractors the web
// timeline does, keeping the one-liners in sync across clients.
func LaneGistForTUI(ev *event.Event) string {
	return laneGist(ev)
}
