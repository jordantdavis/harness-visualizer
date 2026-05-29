package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// eventStatus is the derived status of an event row.
type eventStatus int

const (
	statusOK      eventStatus = iota // tool exited 0 / non-error lifecycle event
	statusError                      // tool exited non-0 / error field present
	statusRunning                    // PreToolUse without a paired Post
	statusNeutral                    // lifecycle events that aren't pass/fail
)

// statusGlyph returns the fixed-width glyph + text tag for noColor rendering.
// Width is always 3: glyph + space + one-char discriminator, so columns align.
// Design: shape differs for OK vs Error — never only color.
//
//	OK:      ✔
//	Error:   ✘
//	Running: ▶
//	Neutral: ·
func statusGlyph(s eventStatus, noColor bool) string {
	if noColor {
		switch s {
		case statusOK:
			return "[✔]"
		case statusError:
			return "[✘]"
		case statusRunning:
			return "[▶]"
		default:
			return "[ ]"
		}
	}
	switch s {
	case statusOK:
		return " ✔ "
	case statusError:
		return " ✘ "
	case statusRunning:
		return " ▶ "
	default:
		return " · "
	}
}

// deriveStatus inspects the event's promoted fields and Raw to determine status.
// Defensive: any parse failure yields statusNeutral.
func deriveStatus(ev *event.Event) eventStatus {
	hook := ev.HookEvent
	switch hook {
	case "PreToolUse":
		return statusRunning
	case "PostToolUse":
		return derivePostStatus(ev.Raw)
	case "Stop", "SubagentStop", "SessionEnd":
		return statusNeutral
	case "SessionStart", "UserPromptSubmit", "Notification", "PreCompact":
		return statusNeutral
	default:
		return statusNeutral
	}
}

// derivePostStatus reads exit_code from tool_response in Raw.
// Returns statusOK for 0, statusError for non-0, statusNeutral if absent.
func derivePostStatus(raw json.RawMessage) eventStatus {
	if len(raw) == 0 {
		return statusNeutral
	}
	var wrapper struct {
		ToolResponse struct {
			ExitCode *int `json:"exit_code"`
		} `json:"tool_response"`
	}
	if err := json.Unmarshal(raw, &wrapper); err != nil {
		return statusNeutral
	}
	if wrapper.ToolResponse.ExitCode == nil {
		return statusNeutral
	}
	if *wrapper.ToolResponse.ExitCode == 0 {
		return statusOK
	}
	return statusError
}

// targetGist extracts a short human-readable description of what the event
// targeted. It reads from Raw defensively; returns "" when nothing useful is found.
// Max length: 40 runes (clipped with "…" if longer).
func targetGist(ev *event.Event) string {
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
	var gist string
	switch ev.HookEvent {
	case "PreToolUse", "PostToolUse":
		gist = toolInputGist(ev.ToolName, fields.ToolInput)
	case "UserPromptSubmit":
		gist = fields.Prompt
	case "Notification":
		gist = fields.Notification
	}
	return clip(gist, 40)
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
			// First line only.
			line := strings.SplitN(inp.Command, "\n", 2)[0]
			return line
		}
	case "Read", "Write", "Edit":
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
		// Generic: return the first string value found.
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

// clip truncates s to maxRunes, appending "…" if clipped. Safe on empty input.
func clip(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-1]) + "…"
}

// eventRow is the pre-formatted data for one event row in the Events pane.
type eventRow struct {
	Time     string // HH:MM:SS
	Status   eventStatus
	Hook     string // e.g. "PreToolUse"
	Tool     string // ev.ToolName, or ""
	Gist     string // targetGist(ev)
	Duration string // blank in Phase 5 (pairing is Phase 7)
	EventID  string // ev.ID (for selection/lookup)
}

// buildEventRow derives an eventRow from an event. Never panics on malformed data.
func buildEventRow(ev *event.Event) eventRow {
	timeStr := ev.CapturedAt.Local().Format("15:04:05")
	return eventRow{
		Time:    timeStr,
		Status:  deriveStatus(ev),
		Hook:    ev.HookEvent,
		Tool:    ev.ToolName,
		Gist:    targetGist(ev),
		EventID: ev.ID,
	}
}

// buildDisplayEventRow derives an eventRow from a displayRow (Phase 7).
//
// For a folded paired op it derives the hook label, status, and duration from
// the pair — the status comes from the Post, the duration from Post−Pre.
// For a standalone row it delegates to buildEventRow.
func buildDisplayEventRow(dr displayRow) eventRow {
	if !dr.IsPair {
		return buildEventRow(dr.Pre)
	}
	// Folded paired op: use Pre for time/hook/tool/gist, Post for status.
	row := buildEventRow(dr.Pre)
	row.Status = dr.EffectiveStatus()
	row.Duration = formatDuration(dr.Duration)
	return row
}

// renderEventRow formats an eventRow into a fixed-width string for the given
// available width. Columns: time(8) sp status(3) sp hook(16) sp tool(12) sp
// gist(flex) sp duration(right-aligned, 7). Duration column is omitted when blank.
// Minimum sensible width is ~50 chars; narrower just shows time+status+hook.
func renderEventRow(r eventRow, width int, noColor bool, selected bool) string {
	const (
		colTime   = 8
		colStatus = 3
		colHook   = 16
		colTool   = 12
		colDur    = 7 // e.g. "1234ms" or "12.3s"
		sep       = " "
	)
	prefix := ""
	if selected {
		prefix = "> "
	}
	usedFixed := colTime + 1 + colStatus + 1 + colHook + 1 + colTool + 1
	contentW := width - len(prefix)

	// Reserve space for duration column when we have a duration to show.
	durStr := r.Duration
	durW := 0
	if durStr != "" && contentW > usedFixed+colDur+1 {
		durW = colDur + 1 // +1 for sep before duration
	} else {
		durStr = ""
	}

	gistW := contentW - usedFixed - durW
	parts := []string{
		padRight(r.Time, colTime),
		statusGlyph(r.Status, noColor),
		padRight(abbreviate(r.Hook, colHook), colHook),
		padRight(r.Tool, colTool),
	}
	if gistW > 0 && r.Gist != "" {
		gist := clip(r.Gist, gistW)
		if durStr != "" {
			// Pad gist to fill its column so duration is right-aligned.
			gist = padRight(gist, gistW)
		}
		parts = append(parts, gist)
	} else if durStr != "" && gistW > 0 {
		// No gist but we have duration: fill gist space with blanks.
		parts = append(parts, strings.Repeat(" ", gistW))
	}
	if durStr != "" {
		parts = append(parts, padRight(durStr, colDur))
	}

	row := prefix + strings.Join(parts, sep)
	return row
}

// renderEventRowHeader renders the column header for the events pane.
func renderEventRowHeader(width int) string {
	const (
		colTime   = 8
		colStatus = 3
		colHook   = 16
		colTool   = 12
		sep       = " "
	)
	parts := []string{
		padRight("TIME", colTime),
		padRight("ST", colStatus),
		padRight("HOOK", colHook),
		padRight("TOOL", colTool),
	}
	used := colTime + 1 + colStatus + 1 + colHook + 1 + colTool + 1
	if width-used > 0 {
		parts = append(parts, "TARGET")
	}
	return strings.Join(parts, sep)
}

// padRight pads or clips s to exactly n bytes (ASCII). For display columns.
func padRight(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// abbreviate clips hook event names to n runes, stripping common prefixes
// ("PreToolUse" → "PreToolUse", "UserPromptSubmit" → "UserPromptSub").
func abbreviate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// formatDuration renders a duration for the duration column. Returns "" for
// zero (Phase 5: pairing not yet implemented).
func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// sessionLabel builds the single-line label shown in the Sessions pane.
// Format: "session-id (N events)". Clips session-id to fit in w runes.
func sessionLabel(id string, count int64, w int) string {
	suffix := fmt.Sprintf(" (%d)", count)
	avail := w - len(suffix)
	if avail < 4 {
		avail = 4
	}
	return clip(id, avail) + suffix
}
