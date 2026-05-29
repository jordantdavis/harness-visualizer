package tui

// Phase 7 integration tests: filter, error-hop, folded ops in the model.
// These follow the deterministic style of model_test.go / live_test.go:
// construct model + events directly, no real network, no real clock.

import (
	"encoding/json"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// phase7Model returns a model pre-loaded with a mix of events exercising
// pairing, errors, and lifecycle events.
func phase7Model() model {
	t0 := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	evs := []*event.Event{
		// Lifecycle
		{ID: "e0", Seq: 0, HookEvent: "SessionStart", SessionID: "s1", CapturedAt: t0},
		// Paired Bash op (exit 0)
		{ID: "e1", Seq: 1, HookEvent: "PreToolUse", ToolName: "Bash", SessionID: "s1",
			CapturedAt: t0.Add(1 * time.Second),
			Raw:        json.RawMessage(`{"tool_use_id":"uid-1"}`)},
		{ID: "e2", Seq: 2, HookEvent: "PostToolUse", ToolName: "Bash", SessionID: "s1",
			CapturedAt: t0.Add(2 * time.Second),
			Raw:        json.RawMessage(`{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)},
		// Paired Read op (exit 1 → error)
		{ID: "e3", Seq: 3, HookEvent: "PreToolUse", ToolName: "Read", SessionID: "s1",
			CapturedAt: t0.Add(3 * time.Second),
			Raw:        json.RawMessage(`{"tool_use_id":"uid-2","tool_input":{"file_path":"/tmp/foo.go"}}`)},
		{ID: "e4", Seq: 4, HookEvent: "PostToolUse", ToolName: "Read", SessionID: "s1",
			CapturedAt: t0.Add(4 * time.Second),
			Raw:        json.RawMessage(`{"tool_use_id":"uid-2","tool_response":{"exit_code":1}}`)},
		// Notification (standalone)
		{ID: "e5", Seq: 5, HookEvent: "Notification", SessionID: "s1", CapturedAt: t0.Add(5 * time.Second)},
	}
	fake := fixtureClient()
	m := newModel(fake, false)
	m.selectedSession = "s1"
	m.events = evs
	m.focusedPane = paneEvents
	m.width = 120
	m.height = 40
	m.now = func() time.Time { return t0.Add(10 * time.Second) }
	return m
}

// --- folded view (default ON) -----------------------------------------------

func TestFoldedViewIsDefault(t *testing.T) {
	m := newModel(fixtureClient(), false)
	if !m.foldedView {
		t.Error("foldedView should be true by default")
	}
}

func TestFoldedViewCollapsesPrePostPairs(t *testing.T) {
	m := phase7Model()
	// 6 raw events → 4 display rows: SessionStart, Bash-pair, Read-pair, Notification
	rows := m.visibleRows()
	if len(rows) != 4 {
		t.Fatalf("folded: want 4 display rows, got %d", len(rows))
	}
	if rows[1].IsPair {
		if rows[1].Pre.ToolName != "Bash" {
			t.Errorf("row[1] should be Bash pair, got %q", rows[1].Pre.ToolName)
		}
	} else {
		t.Error("row[1] should be a paired op")
	}
}

func TestOTogglesView(t *testing.T) {
	m := phase7Model()
	if !m.foldedView {
		t.Fatal("precondition: foldedView should be true")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m2 := updated.(model)
	if m2.foldedView {
		t.Error("o should toggle foldedView off")
	}
	// Flat view: 6 raw events = 6 rows
	rows := m2.visibleRows()
	if len(rows) != 6 {
		t.Fatalf("flat: want 6 rows, got %d", len(rows))
	}
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("o")})
	m3 := updated2.(model)
	if !m3.foldedView {
		t.Error("o should toggle foldedView back on")
	}
}

func TestFoldedOpDurationPopulated(t *testing.T) {
	m := phase7Model()
	rows := m.visibleRows()
	// row[1] is the Bash pair (1s duration)
	if !rows[1].IsPair {
		t.Fatal("row[1] should be a pair")
	}
	if rows[1].Duration != 1*time.Second {
		t.Errorf("Bash pair duration = %v, want 1s", rows[1].Duration)
	}
}

// --- error-hop (e / E) -------------------------------------------------------

func TestErrorHopForward(t *testing.T) {
	m := phase7Model()
	m.eventCursor = 0 // start at SessionStart

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m2 := updated.(model)

	// The only error row is row[2] (Read pair, exit_code=1).
	rows := m2.visibleRows()
	if m2.eventCursor >= len(rows) {
		t.Fatalf("cursor out of range: %d", m2.eventCursor)
	}
	if rows[m2.eventCursor].EffectiveStatus() != statusError {
		t.Errorf("e should move cursor to error row; cursor=%d status=%v",
			m2.eventCursor, rows[m2.eventCursor].EffectiveStatus())
	}
}

func TestErrorHopBackward(t *testing.T) {
	m := phase7Model()
	m.eventCursor = 3 // Notification (last row)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("E")})
	m2 := updated.(model)

	rows := m2.visibleRows()
	if rows[m2.eventCursor].EffectiveStatus() != statusError {
		t.Errorf("E should move cursor to error row; cursor=%d status=%v",
			m2.eventCursor, rows[m2.eventCursor].EffectiveStatus())
	}
}

func TestErrorHopNoErrors(t *testing.T) {
	m := phase7Model()
	// Remove the Read error by reverting to just Bash (no error).
	t0 := time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)
	m.events = []*event.Event{
		{ID: "e1", Seq: 1, HookEvent: "PreToolUse", ToolName: "Bash", SessionID: "s1",
			CapturedAt: t0, Raw: json.RawMessage(`{"tool_use_id":"uid-1"}`)},
		{ID: "e2", Seq: 2, HookEvent: "PostToolUse", ToolName: "Bash", SessionID: "s1",
			CapturedAt: t0.Add(time.Second),
			Raw:        json.RawMessage(`{"tool_use_id":"uid-1","tool_response":{"exit_code":0}}`)},
	}
	m.eventCursor = 0
	initialCursor := m.eventCursor
	// Pressing e with no errors should not crash or move cursor to an invalid row.
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("e")})
	m2 := updated.(model)
	_ = initialCursor // cursor may stay put — just must not crash
	rows := m2.visibleRows()
	if m2.eventCursor < 0 || m2.eventCursor >= max(1, len(rows)) {
		t.Errorf("cursor out of range after e with no errors: %d", m2.eventCursor)
	}
}

func TestErrorCount(t *testing.T) {
	m := phase7Model()
	// phase7Model has 1 error (Read pair exit_code=1)
	if got := m.errorCount(); got != 1 {
		t.Errorf("errorCount = %d, want 1", got)
	}
}

// --- filter input mode -------------------------------------------------------

func TestSlashEntersFilterMode(t *testing.T) {
	m := phase7Model()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m2 := updated.(model)
	if !m2.filterMode {
		t.Error("/ should set filterMode=true")
	}
}

func TestFilterModeTypingBuildsInput(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = ""

	// Type "tool:Bash"
	keys := "tool:Bash"
	var mm tea.Model = m
	for _, r := range keys {
		mm, _ = mm.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
	m2 := mm.(model)
	if m2.filterInput != "tool:Bash" {
		t.Errorf("filterInput = %q, want %q", m2.filterInput, "tool:Bash")
	}
}

func TestFilterModeEnterApplies(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = "tool:Bash"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)
	if m2.filterMode {
		t.Error("Enter should exit filter mode")
	}
	if m2.filter.raw != "tool:Bash" {
		t.Errorf("filter.raw = %q, want tool:Bash", m2.filter.raw)
	}
}

func TestFilterModeEscCancels(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = "something"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(model)
	if m2.filterMode {
		t.Error("Esc should exit filter mode")
	}
	// Input is cleared; existing filter is preserved (since input was non-empty).
	if m2.filterInput != "" {
		t.Errorf("filterInput after Esc = %q, want empty", m2.filterInput)
	}
}

func TestFilterModeEscEmptyInputClearsFilter(t *testing.T) {
	m := phase7Model()
	m.filter = parseFilter("tool:Bash")
	m.filterMode = true
	m.filterInput = ""

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m2 := updated.(model)
	if !m2.filter.IsEmpty() {
		t.Error("Esc on empty input should clear active filter")
	}
}

func TestFilterModeBackspace(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = "abc"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m2 := updated.(model)
	if m2.filterInput != "ab" {
		t.Errorf("backspace: filterInput = %q, want ab", m2.filterInput)
	}
}

func TestFilterModeNormalKeysDisabled(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = ""
	// "q" while in filter mode should NOT quit but type into the input.
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m2 := updated.(model)
	if cmd != nil {
		// Verify it's not a quit command by checking it's not tea.Quit.
		msg := cmd()
		if _, isQuit := msg.(tea.QuitMsg); isQuit {
			t.Error("q in filter mode must not quit")
		}
	}
	if m2.filterInput != "q" {
		t.Errorf("q in filter mode should type into input, got %q", m2.filterInput)
	}
}

// --- filter applied to visible rows -----------------------------------------

func TestFilterReducesVisibleRows(t *testing.T) {
	m := phase7Model()
	m.filter = parseFilter("tool:Bash")
	rows := m.visibleRows()
	// Only the Bash pair should be visible.
	if len(rows) != 1 {
		t.Fatalf("filter tool:Bash: want 1 row, got %d", len(rows))
	}
	if rows[0].Pre.ToolName != "Bash" {
		t.Errorf("filtered row tool = %q, want Bash", rows[0].Pre.ToolName)
	}
}

func TestFilterStatusError(t *testing.T) {
	m := phase7Model()
	m.filter = parseFilter("status:error")
	rows := m.visibleRows()
	if len(rows) != 1 {
		t.Fatalf("filter status:error: want 1 row, got %d", len(rows))
	}
	if rows[0].EffectiveStatus() != statusError {
		t.Errorf("filtered row status = %v, want statusError", rows[0].EffectiveStatus())
	}
}

func TestFilterEmptyShowsAll(t *testing.T) {
	m := phase7Model()
	m.filter = parsedFilter{}
	rows := m.visibleRows()
	if len(rows) != 4 { // 4 display rows in folded mode
		t.Fatalf("empty filter: want 4 rows, got %d", len(rows))
	}
}

func TestFilterAppliedInFlatMode(t *testing.T) {
	m := phase7Model()
	m.foldedView = false
	m.filter = parseFilter("hook:PreToolUse")
	rows := m.visibleRows()
	// 2 PreToolUse events (Bash, Read)
	if len(rows) != 2 {
		t.Fatalf("flat+filter hook:PreToolUse: want 2 rows, got %d", len(rows))
	}
}

// --- cursor clamping after filter / fold change ----------------------------

func TestCursorClampedAfterFilter(t *testing.T) {
	m := phase7Model()
	m.eventCursor = 3 // last row

	m.filterMode = true
	m.filterInput = "tool:Bash"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m2 := updated.(model)

	// After filtering to 1 row, cursor must be 0.
	if m2.eventCursor != 0 {
		t.Errorf("cursor after filter = %d, want 0", m2.eventCursor)
	}
}

// --- view rendering smoke tests (no panic) ----------------------------------

func TestViewWithFilterChips(t *testing.T) {
	m := phase7Model()
	m.filter = parseFilter("tool:Bash")
	m.layout = chooseLayout(m.width, m.height)
	view := m.View()
	if view == "" {
		t.Error("View with active filter should not be empty")
	}
	// Key bar should mention filter.
	// (We can't check exact position but at minimum it renders.)
}

func TestViewFilterInputMode(t *testing.T) {
	m := phase7Model()
	m.filterMode = true
	m.filterInput = "foo"
	m.layout = chooseLayout(m.width, m.height)
	view := m.View()
	if view == "" {
		t.Error("View in filter mode should not be empty")
	}
}

func TestViewErrorTapeShown(t *testing.T) {
	m := phase7Model()
	m.layout = chooseLayout(m.width, m.height)
	view := m.View()
	if !contains(view, "error") {
		t.Error("status bar should contain error tape text")
	}
}

func TestViewFoldedToggleRendering(t *testing.T) {
	m := phase7Model()
	m.layout = chooseLayout(m.width, m.height)
	for _, folded := range []bool{true, false} {
		m.foldedView = folded
		view := m.View()
		if view == "" {
			t.Errorf("View (foldedView=%v) should not be empty", folded)
		}
	}
}

// --- selectedEvent in folded mode returns Post for inspector -----------------

func TestSelectedEventReturnedPostForPair(t *testing.T) {
	m := phase7Model()
	// row[1] is the Bash pair (folded). Select it.
	m.eventCursor = 1

	ev := m.selectedEvent()
	if ev == nil {
		t.Fatal("selectedEvent() should not be nil for a paired row")
	}
	// For a paired row we want the Post event (has the result) in the inspector.
	if ev.HookEvent != "PostToolUse" {
		t.Errorf("selectedEvent for paired row = %q, want PostToolUse", ev.HookEvent)
	}
}
