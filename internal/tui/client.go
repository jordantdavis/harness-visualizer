package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/paths"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

// defaultPort is the well-known daemon port, used when the port file is absent.
const defaultPort = 7842

// Client is the interface the TUI uses to communicate with the daemon.
// It is factored out so tests can inject a fake without a running process.
type Client interface {
	// Sessions lists all recorded sessions.
	Sessions() ([]store.SessionInfo, error)

	// Events returns events for sessionID with Seq > sinceSeq, in order.
	// sinceSeq 0 returns all events.
	Events(sessionID string, sinceSeq int64) ([]*event.Event, error)

	// Health returns nil when the daemon is reachable and healthy.
	Health() error
}

// HTTPClient is the production Client that talks to a running daemon over HTTP.
type HTTPClient struct {
	base string       // e.g. "http://127.0.0.1:7842"
	http *http.Client // injectable for tests
}

// NewHTTPClient creates an HTTPClient that auto-discovers the daemon port from
// the port file (paths.PortFile), falling back to defaultPort if the file is
// absent or unreadable.
func NewHTTPClient() *HTTPClient {
	port := defaultPort
	if pf, err := paths.PortFile(); err == nil {
		if b, err := os.ReadFile(pf); err == nil {
			s := strings.TrimSpace(string(b))
			if p, err := strconv.Atoi(s); err == nil && p > 0 {
				port = p
			}
		}
	}
	return &HTTPClient{
		base: fmt.Sprintf("http://127.0.0.1:%d", port),
		http: &http.Client{Timeout: 10 * time.Second},
	}
}

// newHTTPClientAt creates an HTTPClient targeting baseURL directly. Used in tests.
func newHTTPClientAt(baseURL string) *HTTPClient {
	return &HTTPClient{
		base: baseURL,
		http: &http.Client{Timeout: 5 * time.Second},
	}
}

// Sessions implements Client.
func (c *HTTPClient) Sessions() ([]store.SessionInfo, error) {
	resp, err := c.http.Get(c.base + "/sessions")
	if err != nil {
		return nil, fmt.Errorf("GET /sessions: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /sessions: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET /sessions: read body: %w", err)
	}
	var infos []store.SessionInfo
	if err := json.Unmarshal(body, &infos); err != nil {
		return nil, fmt.Errorf("GET /sessions: decode: %w", err)
	}
	return infos, nil
}

// Events implements Client.
func (c *HTTPClient) Events(sessionID string, sinceSeq int64) ([]*event.Event, error) {
	url := fmt.Sprintf("%s/sessions/%s/events?since=%d", c.base, sessionID, sinceSeq)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("GET /sessions/%s/events: %w", sessionID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /sessions/%s/events: status %d", sessionID, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("GET /sessions/%s/events: read body: %w", sessionID, err)
	}
	var evs []*event.Event
	if err := json.Unmarshal(body, &evs); err != nil {
		return nil, fmt.Errorf("GET /sessions/%s/events: decode: %w", sessionID, err)
	}
	return evs, nil
}

// Health implements Client.
func (c *HTTPClient) Health() error {
	resp, err := c.http.Get(c.base + "/healthz")
	if err != nil {
		return fmt.Errorf("GET /healthz: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET /healthz: status %d", resp.StatusCode)
	}
	return nil
}

// FakeClient is a test double that serves canned data without a running daemon.
// Populate Sessions_ and Events_ directly before use.
type FakeClient struct {
	Sessions_ []store.SessionInfo
	Events_   map[string][]*event.Event // keyed by sessionID
	HealthErr error
}

// Sessions implements Client.
func (f *FakeClient) Sessions() ([]store.SessionInfo, error) {
	return f.Sessions_, nil
}

// Events implements Client. Filters by sinceSeq exactly as the daemon would.
func (f *FakeClient) Events(sessionID string, sinceSeq int64) ([]*event.Event, error) {
	all := f.Events_[sessionID]
	var out []*event.Event
	for _, ev := range all {
		if ev.Seq > sinceSeq {
			out = append(out, ev)
		}
	}
	return out, nil
}

// Health implements Client.
func (f *FakeClient) Health() error { return f.HealthErr }
