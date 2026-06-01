package tui

// Phase 8 tests: two-column slide nav, stable selection idiom, fixed-width
// columns, viewport scrolling, type-aware truncation, title-led session rows,
// and NO_COLOR degradation. Written TDD — these fail until implementation lands.

import (
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// ============================================================
// Helpers
// ============================================================

// phase8Model returns a model loaded with enough state to exercise Phase 8
// rendering: sessions with Title + CWD, a rich event set, wide layout.
func phase8Model() model {
	t0 := time.Date(2026, 5, 29, 14, 0, 0, 0, time.UTC)
	sessions := []store.SessionInfo{
		{
			ID:         "sess-a",
			EventCount: 214,
			LastSeq:    214,
			ModTime:    t0,
			CWD:        "/home/user/workspace/harness-visualizer",
			Title:      "Improve TUI design and UX issues",
		},
		{
			ID:         "sess-b",
			EventCount: 57,
			LastSeq:    57,
			ModTime:    t0.Add(-3 * time.Minute),
			CWD:        "/home/user/workspace/harness-visualizer",
			Title:      "Fix daemon reconnect race",
		},
		{
			ID:         "sess-c",
			EventCount: 8,
			LastSeq:    8,
			ModTime:    t0.Add(-48 * time.Hour),
			CWD:        "/home/user/workspace/dotfiles",
			Title:      "Dotfile symlink cleanup",
		},
	}

	evs := makePhase8Events(t0)
	fake := &FakeClient{
		Sessions_: sessions,
		Events_:   map[string][]*event.Event{"sess-a": evs},
	}
	m := newModel(fake, false, false)
	m.sessions = sessions
	m.selectedSession = "sess-a"
	m.events = evs
	m.focusedPane = paneEvents
	m.width = 110
	m.height = 30
	m.layout = LayoutBrowse
	m.now = func() time.Time { return t0 }
	return m
}

// makePhase8Events returns a diverse event slice for rendering tests.
func makePhase8Events(t0 time.Time) []*event.Event {
	return []*event.Event{
		{
			ID: "e1", Seq: 1, HookEvent: "SessionStart", SessionID: "sess-a",
			CapturedAt: t0.Add(1 * time.Second),
		},
		{
			ID: "e2", Seq: 2, HookEvent: "PreToolUse", ToolName: "Read", SessionID: "sess-a",
			CapturedAt: t0.Add(2 * time.Second),
			Raw:        jsonRaw(`{"tool_use_id":"uid-1","tool_input":{"file_path":"internal/auth/middleware.go"}}`),
		},
		{
			ID: "e3", Seq: 3, HookEvent: "PostToolUse", ToolName: "Read", SessionID: "sess-a",
			CapturedAt: t0.Add(2*time.Second + 12*time.Millisecond),
			Raw:        jsonRaw(`{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`),
		},
		{
			ID: "e4", Seq: 4, HookEvent: "PreToolUse", ToolName: "Edit", SessionID: "sess-a",
			CapturedAt: t0.Add(7 * time.Second),
			Raw:        jsonRaw(`{"tool_use_id":"uid-2","tool_input":{"file_path":"internal/auth/middleware.go"}}`),
		},
		{
			ID: "e5", Seq: 5, HookEvent: "PostToolUse", ToolName: "Edit", SessionID: "sess-a",
			CapturedAt: t0.Add(7*time.Second + 140*time.Millisecond),
			Raw:        jsonRaw(`{"tool_use_id":"uid-2","tool_response":{"exit_code":0}}`),
		},
		{
			ID: "e6", Seq: 6, HookEvent: "PreToolUse", ToolName: "Bash", SessionID: "sess-a",
			CapturedAt: t0.Add(8 * time.Second),
			Raw:        jsonRaw(`{"tool_use_id":"uid-3","tool_input":{"command":"go test ./internal/auth/..."}}`),
		},
		{
			ID: "e7", Seq: 7, HookEvent: "PostToolUse", ToolName: "Bash", SessionID: "sess-a",
			CapturedAt: t0.Add(8*time.Second + 1800*time.Millisecond),
			Raw:        jsonRaw(`{"tool_use_id":"uid-3","tool_response":{"exit_code":1}}`),
		},
		{
			ID: "e8", Seq: 8, HookEvent: "Stop", SessionID: "sess-a",
			CapturedAt: t0.Add(18 * time.Second),
		},
	}
}

func jsonRaw(s string) []byte { return []byte(s) }

// ============================================================
// 8a — Two-column slide navigation
// ============================================================

func TestLayoutBrowseAndDrillExist(t *testing.T) {
	// Both layout constants must exist and be distinct.
	if LayoutBrowse == LayoutDrill {
		t.Error("LayoutBrowse and LayoutDrill must be distinct")
	}
}

func TestChooseLayoutBrowseAt80(t *testing.T) {
	// ≥80 cols, ≥minRows → LayoutBrowse (default depth state).
	got := chooseLayout(80, 24)
	if got != LayoutBrowse {
		t.Errorf("chooseLayout(80, 24) = %v, want LayoutBrowse", got)
	}
}

func TestChooseLayoutBrowseAt119(t *testing.T) {
	got := chooseLayout(119, 24)
	if got != LayoutBrowse {
		t.Errorf("chooseLayout(119, 24) = %v, want LayoutBrowse", got)
	}
}

func TestChooseLayoutBrowseAtWide(t *testing.T) {
	// Even ≥120 cols → LayoutBrowse (no separate Wide 3-column mode).
	got := chooseLayout(140, 24)
	if got != LayoutBrowse {
		t.Errorf("chooseLayout(140, 24) = %v, want LayoutBrowse (no 3-col mode)", got)
	}
}

func TestChooseLayoutNarrowBelow80(t *testing.T) {
	got := chooseLayout(79, 24)
	if got != LayoutNarrow {
		t.Errorf("chooseLayout(79, 24) = %v, want LayoutNarrow", got)
	}
}

func TestChooseLayoutTooSmall(t *testing.T) {
	if chooseLayout(30, 5) != LayoutTooSmall {
		t.Error("30×5 must be LayoutTooSmall")
	}
}

// Enter on a session focuses Events (stays in Browse, depth stays Browse).
func TestEnterOnSessionFocusesEventsStaysBrowse(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneSessions
	m.depthDrill = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)

	if m2.focusedPane != paneEvents {
		t.Errorf("Enter on session: focus = %v, want paneEvents", m2.focusedPane)
	}
	if m2.depthDrill {
		t.Error("Enter on session must not enter Drill depth")
	}
}

// Enter on an event enters Drill (Sessions hide, Inspector reveals).
func TestEnterOnEventEntersDrill(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.depthDrill = false
	m.eventCursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)

	if !m2.depthDrill {
		t.Error("Enter on event must enter Drill depth")
	}
	if m2.focusedPane != paneInspector {
		t.Errorf("Enter on event: focus = %v, want paneInspector", m2.focusedPane)
	}
}

// l on an event from the events pane also enters Drill.
func TestLOnEventEntersDrill(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.depthDrill = false
	m.eventCursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m2 := updated.(model)

	if !m2.depthDrill {
		t.Error("l on event must enter Drill depth")
	}
}

// Esc from Inspector → Events (back to Browse).
func TestEscFromInspectorPopsToEvents(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneInspector
	m.depthDrill = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(model)

	if m2.depthDrill {
		t.Error("Esc from inspector must leave Drill depth")
	}
	if m2.focusedPane != paneEvents {
		t.Errorf("Esc from inspector: focus = %v, want paneEvents", m2.focusedPane)
	}
}

// Esc from Events → Sessions (still Browse, just shifts focus).
func TestEscFromEventsPopsToSessions(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.depthDrill = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(model)

	if m2.focusedPane != paneSessions {
		t.Errorf("Esc from events: focus = %v, want paneSessions", m2.focusedPane)
	}
	if m2.depthDrill {
		t.Error("Esc from events must stay in Browse depth")
	}
}

// Esc from Sessions → stops (no further back, no quit).
func TestEscFromSessionsDoesNotQuit(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneSessions
	m.depthDrill = false

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(model)

	if m2.focusedPane != paneSessions {
		t.Errorf("Esc from sessions: focus shifted, got %v", m2.focusedPane)
	}
	if cmd != nil {
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("Esc must never quit")
		}
	}
}

// h key behaves identically to Esc in each context.
func TestHKeyActsAsEscFromInspector(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneInspector
	m.depthDrill = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m2 := updated.(model)

	if m2.depthDrill {
		t.Error("h from inspector must exit Drill")
	}
	if m2.focusedPane != paneEvents {
		t.Errorf("h from inspector: focus = %v, want paneEvents", m2.focusedPane)
	}
}

// 3 from Browse enters Drill and focuses Inspector.
func TestKey3FromBrowseEntersDrill(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.depthDrill = false
	m.eventCursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("3")})
	m2 := updated.(model)

	if !m2.depthDrill {
		t.Error("3 from Browse must enter Drill")
	}
	if m2.focusedPane != paneInspector {
		t.Errorf("3 from Browse: focus = %v, want paneInspector", m2.focusedPane)
	}
}

// 1 from Drill pops back to Browse and focuses Sessions.
func TestKey1FromDrillPopsToSessions(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneInspector
	m.depthDrill = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("1")})
	m2 := updated.(model)

	if m2.depthDrill {
		t.Error("1 from Drill must exit Drill")
	}
	if m2.focusedPane != paneSessions {
		t.Errorf("1 from Drill: focus = %v, want paneSessions", m2.focusedPane)
	}
}

// 2 focuses Events in either depth.
func TestKey2FocusesEventsInBrowse(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneSessions
	m.depthDrill = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m2 := updated.(model)
	if m2.focusedPane != paneEvents {
		t.Errorf("2 in Browse: focus = %v, want paneEvents", m2.focusedPane)
	}
}

func TestKey2FocusesEventsInDrill(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneInspector
	m.depthDrill = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("2")})
	m2 := updated.(model)
	if m2.focusedPane != paneEvents {
		t.Errorf("2 in Drill: focus = %v, want paneEvents", m2.focusedPane)
	}
}

// Tab in Browse cycles Sessions ↔ Events only.
func TestTabInBrowseCyclesSessionsEvents(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneSessions
	m.depthDrill = false

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(model)
	if m2.focusedPane != paneEvents {
		t.Errorf("Tab from Sessions in Browse: focus = %v, want paneEvents", m2.focusedPane)
	}

	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := updated2.(model)
	if m3.focusedPane != paneSessions {
		t.Errorf("Tab from Events in Browse: focus = %v, want paneSessions", m3.focusedPane)
	}
}

// Tab in Drill cycles Events ↔ Inspector only.
func TestTabInDrillCyclesEventsInspector(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.depthDrill = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(model)
	if m2.focusedPane != paneInspector {
		t.Errorf("Tab from Events in Drill: focus = %v, want paneInspector", m2.focusedPane)
	}

	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := updated2.(model)
	if m3.focusedPane != paneEvents {
		t.Errorf("Tab from Inspector in Drill: focus = %v, want paneEvents", m3.focusedPane)
	}
}

// Narrow (<80) stacked layout: Enter on session keeps single-pane stacked.
func TestNarrowEnterOnSessionFocusesEvents(t *testing.T) {
	m := phase8Model()
	m.width = 70
	m.height = 24
	m.layout = LayoutNarrow
	m.focusedPane = paneSessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)
	if m2.focusedPane != paneEvents {
		t.Errorf("narrow Enter on session: focus = %v, want paneEvents", m2.focusedPane)
	}
}

// Narrow: Enter on event goes to inspector pane.
func TestNarrowEnterOnEventGoesToInspector(t *testing.T) {
	m := phase8Model()
	m.width = 70
	m.height = 24
	m.layout = LayoutNarrow
	m.focusedPane = paneEvents
	m.eventCursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)
	if m2.focusedPane != paneInspector {
		t.Errorf("narrow Enter on event: focus = %v, want paneInspector", m2.focusedPane)
	}
}

// ============================================================
// 8b — Stable single-selection idiom (no reverse-video, no prefix shift)
// ============================================================

// renderEventRow must produce identical rune-width for selected vs unselected.
func TestSelectionRenderStability(t *testing.T) {
	row := eventRow{
		Time:     "14:02:07",
		Status:   statusOK,
		Hook:     "PreToolUse·Post",
		Tool:     "Edit",
		Gist:     "internal/auth/middleware.go",
		Duration: "140ms",
	}
	const width = 100
	notSelected := renderEventRow(row, width, false, false)
	isSelected := renderEventRow(row, width, false, true)

	wNot := utf8.RuneCountInString(stripANSI(notSelected))
	wSel := utf8.RuneCountInString(stripANSI(isSelected))
	if wNot != wSel {
		t.Errorf("selection changes row width: unselected=%d selected=%d\nnot: %q\nsel: %q",
			wNot, wSel, notSelected, isSelected)
	}
}

// The selected row must NOT contain a "> " prefix.
func TestNoGTPrefix(t *testing.T) {
	row := eventRow{
		Time:   "14:02:07",
		Status: statusOK,
		Hook:   "PostToolUse",
		Tool:   "Edit",
	}
	rendered := renderEventRow(row, 80, false, true)
	if strings.Contains(rendered, "> ") {
		t.Errorf("selected row must not contain '> ' prefix, got %q", rendered)
	}
}

// The selected row must contain the caret glyph ▸ (or » in NO_COLOR).
func TestSelectedRowHasCaret(t *testing.T) {
	row := eventRow{
		Time:   "14:02:07",
		Status: statusOK,
		Hook:   "PostToolUse",
		Tool:   "Edit",
	}
	rendered := renderEventRow(row, 80, false, true)
	if !strings.Contains(rendered, "▸") {
		t.Errorf("selected row must contain caret ▸, got %q", rendered)
	}
}

// The unselected row must have a blank in the caret position (same width).
func TestUnselectedRowHasBlankCaret(t *testing.T) {
	row := eventRow{
		Time:   "14:02:07",
		Status: statusOK,
		Hook:   "PostToolUse",
		Tool:   "Edit",
	}
	rendered := renderEventRow(row, 80, false, false)
	// Must not contain the caret glyph.
	if strings.Contains(rendered, "▸") {
		t.Errorf("unselected row must not contain caret ▸, got %q", rendered)
	}
}

// NO_COLOR selected row uses » (or >) caret, no ANSI escapes.
func TestNoColorSelectedRowCaret(t *testing.T) {
	row := eventRow{
		Time:   "14:02:07",
		Status: statusOK,
		Hook:   "PostToolUse",
		Tool:   "Edit",
	}
	rendered := renderEventRow(row, 80, true, true)
	// Must not contain ANSI escapes.
	for _, ch := range rendered {
		if ch == '\x1b' {
			t.Errorf("NO_COLOR selected row must not contain ANSI, got %q", rendered)
			return
		}
	}
	// Must contain a caret marker (» or >).
	if !strings.ContainsAny(rendered, "»>") {
		t.Errorf("NO_COLOR selected row must contain » or >, got %q", rendered)
	}
}

// Header row has the same width as data rows (caret gutter reserved).
func TestHeaderRowSameWidthAsDataRow(t *testing.T) {
	const w = 100
	header := renderEventRowHeader(w)
	row := eventRow{
		Time: "14:02:07", Status: statusOK,
		Hook: "PostToolUse", Tool: "Edit",
	}
	dataRow := renderEventRow(row, w, false, false)

	hW := utf8.RuneCountInString(stripANSI(header))
	dW := utf8.RuneCountInString(stripANSI(dataRow))
	if hW != dW {
		t.Errorf("header width %d != data row width %d\nheader:  %q\ndata:    %q",
			hW, dW, header, dataRow)
	}
}

// ============================================================
// 8c — Fixed-width columns + scroll viewport
// ============================================================

// With more rows than height, moving the cursor to the last row must keep it
// visible (viewTop scrolls so cursor is within [top, top+visibleH)).
func TestViewportKeepsCursorVisible(t *testing.T) {
	m := phase8Model()
	m.focusedPane = paneEvents
	m.layout = LayoutBrowse

	// Make height small enough that not all rows fit.
	m.height = 10 // 2 rows for bars → 8 content rows; 2 header → 6 event rows

	rows := m.visibleRows()
	if len(rows) < 2 {
		t.Skip("need at least 2 visible rows")
	}

	// Move cursor to last row.
	for range rows {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(model)
	}

	// After scrolling to the end the cursor must equal len(rows)-1.
	if m.eventCursor != len(rows)-1 {
		t.Errorf("cursor = %d, want %d (last row)", m.eventCursor, len(rows)-1)
	}

	// The eventTop viewport must place the cursor within the visible window.
	contentH := m.height - 2 // status + key bars
	paneH := contentH - 1    // breadcrumb/pane-title row
	headerH := 1
	visibleH := paneH - headerH
	if visibleH < 1 {
		visibleH = 1
	}
	if m.eventTop > m.eventCursor || m.eventCursor >= m.eventTop+visibleH {
		t.Errorf("cursor %d not in viewport [%d, %d)",
			m.eventCursor, m.eventTop, m.eventTop+visibleH)
	}
}

// Session viewport: moving cursor past the bottom scrolls sessionTop.
func TestSessionViewportKeepsCursorVisible(t *testing.T) {
	// Build a model with many sessions and a small pane height.
	sessions := make([]store.SessionInfo, 15)
	for i := range sessions {
		sessions[i] = store.SessionInfo{
			ID:    "s" + string(rune('a'+i)),
			Title: "Session " + string(rune('A'+i)),
			CWD:   "/repo/proj",
		}
	}
	m := phase8Model()
	m.sessions = sessions
	m.sessionCursor = 0
	m.sessionTop = 0
	m.focusedPane = paneSessions
	m.height = 10 // pane height very small

	// Move cursor to the last session.
	for range sessions {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(model)
	}

	if m.sessionCursor != len(sessions)-1 {
		t.Errorf("sessionCursor = %d, want %d", m.sessionCursor, len(sessions)-1)
	}
	// sessionTop must have scrolled.
	if m.sessionTop == 0 && len(sessions) > 3 {
		t.Errorf("sessionTop should have scrolled, still 0 with %d sessions", len(sessions))
	}
}

// ============================================================
// 8e — Type-aware truncation helpers
// ============================================================

// --- paths ---

func TestTruncatePathKeepsBasenameShort(t *testing.T) {
	// Fits whole — no truncation marker.
	got := truncateSmart("middleware.go", 30, false)
	if got != "middleware.go" {
		t.Errorf("short path: got %q, want unchanged", got)
	}
}

func TestTruncatePathMiddleElision(t *testing.T) {
	// Long path: middle segments elided, basename preserved.
	path := "internal/auth/middleware.go"
	got := truncateSmart(path, 20, false)

	// Basename must survive.
	if !strings.HasSuffix(stripANSI(got), "middleware.go") {
		t.Errorf("basename not preserved in %q", got)
	}
	// Must contain the ellipsis marker.
	if !strings.Contains(stripANSI(got), "…") && !strings.Contains(stripANSI(got), "~") {
		t.Errorf("middle-elided path must contain … or ~, got %q", got)
	}
	// Must not exceed width.
	if utf8.RuneCountInString(stripANSI(got)) > 20 {
		t.Errorf("truncated path too long: %d runes in %q", utf8.RuneCountInString(stripANSI(got)), got)
	}
}

func TestTruncatePathLeadingElisionWhenVeryNarrow(t *testing.T) {
	// Very narrow: only …/basename fits.
	path := "internal/daemon/handlers/sessions_handler.go"
	got := truncateSmart(path, 22, false)

	stripped := stripANSI(got)
	// Basename must be present.
	if !strings.HasSuffix(stripped, "sessions_handler.go") {
		t.Errorf("basename lost in narrow truncation: %q", got)
	}
	if utf8.RuneCountInString(stripped) > 22 {
		t.Errorf("narrow path too long: %d > 22 in %q", utf8.RuneCountInString(stripped), got)
	}
}

func TestTruncatePathBasenameNeverSacrificed(t *testing.T) {
	// Even when the basename itself is longer than width, we still show
	// as much of it as possible with the marker at the start.
	path := "internal/auth/this_is_a_very_long_filename_indeed.go"
	got := truncateSmart(path, 15, false)
	stripped := stripANSI(got)
	// The .go extension should still be present.
	if !strings.Contains(stripped, ".go") {
		t.Errorf("path truncation should preserve extension, got %q", got)
	}
	if utf8.RuneCountInString(stripped) > 15 {
		t.Errorf("path still too long: %d > 15 in %q", utf8.RuneCountInString(stripped), got)
	}
}

// --- commands ---

func TestTruncateCommandKeepsHead(t *testing.T) {
	cmd := "go test ./internal/auth/..."
	got := truncateSmart(cmd, 20, false)
	stripped := stripANSI(got)
	if !strings.HasPrefix(stripped, "go test") {
		t.Errorf("command truncation must keep head, got %q", got)
	}
	if !strings.Contains(stripped, "…") && !strings.Contains(stripped, "~") {
		t.Errorf("command truncation must have trailing ellipsis, got %q", got)
	}
	if utf8.RuneCountInString(stripped) > 20 {
		t.Errorf("command too long: %d > 20 in %q", utf8.RuneCountInString(stripped), got)
	}
}

// --- quoted strings ---

func TestTruncateQuotedPreservesCloseQuote(t *testing.T) {
	q := `"func.*Handler regex pattern"`
	got := truncateSmart(q, 18, false)
	stripped := stripANSI(got)
	// Must end with closing quote or ellipsis-then-quote or be unchanged.
	if !strings.HasSuffix(stripped, `"`) && !strings.HasSuffix(stripped, `…"`) &&
		stripped != q {
		t.Errorf("quoted truncation must preserve close-quote, got %q", got)
	}
	if utf8.RuneCountInString(stripped) > 18 {
		t.Errorf("quoted string too long: %d > 18 in %q", utf8.RuneCountInString(stripped), got)
	}
}

// --- NO_COLOR ellipsis fallback ---

func TestNoColorEllipsisFallback(t *testing.T) {
	// In NO_COLOR mode the marker must be a plain glyph (no ANSI).
	path := "internal/auth/middleware.go"
	got := truncateSmart(path, 15, true /* noColor */)
	for _, ch := range got {
		if ch == '\x1b' {
			t.Errorf("NO_COLOR truncation must not contain ANSI, got %q", got)
			return
		}
	}
}

// TestNoColorMarkerIsTilde asserts that noColor=true produces "~" as the marker
// instead of "…" — both for a path (interior/leading elision) and for a
// head-keep (trailing) case.
func TestNoColorMarkerIsTilde(t *testing.T) {
	// Path: interior/leading elision marker must be "~", not "…".
	path := "internal/daemon/handlers/sessions_handler.go"
	gotPath := truncateSmart(path, 22, true)
	if strings.Contains(gotPath, "…") {
		t.Errorf("noColor path: must not contain '…', got %q", gotPath)
	}
	if !strings.Contains(gotPath, "~") {
		t.Errorf("noColor path: must contain '~' marker, got %q", gotPath)
	}

	// Head-keep (command): trailing marker must be "~", not "…".
	cmd := "go test ./internal/auth/..."
	gotCmd := truncateSmart(cmd, 20, true)
	if strings.Contains(gotCmd, "…") {
		t.Errorf("noColor command: must not contain '…', got %q", gotCmd)
	}
	if !strings.Contains(gotCmd, "~") {
		t.Errorf("noColor command: must contain '~' marker, got %q", gotCmd)
	}
}

// TestColorMarkerIsEllipsis confirms the default (noColor=false) path still
// produces "…" and never "~".
func TestColorMarkerIsEllipsis(t *testing.T) {
	path := "internal/daemon/handlers/sessions_handler.go"
	got := truncateSmart(path, 22, false)
	if strings.Contains(got, "~") {
		t.Errorf("color mode must not use '~', got %q", got)
	}
	if !strings.Contains(stripANSI(got), "…") {
		t.Errorf("color mode must use '…' marker, got %q", got)
	}

	cmd := "go test ./internal/auth/..."
	gotCmd := truncateSmart(cmd, 20, false)
	if strings.Contains(gotCmd, "~") {
		t.Errorf("color mode command must not use '~', got %q", gotCmd)
	}
	if !strings.Contains(stripANSI(gotCmd), "…") {
		t.Errorf("color mode command must use '…' marker, got %q", gotCmd)
	}
}

// --- fits whole (no marker) ---

func TestTruncateNoMarkerWhenFits(t *testing.T) {
	s := "short.go"
	got := truncateSmart(s, 30, false)
	if strings.Contains(stripANSI(got), "…") || strings.Contains(stripANSI(got), "~") {
		t.Errorf("short value must have no truncation marker, got %q", got)
	}
}

// ============================================================
// 8d — Title-led session rows
// ============================================================

func TestSessionRowShowsTitle(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	paneStr := m.viewSessionsPane(36, 20)
	if !strings.Contains(paneStr, "Improve TUI design") {
		t.Errorf("session pane must show title, got:\n%s", paneStr)
	}
}

func TestSessionRowShowsProject(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	paneStr := m.viewSessionsPane(36, 20)
	// project = filepath.Base(CWD) = "harness-visualizer"
	if !strings.Contains(paneStr, "harness-visualizer") {
		t.Errorf("session pane must show project name, got:\n%s", paneStr)
	}
}

func TestSessionRowShowsRecency(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	paneStr := m.viewSessionsPane(36, 20)
	// sess-a is at t0 (same as now) → "live" or "0s"
	// sess-c is 48h ago → "2d"
	if !strings.Contains(paneStr, "live") && !strings.Contains(paneStr, "0s") {
		t.Errorf("session pane must show live/recency for recent session, got:\n%s", paneStr)
	}
}

func TestSessionRowFallbackWhenNoTitle(t *testing.T) {
	m := phase8Model()
	// Session with no title — should fall back gracefully (project·shortid).
	m.sessions[0].Title = ""
	paneStr := m.viewSessionsPane(36, 20)
	if paneStr == "" {
		t.Error("session pane must not be empty when title is blank")
	}
}

// ============================================================
// 8f — NO_COLOR rendering carries through
// ============================================================

func TestNoColorViewNoPanic(t *testing.T) {
	m := phase8Model()
	m.noColor = true
	m.reducedMotion = true
	m.layout = LayoutBrowse
	view := m.View()
	if view == "" {
		t.Error("NO_COLOR view must not be empty")
	}
	for _, ch := range view {
		if ch == '\x1b' {
			t.Error("NO_COLOR view must not contain ANSI escapes")
			return
		}
	}
}

func TestNoColorSessionsPane(t *testing.T) {
	m := phase8Model()
	m.noColor = true
	m.layout = LayoutBrowse
	pane := m.viewSessionsPane(36, 20)
	for _, ch := range pane {
		if ch == '\x1b' {
			t.Error("NO_COLOR sessions pane must not contain ANSI, got escape")
			return
		}
	}
}

// ============================================================
// Browse/Drill view rendering (smoke + layout tests)
// ============================================================

func TestViewBrowseRenders(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = false
	view := m.View()
	if view == "" {
		t.Error("Browse view must not be empty")
	}
	// Sessions pane should be visible in Browse.
	if !strings.Contains(view, "SESSIONS") {
		t.Errorf("Browse view must contain SESSIONS pane, got:\n%s", view)
	}
}

func TestViewDrillRenders(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = true
	view := m.View()
	if view == "" {
		t.Error("Drill view must not be empty")
	}
	// Inspector pane should be visible in Drill.
	if !strings.Contains(view, "INSPECTOR") {
		t.Errorf("Drill view must contain INSPECTOR pane, got:\n%s", view)
	}
	// Sessions pane should NOT be visible in Drill.
	if strings.Contains(view, "SESSIONS") {
		t.Errorf("Drill view must NOT contain SESSIONS pane, got:\n%s", view)
	}
}

// humanizeAge helper tests.
func TestHumanizeAgeLive(t *testing.T) {
	got := humanizeAge(0)
	if got != "live" {
		t.Errorf("humanizeAge(0) = %q, want live", got)
	}
}

func TestHumanizeAgeSeconds(t *testing.T) {
	got := humanizeAge(30 * time.Second)
	if got != "30s" {
		t.Errorf("humanizeAge(30s) = %q, want 30s", got)
	}
}

func TestHumanizeAgeMinutes(t *testing.T) {
	got := humanizeAge(5 * time.Minute)
	if got != "5m" {
		t.Errorf("humanizeAge(5m) = %q, want 5m", got)
	}
}

func TestHumanizeAgeDays(t *testing.T) {
	got := humanizeAge(48 * time.Hour)
	if got != "2d" {
		t.Errorf("humanizeAge(2d) = %q, want 2d", got)
	}
}

// ============================================================
// 8-visual — Color/ANSI regression tests (Phase 8 visual layer)
// ============================================================

// TestANSIWidthEveryLineBrowse asserts that every line of a rendered Browse
// frame has lipgloss.Width == model.width (no over/under-wide lines from ANSI
// escape miscounting in padRight/padBlock/getLine).
func TestANSIWidthEveryLineBrowse(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = false
	m.foldedView = true
	m.eventCursor = 2
	m.daemonOK = true
	m.streamUp = true

	view := m.View()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		got := ansiWidth(line)
		if got != m.width {
			t.Errorf("Browse line %d: lipgloss.Width = %d, want %d\n  raw: %q",
				i, got, m.width, line)
		}
	}
}

// TestANSIWidthEveryLineDrill asserts that every line of a rendered Drill frame
// has lipgloss.Width == model.width.
func TestANSIWidthEveryLineDrill(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = true
	m.focusedPane = paneInspector
	m.foldedView = true
	m.eventCursor = 2
	m.daemonOK = true
	m.streamUp = true

	view := m.View()
	lines := strings.Split(view, "\n")
	for i, line := range lines {
		got := ansiWidth(line)
		if got != m.width {
			t.Errorf("Drill line %d: lipgloss.Width = %d, want %d\n  raw: %q",
				i, got, m.width, line)
		}
	}
}

// TestSelectedRowBandResetBeforeDivider asserts that every selection band in the
// rendered Browse frame closes with a reset (^[[0m) within the same display
// line. This guarantees the band cannot bleed into adjacent panes or across
// the │ divider — regardless of which pane holds the band.
//
// Design: in Browse, both the sessions pane AND the events pane may show a band
// on the same physical line (one left of │, one right of │). Each must reset
// within its own pane segment. We verify this by checking that every band opener
// has a matching reset somewhere later on the same line.
func TestSelectedRowBandResetBeforeDivider(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = false
	m.foldedView = true
	m.focusedPane = paneEvents
	m.eventCursor = 2

	view := m.View()
	lines := strings.Split(view, "\n")

	const bandEscFocused = "\x1b[48;5;236m"
	const bandEscUnfocused = "\x1b[48;5;235m"
	resetStr := "\x1b[0m"

	foundBand := false
	for i, line := range lines {
		if !strings.Contains(line, bandEscFocused) && !strings.Contains(line, bandEscUnfocused) {
			continue
		}
		foundBand = true
		// Every band on this line must have a reset later on the same line.
		// Scan left-to-right: when we see a band opener, track whether a reset follows.
		rest := line
		pos := 0
		for {
			f := strings.Index(rest, bandEscFocused)
			u := strings.Index(rest, bandEscUnfocused)
			// Pick the earlier opener.
			bandIdx := -1
			bandLen := 0
			if f >= 0 && (u < 0 || f < u) {
				bandIdx = f
				bandLen = len(bandEscFocused)
			} else if u >= 0 {
				bandIdx = u
				bandLen = len(bandEscUnfocused)
			}
			if bandIdx < 0 {
				break
			}
			// Check for reset after this band opener.
			after := rest[bandIdx+bandLen:]
			if !strings.Contains(after, resetStr) {
				t.Errorf("Line %d (pos %d): band opener has no reset in remainder of line\n  raw: %q",
					i, pos+bandIdx, line)
			}
			rest = rest[bandIdx+bandLen:]
			pos += bandIdx + bandLen
		}
	}
	if !foundBand {
		t.Error("expected to find at least one selection band; none found")
	}
}

// TestSuccessColorAppearedInEvents asserts that a rendered events pane with an
// OK row contains the success-token escape sequence (proves glyphs are colored).
func TestSuccessColorAppearedInEvents(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.foldedView = true
	m.focusedPane = paneEvents

	tok := defaultTheme()
	// Build a reference success-colored glyph.
	successRendered := tok.success.Render(" ✔ ")

	sw, ew, _ := paneWidths(LayoutBrowse, m.width)
	_ = sw
	eventsPane := m.viewEventsPane(ew, m.height-2)

	if !strings.Contains(eventsPane, successRendered) {
		// Fallback: at least one ^[[92m (bright-green, ANSI color 10 = 92 in 8-color) escape.
		if !strings.Contains(eventsPane, "\x1b[92m") &&
			!strings.Contains(eventsPane, "\x1b[32m") {
			t.Error("events pane should contain success-token colored glyph (✔ in green)")
		}
	}
}

// TestEventsEscapeCountAboveThreshold asserts that a rendered Browse frame
// contains significantly more ANSI escapes than the old uncolored baseline (2).
// This proves that semantic colors are actually being applied throughout.
func TestEventsEscapeCountAboveThreshold(t *testing.T) {
	m := phase8Model()
	m.layout = LayoutBrowse
	m.depthDrill = false
	m.foldedView = true
	m.daemonOK = true
	m.streamUp = true

	view := m.View()
	count := strings.Count(view, "\x1b[")
	const minEscapes = 30 // was 2 before coloring; should now be 30+
	if count < minEscapes {
		t.Errorf("Browse frame has only %d ANSI escapes, want ≥%d (coloring not applied)", count, minEscapes)
	}
}

// TestNoColorModeSuppressesAllEscapes asserts that noColor=true produces no ANSI
// escapes in the full Browse frame.
func TestNoColorModeSuppressesAllEscapes(t *testing.T) {
	m := phase8Model()
	m.noColor = true
	m.layout = LayoutBrowse
	m.depthDrill = false
	m.foldedView = true

	view := m.View()
	if strings.Contains(view, "\x1b[") {
		t.Error("noColor=true Browse frame must not contain any ANSI escapes")
	}
}

// ============================================================
// stripANSI helper (for width assertions in tests)
// ============================================================

// stripANSI removes ANSI escape sequences from s for plain-text width measurement.
func stripANSI(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}
