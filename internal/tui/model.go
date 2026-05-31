package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	osc52 "github.com/aymanbagabas/go-osc52/v2"
	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

// pane identifies which panel currently has keyboard focus.
type pane int

const (
	paneSessions pane = iota
	paneEvents
	paneInspector
)

// inspectorSection identifies what the inspector is showing for foldable tree.
type inspectorSection int

const (
	inspectorSectionHeader inspectorSection = iota
	inspectorSectionJSON
)

// --- messages ---------------------------------------------------------------

// msgSessionsLoaded is emitted when the sessions list fetch completes.
type msgSessionsLoaded struct {
	sessions []store.SessionInfo
	err      error
}

// msgEventsLoaded is emitted when an events-for-session fetch completes.
type msgEventsLoaded struct {
	sessionID string
	events    []*event.Event
	err       error
}

// msgHealthResult is emitted after the initial health check.
type msgHealthResult struct{ err error }

// msgTick is the periodic heartbeat tick.
type msgTick struct{}

// --- model ------------------------------------------------------------------

// model is the root bubbletea model for the TUI.
type model struct {
	client Client

	// terminal dimensions
	width  int
	height int

	// layout: LayoutBrowse, LayoutNarrow, or LayoutTooSmall (see layout.go).
	layout Layout

	// depthDrill is true when we are in Drill mode (Events │ Inspector).
	// In Browse mode (false) the display is Sessions │ Events.
	// This is orthogonal to layout — it can only be true when layout==LayoutBrowse.
	depthDrill bool

	// sessions pane state
	sessions      []store.SessionInfo
	sessionCursor int // index into sessions
	sessionTop    int // scroll-viewport top for the sessions pane
	sessionsErr   error

	// events pane state
	selectedSession string
	events          []*event.Event
	eventCursor     int // index into events
	eventTop        int // scroll-viewport top for the events pane
	eventsErr       error
	eventsLoading   bool

	// inspector pane state
	inspectorOpen    bool
	inspectorSection inspectorSection // which section has focus in inspector
	inspectorScroll  int              // scroll offset in JSON view
	rawPager         bool             // true when showing raw JSON full-screen

	// focus
	focusedPane pane
	showHelp    bool

	// daemon state
	daemonOK    bool
	daemonError string
	lastCheck   time.Time

	// live (SSE) state
	streamCh     <-chan StreamEvent   // current live subscription; nil until opened
	streamUp     bool                 // true while the SSE connection is healthy
	streamErr    string               // last disconnect reason (for the banner)
	follow       bool                 // tail-follow the selected session's events
	pendingCount int                  // new events buffered below while paused
	liveSessions map[string]time.Time // sessionID -> last live event time (for ●)
	lastEventAt  time.Time            // wall-clock of the most recent live event
	recentEvents []time.Time          // sliding window of recent event times (rate)

	// now is the clock, injectable for deterministic tests.
	now func() time.Time

	// NO_COLOR
	noColor bool

	// --- Phase 7: filter, error-hop, folded ops ---

	// filter is the active parsed filter; IsEmpty() == true means "show all".
	filter parsedFilter
	// filterInput is the raw text the user is currently typing (filter mode).
	filterInput string
	// filterMode is true while the filter text-input line is open.
	filterMode bool

	// foldedView is true (default) for the folded Pre/Post op view.
	// false = flat chronological per-event.
	foldedView bool
	// yankFn is called by 'y'/'Y' to copy text to the clipboard. It is
	// injectable so tests can assert the exact string yanked without real
	// clipboard side-effects. newModel wires in the OSC 52 default.
	yankFn func(string) error

	// statusMsg is a transient "toast" line shown in the key-hint bar after
	// operations like yank. It is set on yank and cleared on the next key press.
	statusMsg string

	// reducedMotion suppresses animated indicators (blinking block character,
	// etc.) when true. Set via --no-animation flag or implied by noColor.
	reducedMotion bool
}

// newModel constructs a model with the given client and rendering options.
//
//   - noColor: true when NO_COLOR env var is set or the terminal is dumb.
//     Renders use glyphs + text tags only; implies reducedMotion.
//   - reducedMotion: true when --no-animation is set. Suppresses animated
//     indicators (blinking ▮ block) while keeping textual rate/idle info.
func newModel(c Client, noColor bool, reducedMotion bool) model {
	return model{
		client:        c,
		noColor:       noColor,
		reducedMotion: reducedMotion || noColor, // noColor implies reduced motion
		layout:        LayoutNarrow,
		follow:        true,
		liveSessions:  make(map[string]time.Time),
		now:           time.Now,
		foldedView:    true, // Phase 7: folded op view is default
		yankFn:        defaultYankFn,
	}
}

// defaultYankFn writes s to the system clipboard via OSC 52 — a terminal
// escape sequence that works over SSH and in most modern terminals without
// any external clipboard tool.
func defaultYankFn(s string) error {
	_, err := fmt.Fprint(os.Stderr, osc52.New(s))
	return err
}

// Init sends the initial commands: health check, session load, live stream
// subscription, and the heartbeat tick.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		cmdHealth(m.client),
		cmdLoadSessions(m.client),
		cmdOpenStream(m.client),
		cmdTick(),
	)
}

// Update is the central message dispatcher.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.layout = chooseLayout(msg.Width, msg.Height)
		return m, nil

	case msgHealthResult:
		if msg.err != nil {
			m.daemonOK = false
			m.daemonError = msg.err.Error()
		} else {
			m.daemonOK = true
			m.daemonError = ""
		}
		m.lastCheck = time.Now()
		return m, nil

	case msgSessionsLoaded:
		m.sessionsErr = msg.err
		if msg.err == nil {
			m.sessions = msg.sessions
			m.sortSessionsLive()
			if len(m.sessions) > 0 && m.selectedSession == "" {
				// Auto-select the first session.
				m.selectedSession = m.sessions[0].ID
				m.eventsLoading = true
				return m, cmdLoadEvents(m.client, m.sessions[0].ID, 0)
			}
		}
		return m, nil

	case msgStreamOpened:
		m.streamCh = msg.ch
		m.streamUp = true
		m.streamErr = ""
		// Resume pulling frames and refresh session counts after (re)connect.
		return m, tea.Batch(cmdWaitForStream(msg.ch), cmdLoadSessions(m.client))

	case msgLiveEvent:
		m.applyLiveEvent(msg.ev)
		if m.streamCh != nil {
			return m, cmdWaitForStream(m.streamCh)
		}
		return m, nil

	case msgStreamError:
		m.streamUp = false
		if msg.err != nil {
			m.streamErr = msg.err.Error()
		}
		m.streamCh = nil
		return m, cmdReconnectAfter(reconnectDelay)

	case msgReconnect:
		return m, cmdOpenStream(m.client)

	case msgTick:
		// Re-render so idle/heartbeat and ● expiry stay current.
		return m, cmdTick()

	case msgEventsLoaded:
		if msg.sessionID == m.selectedSession {
			m.eventsLoading = false
			m.eventsErr = msg.err
			if msg.err == nil {
				m.events = msg.events
				m.eventCursor = 0
				m.eventTop = 0
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.rawPager {
			return m.updateRawPager(msg)
		}
		if m.filterMode {
			return m.updateFilterInput(msg)
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m.updateKeys(msg)
	}

	return m, nil
}

// updateKeys handles keyboard input in normal (non-pager, non-help, non-filter) mode.
func (m model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Clear transient toast on any keypress.
	m.statusMsg = ""

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "?":
		m.showHelp = true
		return m, nil

	case "f":
		// Toggle tail-follow. Re-following snaps to the latest event.
		m.follow = !m.follow
		if m.follow {
			m.pendingCount = 0
			if len(m.events) > 0 {
				m.eventCursor = len(m.events) - 1
			}
		}
		return m, nil

	case "/":
		// Enter filter input mode.
		m.filterMode = true
		m.filterInput = m.filter.raw // pre-populate with current filter
		return m, nil

	case "o":
		// Toggle folded ↔ flat chronological view.
		m.foldedView = !m.foldedView
		m.clampEventCursor()
		return m, nil

	// --- depth-aware pane switching (8a) ---

	case "tab":
		m.focusedPane = nextPane(m.focusedPane, m.layout, m.depthDrill)
		return m, nil

	case "1":
		// Jump to Sessions; if in Drill, pop back to Browse first.
		if m.depthDrill {
			m.depthDrill = false
		}
		m.focusedPane = paneSessions
		return m, nil

	case "2":
		// Jump to Events in either depth state.
		m.focusedPane = paneEvents
		return m, nil

	case "3":
		// Jump to Inspector; if in Browse, enter Drill first.
		if !m.depthDrill && m.layout == LayoutBrowse {
			m.depthDrill = true
		}
		m.focusedPane = paneInspector
		return m, nil

	case "esc":
		// Esc pops exactly one step shallower; never quits.
		return m.popDepth(), nil
	}

	switch m.focusedPane {
	case paneSessions:
		return m.updateSessionsPane(msg)
	case paneEvents:
		return m.updateEventsPane(msg)
	case paneInspector:
		return m.updateInspectorPane(msg)
	}
	return m, nil
}

// popDepth pops one level in the navigation ladder:
//
//	filter mode → handled before popDepth is called
//	Inspector (Drill) → Events (back to Browse)
//	Events (Browse) → Sessions
//	Sessions → stop (no-op)
//
// Esc never quits.
func (m model) popDepth() model {
	switch m.focusedPane {
	case paneInspector:
		m.depthDrill = false
		m.focusedPane = paneEvents
	case paneEvents:
		m.focusedPane = paneSessions
	// Sessions: no further back, just stay
	}
	return m
}

// updateSessionsPane handles keys when the sessions pane is focused.
func (m model) updateSessionsPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.sessionCursor < len(m.sessions)-1 {
			m.sessionCursor++
			m.scrollSessionViewport()
		}
		return m, nil
	case "k", "up":
		if m.sessionCursor > 0 {
			m.sessionCursor--
			m.scrollSessionViewport()
		}
		return m, nil
	case "g":
		m.sessionCursor = 0
		m.sessionTop = 0
		return m, nil
	case "G":
		if len(m.sessions) > 0 {
			m.sessionCursor = len(m.sessions) - 1
			m.scrollSessionViewport()
		}
		return m, nil
	case "ctrl+d":
		m.sessionCursor = min(m.sessionCursor+10, max(0, len(m.sessions)-1))
		m.scrollSessionViewport()
		return m, nil
	case "ctrl+u":
		m.sessionCursor = max(m.sessionCursor-10, 0)
		m.scrollSessionViewport()
		return m, nil
	case "enter", "l":
		if len(m.sessions) == 0 {
			return m, nil
		}
		sid := m.sessions[m.sessionCursor].ID
		if sid != m.selectedSession {
			m.selectedSession = sid
			m.events = nil
			m.eventCursor = 0
			m.eventTop = 0
			m.eventsLoading = true
		}
		// Enter on a session just focuses Events; stays in Browse (8a).
		m.focusedPane = paneEvents
		return m, cmdLoadEvents(m.client, sid, 0)
	case "h":
		// h in sessions pane: no further back.
		return m, nil
	}
	return m, nil
}

// updateEventsPane handles keys when the events pane is focused.
//
// Manual vertical motion auto-pauses follow (the tail-f model); G snaps back to
// the latest event and re-follows, clearing the buffered-new count.
//
// Phase 8a: Enter / l on an event enters Drill depth (reveals Inspector).
// Esc / h pops back: Inspector → Events → Sessions.
func (m model) updateEventsPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	rows := m.visibleRows()
	n := len(rows)
	switch msg.String() {
	case "j", "down":
		m.follow = false
		if n > 0 && m.eventCursor < n-1 {
			m.eventCursor++
			m.scrollEventViewport()
		}
	case "k", "up":
		m.follow = false
		if m.eventCursor > 0 {
			m.eventCursor--
			m.scrollEventViewport()
		}
	case "g":
		m.follow = false
		m.eventCursor = 0
		m.eventTop = 0
	case " ", "space":
		// Explicit pause.
		m.follow = false
	case "G":
		m.follow = true
		m.pendingCount = 0
		if n > 0 {
			m.eventCursor = n - 1
			m.scrollEventViewport()
		}
	case "ctrl+d":
		m.follow = false
		m.eventCursor = min(m.eventCursor+10, max(0, n-1))
		m.scrollEventViewport()
	case "ctrl+u":
		m.follow = false
		m.eventCursor = max(m.eventCursor-10, 0)
		m.scrollEventViewport()
	case "enter", "l":
		// Enter on an event enters Drill depth (8a).
		if n > 0 && m.layout == LayoutBrowse {
			m.depthDrill = true
			m.focusedPane = paneInspector
			m.inspectorOpen = true
		} else if n > 0 {
			// Narrow: just go to inspector pane.
			m.focusedPane = paneInspector
			m.inspectorOpen = true
		}
	case "esc", "h":
		// Pop one step shallower.
		m = m.popDepth()
	case "e":
		// Error-hop forward: next error row (wrapping).
		m.follow = false
		m.hopToError(+1)
	case "E":
		// Error-hop backward: previous error row (wrapping).
		m.follow = false
		m.hopToError(-1)
	}
	return m, nil
}

// updateInspectorPane handles keys when the inspector pane is focused.
func (m model) updateInspectorPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		m.inspectorScroll++
	case "k", "up":
		if m.inspectorScroll > 0 {
			m.inspectorScroll--
		}
	case "g":
		m.inspectorScroll = 0
	case "r":
		if len(m.events) > 0 {
			m.rawPager = true
		}
	case "y":
		// Yank the focused event's Raw payload (the hook input/response JSON).
		if ev := m.selectedEvent(); ev != nil && m.yankFn != nil {
			if err := m.yankFn(string(ev.Raw)); err == nil {
				m.statusMsg = "yanked value"
			}
		}
	case "Y":
		// Yank the whole event Raw, pretty-printed.
		if ev := m.selectedEvent(); ev != nil && m.yankFn != nil {
			s := prettyJSON(ev.Raw)
			if err := m.yankFn(s); err == nil {
				m.statusMsg = "yanked raw event"
			}
		}
	case "esc", "h":
		// Pop back from Inspector → Events + leave Drill.
		m = m.popDepth()
	}
	return m, nil
}

// prettyJSON returns a pretty-printed version of raw. Falls back to the
// compact original if marshaling fails.
func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	pretty, err := json.MarshalIndent(json.RawMessage(raw), "", "  ")
	if err != nil {
		return string(raw)
	}
	return string(pretty)
}

// updateRawPager handles keys while the raw JSON pager is open.
func (m model) updateRawPager(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.rawPager = false
	}
	return m, nil
}

// updateFilterInput handles keyboard input while the filter text-input is open.
// Enter applies the filter and exits; Esc cancels (or clears active filter on
// empty input); any other key edits the input text.
func (m model) updateFilterInput(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "enter":
		m.filter = parseFilter(m.filterInput)
		m.filterMode = false
		m.clampEventCursor()
	case "esc":
		if m.filterInput == "" {
			// Clear active filter.
			m.filter = parsedFilter{}
		}
		m.filterMode = false
		m.filterInput = ""
		m.clampEventCursor()
	case "backspace", "ctrl+h":
		if len(m.filterInput) > 0 {
			runes := []rune(m.filterInput)
			m.filterInput = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.filterInput = ""
	default:
		// Append printable runes.
		if msg.Type == tea.KeyRunes {
			m.filterInput += string(msg.Runes)
		}
	}
	return m, nil
}

// clampEventCursor ensures eventCursor is within the visible rows after a
// filter or fold change.
func (m *model) clampEventCursor() {
	rows := m.visibleRows()
	n := len(rows)
	if n == 0 {
		m.eventCursor = 0
		return
	}
	if m.eventCursor >= n {
		m.eventCursor = n - 1
	}
	m.scrollEventViewport()
}

// visibleRows returns the display rows visible under the current filter and
// fold mode. In folded view it pairs Pre/Post events; in flat view it wraps
// each event as a standalone row.
func (m *model) visibleRows() []displayRow {
	var rows []displayRow
	if m.foldedView {
		rows = buildDisplayRows(m.events)
	} else {
		rows = make([]displayRow, len(m.events))
		for i, ev := range m.events {
			rows[i] = displayRow{Pre: ev}
		}
	}
	if m.filter.IsEmpty() {
		return rows
	}
	out := rows[:0:0]
	for _, r := range rows {
		if matchEvent(m.filter, r) {
			out = append(out, r)
		}
	}
	return out
}

// selectedDisplayRow returns the currently selected displayRow, or a zero value.
func (m *model) selectedDisplayRow() (displayRow, bool) {
	rows := m.visibleRows()
	if len(rows) == 0 || m.eventCursor >= len(rows) {
		return displayRow{}, false
	}
	return rows[m.eventCursor], true
}

// errorCount returns the number of visible rows with statusError.
func (m *model) errorCount() int {
	n := 0
	for _, r := range m.visibleRows() {
		if r.EffectiveStatus() == statusError {
			n++
		}
	}
	return n
}

// hopToNextError moves the cursor to the next (or previous) error row,
// wrapping around. dir=+1 for forward, -1 for backward.
func (m *model) hopToError(dir int) {
	rows := m.visibleRows()
	if len(rows) == 0 {
		return
	}
	start := m.eventCursor
	i := (start + dir + len(rows)) % len(rows)
	for i != start {
		if rows[i].EffectiveStatus() == statusError {
			m.eventCursor = i
			m.scrollEventViewport()
			return
		}
		i = (i + dir + len(rows)) % len(rows)
	}
	// Check start itself if it's the only error.
	if rows[start].EffectiveStatus() == statusError {
		m.eventCursor = start
	}
}

// nextPane cycles focus through the panes currently visible at the given depth.
//
// Browse (depthDrill==false): Sessions ↔ Events
// Drill  (depthDrill==true):  Events ↔ Inspector
// Narrow: same as Browse (Sessions ↔ Events ↔ Inspector one at a time).
func nextPane(current pane, layout Layout, drill bool) pane {
	if layout == LayoutNarrow {
		return (current + 1) % 3
	}
	if drill {
		// Drill: cycle Events ↔ Inspector.
		if current == paneEvents {
			return paneInspector
		}
		return paneEvents
	}
	// Browse: cycle Sessions ↔ Events.
	if current == paneSessions {
		return paneEvents
	}
	return paneSessions
}

// scrollEventViewport updates eventTop so that eventCursor is within the
// visible window [eventTop, eventTop+visibleH).
//
// visibleH is computed from m.height: subtract 2 bars, 1 pane-title row, 1
// header row. Falls back to 1 if the result would be non-positive.
func (m *model) scrollEventViewport() {
	visibleH := m.eventsVisibleH()
	if visibleH < 1 {
		visibleH = 1
	}
	if m.eventCursor < m.eventTop {
		m.eventTop = m.eventCursor
	}
	if m.eventCursor >= m.eventTop+visibleH {
		m.eventTop = m.eventCursor - visibleH + 1
	}
}

// eventsVisibleH returns the number of event data rows that fit in the pane
// (excluding the title and header rows).
func (m *model) eventsVisibleH() int {
	contentH := m.height - 2 // top status + bottom key bars
	if contentH < 1 {
		contentH = 1
	}
	// Subtract pane title row (1) and column header row (1).
	h := contentH - 2
	if h < 1 {
		h = 1
	}
	return h
}

// scrollSessionViewport updates sessionTop so that sessionCursor is within the
// visible window.  Each session occupies 2 display lines (title + meta).
func (m *model) scrollSessionViewport() {
	visibleH := m.sessionsVisibleH()
	if visibleH < 1 {
		visibleH = 1
	}
	if m.sessionCursor < m.sessionTop {
		m.sessionTop = m.sessionCursor
	}
	if m.sessionCursor >= m.sessionTop+visibleH {
		m.sessionTop = m.sessionCursor - visibleH + 1
	}
}

// sessionsVisibleH returns the number of session rows that fit in the sessions pane.
// Each session is 2 physical lines; we divide available height by 2.
func (m *model) sessionsVisibleH() int {
	contentH := m.height - 2 // bars
	if contentH < 1 {
		contentH = 1
	}
	h := contentH - 1 // pane title row
	// Each session row is 2 physical lines.
	rows := h / 2
	if rows < 1 {
		rows = 1
	}
	return rows
}

// --- commands ---------------------------------------------------------------

// cmdHealth returns a tea.Cmd that checks daemon health.
func cmdHealth(c Client) tea.Cmd {
	return func() tea.Msg {
		return msgHealthResult{err: c.Health()}
	}
}

// cmdLoadSessions returns a tea.Cmd that fetches the sessions list.
func cmdLoadSessions(c Client) tea.Cmd {
	return func() tea.Msg {
		sessions, err := c.Sessions()
		return msgSessionsLoaded{sessions: sessions, err: err}
	}
}

// cmdLoadEvents returns a tea.Cmd that fetches events for sessionID.
func cmdLoadEvents(c Client, sessionID string, sinceSeq int64) tea.Cmd {
	return func() tea.Msg {
		evs, err := c.Events(sessionID, sinceSeq)
		return msgEventsLoaded{sessionID: sessionID, events: evs, err: err}
	}
}

// --- helpers ----------------------------------------------------------------

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// selectedEvent returns the event to show in the inspector for the current
// selection. For a paired op it returns the Post (which has the result); for a
// standalone or running op it returns the Pre. Returns nil when nothing is selected.
func (m *model) selectedEvent() *event.Event {
	dr, ok := m.selectedDisplayRow()
	if !ok {
		return nil
	}
	if dr.IsPair && dr.Post != nil {
		return dr.Post
	}
	return dr.Pre
}

// daemonStatusText returns a terse string for the status bar.
func (m *model) daemonStatusText() string {
	if m.lastCheck.IsZero() {
		return "connecting…"
	}
	if m.daemonOK {
		return "daemon: ok"
	}
	return fmt.Sprintf("daemon: %s", truncateErr(m.daemonError, 30))
}

// truncateErr clips an error string for the status bar.
func truncateErr(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}
