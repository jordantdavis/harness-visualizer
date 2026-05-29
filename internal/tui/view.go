package tui

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
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
	case LayoutWide:
		content = m.viewWide(contentH)
	case LayoutMedium:
		content = m.viewMedium(contentH)
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
// session. The live segment is a disconnect banner when the stream is down,
// otherwise the heartbeat / idle indicator.
func (m model) viewStatusBar() string {
	appName := "cchv"
	daemonStatus := m.daemonStatusText()

	parts := []string{appName, daemonStatus}
	if live := m.liveSegment(); live != "" {
		parts = append(parts, live)
	}
	if m.selectedSession != "" {
		parts = append(parts, "session: "+clip(m.selectedSession, 24))
	}
	bar := strings.Join(parts, "  │  ")
	return padRight(bar, m.width)
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
func (m model) viewKeyBar() string {
	var hints []string
	switch m.focusedPane {
	case paneSessions:
		hints = []string{"j/k:move", "enter:select", "tab:focus", "?:help", "q:quit"}
	case paneEvents:
		follow := "f:follow"
		if m.follow {
			follow = "f:following"
		}
		hints = []string{"j/k:move", "enter:inspect", follow, "G:live", "esc:back", "?:help", "q:quit"}
	case paneInspector:
		hints = []string{"j/k:scroll", "r:raw", "esc:back", "?:help", "q:quit"}
	}
	bar := strings.Join(hints, "  ")
	return padRight(bar, m.width)
}

// --- Layout: Wide (≥120) ----------------------------------------------------

func (m model) viewWide(contentH int) string {
	sw, ew, iw := paneWidths(LayoutWide, m.width)
	sessions := m.viewSessionsPane(sw, contentH)
	events := m.viewEventsPane(ew, contentH)
	inspector := m.viewInspectorPane(iw, contentH)

	// Join with dividers.
	sLines := splitLines(sessions, contentH)
	eLines := splitLines(events, contentH)
	iLines := splitLines(inspector, contentH)

	var rows []string
	for i := 0; i < contentH; i++ {
		sl := getLine(sLines, i, sw)
		el := getLine(eLines, i, ew)
		il := getLine(iLines, i, iw)
		rows = append(rows, sl+"│"+el+"│"+il)
	}
	return strings.Join(rows, "\n")
}

// --- Layout: Medium (80–119) -----------------------------------------------

func (m model) viewMedium(contentH int) string {
	sw, ew, iw := paneWidths(LayoutMedium, m.width)

	if m.focusedPane == paneInspector || m.inspectorOpen {
		// Show events + inspector as stacked drawer.
		inspectorH := min(contentH/2, 20)
		eventsH := contentH - inspectorH - 1 // 1 for drawer separator

		sessions := m.viewSessionsPane(sw, contentH)
		events := m.viewEventsPane(ew, eventsH)
		inspector := m.viewInspectorPane(iw, inspectorH)

		sLines := splitLines(sessions, contentH)
		eLines := splitLines(events, eventsH)
		iLines := splitLines(inspector, inspectorH)

		var rows []string
		for i := 0; i < eventsH; i++ {
			sl := getLine(sLines, i, sw)
			el := getLine(eLines, i, ew)
			rows = append(rows, sl+"│"+el)
		}
		rows = append(rows, strings.Repeat("─", m.width))
		for i := 0; i < inspectorH; i++ {
			il := getLine(iLines, i, iw)
			rows = append(rows, padRight(il, m.width))
		}
		return strings.Join(rows, "\n")
	}

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

// --- Layout: Narrow (<80) --------------------------------------------------

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
		parts = append(parts, clip(m.selectedSession, 20))
	}
	switch m.focusedPane {
	case paneEvents:
		if m.selectedSession != "" {
			parts = []string{"Sessions", clip(m.selectedSession, 20), "Events"}
		}
	case paneInspector:
		parts = append(parts, "Inspector")
	}
	return strings.Join(parts, " > ")
}

// --- Pane renderers ---------------------------------------------------------

// viewSessionsPane renders the sessions list pane.
func (m model) viewSessionsPane(w, h int) string {
	title := padRight("SESSIONS", w)
	lines := []string{title}

	switch {
	case m.sessionsErr != nil:
		lines = append(lines, clip("Error: "+m.sessionsErr.Error(), w))
		lines = append(lines, "")
		if !m.daemonOK {
			lines = append(lines, clip("Is the daemon running?", w))
			lines = append(lines, clip("Run: cchv daemon --foreground", w))
		}
	case len(m.sessions) == 0:
		lines = append(lines, "")
		lines = append(lines, clip("No sessions yet.", w))
		lines = append(lines, "")
		lines = append(lines, clip("Start Claude Code with the cchv", w))
		lines = append(lines, clip("plugin installed to capture events.", w))
		lines = append(lines, "")
		lines = append(lines, clip("Daemon: "+m.daemonStatusText(), w))
	default:
		for i, s := range m.sessions {
			// Row = cursor(2) + live-marker(2) + label. Live sessions get a ●;
			// others a blank of equal width so labels stay column-aligned.
			cursor := "  "
			if i == m.sessionCursor {
				cursor = "> "
			}
			marker := "  "
			if m.isLive(s.ID) {
				marker = "● "
			}
			label := padRight(cursor+marker+sessionLabel(s.ID, s.EventCount, w-4), w)
			if m.focusedPane == paneSessions && i == m.sessionCursor {
				label = focusMark(label, m.noColor)
			}
			lines = append(lines, label)
		}
	}

	return padBlock(lines, w, h)
}

// viewEventsPane renders the events list pane.
func (m model) viewEventsPane(w, h int) string {
	// Title carries a follow/pause indicator and, when paused, the count of
	// new events buffered below the viewport (↓ N new).
	titleText := "EVENTS"
	if m.selectedSession != "" {
		titleText = "EVENTS  " + clip(m.selectedSession, w-9)
	}
	if !m.follow && m.pendingCount > 0 {
		badge := fmt.Sprintf("↓ %d new", m.pendingCount)
		pad := w - len([]rune(titleText)) - len([]rune(badge))
		if pad < 1 {
			pad = 1
		}
		titleText = titleText + strings.Repeat(" ", pad) + badge
	}
	lines := []string{padRight(titleText, w)}

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
		// Header row.
		lines = append(lines, renderEventRowHeader(w))
		for i, ev := range m.events {
			row := buildEventRow(ev)
			selected := i == m.eventCursor
			rendered := renderEventRow(row, w, m.noColor, selected)
			if m.focusedPane == paneEvents && selected {
				rendered = focusMark(rendered, m.noColor)
			}
			lines = append(lines, padRight(rendered, w))
		}
	}

	return padBlock(lines, w, h)
}

// viewInspectorPane renders the inspector pane.
func (m model) viewInspectorPane(w, h int) string {
	title := padRight("INSPECTOR", w)
	lines := []string{title}

	ev := m.selectedEvent()
	if ev == nil {
		lines = append(lines, "")
		lines = append(lines, clip("Select an event to inspect.", w))
		return padBlock(lines, w, h)
	}

	// Key-facts header.
	lines = append(lines, padRight(fmt.Sprintf("Hook:    %s", ev.HookEvent), w))
	lines = append(lines, padRight(fmt.Sprintf("Tool:    %s", ev.ToolName), w))
	lines = append(lines, padRight(fmt.Sprintf("Session: %s", clip(ev.SessionID, w-9)), w))
	lines = append(lines, padRight(fmt.Sprintf("Time:    %s", ev.CapturedAt.Local().Format("2006-01-02 15:04:05")), w))
	lines = append(lines, padRight(fmt.Sprintf("Seq:     %d", ev.Seq), w))
	lines = append(lines, strings.Repeat("─", w))

	// Foldable JSON tree (flat for Phase 5 — folding is Phase 7).
	lines = append(lines, padRight("Raw JSON:", w))
	jsonLines := formatJSONLines(ev.Raw, w)
	lines = append(lines, jsonLines...)

	// Apply scroll offset.
	startLine := 1 // keep header visible
	jsonStart := 8 // lines above json content
	scrollable := lines[jsonStart:]
	if m.inspectorScroll > 0 && m.inspectorScroll < len(scrollable) {
		scrollable = scrollable[m.inspectorScroll:]
	}
	visible := append(lines[:startLine], lines[1:jsonStart]...)
	visible = append(visible, scrollable...)

	return padBlock(visible, w, h)
}

// viewHelp renders the ? help overlay.
func (m model) viewHelp() string {
	lines := []string{
		padRight("HELP — cchv keyboard shortcuts", m.width),
		padRight("", m.width),
		padRight("  Tab / 1 2 3    Switch pane focus", m.width),
		padRight("  j / k          Move cursor up/down", m.width),
		padRight("  Ctrl-d / Ctrl-u  Page down / page up", m.width),
		padRight("  g / G          Jump to first / last", m.width),
		padRight("  Enter / l      Select / go deeper", m.width),
		padRight("  Esc / h        Go back", m.width),
		padRight("  f              Toggle tail-follow (live)", m.width),
		padRight("  G              Jump to latest & re-follow", m.width),
		padRight("  space          Pause follow", m.width),
		padRight("  r              Raw JSON pager (inspector)", m.width),
		padRight("  q              Quit", m.width),
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

// getLine returns lines[i] padded to w, or a blank padded line if out of bounds.
func getLine(lines []string, i, w int) string {
	if i >= len(lines) {
		return strings.Repeat(" ", w)
	}
	return padRight(lines[i], w)
}

// padBlock pads a slice of lines to exactly h lines of width w.
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

// focusMark applies visual focus highlight to a line. In noColor mode this
// adds a bold "[*]" prefix. In color mode it uses reverse-video via lipgloss.
func focusMark(line string, noColor bool) string {
	if noColor {
		return line
	}
	return lipgloss.NewStyle().Reverse(true).Render(line)
}
