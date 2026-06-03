// internal/daemon/api_test.go
package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/model"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// newTestServer builds a Server over a temp store and appends the given events.
func newTestServer(t *testing.T, events ...*event.Event) *Server {
	t.Helper()
	st, err := store.New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, e := range events {
		if err := st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return NewServer(st)
}

func TestAPISessions_ReturnsArray(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []store.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != 1 || got[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %+v", got)
	}
}

// TestAPISessions_StartedAtPresentWhenEventsExist verifies that a session with
// at least one event with a non-zero CapturedAt exposes started_at in the JSON
// response.
func TestAPISessions_StartedAtPresentWhenEventsExist(t *testing.T) {
	ts := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	srv := newTestServer(t, &event.Event{
		SessionID:  "s2",
		HookEvent:  "SessionStart",
		CapturedAt: ts,
		Raw:        []byte(`{}`),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	var got []store.SessionInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != 1 {
		t.Fatalf("want 1 session, got %d", len(got))
	}
	if got[0].StartedAt.IsZero() {
		t.Errorf("started_at is zero; want %v", ts)
	}
	if !got[0].StartedAt.Equal(ts) {
		t.Errorf("started_at = %v, want %v", got[0].StartedAt, ts)
	}
}

func TestAPITimeline_MergesOpsAndTurns(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a","new_string":"b"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PostToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/timeline", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	var items []map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// No transcript on disk for s1 -> ops-only, exactly one operation item.
	if len(items) != 1 || items[0]["kind"] != "operation" {
		t.Fatalf("want one operation item, got %+v", items)
	}
}

func TestAPIOperationDetail_ReturnsDiff(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_input":{"file_path":"x.go","old_string":"a\nb","new_string":"a\nB"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PostToolUse", ToolName: "Edit",
			Raw: []byte(`{"tool_use_id":"a","tool_response":{"exit_code":0}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/a", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	var d struct {
		DetailKind string `json:"detail_kind"`
		FilePath   string `json:"file_path"`
		Diff       []struct {
			Kind string `json:"kind"`
			Text string `json:"text"`
		} `json:"diff"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if d.DetailKind != "diff" || d.FilePath != "x.go" || len(d.Diff) == 0 {
		t.Fatalf("unexpected detail: %+v", d)
	}
}

func TestAPIOperationDetail_NotFound(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/missing", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404", rec.Code)
	}
}

// An empty opID (trailing slash) must not match events lacking a tool_use_id.
func TestAPIOperationDetail_EmptyOpIDNotFound(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Bash",
			Raw: []byte(`{"tool_input":{"command":"echo hi"}}`)}, // no tool_use_id
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d, want 404 (empty opID must not match)", rec.Code)
	}
}

func TestRootServesWebPage(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("root status %d (body=%s)", rec.Code, rec.Body.String())
	}
}

// TestAPIOperationDetail_PostToolUseFailure verifies that the detail endpoint
// surfaces the post event when a tool call ends with PostToolUseFailure (not
// just PostToolUse). Without the fix, post would be nil and the response would
// carry an incomplete detail instead of the failure information.
func TestAPIOperationDetail_PostToolUseFailure(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Bash",
			Raw: []byte(`{"tool_use_id":"b","tool_input":{"command":"exit 1"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PostToolUseFailure", ToolName: "Bash",
			Raw: []byte(`{"tool_use_id":"b","tool_response":{"exit_code":1,"stderr":"command not found"}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/operations/b", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	// raw_post is only set when BuildOperationDetail receives a non-nil post
	// event. Before the fix, the PostToolUseFailure case was missing from the
	// switch, so post remained nil and raw_post was absent.
	var d struct {
		RawPost json.RawMessage `json:"raw_post"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &d); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(d.RawPost) == 0 {
		t.Fatalf("raw_post absent: PostToolUseFailure was not matched as the post event (body=%s)", rec.Body.String())
	}
}

func TestAPIStillRoutesUnderRootMount(t *testing.T) {
	srv := newTestServer(t, &event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: []byte(`{}`)})
	req := httptest.NewRequest(http.MethodGet, "/api/sessions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("/api/sessions status %d — root mount shadowed the API", rec.Code)
	}
}

func TestAPIHooks_ReturnsRegistry(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/hooks", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("content-type = %q", got)
	}
	var got []event.HookMeta
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	if len(got) != len(event.Hooks) {
		t.Errorf("got %d entries, want %d", len(got), len(event.Hooks))
	}
	if got[0].Name == "" || got[0].Glyph == "" {
		t.Errorf("first entry incomplete: %+v", got[0])
	}
}

func TestAPIHooks_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/hooks", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", rec.Code)
	}
}

// newTestServerInDir builds a Server over a store rooted at dir (so tests can
// inspect or perturb the on-disk session files) and appends the given events.
func newTestServerInDir(t *testing.T, dir string, events ...*event.Event) *Server {
	t.Helper()
	st, err := store.New(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	for _, e := range events {
		if err := st.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	return NewServer(st)
}

// TestAPISessionDelete_RemovesFile verifies DELETE returns 204 and the backing
// .jsonl file is gone from the sessions dir.
func TestAPISessionDelete_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerInDir(t, dir, &event.Event{SessionID: "s1", HookEvent: "Stop", Raw: []byte(`{}`)})
	path := filepath.Join(dir, "s1.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("precondition: session file missing: %v", err)
	}

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/s1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still present after DELETE (err=%v)", err)
	}
}

// TestAPISessionDelete_MissingIsIdempotent: deleting an unknown id is graceful
// (204, no 500).
func TestAPISessionDelete_MissingIsIdempotent(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/ghost", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent {
		t.Errorf("status %d, want 204", rec.Code)
	}
}

// TestAPISessionDelete_InvalidID: a non-safe id is rejected with 400.
func TestAPISessionDelete_InvalidID(t *testing.T) {
	srv := newTestServer(t)
	// "a.b" contains a dot, which sanitization would rewrite -> rejected.
	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/a.b", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status %d, want 400", rec.Code)
	}
}

// TestAPISessionDelete_MethodNotAllowed: a non-DELETE method on the bare-id
// path returns 405.
func TestAPISessionDelete_MethodNotAllowed(t *testing.T) {
	srv := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/sessions/s1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status %d, want 405", rec.Code)
	}
}

// TestAPISessionDelete_StoreErrorIs500: a real store failure (here: an
// unwritable sessions dir, so os.Remove fails with a permission error) surfaces
// as 500, not 204.
func TestAPISessionDelete_StoreErrorIs500(t *testing.T) {
	dir := t.TempDir()
	srv := newTestServerInDir(t, dir, &event.Event{SessionID: "s1", HookEvent: "Stop", Raw: []byte(`{}`)})

	// Removing a file requires write permission on its parent directory. Strip
	// it so os.Remove returns a non-NotExist error.
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // let t.TempDir clean up

	req := httptest.NewRequest(http.MethodDelete, "/api/sessions/s1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status %d, want 500 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestAPITimeline_IncludesLaneEvents(t *testing.T) {
	srv := newTestServer(t,
		&event.Event{SessionID: "s1", HookEvent: "PreToolUse", ToolName: "Bash",
			Raw: []byte(`{"tool_use_id":"u1","tool_input":{"command":"ls"}}`)},
		&event.Event{SessionID: "s1", HookEvent: "PermissionRequest", ToolName: "Bash",
			Raw: []byte(`{"tool_name":"Bash","tool_input":{"command":"ls"}}`)},
	)
	req := httptest.NewRequest(http.MethodGet, "/api/sessions/s1/timeline", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status %d (body=%s)", rec.Code, rec.Body.String())
	}
	var items []model.TimelineItem
	if err := json.Unmarshal(rec.Body.Bytes(), &items); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rec.Body.String())
	}
	var sawEvent bool
	for _, it := range items {
		if it.Kind == "event" && it.Event != nil && it.Event.HookEvent == "PermissionRequest" {
			sawEvent = true
			if it.Event.Lane != "permission" {
				t.Errorf("event.Lane = %q, want permission", it.Event.Lane)
			}
			if it.Event.Gist == "" {
				t.Errorf("event.Gist empty")
			}
		}
	}
	if !sawEvent {
		t.Errorf("no lane event in timeline; items=%+v", items)
	}
}
