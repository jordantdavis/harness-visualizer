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

// liveEv builds a minimal live event for tests.
func liveEv(seq int64, session, hook string) *event.Event {
	return &event.Event{
		ID:        fmt.Sprintf("ev-%s-%d", session, seq),
		Seq:       seq,
		HookEvent: hook,
		SessionID: session,
		Raw:       json.RawMessage(fmt.Sprintf(`{"seq":%d,"hook_event":%q}`, seq, hook)),
	}
}

// liveModel returns a model with one selected session of two events, following.
func liveModel() model {
	fake := fixtureClient()
	m := newModel(fake, false)
	m.sessions = []store.SessionInfo{
		{ID: "sess-1", EventCount: 2, LastSeq: 2},
		{ID: "sess-2", EventCount: 0, LastSeq: 0},
	}
	m.selectedSession = "sess-1"
	m.events = []*event.Event{
		liveEv(1, "sess-1", "PreToolUse"),
		liveEv(2, "sess-1", "PostToolUse"),
	}
	m.eventCursor = 1
	m.focusedPane = paneEvents
	m.now = func() time.Time { return time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC) }
	return m
}

// --- defaults ---

func TestNewModelFollowsByDefault(t *testing.T) {
	m := newModel(fixtureClient(), false)
	if !m.follow {
		t.Error("new model should start in follow mode")
	}
	if m.liveSessions == nil {
		t.Error("liveSessions map should be initialized")
	}
	if m.now == nil {
		t.Error("now clock should be initialized")
	}
}

// --- appending live events ---

func TestLiveEventAppendsAndFollows(t *testing.T) {
	m := liveModel()
	updated, _ := m.Update(msgLiveEvent{ev: liveEv(3, "sess-1", "PreToolUse")})
	m2 := updated.(model)

	if len(m2.events) != 3 {
		t.Fatalf("events = %d, want 3", len(m2.events))
	}
	if m2.eventCursor != 2 {
		t.Errorf("following: cursor = %d, want 2 (last)", m2.eventCursor)
	}
	if m2.pendingCount != 0 {
		t.Errorf("following: pendingCount = %d, want 0", m2.pendingCount)
	}
}

func TestLiveEventPausedBuffersCount(t *testing.T) {
	m := liveModel()
	m.follow = false
	m.eventCursor = 0 // user scrolled up

	updated, _ := m.Update(msgLiveEvent{ev: liveEv(3, "sess-1", "Stop")})
	m2 := updated.(model)

	if len(m2.events) != 3 {
		t.Fatalf("paused should still append: events = %d, want 3", len(m2.events))
	}
	if m2.eventCursor != 0 {
		t.Errorf("paused: cursor moved to %d, want 0 (held)", m2.eventCursor)
	}
	if m2.pendingCount != 1 {
		t.Errorf("paused: pendingCount = %d, want 1", m2.pendingCount)
	}
}

func TestLiveEventDedupesBySeq(t *testing.T) {
	m := liveModel()
	// Seq 2 already present (history overlap with live).
	updated, _ := m.Update(msgLiveEvent{ev: liveEv(2, "sess-1", "PostToolUse")})
	m2 := updated.(model)
	if len(m2.events) != 2 {
		t.Errorf("duplicate seq should be ignored: events = %d, want 2", len(m2.events))
	}
}

func TestLiveEventOtherSessionNotAppended(t *testing.T) {
	m := liveModel()
	updated, _ := m.Update(msgLiveEvent{ev: liveEv(1, "sess-2", "PreToolUse")})
	m2 := updated.(model)

	if len(m2.events) != 2 {
		t.Errorf("event for non-selected session must not append: events = %d, want 2", len(m2.events))
	}
	if m2.pendingCount != 0 {
		t.Errorf("other-session event must not bump selected pendingCount: %d", m2.pendingCount)
	}
	// But sess-2 should be marked live and its count updated.
	if !m2.isLive("sess-2") {
		t.Error("sess-2 should be marked live")
	}
}

func TestLiveEventNewSessionAdded(t *testing.T) {
	m := liveModel()
	updated, _ := m.Update(msgLiveEvent{ev: liveEv(1, "sess-NEW", "SessionStart")})
	m2 := updated.(model)

	found := false
	for _, s := range m2.sessions {
		if s.ID == "sess-NEW" {
			found = true
		}
	}
	if !found {
		t.Error("a live event for an unknown session should add it to the list")
	}
	if !m2.isLive("sess-NEW") {
		t.Error("new session should be live")
	}
}

func TestMultipleConcurrentLiveSessions(t *testing.T) {
	m := liveModel()
	// Events interleave across three sessions (one brand new).
	for _, ev := range []*event.Event{
		liveEv(3, "sess-1", "PreToolUse"),
		liveEv(1, "sess-2", "PreToolUse"),
		liveEv(1, "sess-NEW", "SessionStart"),
		liveEv(2, "sess-2", "PostToolUse"),
	} {
		updated, _ := m.Update(msgLiveEvent{ev: ev})
		m = updated.(model)
	}

	for _, id := range []string{"sess-1", "sess-2", "sess-NEW"} {
		if !m.isLive(id) {
			t.Errorf("session %s should be tracked live", id)
		}
	}
	// Only the selected session's events populate the events pane.
	if len(m.events) != 3 { // 2 seed + seq 3
		t.Errorf("selected-session events = %d, want 3", len(m.events))
	}
}

// --- pin live sessions to top ---

func TestLiveSessionsPinnedToTop(t *testing.T) {
	m := liveModel()
	m.selectedSession = "sess-1"
	m.sessionCursor = 0
	// sess-2 receives a live event → should jump above sess-1.
	updated, _ := m.Update(msgLiveEvent{ev: liveEv(1, "sess-2", "PreToolUse")})
	m2 := updated.(model)

	if m2.sessions[0].ID != "sess-2" {
		t.Errorf("live session not pinned to top: order = %v", sessionIDs(m2.sessions))
	}
	// Selection must remain on sess-1 (follow the ID across the re-sort).
	if m2.selectedSession != "sess-1" {
		t.Errorf("selection changed across re-sort: %q", m2.selectedSession)
	}
}

func sessionIDs(ss []store.SessionInfo) []string {
	out := make([]string, len(ss))
	for i, s := range ss {
		out[i] = s.ID
	}
	return out
}

// --- follow controls ---

func TestManualScrollPausesFollow(t *testing.T) {
	m := liveModel()
	if !m.follow {
		t.Fatal("precondition: should be following")
	}
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m2 := updated.(model)
	if m2.follow {
		t.Error("manual scroll (k) should pause follow")
	}
}

func TestGReFollows(t *testing.T) {
	m := liveModel()
	m.follow = false
	m.pendingCount = 5
	m.eventCursor = 0
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m2 := updated.(model)
	if !m2.follow {
		t.Error("G should re-enable follow")
	}
	if m2.pendingCount != 0 {
		t.Errorf("G should clear pendingCount, got %d", m2.pendingCount)
	}
	if m2.eventCursor != len(m2.events)-1 {
		t.Errorf("G should jump to last, cursor = %d", m2.eventCursor)
	}
}

func TestFTogglesFollow(t *testing.T) {
	m := liveModel()
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m2 := updated.(model)
	if m2.follow {
		t.Error("f should toggle follow off")
	}
	updated2, _ := m2.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("f")})
	m3 := updated2.(model)
	if !m3.follow {
		t.Error("f should toggle follow back on")
	}
}

// --- stream lifecycle ---

func TestStreamErrorMarksDisconnected(t *testing.T) {
	m := liveModel()
	m.streamUp = true
	updated, cmd := m.Update(msgStreamError{err: fmt.Errorf("connection reset")})
	m2 := updated.(model)
	if m2.streamUp {
		t.Error("stream error should mark streamUp=false")
	}
	if m2.streamErr == "" {
		t.Error("stream error should record an error message")
	}
	if cmd == nil {
		t.Error("stream error should schedule a reconnect cmd")
	}
}

func TestStreamOpenedMarksConnected(t *testing.T) {
	m := liveModel()
	m.streamUp = false
	m.streamErr = "was down"
	ch := make(chan StreamEvent, 1)
	updated, cmd := m.Update(msgStreamOpened{ch: ch})
	m2 := updated.(model)
	if !m2.streamUp {
		t.Error("stream opened should mark streamUp=true")
	}
	if m2.streamErr != "" {
		t.Error("stream opened should clear streamErr")
	}
	if cmd == nil {
		t.Error("stream opened should start waiting for events")
	}
}

// --- heartbeat / idle ---

func TestLiveStatusIdleWhenSilent(t *testing.T) {
	m := liveModel()
	m.streamUp = true
	m.lastEventAt = m.now().Add(-7 * time.Second)
	s := m.liveStatusText()
	if s == "" {
		t.Fatal("liveStatusText empty")
	}
	if !contains(s, "idle") {
		t.Errorf("expected idle indicator when silent, got %q", s)
	}
}

func TestLiveStatusRateWhenStreaming(t *testing.T) {
	m := liveModel()
	m.streamUp = true
	now := m.now()
	m.recentEvents = []time.Time{now.Add(-500 * time.Millisecond), now.Add(-200 * time.Millisecond), now}
	m.lastEventAt = now
	s := m.liveStatusText()
	if contains(s, "idle") {
		t.Errorf("should not be idle while events are flowing, got %q", s)
	}
	if s == "" {
		t.Error("expected a rate indicator")
	}
}

func contains(haystack, needle string) bool {
	return len(haystack) >= len(needle) && (haystack == needle || indexOf(haystack, needle) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
