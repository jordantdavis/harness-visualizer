package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// inspectLines renders the inspector body for ev into a slice of display lines,
// each at most w runes wide. It applies syntax-aware rendering for known tool
// shapes (Bash commands, Edit diffs, file paths). Falls back gracefully to
// plain formatted JSON on any parse error — never panics.
func inspectLines(ev *event.Event, w int) []string {
	if len(ev.Raw) == 0 {
		return []string{"(no payload)"}
	}

	// Best-effort parse of the outer shape.
	var outer struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if err := json.Unmarshal(ev.Raw, &outer); err != nil || len(outer.ToolInput) == 0 {
		// Not a tool event or malformed — fall back to flat JSON.
		return formatJSONLines(ev.Raw, w)
	}

	// Delegate to tool-specific renderers; fall back to generic key rendering.
	switch ev.ToolName {
	case "Bash":
		lines := renderBashInput(outer.ToolInput, w)
		if len(lines) > 0 {
			return lines
		}
	case "Edit":
		lines := renderEditInput(outer.ToolInput, w)
		if len(lines) > 0 {
			return lines
		}
	}

	// Generic: iterate JSON object keys with syntax-aware value rendering.
	return renderGenericInput(outer.ToolInput, w)
}

// renderBashInput renders tool_input for Bash events. The command field is
// displayed as a shell block; other keys fall through to generic rendering.
// Returns nil if the input cannot be parsed as a Bash input shape.
func renderBashInput(raw json.RawMessage, w int) []string {
	var inp struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Command == "" {
		return nil
	}
	var lines []string
	lines = append(lines, padRight("command:", w))
	for _, l := range strings.Split(inp.Command, "\n") {
		lines = append(lines, padRight("  $ "+l, w))
	}
	// Append remaining keys via generic renderer.
	lines = append(lines, renderGenericInputExcluding(raw, w, "command")...)
	return lines
}

// renderEditInput renders tool_input for Edit events. old_string / new_string
// are displayed with -/+ framing; other keys use generic rendering.
// Returns nil if the input cannot be parsed as an Edit input shape.
func renderEditInput(raw json.RawMessage, w int) []string {
	var inp struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(raw, &inp); err != nil {
		return nil
	}
	if inp.OldString == "" && inp.NewString == "" {
		return nil
	}
	var lines []string
	if inp.FilePath != "" {
		lines = append(lines, padRight(fmt.Sprintf("  %s %s", pathGlyph, inp.FilePath), w))
	}
	// Before block.
	lines = append(lines, padRight("--- before", w))
	for _, l := range strings.Split(inp.OldString, "\n") {
		lines = append(lines, padRight("- "+l, w))
	}
	// After block.
	lines = append(lines, padRight("+++ after", w))
	for _, l := range strings.Split(inp.NewString, "\n") {
		lines = append(lines, padRight("+ "+l, w))
	}
	// Remaining keys.
	lines = append(lines, renderGenericInputExcluding(raw, w, "file_path", "old_string", "new_string")...)
	return lines
}

// pathGlyph is the marker prepended to file path values.
const pathGlyph = "📄"

// renderGenericInput renders a JSON object's key-value pairs using
// renderValueAware for each string value. Numeric/bool/object values fall back
// to their JSON representation.
func renderGenericInput(raw json.RawMessage, w int) []string {
	return renderGenericInputExcluding(raw, w)
}

// renderGenericInputExcluding renders as renderGenericInput but skips the
// specified keys (already rendered by a tool-specific renderer).
func renderGenericInputExcluding(raw json.RawMessage, w int, skip ...string) []string {
	if len(raw) == 0 {
		return nil
	}
	skipSet := make(map[string]bool, len(skip))
	for _, k := range skip {
		skipSet[k] = true
	}

	// Unmarshal into an ordered-ish map. Go maps don't preserve order, but
	// this is a best-effort display — exact key order is not load-bearing.
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return formatJSONLines(raw, w)
	}

	var lines []string
	for k, v := range m {
		if skipSet[k] {
			continue
		}
		// Attempt to get the string value.
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			// It's a string — use syntax-aware rendering.
			rendered := renderValueAware(k, s)
			lines = append(lines, padRight(fmt.Sprintf("  %s: %s", k, rendered), w))
		} else {
			// Non-string — fall back to compact JSON.
			lines = append(lines, padRight(fmt.Sprintf("  %s: %s", k, string(v)), w))
		}
	}
	return lines
}

// renderValueAware returns a display string for a JSON string value, applying
// light syntax awareness based on the key name and value content:
//
//   - key=="command" or key contains "cmd": rendered as a shell block ($ prefix per line).
//   - key=="file_path" or value looks like a path (starts with /): shown with path glyph.
//   - otherwise: value returned as-is.
//
// Always returns a non-empty string when value is non-empty. Never panics.
func renderValueAware(key, value string) string {
	if value == "" {
		return ""
	}

	// Shell command block: key is "command" or contains "cmd".
	lk := strings.ToLower(key)
	if lk == "command" || strings.Contains(lk, "cmd") {
		var sb strings.Builder
		lines := strings.Split(value, "\n")
		for i, l := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("$ ")
			sb.WriteString(l)
		}
		return sb.String()
	}

	// File path: key is file_path/path, or value starts with '/'.
	if lk == "file_path" || lk == "path" || lk == "filepath" || strings.HasPrefix(value, "/") {
		return pathGlyph + " " + value
	}

	return value
}
