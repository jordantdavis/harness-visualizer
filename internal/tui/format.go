package tui

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"jordandavis.dev/harness-visualizer/internal/event"
	hvmodel "jordandavis.dev/harness-visualizer/internal/model"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
	"jordandavis.dev/harness-visualizer/internal/store"
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

// statusGlyphStyled returns the status glyph with semantic color applied.
// Under noColor it returns the plain tag form.
func statusGlyphStyled(s eventStatus, noColor bool, tok tokens) string {
	if noColor {
		return statusGlyph(s, true)
	}
	switch s {
	case statusOK:
		return tok.success.Render(" ✔ ")
	case statusError:
		return tok.failure.Render(" ✘ ")
	case statusRunning:
		return tok.running.Render(" ▶ ")
	default:
		return tok.muted.Render(" · ")
	}
}

// deriveStatus inspects the event's promoted fields and Raw to determine status.
// Defensive: nil ev yields statusNeutral; any parse failure yields statusNeutral.
func deriveStatus(ev *event.Event) eventStatus {
	if ev == nil {
		return statusNeutral
	}
	switch ev.HookEvent {
	case "PreToolUse":
		return statusRunning
	case "PostToolUse":
		return derivePostStatus(ev.Raw)
	case "PostToolUseFailure":
		return statusError
	case "SubagentStop":
		if hooks.SubagentHasError(ev.Raw) {
			return statusError
		}
		return statusOK
	case "PostCompact":
		return statusOK
	case "PermissionDenied", "StopFailure":
		return statusError
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
// The result is capped at 40 runes for list rendering; the inspector shows the
// full value via the raw payload.
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
	default:
		// Lane events: delegate to the model extractor so TUI and web share
		// the same one-liners.
		if _, ok := event.Lookup(ev.HookEvent); ok {
			gist = hvmodel.LaneGistForTUI(ev)
		}
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
			return strings.SplitN(inp.Command, "\n", 2)[0]
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
// NOTE: clip is a simple head-keep truncation used in non-TARGET contexts (error
// messages, session IDs, status bar text). Use truncateSmart for TARGET column.
func clip(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	if utf8.RuneCountInString(s) <= maxRunes {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxRunes-1]) + "…"
}

// ============================================================
// Type-aware truncation helpers (Phase 8e)
// ============================================================

// truncateSmart truncates s to fit within maxRunes using type-aware heuristics:
//
//   - If s fits whole, it is returned unchanged (no marker).
//   - Paths (contains '/') → basename-priority middle-elision: the basename is
//     never sacrificed; segments are dropped from the middle first, then from the
//     front, with a dim "…" marker at the elision point.
//   - Quoted strings (starts+ends with '"') → head-keep, preserve close-quote,
//     trailing "…" before the closing quote.
//   - Otherwise (commands, plain text) → head-keep, trailing "…".
//
// noColor controls whether the marker is styled (it is always a "…" glyph; the
// function does not emit ANSI in noColor mode). The returned string may be
// shorter than maxRunes if the input is short; it is never longer.
// ellipsisMarker returns the truncation marker character appropriate for the
// rendering mode: "…" normally, "~" under NO_COLOR (for terminals where the
// ellipsis glyph is unsupported or unwanted). Both are a single rune wide.
func ellipsisMarker(noColor bool) string {
	if noColor {
		return "~"
	}
	return "…"
}

func truncateSmart(s string, maxRunes int, noColor bool) string {
	if maxRunes < 1 {
		return ""
	}
	n := utf8.RuneCountInString(s)
	if n <= maxRunes {
		return s
	}

	// Dispatch by value type.
	if looksLikePath(s) {
		return truncatePath(s, maxRunes, noColor)
	}
	if isQuotedString(s) {
		return truncateQuoted(s, maxRunes, noColor)
	}
	return truncateHead(s, maxRunes, noColor)
}

// looksLikePath reports whether s is likely a file path (contains '/').
func looksLikePath(s string) bool {
	return strings.Contains(s, "/")
}

// isQuotedString reports whether s is wrapped in double quotes.
func isQuotedString(s string) bool {
	r := []rune(s)
	return len(r) >= 2 && r[0] == '"' && r[len(r)-1] == '"'
}

// truncatePath performs basename-priority middle-elision on a path string.
//
// Strategy (decreasing priority):
//  1. dir/M/basename  — drop middle segments; keep first dir component + basename.
//  2. M/basename      — drop everything before basename.
//  3. If even M/basename won't fit, keep as much of basename as possible with
//     a leading M marker.
//
// M is ellipsisMarker(noColor): "…" normally, "~" under NO_COLOR.
// The marker is always a single rune (width 1).
func truncatePath(s string, maxRunes int, noColor bool) string {
	marker := ellipsisMarker(noColor)
	markerW := 1 // always one rune

	base := filepath.Base(s)
	baseW := utf8.RuneCountInString(base)

	// Strategy 2: "M/" + base
	if 2+markerW+baseW <= maxRunes {
		// Try strategy 1 first: keep dir prefix too.
		dir := filepath.Dir(s)
		if dir != "." && dir != "/" {
			// Take the first component of dir.
			parts := strings.SplitN(dir, "/", 2)
			prefix := parts[0]
			// prefix + "/M/" + base
			candidate := prefix + "/" + marker + "/" + base
			if utf8.RuneCountInString(candidate) <= maxRunes {
				return candidate
			}
		}
		// Fall back to "M/base".
		return marker + "/" + base
	}

	// Strategy 3: even "M/base" is too long — show leading M + tail of base.
	keep := maxRunes - markerW
	if keep < 1 {
		keep = 1
	}
	baseRunes := []rune(base)
	if len(baseRunes) > keep {
		baseRunes = baseRunes[len(baseRunes)-keep:]
	}
	return marker + string(baseRunes)
}

// truncateQuoted truncates a quoted string while preserving the closing quote.
// The trailing marker appears just before the closing quote.
//
// Example (noColor=false): `"func.*Handler regex pattern"` → `"func.*Handl…"` (width 15).
// Example (noColor=true):  `"func.*Handler regex pattern"` → `"func.*Handl~"` (width 15).
func truncateQuoted(s string, maxRunes int, noColor bool) string {
	marker := ellipsisMarker(noColor)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	// Reserve: opening quote (1) + content + marker (1) + closing quote (1) = 3 overhead.
	if maxRunes < 3 {
		return string(runes[:maxRunes])
	}
	keep := maxRunes - 3 // content chars we can show
	head := string(runes[1 : 1+keep])
	return `"` + head + marker + `"`
}

// truncateHead keeps the head of s and appends a trailing marker.
// Used for commands, plain text, and the fallback case.
func truncateHead(s string, maxRunes int, noColor bool) string {
	marker := ellipsisMarker(noColor)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	if maxRunes <= 1 {
		return marker
	}
	return string(runes[:maxRunes-1]) + marker
}

// ============================================================
// Event row rendering (Phase 8b/8c + 8-visual)
// ============================================================

// eventRow is the pre-formatted data for one event row in the Events pane.
type eventRow struct {
	Time     string // HH:MM:SS
	Status   eventStatus
	Hook     string // e.g. "PreToolUse"
	Tool     string // ev.ToolName, or ""
	Gist     string // raw targetGist (unclipped; truncateSmart applied in render)
	Duration string // blank when no duration (lifecycle events)
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
		Gist:    targetGist(ev), // unclipped; truncateSmart applied in render
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
	// Fold label: "PreToolUse·Post"
	row.Hook = abbreviatePair(dr.Pre.HookEvent, colHook)
	return row
}

// caretGlyph is the selection caret character (one rune wide).
const caretGlyph = "▸"

// caretGlyphNC is the NO_COLOR selection caret.
const caretGlyphNC = "»"

// caretBlank is the placeholder used on unselected rows (1 rune wide).
const caretBlank = " "

// Column widths (Phase 8c — fixed, never reflow on selection).
const (
	colCaret  = 1  // reserved caret gutter on every row including header
	colTime   = 8  // HH:MM:SS
	colStatus = 3  // glyph (width 3)
	colHook   = 16 // hook event name (abbreviated)
	colTool   = 12 // tool name
	colDur    = 7  // duration (right-aligned); donated to TARGET when blank
	colSep    = 1  // single space between columns
)

// fixedWidth is the total of all fixed-width columns including the separators
// that strings.Join(" ") adds between them. There are 4 separators between
// the 5 fixed columns (caret, time, status, hook, tool).
//
// When gist+dur are appended, 1 more separator each is added by Join.
// Formula for full row width:
//
//	colCaret + colTime + colStatus + colHook + colTool + gistW + colDur + 6 sep
//	= (colCaret + colTime + colStatus + colHook + colTool + 4) + gistW + 1 + colDur + 1
//	= fixedWidth() + gistW + 1 + colDur + 1
//
// fixedWidth = colCaret + colTime + colStatus + colHook + colTool + 4 (seps between 5)
func fixedWidth() int {
	return colCaret + colTime + colStatus + colHook + colTool + 4
}

// isLifecycleHook reports whether the hook event is a lifecycle-only event
// (SessionStart, UserPromptSubmit, Notification, Stop, SessionEnd, MessageDisplay).
// These rows are rendered dim across the HOOK+TARGET columns.
//
// SubagentStop and PreCompact are intentionally excluded: they are part of
// paired ops with real status glyphs and must not be dimmed as lifecycle.
func isLifecycleHook(hook string) bool {
	switch hook {
	case "SessionStart", "UserPromptSubmit", "Notification", "Stop",
		"SessionEnd", "MessageDisplay":
		return true
	}
	// Also lifecycle if it's a folded pair with only lifecycle prefix.
	// "PreToolUse·Post" is NOT lifecycle.
	return false
}

// targetColorSegments returns the styled TARGET string for the given row data.
// It applies:
//   - error row target → tok.failure (red) — highest priority
//   - lifecycle rows → tok.muted (dim)
//   - file-tool (Read/Write/Edit) path values → tok.pathColor (info/blue)
//   - command/quoted/other (Bash, Grep, etc.) → tok.muted (dim)
//
// tool is the ToolName from the event row (may be "").
// plain is the already-truncated plain text; the styling wraps it in ANSI.
func targetColorSegments(plain string, status eventStatus, lifecycle bool, tool string, noColor bool, tok tokens) string {
	if noColor || plain == "" {
		return plain
	}
	if lifecycle {
		return tok.muted.Render(plain)
	}
	if status == statusError {
		return tok.failure.Render(plain)
	}
	// Only path-type tools get path coloring (blue).
	if isFilePathTool(tool) {
		return tok.pathColor.Render(plain)
	}
	// Commands, grep patterns, quoted strings, user prompts → dim.
	return tok.muted.Render(plain)
}

// isFilePathTool reports whether the tool operates on a file path (so the
// TARGET value should be colored as a path in blue).
func isFilePathTool(tool string) bool {
	switch tool {
	case "Read", "Write", "Edit", "MultiEdit":
		return true
	}
	return false
}

// renderEventRow formats an eventRow into a plain-text string of exactly
// `width` runes (ANSI-free) for width-calculation purposes, OR a styled string
// when noColor=false and the caller needs colors.
//
// Layout (Phase 8b/8c):
//
//	[caret(1)] [TIME(8)] [ST(3)] [HOOK(16)] [TOOL(12)] [TARGET(flex)] [DUR(7)]
//
// Each "[col]" is separated by a single space. The caret gutter is ALWAYS
// reserved on every row (selected or not) so that no column ever shifts.
// Selected row gets the caret glyph; unselected gets a blank.
//
// ANSI safety: the returned string's plain-text width (lipgloss.Width) equals
// `width`. Callers must NOT pass this string through padRight/padBlock with
// rune-counting — use ansiPadRight / ansiPadBlock instead, or ensure styling is
// applied as the final step on an already-padded plain line.
//
// DUR (or "running") is shown only when available. Lifecycle rows donate
// the DUR column width to TARGET, giving paths more room to display whole.
func renderEventRow(r eventRow, width int, noColor bool, selected bool) string {
	tok := themeFor(noColor)

	// --- Caret ---
	var caretStr string
	if selected {
		if noColor {
			caretStr = caretGlyphNC
		} else {
			caretStr = tok.accent.Render(caretGlyph)
		}
	} else {
		caretStr = caretBlank
	}

	// --- Status glyph ---
	glyphStr := statusGlyphStyled(r.Status, noColor, tok)

	// --- Hook ---
	hookAbbr := padRight(abbreviate(r.Hook, colHook), colHook)
	lifecycle := isLifecycleHook(r.Hook)
	var hookStr string
	if !noColor && lifecycle {
		hookStr = tok.muted.Render(hookAbbr)
	} else {
		hookStr = hookAbbr
	}

	// --- Tool ---
	toolStr := padRight(r.Tool, colTool)
	// Tool column: no coloring (default fg) — it's always a simple name.

	// --- Duration ---
	durStr := r.Duration
	showRunning := false
	if durStr == "" && r.Status == statusRunning {
		durStr = "running"
		showRunning = true
	}

	// --- Width budget ---
	fixed := fixedWidth()
	hasDur := durStr != "" && width > fixed+1+1+colDur

	var gistW int
	if hasDur {
		gistW = width - fixed - 1 - 1 - colDur
	} else {
		gistW = width - fixed - 1
	}
	if gistW < 0 {
		gistW = 0
	}

	// --- Build parts: plain text for positions, styled for output ---
	// We join styled segments with " " separators. Since each segment has
	// a known plain width, the total plain width is deterministic.

	parts := []string{
		caretStr,
		padRight(r.Time, colTime), // time: default fg
		glyphStr,
		hookStr,
		toolStr,
	}

	if gistW > 0 {
		var plainGist string
		if r.Gist != "" {
			plainGist = truncateSmart(r.Gist, gistW, noColor)
		}

		// Pad plain gist to fill its column.
		paddedPlainGist := padRight(plainGist, gistW)

		// Apply color to the padded gist.
		var styledGist string
		if !noColor && paddedPlainGist != "" {
			styledGist = targetColorSegments(paddedPlainGist, r.Status, lifecycle, r.Tool, noColor, tok)
		} else {
			styledGist = paddedPlainGist
		}

		parts = append(parts, styledGist)
	}

	if hasDur {
		var durPart string
		if !noColor {
			if showRunning {
				durPart = tok.running.Render(padRight(durStr, colDur))
			} else {
				durPart = tok.muted.Render(padRight(durStr, colDur))
			}
		} else {
			durPart = padRight(durStr, colDur)
		}
		parts = append(parts, durPart)
	}

	// Join with single-space separators.
	// Do NOT pass through padRight after joining — it would rune-count ANSI bytes.
	joined := strings.Join(parts, " ")

	// Pad to width using ANSI-safe padding (only adds trailing spaces, never clips).
	return ansiPadRight(joined, width)
}

// renderEventRowHeader renders the column header for the events pane.
// It reserves the same caret gutter (1 char) as data rows so that the column
// labels align exactly over the data. This fixes the Phase 8c requirement that
// the header and data rows share identical column positions.
//
// The header always shows TARGET + DUR (even when rows may donate DUR to TARGET)
// so the user sees the column labels. Width must be the same as passed to
// renderEventRow for the columns to align.
//
// The header is rendered muted (dim grey) to match the mockup.
func renderEventRowHeader(width int) string {
	fixed := fixedWidth()
	// Same formula as renderEventRow with hasDur=true:
	// total = fixed + 1(sep) + gistW + 1(sep) + colDur = width
	gistW := width - fixed - 1 - 1 - colDur
	if gistW < 0 {
		gistW = 0
	}

	parts := []string{
		caretBlank, // caret gutter — always blank on the header
		padRight("TIME", colTime),
		padRight("ST", colStatus),
		padRight("HOOK", colHook),
		padRight("TOOL", colTool),
	}
	if gistW > 0 {
		parts = append(parts, padRight("TARGET", gistW))
	}
	parts = append(parts, padRight("DUR", colDur))

	plain := padRight(strings.Join(parts, " "), width)
	return plain // styling applied by caller (viewEventsPane renders header muted)
}

// padRight pads or clips s to exactly n runes. For plain-text (unstyled) columns.
// NEVER pass a string containing ANSI escapes to padRight — use ansiPadRight.
func padRight(s string, n int) string {
	r := []rune(s)
	if len(r) >= n {
		return string(r[:n])
	}
	return s + strings.Repeat(" ", n-len(r))
}

// ansiPadRight pads s to at least n visible runes by appending spaces.
// It uses lipgloss.Width (ANSI-aware) for width measurement.
// Unlike padRight, it never clips — it only adds trailing spaces.
// If the visible width already meets or exceeds n, s is returned unchanged.
func ansiPadRight(s string, n int) string {
	w := ansiWidth(s)
	if w >= n {
		return s
	}
	return s + strings.Repeat(" ", n-w)
}

// abbreviate clips hook event names to n runes, stripping common prefixes
// ("PreToolUse" → "PreToolUse", "UserPromptSubmit" → "UserPromptSub").
func abbreviate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// abbreviatePair formats a folded Pre/Post pair hook label, e.g.
// "PreToolUse" → "PreToolUse·Post". Clips to n runes.
func abbreviatePair(preHook string, n int) string {
	label := preHook + "·Post"
	if utf8.RuneCountInString(label) <= n {
		return label
	}
	return string([]rune(label)[:n])
}

// formatDuration renders a duration for the duration column. Returns "" for
// zero (no pairing).
func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

// ============================================================
// Session row rendering (Phase 8d + 8-visual)
// ============================================================

// humanizeAge converts a duration (age since ModTime) into a terse human label.
//
//	0–4s → "live"
//	5s–59s → "Ns"
//	1m–59m → "Nm"
//	1h–23h → "Nh"
//	≥1d   → "Nd"
func humanizeAge(d time.Duration) string {
	if d < 5*time.Second {
		return "live"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// sessionLabel builds the single-line label shown in the Sessions pane.
// Legacy API used by older callers. Clips to fit in w runes.
// Phase 8d: prefer the richer two-line sessionRowLines when available.
func sessionLabel(id string, count int64, w int) string {
	suffix := fmt.Sprintf(" (%d)", count)
	avail := w - len(suffix)
	if avail < 4 {
		avail = 4
	}
	return clip(id, avail) + suffix
}

// sessionRowLines returns the two display lines for a session row (Phase 8d):
//
//	line 0: [caret] [live●] title
//	line 1: [blank] [blank ] project · recency · count
//
// The caret gutter (1 rune) is reserved on both lines matching the events
// pane idiom (stable width). Both lines are padded/clipped to w as PLAIN TEXT
// (no ANSI). Semantic coloring is applied by the caller (viewSessionsPane).
//
// If s.Title is blank the fallback is "project · shortid".
func sessionRowLines(s store.SessionInfo, w int, selected bool, focused bool, live bool, now time.Time, noColor bool) (string, string) {
	caret := caretBlank
	if selected {
		if noColor {
			caret = caretGlyphNC
		} else {
			caret = caretGlyph
		}
	}

	liveMarker := "  " // 2 runes: "● " or "  "
	if live {
		liveMarker = "● "
	}

	// Build title.
	project := filepath.Base(s.CWD)
	if project == "" || project == "." {
		project = s.ID
	}
	title := s.Title
	if title == "" {
		// Fallback: "project · shortid".
		short := s.ID
		if len(short) > 8 {
			short = short[:8]
		}
		title = project + " · " + short
	}

	// Recency.
	age := now.Sub(s.ModTime)
	recency := humanizeAge(age)

	// Count: abbreviated (1.2k for ≥1000).
	countStr := formatCount(s.EventCount)

	// Meta line content: project · recency · count
	meta := project + " · " + recency + " · " + countStr

	// Compute available widths: caret(1) + liveMarker(2) + content.
	contentW := w - 1 - 2 // caret + liveMarker
	if contentW < 1 {
		contentW = 1
	}

	titleClipped := padRight(clip(title, contentW), contentW)
	metaContent := padRight(clip(meta, contentW), contentW)

	// Return plain text lines — callers apply color after this.
	// The caret and live marker are plain chars here; viewSessionsPane styles them.
	line0 := caret + liveMarker + titleClipped
	line1 := caretBlank + "  " + metaContent

	// Clip entire lines to w (plain-text, no ANSI yet).
	line0 = padRight(line0, w)
	line1 = padRight(line1, w)

	return line0, line1
}

// sessionRowLinesStyled returns session row lines with semantic coloring applied.
// It builds plain lines via sessionRowLines then applies token-based styles.
// Returns plain lines when noColor is true.
//
// The returned strings have ANSI codes and must be padded with ansiPadRight,
// not padRight.
func sessionRowLinesStyled(s store.SessionInfo, w int, selected bool, focused bool, live bool, now time.Time, noColor bool) (string, string) {
	if noColor {
		return sessionRowLines(s, w, selected, focused, live, now, noColor)
	}

	tok := themeFor(false)

	caret := caretBlank
	if selected {
		caret = caretGlyph
	}

	// Build plain content.
	project := filepath.Base(s.CWD)
	if project == "" || project == "." {
		project = s.ID
	}
	title := s.Title
	if title == "" {
		short := s.ID
		if len(short) > 8 {
			short = short[:8]
		}
		title = project + " · " + short
	}

	age := now.Sub(s.ModTime)
	recency := humanizeAge(age)
	countStr := formatCount(s.EventCount)
	meta := project + " · " + recency + " · " + countStr

	contentW := w - 1 - 2
	if contentW < 1 {
		contentW = 1
	}

	titleClipped := padRight(clip(title, contentW), contentW)
	metaContent := padRight(clip(meta, contentW), contentW)

	// Style caret.
	var styledCaret string
	if selected {
		if focused {
			styledCaret = tok.accent.Render(caret)
		} else {
			styledCaret = tok.accentDim.Render(caret)
		}
	} else {
		styledCaret = caretBlank
	}

	// Style live marker.
	var styledLive string
	if live {
		styledLive = tok.success.Render("●") + " "
	} else {
		styledLive = "  "
	}

	// Style title: bold/bright when selected, default fg otherwise.
	var styledTitle string
	if selected {
		styledTitle = tok.header.Render(titleClipped)
	} else {
		styledTitle = titleClipped
	}

	// Style meta: always muted.
	styledMeta := tok.muted.Render(metaContent)

	// Build lines from styled parts.
	// Plain width: 1(caret) + 2(live) + contentW = w.
	// Styled: same visible width since each styled part wraps the same rune content.
	line0 := styledCaret + styledLive + styledTitle
	line1 := caretBlank + "  " + styledMeta

	// Pad to w using ANSI-aware padding.
	line0 = ansiPadRight(line0, w)
	line1 = ansiPadRight(line1, w)

	return line0, line1
}

// formatCount formats an int64 event count as a short string.
// ≥1,000,000 → "1.0M"; ≥1,000 → "1.2k"; otherwise decimal.
func formatCount(n int64) string {
	if n >= 1_000_000 {
		return fmt.Sprintf("%.1fM", float64(n)/1_000_000)
	}
	if n >= 1000 {
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%d", n)
}
