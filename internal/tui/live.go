package tui

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// Live-mode tuning constants.
const (
	liveWindow     = 30 * time.Second // how long a session shows the ● live marker
	idleAfter      = 3 * time.Second  // silence before the status reads "idle Ns"
	rateWindow     = time.Second      // window for the events/sec figure
	recentWindow   = 5 * time.Second  // how much rate history to retain
	reconnectDelay = 2 * time.Second  // backoff before retrying a dropped stream
	tickPeriod     = time.Second      // heartbeat re-render cadence
)

// errStreamClosed signals the SSE channel closed without an explicit error.
var errStreamClosed = errors.New("stream closed")

// --- live messages ----------------------------------------------------------

// msgStreamOpened carries a freshly opened live subscription channel.
type msgStreamOpened struct{ ch <-chan StreamEvent }

// msgLiveEvent delivers one event received over the live stream.
type msgLiveEvent struct{ ev *event.Event }

// msgStreamError signals the live stream dropped; a reconnect is scheduled.
type msgStreamError struct{ err error }

// msgReconnect fires after the backoff to re-open the stream.
type msgReconnect struct{}

// --- live commands ----------------------------------------------------------

// cmdOpenStream opens the all-sessions live subscription.
func cmdOpenStream(c Client) tea.Cmd {
	return func() tea.Msg {
		ch, err := c.Stream(context.Background(), "")
		if err != nil {
			return msgStreamError{err: err}
		}
		return msgStreamOpened{ch: ch}
	}
}

// cmdWaitForStream blocks for the next live frame, translating it to a message.
// It is re-issued after each frame to keep pulling from the channel.
func cmdWaitForStream(ch <-chan StreamEvent) tea.Cmd {
	return func() tea.Msg {
		se, ok := <-ch
		if !ok {
			return msgStreamError{err: errStreamClosed}
		}
		if se.Err != nil {
			return msgStreamError{err: se.Err}
		}
		return msgLiveEvent{ev: se.Event}
	}
}

// cmdReconnectAfter schedules a reconnect attempt after d.
func cmdReconnectAfter(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return msgReconnect{} })
}

// cmdTick schedules the next heartbeat re-render.
func cmdTick() tea.Cmd {
	return tea.Tick(tickPeriod, func(time.Time) tea.Msg { return msgTick{} })
}

// --- live processing --------------------------------------------------------

// applyLiveEvent folds one streamed event into the model: it updates rate and
// liveness bookkeeping, refreshes the session summary, pins live sessions to
// the top, and appends to the events pane when the event belongs to the
// selected session (respecting follow / pause).
func (m *model) applyLiveEvent(ev *event.Event) {
	if ev == nil {
		return
	}
	now := m.now()
	m.lastEventAt = now
	m.recentEvents = trimRecent(append(m.recentEvents, now), now, recentWindow)

	m.liveSessions[ev.SessionID] = now
	m.touchSession(ev, now)
	m.sortSessionsLive()

	if ev.SessionID != m.selectedSession {
		return
	}
	if !m.appendSelectedEvent(ev) {
		return
	}
	if m.follow {
		m.eventCursor = len(m.events) - 1
		m.pendingCount = 0
	} else {
		m.pendingCount++
	}
}

// touchSession updates the summary row for ev's session, inserting one if the
// session is appearing for the first time.
func (m *model) touchSession(ev *event.Event, now time.Time) {
	for i := range m.sessions {
		if m.sessions[i].ID == ev.SessionID {
			m.sessions[i].EventCount++
			if ev.Seq > m.sessions[i].LastSeq {
				m.sessions[i].LastSeq = ev.Seq
			}
			m.sessions[i].ModTime = now
			return
		}
	}
	m.sessions = append(m.sessions, store.SessionInfo{
		ID:         ev.SessionID,
		EventCount: 1,
		LastSeq:    ev.Seq,
		ModTime:    now,
	})
}

// appendSelectedEvent appends ev to the events pane unless its Seq is already
// present (history/live overlap or a duplicate). Returns true when appended.
func (m *model) appendSelectedEvent(ev *event.Event) bool {
	if n := len(m.events); n > 0 && ev.Seq <= m.events[n-1].Seq {
		for _, e := range m.events {
			if e.Seq == ev.Seq {
				return false
			}
		}
	}
	m.events = append(m.events, ev)
	return true
}

// sortSessionsLive partitions live sessions to the top while preserving the
// relative order within each group (stable) and the cursor's target session.
func (m *model) sortSessionsLive() {
	if len(m.sessions) < 2 {
		return
	}
	var cursorID string
	if m.sessionCursor >= 0 && m.sessionCursor < len(m.sessions) {
		cursorID = m.sessions[m.sessionCursor].ID
	}
	sort.SliceStable(m.sessions, func(i, j int) bool {
		li, lj := m.isLive(m.sessions[i].ID), m.isLive(m.sessions[j].ID)
		if li != lj {
			return li // live sessions sort first
		}
		return false
	})
	if cursorID != "" {
		for i, s := range m.sessions {
			if s.ID == cursorID {
				m.sessionCursor = i
				break
			}
		}
	}
}

// isLive reports whether id received a live event within liveWindow.
func (m *model) isLive(id string) bool {
	t, ok := m.liveSessions[id]
	if !ok {
		return false
	}
	return m.now().Sub(t) < liveWindow
}

// liveStatusText renders the heartbeat segment for the status bar: a rate while
// events flow, "idle Ns" when quiet, or "" when the stream is down (the
// disconnect banner covers that case).
//
// When reducedMotion is set the animated ▮ block character is replaced with a
// static ">" indicator so the textual rate/idle info is preserved without
// any blinking.
func (m *model) liveStatusText() string {
	if !m.streamUp {
		return ""
	}
	now := m.now()
	if m.lastEventAt.IsZero() {
		return "live · idle"
	}
	silent := now.Sub(m.lastEventAt)
	if silent >= idleAfter {
		return fmt.Sprintf("live · idle %ds", int(silent.Seconds()))
	}
	if m.reducedMotion {
		// Static indicator: no blinking/animated characters.
		return fmt.Sprintf("live · > %.0f/s", m.eventsPerSec(now))
	}
	return fmt.Sprintf("live · ▮ %.0f/s", m.eventsPerSec(now))
}

// eventsPerSec counts events within the last rateWindow.
func (m *model) eventsPerSec(now time.Time) float64 {
	cut := now.Add(-rateWindow)
	var n int
	for _, t := range m.recentEvents {
		if !t.Before(cut) {
			n++
		}
	}
	return float64(n) / rateWindow.Seconds()
}

// trimRecent drops timestamps older than window from the front of ts.
func trimRecent(ts []time.Time, now time.Time, window time.Duration) []time.Time {
	cut := now.Add(-window)
	i := 0
	for i < len(ts) && ts[i].Before(cut) {
		i++
	}
	return ts[i:]
}
