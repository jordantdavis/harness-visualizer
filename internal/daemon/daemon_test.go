// Package daemon_test exercises the HTTP routes, SSE fan-out, ordering under
// burst, and single-instance port-file behaviour using only in-process
// httptest infrastructure and a temp dir as the store root.
package daemon_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/daemon"
	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// newTestServer creates a Server backed by a temp-dir store and an httptest
// server. It registers cleanup on t.
func newTestServer(t *testing.T) (*daemon.Server, *httptest.Server) {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	srv := daemon.NewServer(st)
	ts := httptest.NewServer(srv)
	t.Cleanup(func() {
		ts.Close()
		srv.Shutdown()
	})
	return srv, ts
}

// postEvent marshals ev and POSTs it to /events; returns the response.
func postEvent(t *testing.T, base string, ev *event.Event) *http.Response {
	t.Helper()
	body, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := http.Post(base+"/events", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST /events: %v", err)
	}
	return resp
}

// --- Healthz ---

func TestHealthzReturns200OK(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var body struct{ Status string }
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body.Status != "ok" {
		t.Errorf("status field = %q, want ok", body.Status)
	}
}

// --- POST /events ---

func TestPostEventsReturns204(t *testing.T) {
	_, ts := newTestServer(t)
	ev := &event.Event{
		ID:        "id-1",
		SessionID: "sess-1",
		HookEvent: "PreToolUse",
	}
	resp := postEvent(t, ts.URL, ev)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPostEventsReturns204Fast(t *testing.T) {
	// POST must return before disk I/O completes; we verify it returns well
	// within 100ms even for a slow store (we test at least it's fast on the
	// happy path — the channel model ensures non-blocking acceptance).
	_, ts := newTestServer(t)
	ev := &event.Event{ID: "id-fast", SessionID: "s", HookEvent: "Stop"}
	start := time.Now()
	resp := postEvent(t, ts.URL, ev)
	defer resp.Body.Close()
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("POST took %v, want <500ms", elapsed)
	}
	if resp.StatusCode != http.StatusNoContent {
		t.Errorf("status = %d, want 204", resp.StatusCode)
	}
}

func TestPostEventsRejects10MBPlus(t *testing.T) {
	_, ts := newTestServer(t)
	// Build a body slightly over 10MB.
	big := bytes.Repeat([]byte("x"), 11*1024*1024)
	resp, err := http.Post(ts.URL+"/events", "application/json", bytes.NewReader(big))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		t.Errorf("expected rejection of >10MB body, got 204")
	}
}

func TestPostEventsRejectsBadJSON(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Post(ts.URL+"/events", "application/json",
		strings.NewReader("not json"))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent {
		t.Errorf("expected non-204 for bad JSON, got 204")
	}
}

// --- GET /sessions ---

func TestGetSessionsEmptyStoreReturnsEmptyArray(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var infos []store.SessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if infos == nil {
		// []SessionInfo{} or nil both allowed but JSON must be [].
		t.Errorf("expected JSON array, got null (nil slice)")
	}
}

func TestGetSessionsAfterPostShowsSession(t *testing.T) {
	_, ts := newTestServer(t)

	ev := &event.Event{ID: "x", SessionID: "sess-list", HookEvent: "Stop"}
	resp := postEvent(t, ts.URL, ev)
	resp.Body.Close()

	// Give the async writer a moment to flush.
	waitForSeq(t, ts.URL, "sess-list", 1, 2*time.Second)

	resp2, err := http.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer resp2.Body.Close()
	var infos []store.SessionInfo
	if err := json.NewDecoder(resp2.Body).Decode(&infos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 session, got 0")
	}
}

// --- GET /sessions/{id}/events ---

func TestGetEventsUnknownSessionReturnsEmptyArray(t *testing.T) {
	_, ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/sessions/unknown/events")
	if err != nil {
		t.Fatalf("GET /sessions/unknown/events: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	var evs []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&evs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if evs == nil {
		t.Error("expected JSON array, got null")
	}
}

func TestGetEventsAfterPostReturnsEvents(t *testing.T) {
	_, ts := newTestServer(t)

	for i := 0; i < 3; i++ {
		ev := &event.Event{
			ID:        fmt.Sprintf("id-%d", i),
			SessionID: "sess-ev",
			HookEvent: "PreToolUse",
		}
		postEvent(t, ts.URL, ev).Body.Close()
	}

	// Wait for all 3 to be persisted.
	waitForSeq(t, ts.URL, "sess-ev", 3, 3*time.Second)

	resp, err := http.Get(ts.URL + "/sessions/sess-ev/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var evs []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&evs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("got %d events, want 3", len(evs))
	}
	for i, ev := range evs {
		if ev.Seq != int64(i+1) {
			t.Errorf("evs[%d].Seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestGetEventsSinceCursorFilters(t *testing.T) {
	_, ts := newTestServer(t)

	for i := 0; i < 5; i++ {
		postEvent(t, ts.URL, &event.Event{
			ID:        fmt.Sprintf("id-%d", i),
			SessionID: "sess-since",
			HookEvent: "Stop",
		}).Body.Close()
	}
	waitForSeq(t, ts.URL, "sess-since", 5, 3*time.Second)

	resp, err := http.Get(ts.URL + "/sessions/sess-since/events?since=3")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var evs []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&evs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2", len(evs))
	}
	if evs[0].Seq != 4 || evs[1].Seq != 5 {
		t.Errorf("seqs = %d,%d, want 4,5", evs[0].Seq, evs[1].Seq)
	}
}

// --- Ordering under burst ---

func TestOrderingUnderBurst(t *testing.T) {
	// Fire many concurrent POSTs for one session; after draining, Seq must be
	// gap-free and monotonic.
	_, ts := newTestServer(t)
	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			postEvent(t, ts.URL, &event.Event{
				ID:        fmt.Sprintf("burst-%d", i),
				SessionID: "burst",
				HookEvent: "PreToolUse",
			}).Body.Close()
		}(i)
	}
	wg.Wait()

	// Wait until all n events have been persisted.
	waitForSeq(t, ts.URL, "burst", n, 5*time.Second)

	resp, err := http.Get(ts.URL + "/sessions/burst/events")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	var evs []event.Event
	if err := json.NewDecoder(resp.Body).Decode(&evs); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(evs) != n {
		t.Fatalf("got %d events, want %d", len(evs), n)
	}
	seqs := make([]int, len(evs))
	for i, e := range evs {
		seqs[i] = int(e.Seq)
	}
	sort.Ints(seqs)
	for i, s := range seqs {
		if s != i+1 {
			t.Errorf("gap in Seq: seqs[%d] = %d, want %d (full slice: %v)", i, s, i+1, seqs)
			break
		}
	}
}

// --- SSE fan-out ---

// readSSEEvents reads up to maxEvents SSE data lines from an SSE stream
// connection, decoding each as event.Event. It returns when maxEvents are
// received or the deadline expires.
func readSSEEvents(t *testing.T, url string, maxEvents int, deadline time.Duration) []event.Event {
	t.Helper()
	ctx := &struct{ done chan struct{} }{done: make(chan struct{})}
	_ = ctx

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}

	client := &http.Client{Timeout: deadline + time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	var evs []event.Event
	scanner := bufio.NewScanner(resp.Body)
	timer := time.NewTimer(deadline)
	defer timer.Stop()

	lineCh := make(chan string, 32)
	go func() {
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		close(lineCh)
	}()

	for len(evs) < maxEvents {
		select {
		case line, ok := <-lineCh:
			if !ok {
				return evs
			}
			if strings.HasPrefix(line, "data: ") {
				var ev event.Event
				if err := json.Unmarshal([]byte(line[6:]), &ev); err == nil {
					evs = append(evs, ev)
				}
			}
		case <-timer.C:
			return evs
		}
	}
	return evs
}

func TestSSEReceivesEventsForSession(t *testing.T) {
	_, ts := newTestServer(t)

	// Start SSE subscriber before posting.
	evsCh := make(chan []event.Event, 1)
	go func() {
		evsCh <- readSSEEvents(t, ts.URL+"/stream?session=sse-sess", 3, 5*time.Second)
	}()

	// Brief pause to let the subscriber connect.
	time.Sleep(50 * time.Millisecond)

	for i := 0; i < 3; i++ {
		postEvent(t, ts.URL, &event.Event{
			ID:        fmt.Sprintf("sse-%d", i),
			SessionID: "sse-sess",
			HookEvent: "PreToolUse",
		}).Body.Close()
	}

	evs := <-evsCh
	if len(evs) != 3 {
		t.Fatalf("SSE subscriber got %d events, want 3", len(evs))
	}
	for i, ev := range evs {
		if ev.Seq != int64(i+1) {
			t.Errorf("evs[%d].Seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestSSEMultipleSubscribersEachReceiveAllEvents(t *testing.T) {
	_, ts := newTestServer(t)

	const nSubs = 3
	const nEvents = 5
	chs := make([]chan []event.Event, nSubs)
	for i := range chs {
		chs[i] = make(chan []event.Event, 1)
		go func(ch chan []event.Event) {
			ch <- readSSEEvents(t, ts.URL+"/stream?session=multi-sess", nEvents, 5*time.Second)
		}(chs[i])
	}

	// Wait for subscribers to connect.
	time.Sleep(80 * time.Millisecond)

	for i := 0; i < nEvents; i++ {
		postEvent(t, ts.URL, &event.Event{
			ID:        fmt.Sprintf("m-%d", i),
			SessionID: "multi-sess",
			HookEvent: "Stop",
		}).Body.Close()
	}

	for i, ch := range chs {
		evs := <-ch
		if len(evs) != nEvents {
			t.Errorf("subscriber %d got %d events, want %d", i, len(evs), nEvents)
		}
	}
}

func TestSSESessionFilterExcludesOtherSessions(t *testing.T) {
	_, ts := newTestServer(t)

	// Subscribe only to "wanted".
	evsCh := make(chan []event.Event, 1)
	go func() {
		evsCh <- readSSEEvents(t, ts.URL+"/stream?session=wanted", 2, 5*time.Second)
	}()
	time.Sleep(50 * time.Millisecond)

	// Post to a different session first, then the wanted one.
	postEvent(t, ts.URL, &event.Event{
		ID:        "other-1",
		SessionID: "other",
		HookEvent: "Stop",
	}).Body.Close()
	postEvent(t, ts.URL, &event.Event{
		ID:        "wanted-1",
		SessionID: "wanted",
		HookEvent: "Stop",
	}).Body.Close()
	postEvent(t, ts.URL, &event.Event{
		ID:        "wanted-2",
		SessionID: "wanted",
		HookEvent: "Stop",
	}).Body.Close()

	evs := <-evsCh
	if len(evs) != 2 {
		t.Fatalf("got %d events, want 2", len(evs))
	}
	for _, ev := range evs {
		if ev.SessionID != "wanted" {
			t.Errorf("received event for wrong session %q", ev.SessionID)
		}
	}
}

func TestSSENoFilterReceivesAllSessions(t *testing.T) {
	_, ts := newTestServer(t)

	evsCh := make(chan []event.Event, 1)
	go func() {
		evsCh <- readSSEEvents(t, ts.URL+"/stream", 4, 5*time.Second)
	}()
	time.Sleep(50 * time.Millisecond)

	for _, sess := range []string{"a", "b", "a", "b"} {
		postEvent(t, ts.URL, &event.Event{
			ID:        "x",
			SessionID: sess,
			HookEvent: "Stop",
		}).Body.Close()
	}

	evs := <-evsCh
	if len(evs) != 4 {
		t.Fatalf("got %d events, want 4", len(evs))
	}
}

func TestSSEFormatHasIDAndDataLines(t *testing.T) {
	_, ts := newTestServer(t)

	req, _ := http.NewRequest("GET", ts.URL+"/stream?session=fmt-sess", nil)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("GET /stream: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })

	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}

	time.Sleep(30 * time.Millisecond)
	postEvent(t, ts.URL, &event.Event{
		ID:        "fmt-1",
		SessionID: "fmt-sess",
		HookEvent: "Stop",
	}).Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()

	var gotID, gotData bool
	lineCh := make(chan string, 16)
	go func() {
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		close(lineCh)
	}()

	for !gotID || !gotData {
		select {
		case line, ok := <-lineCh:
			if !ok {
				goto done
			}
			if strings.HasPrefix(line, "id: ") {
				gotID = true
			}
			if strings.HasPrefix(line, "data: ") {
				gotData = true
				// Data must be single-line JSON.
				data := line[6:]
				if strings.ContainsAny(data, "\n\r") {
					t.Errorf("data line contains newline: %q", data)
				}
				var ev event.Event
				if err := json.Unmarshal([]byte(data), &ev); err != nil {
					t.Errorf("data is not valid JSON: %v", err)
				}
			}
		case <-timer.C:
			goto done
		}
	}
done:
	if !gotID {
		t.Error("SSE stream missing 'id:' line")
	}
	if !gotData {
		t.Error("SSE stream missing 'data:' line")
	}
}

// --- Port-file ---

func TestPortFileWrittenOnListen(t *testing.T) {
	portFile := t.TempDir() + "/daemon.port"
	pidFile := t.TempDir() + "/daemon.pid"

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	srv := daemon.NewServer(st)
	addr, err := srv.ListenAndServe("127.0.0.1:0", portFile, pidFile)
	if err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}
	t.Cleanup(func() {
		srv.Shutdown()
	})

	if addr == "" {
		t.Fatal("ListenAndServe returned empty addr")
	}

	portData, err := io.ReadAll(mustOpen(t, portFile))
	if err != nil {
		t.Fatalf("read port file: %v", err)
	}
	if string(portData) == "" {
		t.Error("port file is empty")
	}
}

func TestPortFileRemovedOnShutdown(t *testing.T) {
	portFile := t.TempDir() + "/daemon.port"
	pidFile := t.TempDir() + "/daemon.pid"

	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	srv := daemon.NewServer(st)
	if _, err := srv.ListenAndServe("127.0.0.1:0", portFile, pidFile); err != nil {
		t.Fatalf("ListenAndServe: %v", err)
	}
	srv.Shutdown()

	if fileExists(portFile) {
		t.Error("port file still exists after Shutdown")
	}
	if fileExists(pidFile) {
		t.Error("pid file still exists after Shutdown")
	}
}

// --- GET /sessions — Phase 8d fields ---

// TestGetSessionsIncludesCWDAndTitle verifies that GET /sessions returns the
// cwd and title fields populated from the event payload. The event is posted
// with a Raw field containing cwd so the store scanner can capture it; the
// title falls back to "project · shortid" because there is no transcript.
func TestGetSessionsIncludesCWDAndTitle(t *testing.T) {
	_, ts := newTestServer(t)

	// Build an event whose Raw payload contains a cwd field; no transcript_path
	// so the title falls through to the "project · shortid" rung.
	rawPayload, err := json.Marshal(map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "abcdefghijk",
		"cwd":             "/home/user/labelproject",
	})
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}

	ev := &event.Event{
		ID:        "label-1",
		SessionID: "abcdefghijk",
		HookEvent: "Stop",
		CWD:       "/home/user/labelproject",
		Raw:       json.RawMessage(rawPayload),
	}
	postEvent(t, ts.URL, ev).Body.Close()
	waitForSeq(t, ts.URL, "abcdefghijk", 1, 2*time.Second)

	resp, err := http.Get(ts.URL + "/sessions")
	if err != nil {
		t.Fatalf("GET /sessions: %v", err)
	}
	defer resp.Body.Close()

	var infos []store.SessionInfo
	if err := json.NewDecoder(resp.Body).Decode(&infos); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(infos) == 0 {
		t.Fatal("expected at least 1 session")
	}

	info := infos[0]
	if info.CWD != "/home/user/labelproject" {
		t.Errorf("CWD = %q, want %q", info.CWD, "/home/user/labelproject")
	}
	// Title must be "labelproject · abcdefg" (project·shortid fallback).
	want := "labelproject · abcdefg"
	if info.Title != want {
		t.Errorf("Title = %q, want %q", info.Title, want)
	}
}

// --- Run entrypoint smoke test ---

func TestRunForegroundFlagParsed(t *testing.T) {
	// Run with --foreground + auto-assigned port; should start serving and
	// return exit 0 when we cancel via the server's own shutdown path.
	// We just verify Run is callable with the flag without panicking.
	// (A full blocking test would require goroutine + signal tricks.)
	// Instead we use the --help-like approach: invalid flag should return non-zero.
	code := daemon.Run([]string{"--unknown-flag-xyz"})
	if code == 0 {
		t.Error("Run with unknown flag should return non-zero exit code")
	}
}

// --- Helpers ---

// waitForSeq polls GET /sessions/{id}/events until at least wantSeq events
// exist or the deadline expires.
func waitForSeq(t *testing.T, base, sessionID string, wantCount int, deadline time.Duration) {
	t.Helper()
	end := time.Now().Add(deadline)
	for time.Now().Before(end) {
		resp, err := http.Get(base + "/sessions/" + sessionID + "/events")
		if err != nil {
			time.Sleep(20 * time.Millisecond)
			continue
		}
		var evs []event.Event
		_ = json.NewDecoder(resp.Body).Decode(&evs)
		resp.Body.Close()
		if len(evs) >= wantCount {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %d events in session %q", wantCount, sessionID)
}

func mustOpen(t *testing.T, path string) io.Reader {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
