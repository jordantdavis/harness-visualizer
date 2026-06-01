// internal/daemon/api.go
package daemon

import (
	"encoding/json"
	"net/http"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/model"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode"
	"jordandavis.dev/harness-visualizer/internal/store"
)

// handleAPISessions: GET /api/sessions — same payload as /sessions, under /api.
func (s *Server) handleAPISessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infos, err := s.st.Sessions()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if infos == nil {
		infos = []store.SessionInfo{}
	}
	writeJSON(w, infos)
}

// handleAPISession routes GET /api/sessions/{id}/timeline and
// GET /api/sessions/{id}/operations/{opID}.
func (s *Server) handleAPISession(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rest := strings.TrimPrefix(r.URL.Path, "/api/sessions/")
	id, tail, _ := strings.Cut(rest, "/")
	switch {
	case tail == "timeline":
		s.handleAPITimeline(w, r, id)
	case strings.HasPrefix(tail, "operations/"):
		s.handleAPIOperation(w, r, id, strings.TrimPrefix(tail, "operations/"))
	default:
		http.NotFound(w, r)
	}
}

// handleAPITimeline builds the merged, interleaved timeline for a session.
func (s *Server) handleAPITimeline(w http.ResponseWriter, r *http.Request, id string) {
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ops := model.BuildOperations(events)
	turns, _ := claudecode.ReadConversation(transcriptPathFromEvents(events))
	items := model.MergeTimeline(ops, turns)
	if items == nil {
		items = []model.TimelineItem{}
	}
	writeJSON(w, items)
}

// transcriptPathFromEvents pulls the last non-empty transcript_path from the
// raw payloads. Returns "" when none is present (-> graceful degradation).
func transcriptPathFromEvents(events []*event.Event) string {
	path := ""
	for _, e := range events {
		var w struct {
			TranscriptPath string `json:"transcript_path"`
		}
		if json.Unmarshal(e.Raw, &w) == nil && w.TranscriptPath != "" {
			path = w.TranscriptPath
		}
	}
	return path
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

// handleAPIOperation returns the heavy detail for one operation, identified by
// its tool_use_id. It re-reads the session and locates the Pre/Post pair.
func (s *Server) handleAPIOperation(w http.ResponseWriter, r *http.Request, id, opID string) {
	// An empty opID (e.g. trailing-slash ".../operations/") must not match
	// events that simply lack a tool_use_id; treat it as not-found.
	if opID == "" {
		http.NotFound(w, r)
		return
	}
	events, err := s.st.Read(id, 0)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	var pre, post *event.Event
	for _, e := range events {
		if toolUseIDOf(e.Raw) != opID {
			continue
		}
		switch e.HookEvent {
		case "PreToolUse":
			pre = e
		case "PostToolUse", "PostToolUseFailure":
			post = e
		}
	}
	if pre == nil {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, model.BuildOperationDetail(pre, post))
}

// toolUseIDOf extracts the top-level tool_use_id from a raw payload.
func toolUseIDOf(raw json.RawMessage) string {
	var w struct {
		ToolUseID string `json:"tool_use_id"`
	}
	if json.Unmarshal(raw, &w) != nil {
		return ""
	}
	return w.ToolUseID
}
