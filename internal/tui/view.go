package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// View renders the full terminal frame. It delegates to layout-specific helpers.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		// Dimensions not yet received.
		return ""
	}

	if m.layout == LayoutTooSmall {
		return viewTooSmall(m.width, m.height)
	}

	if m.rawPager {
		return m.viewRawPager()
	}

	if m.showHelp {
		return m.viewHelp()
	}

	statusBar := m.viewStatusBar()
	keyBar := m.viewKeyBar()
	// Reserve 2 rows for bars (1 top + 1 bottom).
	contentH := m.height - 2
	if contentH < 1 {
		contentH = 1
	}

	var content string
	switch m.layout {
	case LayoutBrowse:
		if m.depthDrill {
			content = m.viewDrill(contentH)
		} else {
			content = m.viewBrowse(contentH)
		}
	default:
		content = m.viewNarrow(contentH)
	}

	return statusBar + "\n" + content + "\n" + keyBar
}

// viewTooSmall renders the "please resize" message.
func viewTooSmall(cols, rows int) string {
	msg := fmt.Sprintf("Terminal too small (%dx%d). Please resize to at least %dx%d.",
		cols, rows, minCols, minRows)
	return msg
}

// viewStatusBar renders the top bar: app name | daemon status | live status |
// session | error tape. The live segment is a disconnect banner when the stream
// is down, otherwise the heartbeat / idle indicator.
//
// All segments are styled with semantic tokens:
//   - "hv" → bold/bright
//   - "● daemon" → success(green) for the bullet when OK
//   - ":port" / separators → muted/faint
//   - live indicator / rate → accent(blue) for ▮ rate, success for "live"
//   - session label → muted
//   - error tape → failure(red) when errors > 0, muted when 0
func (m model) viewStatusBar() string {
	tok := themeFor(m.noColor)

	// "hv" — bold/bright.
	var appName string
	if m.noColor {
		appName = "hv"
	} else {
		appName = tok.header.Render("hv")
	}

	sep := " │ "
	if !m.noColor {
		sep = " " + tok.muted.Render("│") + " "
	}

	daemonStatus := m.daemonStatusTextStyled(tok)

	parts := []string{appName, daemonStatus}
	if live := m.liveSegmentStyled(tok); live != "" {
		parts = append(parts, live)
	}
	if m.selectedSession != "" {
		label := m.selectedSessionLabel()
		var sessionPart string
		if m.noColor {
			sessionPart = "session " + label
		} else {
			sessionPart = "session " + tok.muted.Render(label)
		}
		parts = append(parts, sessionPart)
	}
	// Error tape: always visible when events are loaded.
	if len(m.events) > 0 {
		parts = append(parts, m.errorTapeTextStyled(tok))
	}
	bar := strings.Join(parts, sep)
	return ansiPadRight(bar, m.width)
}

// daemonStatusTextStyled renders the daemon status segment with color.
func (m model) daemonStatusTextStyled(tok tokens) string {
	plain := m.daemonStatusText()
	if m.noColor {
		return plain
	}
	// Color the "●" green when daemon is OK; entire segment muted otherwise.
	if m.daemonOK {
		// "● daemon :port" — bullet green, rest muted.
		// daemonStatusText returns something like "● daemon :7842"
		// Split on first space to isolate bullet.
		idx := strings.Index(plain, " ")
		if idx > 0 {
			bullet := plain[:idx]
			rest := plain[idx:]
			return tok.success.Render(bullet) + tok.muted.Render(rest)
		}
		return tok.success.Render(plain)
	}
	return tok.muted.Render(plain)
}

// liveSegmentStyled renders the live segment with semantic color.
func (m model) liveSegmentStyled(tok tokens) string {
	if m.noColor {
		return m.liveSegment()
	}
	if !m.streamUp && m.streamErr != "" {
		return m.liveSegment() // disconnect banner — no extra styling needed
	}
	plain := m.liveStatusText()
	if plain == "" {
		return ""
	}
	// "live · ▮ 4/s" or "idle 2s"
	// Color "live" green, "▮ N/s" accent, "·" muted.
	if strings.HasPrefix(plain, "live") {
		// Find the "·" separator.
		idx := strings.Index(plain, " · ")
		if idx > 0 {
			livePart := tok.success.Render(plain[:idx])
			sep := tok.muted.Render(" · ")
			rest := plain[idx+3:]
			// Color the rate part (▮ N/s) in accent.
			return livePart + sep + tok.info.Render(rest)
		}
		return tok.success.Render(plain)
	}
	// "idle Ns" — muted.
	return tok.muted.Render(plain)
}

// errorTapeTextStyled renders the error tape with color.
func (m model) errorTapeTextStyled(tok tokens) string {
	n := m.errorCount()
	if m.noColor {
		return m.errorTapeText()
	}
	if n == 0 {
		return tok.muted.Render("no errors")
	}
	text := fmt.Sprintf("✘ %d error%s", n, pluralS(n))
	return tok.failure.Render(text)
}

// selectedSessionLabel returns a short label for the status bar.
// Prefers "Title… · shortID", falls back to clip(id, 24).
func (m model) selectedSessionLabel() string {
	for _, s := range m.sessions {
		if s.ID == m.selectedSession {
			short := s.ID
			if len(short) > 7 {
				short = short[:7]
			}
			if s.Title != "" {
				title := clip(s.Title, 20)
				return title + " · " + short
			}
			return clip(s.ID, 24)
		}
	}
	return clip(m.selectedSession, 24)
}

// errorTapeText returns the sticky error indicator for the status bar.
// Format: "✘ N errors" or "no errors". Redundant glyph+text for noColor safety.
func (m model) errorTapeText() string {
	n := m.errorCount()
	if n == 0 {
		return "no errors"
	}
	return fmt.Sprintf("✘ %d error%s", n, pluralS(n))
}

// pluralS returns "s" when n != 1, "" otherwise.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// liveSegment is the status-bar text for the live stream: a disconnect banner
// while retrying, the heartbeat/idle indicator while connected, or "".
func (m model) liveSegment() string {
	if !m.streamUp && m.streamErr != "" {
		return "⚠ stream disconnected — retrying (" + truncateErr(m.streamErr, 24) + ")"
	}
	return m.liveStatusText()
}

// viewKeyBar renders the bottom context-sensitive key hint bar.
// When filter input mode is active it shows the input line instead, with the
// active filter tokens shown as persistent chips. When a transient statusMsg is
// set (e.g. after a yank), it is shown instead of the normal hints.
func (m model) viewKeyBar() string {
	// Filter input mode: show the text entry line.
	if m.filterMode {
		prompt := "/ " + m.filterInput + "▐"
		hint := "  [Enter:apply  Esc:cancel]"
		return padRight(prompt+hint, m.width)
	}
	if m.statusMsg != "" {
		return padRight("  "+m.statusMsg, m.width)
	}
	var hints []string
	switch m.focusedPane {
	case paneSessions:
		hints = []string{"j/k:move", "enter:select", "tab:focus", "?:help", "q:quit"}
	case paneEvents:
		follow := "f:follow"
		if m.follow {
			follow = "f:following"
		}
		fold := "o:folded"
		if !m.foldedView {
			fold = "o:flat"
		}
		if m.depthDrill {
			hints = []string{"j/k:scroll", "y:yank", "Y:yank-raw", "r:raw",
				"tab:◂events", "esc:◂browse", follow, fold, "?:help", "q:quit"}
		} else {
			hints = []string{"j/k:move", "enter:open▸", "tab:◂sessions",
				follow, "G:live", "/:filter", "e:err-hop", fold, "?:help", "q:quit"}
		}
	case paneInspector:
		hints = []string{"j/k:scroll", "y:yank", "Y:yank-raw", "r:raw",
			"tab:◂events", "esc:◂back", "?:help", "q:quit"}
	}
	bar := strings.Join(hints, "  ")

	// Append active filter chips.
	if !m.filter.IsEmpty() {
		bar += "  [filter: " + m.filter.raw + "]"
	}

	return padRight(bar, m.width)
}

// --- Browse layout (≥80 cols, depthDrill==false) ----------------------------
// Sessions(36ch) │ Events(fill)

func (m model) viewBrowse(contentH int) string {
	sw, ew, _ := paneWidths(LayoutBrowse, m.width)
	sessions := m.viewSessionsPane(sw, contentH)
	events := m.viewEventsPane(ew, contentH)

	sLines := splitLines(sessions, contentH)
	eLines := splitLines(events, contentH)

	var rows []string
	for i := 0; i < contentH; i++ {
		sl := getLine(sLines, i, sw)
		el := getLine(eLines, i, ew)
		rows = append(rows, sl+"│"+el)
	}
	return strings.Join(rows, "\n")
}

// --- Drill layout (≥80 cols, depthDrill==true) ------------------------------
// Events(fill) │ Inspector(46ch)

func (m model) viewDrill(contentH int) string {
	ew, iw := paneWidthsDrill(m.width)
	events := m.viewEventsPane(ew, contentH)
	inspector := m.viewInspectorPane(iw, contentH)

	eLines := splitLines(events, contentH)
	iLines := splitLines(inspector, contentH)

	var rows []string
	for i := 0; i < contentH; i++ {
		el := getLine(eLines, i, ew)
		il := getLine(iLines, i, iw)
		rows = append(rows, el+"│"+il)
	}
	return strings.Join(rows, "\n")
}

// --- Narrow layout (<80 cols) -----------------------------------------------

func (m model) viewNarrow(contentH int) string {
	breadcrumb := m.viewBreadcrumb()
	bodyH := contentH - 1
	if bodyH < 1 {
		bodyH = 1
	}
	var body string
	switch m.focusedPane {
	case paneSessions:
		body = m.viewSessionsPane(m.width, bodyH)
	case paneEvents:
		body = m.viewEventsPane(m.width, bodyH)
	case paneInspector:
		body = m.viewInspectorPane(m.width, bodyH)
	}
	return breadcrumb + "\n" + body
}

// viewBreadcrumb renders the navigation crumb for narrow layout.
func (m model) viewBreadcrumb() string {
	parts := []string{"Sessions"}
	if m.selectedSession != "" {
		title := clip(m.selectedSession, 20)
		for _, s := range m.sessions {
			if s.ID == m.selectedSession && s.Title != "" {
				title = clip(s.Title, 20)
				break
			}
		}
		switch m.focusedPane {
		case paneEvents:
			parts = []string{"Sessions", title, "Events"}
		case paneInspector:
			parts = []string{"Sessions", title, "Events", "Inspector"}
		default:
			parts = append(parts, title)
		}
	}
	return strings.Join(parts, " > ")
}

// --- Pane renderers ---------------------------------------------------------

// paneTitleLine renders a styled pane title line of exactly w chars.
// When focused, the title uses the header style (bold/bright).
// When unfocused, the title is muted (dim grey).
// badge is an optional right-aligned plain-text badge (e.g. "↓ 3 new"); it
// will be styled in the chip/running token (yellow) when noColor=false.
func paneTitleLine(text, badge string, w int, focused bool, noColor bool) string {
	tok := themeFor(noColor)

	var result string
	if badge == "" {
		// Simple case: just pad and style the title.
		plain := padRight(text, w)
		if noColor {
			return plain
		}
		if focused {
			return tok.header.Render(plain)
		}
		return tok.muted.Render(plain)
	}

	// Badge present: title gets styled; badge gets chip color (yellow).
	// Compute spacing using plain-text widths.
	titlePlainLen := utf8.RuneCountInString(text)
	badgePlainLen := utf8.RuneCountInString(badge)
	pad := w - titlePlainLen - badgePlainLen
	if pad < 1 {
		pad = 1
	}

	// Truncate title if it would leave no room.
	if titlePlainLen+1+badgePlainLen > w {
		maxTitle := w - 1 - badgePlainLen
		if maxTitle < 0 {
			maxTitle = 0
		}
		text = padRight(text, maxTitle)
		pad = 1
	}

	if noColor {
		result = text + strings.Repeat(" ", pad) + badge
		return padRight(result, w)
	}

	var styledTitle string
	if focused {
		styledTitle = tok.header.Render(text)
	} else {
		styledTitle = tok.muted.Render(text)
	}
	styledBadge := tok.chip.Render(badge)

	result = styledTitle + strings.Repeat(" ", pad) + styledBadge
	return ansiPadRight(result, w)
}

// viewSessionsPane renders the sessions list pane (Phase 8b/8d + 8-visual).
//
// Each session occupies two display lines (title + meta) as a single selectable
// unit. The caret gutter is reserved on both lines — same idiom as the events
// pane (no reverse-video, no "> " prefix).
//
// ANSI safety: selection bands are applied to already-padded plain lines before
// any further processing by ansiPadBlock.
func (m model) viewSessionsPane(w, h int) string {
	tok := themeFor(m.noColor)
	focused := m.focusedPane == paneSessions

	title := paneTitleLine("SESSIONS", "", w, focused, m.noColor)
	lines := []string{title}

	now := m.now()

	switch {
	case m.sessionsErr != nil:
		lines = append(lines, clip("Error: "+m.sessionsErr.Error(), w))
		lines = append(lines, "")
		if !m.daemonOK {
			lines = append(lines, clip("Is the daemon running?", w))
			lines = append(lines, clip("Run: hv daemon --foreground", w))
		}
	case len(m.sessions) == 0:
		lines = append(lines, "")
		lines = append(lines, clip("No sessions yet.", w))
		lines = append(lines, "")
		lines = append(lines, clip("Start Claude Code with the hv", w))
		lines = append(lines, clip("plugin installed to capture events.", w))
		lines = append(lines, "")
		lines = append(lines, clip("Daemon: "+m.daemonStatusText(), w))
	default:
		// Compute viewport window.
		// Each session is 2 physical lines; title row is 1.
		availH := h - 1 // subtract the "SESSIONS" title row
		visibleN := availH / 2
		if visibleN < 1 {
			visibleN = 1
		}
		top := m.sessionTop
		if top < 0 {
			top = 0
		}
		if top+visibleN > len(m.sessions) {
			top = max(0, len(m.sessions)-visibleN)
		}
		end := top + visibleN
		if end > len(m.sessions) {
			end = len(m.sessions)
		}

		for i := top; i < end; i++ {
			s := m.sessions[i]
			selected := i == m.sessionCursor
			live := m.isLive(s.ID)

			// Build styled lines (caret, live●, title bold/dim, meta muted).
			l0, l1 := sessionRowLinesStyled(s, w, selected, focused, live, now, m.noColor)

			// Apply selection band (background) AFTER lines are at correct width.
			// The lines from sessionRowLinesStyled are already ansiPadRight'd to w.
			if selected && !m.noColor {
				if focused {
					l0 = tok.selBandFocused.Render(l0)
					l1 = tok.selBandFocused.Render(l1)
				} else {
					l0 = tok.selBand.Render(l0)
					l1 = tok.selBand.Render(l1)
				}
			}

			lines = append(lines, l0, l1)
		}
	}

	return ansiPadBlock(lines, w, h)
}

// viewEventsPane renders the events list pane (Phase 8b/8c + 8-visual).
//
// The caret gutter is reserved on every row (including the header) so columns
// never shift on selection. The selected row gets a background band via the
// theme tokens (never reverse-video). A scroll viewport keeps the cursor visible.
//
// ANSI safety: renderEventRow returns a string whose plain width == w. The
// selection band is applied AFTER padding. ansiPadBlock is used for final assembly
// so ANSI-bearing lines are not rune-sliced.
func (m model) viewEventsPane(w, h int) string {
	tok := themeFor(m.noColor)
	focused := m.focusedPane == paneEvents

	// Title: show session label and optional "↓ N new" badge.
	titleText := "EVENTS"
	if m.selectedSession != "" {
		sessionLabel := clip(m.selectedSession, w-9)
		for _, s := range m.sessions {
			if s.ID == m.selectedSession && s.Title != "" {
				sessionLabel = clip(s.Title, w-9)
				break
			}
		}
		titleText = "EVENTS  " + sessionLabel
	}
	var badge string
	if !m.follow && m.pendingCount > 0 {
		badge = fmt.Sprintf("↓ %d new", m.pendingCount)
	}

	title := paneTitleLine(titleText, badge, w, focused, m.noColor)
	lines := []string{title}

	switch {
	case m.selectedSession == "":
		lines = append(lines, "")
		lines = append(lines, clip("Select a session →", w))
	case m.eventsLoading:
		lines = append(lines, "")
		lines = append(lines, clip("Loading…", w))
	case m.eventsErr != nil:
		lines = append(lines, clip("Error: "+m.eventsErr.Error(), w))
	case len(m.events) == 0:
		lines = append(lines, "")
		lines = append(lines, clip("No events in this session.", w))
	default:
		// Header row: plain text, rendered muted.
		headerPlain := renderEventRowHeader(w)
		var headerLine string
		if m.noColor {
			headerLine = headerPlain
		} else {
			headerLine = tok.muted.Render(headerPlain)
		}
		lines = append(lines, headerLine)

		rows := m.visibleRows()

		// Compute scroll viewport.
		// h rows total; 1 title + 1 header = 2 overhead; rest for data rows.
		headerRows := 2 // title + column header
		dataH := h - headerRows
		if dataH < 1 {
			dataH = 1
		}

		top := m.eventTop
		if top < 0 {
			top = 0
		}
		end := top + dataH
		if end > len(rows) {
			end = len(rows)
		}

		for i := top; i < end; i++ {
			dr := rows[i]
			row := buildDisplayEventRow(dr)
			selected := i == m.eventCursor

			// renderEventRow returns an ANSI-width-correct string (plain width == w).
			rendered := renderEventRow(row, w, m.noColor, selected)

			// Apply selection band AFTER padding — the rendered string is already at w.
			if selected && !m.noColor {
				if focused {
					rendered = tok.selBandFocused.Render(rendered)
				} else {
					rendered = tok.selBand.Render(rendered)
				}
			}

			lines = append(lines, rendered)
		}
	}

	return ansiPadBlock(lines, w, h)
}

// viewInspectorPane renders the inspector pane (Phase 8e — soft-wrap values).
//
// Key-facts labels are muted; values are bright/default. The divider is faint.
// JSON syntax coloring is applied by inspectLinesSoftWrap.
func (m model) viewInspectorPane(w, h int) string {
	tok := themeFor(m.noColor)
	focused := m.focusedPane == paneInspector

	title := paneTitleLine("INSPECTOR", "", w, focused, m.noColor)
	lines := []string{title}

	ev := m.selectedEvent()
	if ev == nil {
		lines = append(lines, "")
		lines = append(lines, clip("Select an event to inspect.", w))
		return ansiPadBlock(lines, w, h)
	}

	dr, _ := m.selectedDisplayRow()

	// Key-facts header: labels muted, values bright.
	addKV := func(label, value string) {
		plain := fmt.Sprintf("%-8s%s", label, value)
		if m.noColor {
			lines = append(lines, padRight(plain, w))
			return
		}
		styledLabel := tok.muted.Render(fmt.Sprintf("%-8s", label))
		styledValue := value // default fg
		lines = append(lines, ansiPadRight(styledLabel+styledValue, w))
	}

	addKV("Hook", ev.HookEvent)
	addKV("Tool", ev.ToolName)
	if dr.IsPair && dr.Duration > 0 {
		addKV("Dur", formatDuration(dr.Duration))
	}
	addKV("Time", ev.CapturedAt.Local().Format("2006-01-02 15:04:05"))
	addKV("Seq", fmt.Sprintf("%d", ev.Seq))

	// Target gist in key-facts.
	gist := targetGist(ev)
	if gist != "" {
		addKV("Target", gist)
	}

	// Divider: faint.
	divider := strings.Repeat("─", w)
	if m.noColor {
		lines = append(lines, divider)
	} else {
		lines = append(lines, tok.muted.Render(divider))
	}

	// Syntax-aware inspector body with soft-wrap (Phase 8e).
	lines = append(lines, inspectLinesSoftWrap(ev, w, m.noColor)...)

	// Apply scroll offset (keep key-facts header visible).
	const fixedHeaderRows = 1 // just the title
	fixedLines := lines[:fixedHeaderRows]
	scrollable := lines[fixedHeaderRows:]
	if m.inspectorScroll > 0 && m.inspectorScroll < len(scrollable) {
		scrollable = scrollable[m.inspectorScroll:]
	}
	visible := append(fixedLines, scrollable...)

	return ansiPadBlock(visible, w, h)
}

// viewHelp renders the ? help overlay.
func (m model) viewHelp() string {
	lines := []string{
		padRight("HELP — hv keyboard shortcuts", m.width),
		padRight("", m.width),
		padRight("  Tab / 1 2 3    Switch pane focus (depth-aware)", m.width),
		padRight("  j / k          Move cursor up/down", m.width),
		padRight("  Ctrl-d / Ctrl-u  Page down / page up", m.width),
		padRight("  g / G          Jump to first / last", m.width),
		padRight("  Enter / l      On event: enter Drill (Inspector); on session: focus Events", m.width),
		padRight("  Esc / h        Pop one step shallower (never quits)", m.width),
		padRight("  1              Sessions (exit Drill if needed)", m.width),
		padRight("  2              Events (either depth)", m.width),
		padRight("  3              Inspector (enter Drill if needed)", m.width),
		padRight("  f              Toggle tail-follow (live)", m.width),
		padRight("  G              Jump to latest & re-follow", m.width),
		padRight("  space          Pause follow", m.width),
		padRight("  r              Raw JSON pager (inspector)", m.width),
		padRight("  y / Y          Yank value / raw event", m.width),
		padRight("  /              Filter events", m.width),
		padRight("  e / E          Hop to next / prev error", m.width),
		padRight("  o              Toggle folded / flat event view", m.width),
		padRight("  q              Quit (Esc never quits)", m.width),
		padRight("  ?              Toggle this help", m.width),
		padRight("", m.width),
		padRight("  Press any key to dismiss.", m.width),
	}
	return strings.Join(lines, "\n")
}

// viewRawPager renders the full-screen raw JSON pager.
func (m model) viewRawPager() string {
	ev := m.selectedEvent()
	if ev == nil {
		return "No event selected. Press q to close."
	}
	header := fmt.Sprintf("Raw JSON — %s  seq:%d  (q/esc to close)",
		ev.HookEvent, ev.Seq)
	var out []byte
	if len(ev.Raw) > 0 {
		pretty, err := json.MarshalIndent(json.RawMessage(ev.Raw), "", "  ")
		if err == nil {
			out = pretty
		} else {
			out = ev.Raw
		}
	}
	return header + "\n" + strings.Repeat("─", m.width) + "\n" + string(out)
}

// --- helpers ----------------------------------------------------------------

// formatJSONLines pretty-prints raw JSON into lines of at most w chars.
func formatJSONLines(raw json.RawMessage, w int) []string {
	if len(raw) == 0 {
		return []string{"(empty)"}
	}
	pretty, err := json.MarshalIndent(json.RawMessage(raw), "", "  ")
	if err != nil {
		return []string{clip("(malformed JSON: "+err.Error()+")", w)}
	}
	var lines []string
	for _, line := range strings.Split(string(pretty), "\n") {
		lines = append(lines, padRight(line, w))
	}
	return lines
}

// inspectLinesSoftWrap soft-wraps long string values in the inspector.
// Long values are wrapped onto continuation lines with a hanging indent aligned
// under the value's start column, so nothing is clipped in the inspector.
// noColor controls whether syntax coloring is applied.
func inspectLinesSoftWrap(ev *event.Event, w int, noColor bool) []string {
	return inspectLinesColored(ev, w, noColor)
}

// splitLines splits a multi-line string into a slice of lines.
func splitLines(s string, capacity int) []string {
	lines := strings.Split(s, "\n")
	if len(lines) < capacity {
		// Pad to capacity so getLine doesn't need bounds checks.
		for len(lines) < capacity {
			lines = append(lines, "")
		}
	}
	return lines
}

// getLine returns lines[i] padded to w using ANSI-aware measurement,
// or a blank padded line if out of bounds.
// ANSI safety: uses ansiPadRight so styled lines are not rune-sliced.
func getLine(lines []string, i, w int) string {
	if i >= len(lines) {
		return strings.Repeat(" ", w)
	}
	return ansiPadRight(lines[i], w)
}

// padBlock pads a slice of plain-text lines to exactly h lines of width w.
// For plain (unstyled) lines only. Use ansiPadBlock for ANSI-bearing lines.
func padBlock(lines []string, w, h int) string {
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = padRight(l, w)
	}
	return strings.Join(lines, "\n")
}

// ansiPadBlock pads a slice of lines (which may contain ANSI escapes) to exactly
// h lines, each with visible width w. Uses ansiPadRight for width measurement and
// padding so styled lines are never rune-sliced. This is the ANSI-safe replacement
// for padBlock when lines may be styled.
func ansiPadBlock(lines []string, w, h int) string {
	for len(lines) < h {
		lines = append(lines, strings.Repeat(" ", w))
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	for i, l := range lines {
		lines[i] = ansiPadRight(l, w)
	}
	return strings.Join(lines, "\n")
}
