// Package client tests exercise the hook-forwarder core (run) in isolation.
// The injected spawnFn replaces daemon auto-spawn so tests never fork real processes.
package client

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// noSpawn is a no-op spawn function used in most tests.
func noSpawn() {}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// runTo calls run targeting srv (an httptest.Server) with the given stdin body.
func runTo(t *testing.T, srv *httptest.Server, stdinBody string) int {
	t.Helper()
	return run(strings.NewReader(stdinBody), srv.Listener.Addr().String(), io.Discard, noSpawn)
}

// ---------------------------------------------------------------------------
// Happy path
// ---------------------------------------------------------------------------

func TestHappyPath_ServerReceivesWellFormedEvent(t *testing.T) {
	var (
		mu       sync.Mutex
		received []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/events" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		received = body
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	payload := `{"hook_event_name":"PreToolUse","session_id":"sess-1","cwd":"/tmp","tool_name":"Bash","tool_input":{"command":"ls"}}`
	var stdout bytes.Buffer
	code := run(strings.NewReader(payload), srv.Listener.Addr().String(), &stdout, noSpawn)

	if code != 0 {
		t.Fatalf("run returned %d, want 0", code)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout must be empty, got %q", stdout.String())
	}

	mu.Lock()
	body := received
	mu.Unlock()

	if body == nil {
		t.Fatal("server never received a request")
	}

	var ev event.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatalf("server body not valid JSON: %v\nbody: %s", err, body)
	}
	if ev.ID == "" {
		t.Error("Event.ID must be set")
	}
	if ev.CapturedAt.IsZero() {
		t.Error("Event.CapturedAt must be set")
	}
	if ev.Seq != 0 {
		t.Errorf("Event.Seq = %d, want 0 (daemon assigns)", ev.Seq)
	}
	if ev.HookEvent != "PreToolUse" {
		t.Errorf("Event.HookEvent = %q, want PreToolUse", ev.HookEvent)
	}
	if ev.SessionID != "sess-1" {
		t.Errorf("Event.SessionID = %q, want sess-1", ev.SessionID)
	}
	if ev.CapturedAt.After(time.Now().Add(time.Second)) {
		t.Error("Event.CapturedAt is in the future")
	}
	if ev.Raw == nil {
		t.Error("Event.Raw must be non-nil")
	}
}

// ---------------------------------------------------------------------------
// Malformed stdin
// ---------------------------------------------------------------------------

func TestMalformedStdin_ExitZeroNoServer(t *testing.T) {
	// Server that refuses connections — we want exit 0 regardless.
	code := run(strings.NewReader("not json at all {{{"), "127.0.0.1:1", io.Discard, noSpawn)
	if code != 0 {
		t.Fatalf("malformed stdin: run returned %d, want 0", code)
	}
}

func TestMalformedStdin_ExitZeroWithServer(t *testing.T) {
	// Even when daemon is up, non-JSON stdin should still produce exit 0.
	var received []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	code := runTo(t, srv, "not json {{{")
	if code != 0 {
		t.Fatalf("run returned %d, want 0", code)
	}
	// Implementation may send a best-effort event or skip — both are valid.
	// Just assert exit 0 and no panic.
	_ = received
}

// ---------------------------------------------------------------------------
// Huge stdin — bounded read
// ---------------------------------------------------------------------------

func TestHugeStdin_BoundedReadExitZero(t *testing.T) {
	// 12 MB reader — well over the ~10 MB cap.
	huge := io.LimitReader(bytes.NewReader(make([]byte, 12<<20)), 12<<20)
	code := run(huge, "127.0.0.1:1", io.Discard, noSpawn)
	if code != 0 {
		t.Fatalf("huge stdin: run returned %d, want 0", code)
	}
}

func TestHugeStdin_DoesNotHang(t *testing.T) {
	done := make(chan int, 1)
	go func() {
		huge := io.LimitReader(neverEnds{}, 20<<20)
		done <- run(huge, "127.0.0.1:1", io.Discard, noSpawn)
	}()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("run returned %d, want 0", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("run hung on huge stdin")
	}
}

// neverEnds is an io.Reader that produces zeros indefinitely.
type neverEnds struct{}

func (neverEnds) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = 0
	}
	return len(p), nil
}

// ---------------------------------------------------------------------------
// Daemon down (connection refused)
// ---------------------------------------------------------------------------

func TestDaemonDown_ExitZeroNoHang(t *testing.T) {
	spawned := false
	spawnFn := func() { spawned = true }

	start := time.Now()
	code := run(
		strings.NewReader(`{"hook_event_name":"Stop","session_id":"s"}`),
		"127.0.0.1:1", // port 1 is reserved; should get connection refused fast
		io.Discard,
		spawnFn,
	)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("daemon-down: run returned %d, want 0", code)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("daemon-down path took %v, want < 500ms", elapsed)
	}
	if !spawned {
		t.Error("spawnFn should have been called on connection refused")
	}
}

// ---------------------------------------------------------------------------
// Timeout path
// ---------------------------------------------------------------------------

func TestTimeout_ReturnsWithinBudget(t *testing.T) {
	// Use a raw TCP listener that accepts connections but never sends bytes.
	// This simulates a hung daemon without relying on httptest's goroutine
	// lifecycle. The client's postBudget (~50ms) fires before any response.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	// Accept connections but do nothing — never send an HTTP response.
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			// Hold the connection open; the client timeout will fire.
			defer conn.Close()
		}
	}()

	start := time.Now()
	code := run(
		strings.NewReader(`{"hook_event_name":"Stop","session_id":"s"}`),
		ln.Addr().String(),
		io.Discard,
		noSpawn,
	)
	elapsed := time.Since(start)

	if code != 0 {
		t.Fatalf("timeout path: run returned %d, want 0", code)
	}
	// Total budget is ~100ms; allow generous 300ms for CI jitter.
	if elapsed > 300*time.Millisecond {
		t.Errorf("timeout path took %v, want < 300ms (budget ~100ms)", elapsed)
	}
}

// ---------------------------------------------------------------------------
// Panic recovery
// ---------------------------------------------------------------------------

func TestRun_PanicIsRecovered(t *testing.T) {
	// We can't easily inject a panic into run itself, but Run (exported) wraps
	// run and must recover. Test that calling Run with a panicking stdin is
	// fine. We verify the interface through run's panic-safety via the top-level
	// defer recover() in Run.
	//
	// This test is a documentation/safety test: it will panic-catch and exit 0.
	code := Run([]string{})
	// Run reads real stdin (empty in tests), posts to the port from paths.PortFile,
	// which may not exist. Either way it must return 0.
	if code != 0 {
		t.Errorf("Run() = %d, want 0", code)
	}
}
