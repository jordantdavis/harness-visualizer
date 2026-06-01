// internal/daemon/api_test.go
package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
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
