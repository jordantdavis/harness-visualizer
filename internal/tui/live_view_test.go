package tui

import (
	"strings"
	"testing"
	"time"
)

// liveViewModel returns a wide-layout model sized for rendering.
func liveViewModel() model {
	m := liveModel()
	m.width = 140
	m.height = 30
	m.layout = LayoutWide
	return m
}

func TestSessionsPaneShowsLiveMarker(t *testing.T) {
	m := liveViewModel()
	m.liveSessions["sess-1"] = m.now() // sess-1 currently live
	pane := m.viewSessionsPane(40, 20)
	if !strings.Contains(pane, "●") {
		t.Errorf("live session should render the ● marker, got:\n%s", pane)
	}
}

func TestSessionsPaneNoMarkerWhenNotLive(t *testing.T) {
	m := liveViewModel()
	// No live sessions registered.
	pane := m.viewSessionsPane(40, 20)
	if strings.Contains(pane, "●") {
		t.Errorf("no session is live; ● should be absent, got:\n%s", pane)
	}
}

func TestEventsPaneShowsPendingCount(t *testing.T) {
	m := liveViewModel()
	m.follow = false
	m.pendingCount = 3
	pane := m.viewEventsPane(80, 20)
	if !strings.Contains(pane, "3 new") {
		t.Errorf("paused pane should show buffered-new count, got:\n%s", pane)
	}
}

func TestEventsPaneNoPendingWhenFollowing(t *testing.T) {
	m := liveViewModel()
	m.follow = true
	m.pendingCount = 0
	pane := m.viewEventsPane(80, 20)
	if strings.Contains(pane, "new") {
		t.Errorf("following pane should not show a pending count, got:\n%s", pane)
	}
}

func TestStatusBarShowsLiveHeartbeat(t *testing.T) {
	m := liveViewModel()
	m.streamUp = true
	m.lastEventAt = m.now()
	m.recentEvents = []time.Time{m.now()}
	bar := m.viewStatusBar()
	if !strings.Contains(bar, "live") {
		t.Errorf("status bar should show live heartbeat, got: %q", bar)
	}
}

func TestStatusBarShowsDisconnectBanner(t *testing.T) {
	m := liveViewModel()
	m.streamUp = false
	m.streamErr = "connection refused"
	bar := m.viewStatusBar()
	if !strings.Contains(bar, "disconnected") && !strings.Contains(bar, "stream") {
		t.Errorf("status bar should surface stream disconnect, got: %q", bar)
	}
}
