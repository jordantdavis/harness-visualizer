package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

// canonicalSessions is the fixture used by client tests.
var canonicalSessions = []store.SessionInfo{
	{ID: "sess-1", EventCount: 2, LastSeq: 2, ModTime: time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC)},
	{ID: "sess-2", EventCount: 1, LastSeq: 1, ModTime: time.Date(2026, 5, 28, 9, 0, 0, 0, time.UTC)},
}

// canonicalEvents are the fixture events for sess-1.
var canonicalEvents = []*event.Event{
	{
		ID: "e1", Seq: 1, HookEvent: "PreToolUse", SessionID: "sess-1",
		ToolName: "Bash", CapturedAt: time.Date(2026, 5, 28, 10, 0, 1, 0, time.UTC),
		Raw: json.RawMessage(`{"hook_event_name":"PreToolUse","tool_input":{"command":"ls"}}`),
	},
	{
		ID: "e2", Seq: 2, HookEvent: "PostToolUse", SessionID: "sess-1",
		ToolName: "Bash", CapturedAt: time.Date(2026, 5, 28, 10, 0, 2, 0, time.UTC),
		Raw: json.RawMessage(`{"hook_event_name":"PostToolUse","tool_response":{"exit_code":0}}`),
	},
}

// newTestServer spins up a minimal daemon stub for client tests.
func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/sessions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(canonicalSessions)
	})

	mux.HandleFunc("/sessions/", func(w http.ResponseWriter, r *http.Request) {
		// Pattern: /sessions/{id}/events
		// Extract query param since=
		sinceStr := r.URL.Query().Get("since")
		var since int64
		if sinceStr != "" {
			var v int64
			if _, err := fmt.Sscanf(sinceStr, "%d", &v); err == nil {
				since = v
			}
		}
		var out []*event.Event
		for _, ev := range canonicalEvents {
			if ev.Seq > since {
				out = append(out, ev)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"ok"}`))
	})

	return httptest.NewServer(mux)
}

func TestHTTPClientSessions(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := newHTTPClientAt(srv.URL)
	sessions, err := c.Sessions()
	if err != nil {
		t.Fatalf("Sessions() error: %v", err)
	}
	if len(sessions) != len(canonicalSessions) {
		t.Fatalf("Sessions() returned %d items, want %d", len(sessions), len(canonicalSessions))
	}
	if sessions[0].ID != "sess-1" {
		t.Errorf("sessions[0].ID = %q, want sess-1", sessions[0].ID)
	}
	if sessions[0].EventCount != 2 {
		t.Errorf("sessions[0].EventCount = %d, want 2", sessions[0].EventCount)
	}
}

func TestHTTPClientEventsAll(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := newHTTPClientAt(srv.URL)
	evs, err := c.Events("sess-1", 0)
	if err != nil {
		t.Fatalf("Events() error: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("Events() returned %d, want 2", len(evs))
	}
	if evs[0].HookEvent != "PreToolUse" {
		t.Errorf("evs[0].HookEvent = %q, want PreToolUse", evs[0].HookEvent)
	}
}

func TestHTTPClientEventsSince(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := newHTTPClientAt(srv.URL)
	evs, err := c.Events("sess-1", 1) // since seq=1 → only seq=2
	if err != nil {
		t.Fatalf("Events(since=1) error: %v", err)
	}
	if len(evs) != 1 {
		t.Fatalf("Events(since=1) returned %d, want 1", len(evs))
	}
	if evs[0].Seq != 2 {
		t.Errorf("evs[0].Seq = %d, want 2", evs[0].Seq)
	}
}

func TestHTTPClientHealth(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	c := newHTTPClientAt(srv.URL)
	if err := c.Health(); err != nil {
		t.Errorf("Health() unexpected error: %v", err)
	}
}

func TestHTTPClientHealthUnreachable(t *testing.T) {
	// Port that nothing is listening on.
	c := newHTTPClientAt("http://127.0.0.1:1")
	if err := c.Health(); err == nil {
		t.Error("Health() on unreachable server should return error")
	}
}

func TestFakeClientSessionsFilter(t *testing.T) {
	fake := &FakeClient{
		Sessions_: canonicalSessions,
		Events_:   map[string][]*event.Event{"sess-1": canonicalEvents},
	}

	sessions, err := fake.Sessions()
	if err != nil {
		t.Fatalf("FakeClient.Sessions() error: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("FakeClient.Sessions() returned %d, want 2", len(sessions))
	}

	evs, err := fake.Events("sess-1", 1)
	if err != nil {
		t.Fatalf("FakeClient.Events() error: %v", err)
	}
	if len(evs) != 1 || evs[0].Seq != 2 {
		t.Errorf("FakeClient.Events(since=1) = %+v, want [{Seq:2}]", evs)
	}
}

func TestFakeClientMissingSession(t *testing.T) {
	fake := &FakeClient{Events_: map[string][]*event.Event{}}
	evs, err := fake.Events("no-such", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("expected empty slice for missing session, got %v", evs)
	}
}
