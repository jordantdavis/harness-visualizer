package tui

import (
	"fmt"
	"strings"
	"time"

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

	// layout
	layout Layout

	// sessions pane state
	sessions      []store.SessionInfo
	sessionCursor int // index into sessions
	sessionsErr   error

	// events pane state
	selectedSession string
	events          []*event.Event
	eventCursor     int // index into events
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

	// NO_COLOR
	noColor bool
}

// newModel constructs a model with the given client. noColor should reflect
// the NO_COLOR env var and whether the terminal supports color.
func newModel(c Client, noColor bool) model {
	return model{
		client:  c,
		noColor: noColor,
		layout:  LayoutNarrow,
	}
}

// Init sends the initial commands: health check + session load.
func (m model) Init() tea.Cmd {
	return tea.Batch(
		cmdHealth(m.client),
		cmdLoadSessions(m.client),
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
			if len(m.sessions) > 0 && m.selectedSession == "" {
				// Auto-select the first session.
				m.selectedSession = m.sessions[0].ID
				m.eventsLoading = true
				return m, cmdLoadEvents(m.client, m.sessions[0].ID, 0)
			}
		}
		return m, nil

	case msgEventsLoaded:
		if msg.sessionID == m.selectedSession {
			m.eventsLoading = false
			m.eventsErr = msg.err
			if msg.err == nil {
				m.events = msg.events
				m.eventCursor = 0
			}
		}
		return m, nil

	case tea.KeyMsg:
		if m.rawPager {
			return m.updateRawPager(msg)
		}
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		return m.updateKeys(msg)
	}

	return m, nil
}

// updateKeys handles keyboard input in normal (non-pager, non-help) mode.
func (m model) updateKeys(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "?":
		m.showHelp = true
		return m, nil

	// pane switching
	case "tab":
		m.focusedPane = nextPane(m.focusedPane, m.layout)
		return m, nil
	case "1":
		m.focusedPane = paneSessions
		return m, nil
	case "2":
		m.focusedPane = paneEvents
		return m, nil
	case "3":
		if m.layout == LayoutWide {
			m.focusedPane = paneInspector
		}
		return m, nil
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

// updateSessionsPane handles keys when the sessions pane is focused.
func (m model) updateSessionsPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.sessionCursor < len(m.sessions)-1 {
			m.sessionCursor++
		}
		return m, nil
	case "k", "up":
		if m.sessionCursor > 0 {
			m.sessionCursor--
		}
		return m, nil
	case "g":
		m.sessionCursor = 0
		return m, nil
	case "G":
		if len(m.sessions) > 0 {
			m.sessionCursor = len(m.sessions) - 1
		}
		return m, nil
	case "ctrl+d":
		m.sessionCursor = min(m.sessionCursor+10, max(0, len(m.sessions)-1))
		return m, nil
	case "ctrl+u":
		m.sessionCursor = max(m.sessionCursor-10, 0)
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
			m.eventsLoading = true
		}
		m.focusedPane = paneEvents
		return m, cmdLoadEvents(m.client, sid, 0)
	}
	return m, nil
}

// updateEventsPane handles keys when the events pane is focused.
func (m model) updateEventsPane(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "j", "down":
		if m.eventCursor < len(m.events)-1 {
			m.eventCursor++
		}
	case "k", "up":
		if m.eventCursor > 0 {
			m.eventCursor--
		}
	case "g":
		m.eventCursor = 0
	case "G":
		if len(m.events) > 0 {
			m.eventCursor = len(m.events) - 1
		}
	case "ctrl+d":
		m.eventCursor = min(m.eventCursor+10, max(0, len(m.events)-1))
	case "ctrl+u":
		m.eventCursor = max(m.eventCursor-10, 0)
	case "enter", "l":
		if len(m.events) > 0 {
			m.focusedPane = paneInspector
			m.inspectorOpen = true
		}
	case "esc", "h":
		m.focusedPane = paneSessions
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
	case "esc", "h":
		m.focusedPane = paneEvents
	}
	return m, nil
}

// updateRawPager handles keys while the raw JSON pager is open.
func (m model) updateRawPager(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "q", "esc":
		m.rawPager = false
	}
	return m, nil
}

// nextPane cycles focus through the panes available in the current layout.
func nextPane(current pane, layout Layout) pane {
	switch layout {
	case LayoutWide:
		return (current + 1) % 3
	default:
		// 2-pane (medium) or narrow: cycle sessions↔events
		if current == paneSessions {
			return paneEvents
		}
		return paneSessions
	}
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

// selectedEvent returns the currently selected event, or nil.
func (m *model) selectedEvent() *event.Event {
	if len(m.events) == 0 || m.eventCursor >= len(m.events) {
		return nil
	}
	return m.events[m.eventCursor]
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
