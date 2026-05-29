package tui

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

// fixtureClient returns a pre-populated FakeClient for model tests.
func fixtureClient() *FakeClient {
	evs := []*event.Event{
		{
			ID: "e1", Seq: 1, HookEvent: "PreToolUse", SessionID: "sess-1",
			ToolName:   "Bash",
			CapturedAt: time.Date(2026, 5, 28, 10, 0, 1, 0, time.UTC),
			Raw:        json.RawMessage(`{"hook_event_name":"PreToolUse","tool_input":{"command":"ls"}}`),
		},
		{
			ID: "e2", Seq: 2, HookEvent: "PostToolUse", SessionID: "sess-1",
			ToolName:   "Bash",
			CapturedAt: time.Date(2026, 5, 28, 10, 0, 2, 0, time.UTC),
			Raw:        json.RawMessage(`{"hook_event_name":"PostToolUse","tool_response":{"exit_code":0}}`),
		},
	}
	return &FakeClient{
		Sessions_: []store.SessionInfo{
			{ID: "sess-1", EventCount: 2, LastSeq: 2},
			{ID: "sess-2", EventCount: 0, LastSeq: 0},
		},
		Events_: map[string][]*event.Event{"sess-1": evs},
	}
}

// dispatchAll drains a tea.Cmd by calling it and applying the resulting
// message(s) until no further commands are produced (depth-1 for synchronous fakes).
func dispatchAll(m tea.Model, cmd tea.Cmd) tea.Model {
	if cmd == nil {
		return m
	}
	msg := cmd()
	if msg == nil {
		return m
	}
	m2, cmd2 := m.Update(msg)
	if cmd2 != nil {
		msg2 := cmd2()
		if msg2 != nil {
			m2, _ = m2.Update(msg2)
		}
	}
	return m2
}

// --- WindowSizeMsg ---

func TestWindowSizeSetsLayout(t *testing.T) {
	tests := []struct {
		w, h   int
		layout Layout
	}{
		{60, 24, LayoutNarrow},
		{100, 24, LayoutMedium},
		{140, 24, LayoutWide},
		{30, 5, LayoutTooSmall},
	}
	for _, tc := range tests {
		m := newModel(fixtureClient(), false)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: tc.w, Height: tc.h})
		m2 := updated.(model)
		if m2.layout != tc.layout {
			t.Errorf("(%dx%d): layout = %v, want %v", tc.w, tc.h, m2.layout, tc.layout)
		}
		if m2.width != tc.w || m2.height != tc.h {
			t.Errorf("(%dx%d): size not stored, got %dx%d", tc.w, tc.h, m2.width, m2.height)
		}
	}
}

// --- sessions loaded msg ---

func TestSessionsLoadedPopulatesState(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	// Simulate the sessions-loaded message.
	updated, cmd := m.Update(msgSessionsLoaded{sessions: fake.Sessions_, err: nil})
	m2 := updated.(model)

	if len(m2.sessions) != 2 {
		t.Fatalf("sessions count = %d, want 2", len(m2.sessions))
	}
	// First session should be auto-selected.
	if m2.selectedSession != "sess-1" {
		t.Errorf("selectedSession = %q, want sess-1", m2.selectedSession)
	}
	if !m2.eventsLoading {
		t.Error("eventsLoading should be true after auto-selecting first session")
	}
	// cmd should be an events-load command.
	if cmd == nil {
		t.Fatal("expected events-load cmd after auto-select")
	}
}

func TestSessionsLoadedError(t *testing.T) {
	m := newModel(fixtureClient(), false)
	updated, cmd := m.Update(msgSessionsLoaded{err: fmt.Errorf("connection refused")})
	m2 := updated.(model)
	if m2.sessionsErr == nil {
		t.Error("sessionsErr should be set on error")
	}
	if cmd != nil {
		t.Error("no follow-on cmd expected on error")
	}
}

// --- events loaded msg ---

func TestEventsLoadedPopulatesState(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	// First load sessions to set selectedSession.
	updated, _ := m.Update(msgSessionsLoaded{sessions: fake.Sessions_})
	m2 := updated.(model)
	// Now deliver events.
	updated2, _ := m2.Update(msgEventsLoaded{
		sessionID: "sess-1",
		events:    fake.Events_["sess-1"],
	})
	m3 := updated2.(model)

	if len(m3.events) != 2 {
		t.Fatalf("events count = %d, want 2", len(m3.events))
	}
	if m3.eventsLoading {
		t.Error("eventsLoading should be cleared after load")
	}
}

func TestEventsLoadedIgnoresStaleSession(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	updated, _ := m.Update(msgSessionsLoaded{sessions: fake.Sessions_})
	m2 := updated.(model)
	// Deliver events for a different session — should be ignored.
	updated2, _ := m2.Update(msgEventsLoaded{
		sessionID: "sess-WRONG",
		events:    fake.Events_["sess-1"],
	})
	m3 := updated2.(model)
	if len(m3.events) != 0 {
		t.Errorf("events for wrong session should be ignored, got %d", len(m3.events))
	}
}

// --- key navigation ---

func TestKeyJMovesSessionCursor(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	m.sessions = fake.Sessions_
	m.focusedPane = paneSessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := updated.(model)
	if m2.sessionCursor != 1 {
		t.Errorf("after j: sessionCursor = %d, want 1", m2.sessionCursor)
	}
}

func TestKeyKMovesSessionCursorUp(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	m.sessions = fake.Sessions_
	m.sessionCursor = 1
	m.focusedPane = paneSessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m2 := updated.(model)
	if m2.sessionCursor != 0 {
		t.Errorf("after k: sessionCursor = %d, want 0", m2.sessionCursor)
	}
}

func TestKeyJDoesNotExceedBounds(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	m.sessions = fake.Sessions_
	m.sessionCursor = len(fake.Sessions_) - 1
	m.focusedPane = paneSessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m2 := updated.(model)
	if m2.sessionCursor != len(fake.Sessions_)-1 {
		t.Errorf("j at end: sessionCursor = %d, want %d", m2.sessionCursor, len(fake.Sessions_)-1)
	}
}

func TestTabCyclesFocus(t *testing.T) {
	m := newModel(fixtureClient(), false)
	m.width = 140
	m.height = 24
	m.layout = LayoutWide
	m.focusedPane = paneSessions

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m2 := updated.(model)
	if m2.focusedPane != paneEvents {
		t.Errorf("after tab: focus = %v, want paneEvents", m2.focusedPane)
	}

	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyTab})
	m3 := updated2.(model)
	if m3.focusedPane != paneInspector {
		t.Errorf("after tab×2: focus = %v, want paneInspector", m3.focusedPane)
	}

	updated3, _ := m3.Update(tea.KeyMsg{Type: tea.KeyTab})
	m4 := updated3.(model)
	if m4.focusedPane != paneSessions {
		t.Errorf("after tab×3: focus = %v, want paneSessions (wrapped)", m4.focusedPane)
	}
}

func TestKeyQQuitsModel(t *testing.T) {
	m := newModel(fixtureClient(), false)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("q should produce tea.Quit command")
	}
	// tea.Quit returns tea.QuitMsg when called.
	msg := cmd()
	if _, ok := msg.(tea.QuitMsg); !ok {
		t.Errorf("q cmd returned %T, want tea.QuitMsg", msg)
	}
}

func TestHelpToggle(t *testing.T) {
	m := newModel(fixtureClient(), false)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	m2 := updated.(model)
	if !m2.showHelp {
		t.Error("? should set showHelp=true")
	}
	// Any key dismisses help.
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m3 := updated2.(model)
	if m3.showHelp {
		t.Error("key while help shown should dismiss help")
	}
}

// --- View smoke tests ---

func TestViewNoPanicAtBreakpoints(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, false)
	m.sessions = fake.Sessions_
	m.selectedSession = "sess-1"
	m.events = fake.Events_["sess-1"]

	for _, tc := range []struct{ w, h int }{
		{60, 24},
		{100, 24},
		{140, 40},
		{30, 5},
	} {
		m.width = tc.w
		m.height = tc.h
		m.layout = chooseLayout(tc.w, tc.h)
		view := m.View()
		if view == "" && (tc.w >= minCols && tc.h >= minRows) {
			t.Errorf("View() empty at %dx%d (expected non-empty)", tc.w, tc.h)
		}
	}
}

func TestViewTooSmallMessage(t *testing.T) {
	m := newModel(fixtureClient(), false)
	m.width = 30
	m.height = 5
	m.layout = LayoutTooSmall
	view := m.View()
	if view == "" {
		t.Error("LayoutTooSmall should produce non-empty view")
	}
}

func TestViewEmptyState(t *testing.T) {
	// No sessions loaded.
	m := newModel(fixtureClient(), false)
	m.width = 100
	m.height = 24
	m.layout = LayoutMedium
	view := m.View()
	if view == "" {
		t.Error("empty state view should be non-empty")
	}
}

func TestViewNoColorRendering(t *testing.T) {
	fake := fixtureClient()
	m := newModel(fake, true) // noColor=true
	m.width = 100
	m.height = 24
	m.layout = LayoutMedium
	m.sessions = fake.Sessions_
	m.selectedSession = "sess-1"
	m.events = fake.Events_["sess-1"]
	view := m.View()
	if view == "" {
		t.Error("noColor view should be non-empty")
	}
}

func TestViewHelp(t *testing.T) {
	m := newModel(fixtureClient(), false)
	m.width = 100
	m.height = 24
	m.showHelp = true
	view := m.View()
	if view == "" {
		t.Error("help view should be non-empty")
	}
}
