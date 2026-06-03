// Package store persists captured events as JSONL, one file per session. The
// daemon is the only writer in production; Store is therefore optimized for a
// single in-process owner but is fully thread-safe. Append assigns a
// monotonic per-session sequence number. The directory is injected so the
// store is testable against a temp dir.
package store

import (
	"bufio"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/paths"
)

// ErrInvalidSessionID is returned by Delete when the given id is not safe to
// resolve to a file directly inside the sessions directory (empty, contains a
// path separator, traversal, or any character sanitization would rewrite).
// Callers should treat it as a client error (e.g. HTTP 400), not a server
// fault — no disk is touched.
var ErrInvalidSessionID = errors.New("store: invalid session id")

// transcriptCacheKey uniquely identifies a cached transcript parse by file
// path and modification time.
type transcriptCacheKey struct {
	path  string
	mtime time.Time
}

const (
	scanBufInit = 64 * 1024
	scanBufMax  = 16 * 1024 * 1024
)

// Store appends and reads per-session JSONL event logs under dir.
type Store struct {
	dir string

	mu    sync.Mutex
	seq   map[string]int64    // sessionID -> last assigned seq
	files map[string]*os.File // sessionID -> open append handle

	// transcriptMu guards transcriptCache; separate from mu so transcript
	// lookups during Sessions() do not contend with Append.
	transcriptMu    sync.Mutex
	transcriptCache map[transcriptCacheKey]transcriptResult // mtime-keyed cache
}

// New creates a Store rooted at dir, creating dir if absent.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dir:             dir,
		seq:             make(map[string]int64),
		files:           make(map[string]*os.File),
		transcriptCache: make(map[transcriptCacheKey]transcriptResult),
	}, nil
}

// Append assigns ev.Seq (next monotonic value for ev.SessionID) and writes ev
// as a single JSON line. Safe for concurrent use.
func (s *Store) Append(ev *event.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	f, err := s.fileFor(ev.SessionID)
	if err != nil {
		return err
	}
	s.seq[ev.SessionID]++
	ev.Seq = s.seq[ev.SessionID]

	line, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_, err = f.Write(line)
	return err
}

// fileFor returns the cached append handle for sessionID, opening it if
// needed and priming the seq counter from the file's last event. Caller must
// hold s.mu.
func (s *Store) fileFor(sessionID string) (*os.File, error) {
	if f, ok := s.files[sessionID]; ok {
		return f, nil
	}
	path := filepath.Join(s.dir, paths.SessionFilename(sessionID))

	if _, primed := s.seq[sessionID]; !primed {
		if sr, err := scanSession(path); err == nil {
			s.seq[sessionID] = sr.last
		}
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	s.files[sessionID] = f
	return f, nil
}

// Read returns the events for sessionID whose Seq is strictly greater than
// sinceSeq, in file (chronological) order. A missing session file yields an
// empty slice and no error. Malformed lines are skipped so a corrupt write
// cannot make history unreadable.
func (s *Store) Read(sessionID string, sinceSeq int64) ([]*event.Event, error) {
	path := filepath.Join(s.dir, paths.SessionFilename(sessionID))
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var out []*event.Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.Seq > sinceSeq {
			cp := ev
			out = append(out, &cp)
		}
	}
	return out, sc.Err()
}

// SessionInfo summarizes one stored session for listing in the web UI.
type SessionInfo struct {
	// ID is the session's true SessionID (from the event payload), falling back
	// to the sanitized filename stem for an empty file.
	ID         string    `json:"id"`
	EventCount int64     `json:"event_count"`
	LastSeq    int64     `json:"last_seq"`
	ModTime    time.Time `json:"mod_time"`
	// StartedAt is the CapturedAt of the first valid event in the session.
	// Zero when the session file is empty or contains no parseable events.
	StartedAt time.Time `json:"started_at"`
	// LastActivity is the CapturedAt of the last valid event in the session —
	// the sort key for "most recent activity". Zero when the session file is
	// empty or contains no parseable events, in which case ModTime is used as a
	// fallback for ordering.
	LastActivity time.Time `json:"last_activity"`

	// CWD is the working directory captured from the session's events
	// (Phase 8d). project = filepath.Base(CWD). Empty when unknown.
	CWD string `json:"cwd,omitempty"`
	// Title is the human-facing session label (Phase 8d). It is resolved via a
	// fallback chain (transcript ai-title → last prompt → first UserPromptSubmit
	// prompt → "project · shortid") and is never blank when the session has any
	// events. Empty only for an empty session file.
	Title string `json:"title,omitempty"`
}

// Sessions lists every recorded session by scanning the store directory.
// Counts and last seq are derived by reading each file; at personal-tool
// scale this is fast enough. A dedicated index is a documented deferral if
// listing ever slows. CWD and Title are populated via a single-pass scan plus
// a cached transcript read.
func (s *Store) Sessions() ([]SessionInfo, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var infos []SessionInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		sr, err := scanSession(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		id := sr.id
		if id == "" {
			id = strings.TrimSuffix(e.Name(), ".jsonl")
		}
		info := SessionInfo{
			ID:           id,
			EventCount:   sr.count,
			LastSeq:      sr.last,
			StartedAt:    sr.startedAt,
			LastActivity: sr.lastActivity,
			CWD:          sr.cwd,
		}
		if fi, err := e.Info(); err == nil {
			info.ModTime = fi.ModTime()
		}
		info.Title = s.resolveTitle(id, sr)
		infos = append(infos, info)
	}

	// Order by most recent activity, newest first. "Activity" is the last
	// event's CapturedAt, falling back to the file's ModTime when no event
	// carried a timestamp. Ties (including both-zero) break by ID ascending so
	// the order is fully deterministic and stable across reloads.
	sort.Slice(infos, func(i, j int) bool {
		ai, aj := effectiveActivity(infos[i]), effectiveActivity(infos[j])
		if !ai.Equal(aj) {
			return ai.After(aj)
		}
		return infos[i].ID < infos[j].ID
	})
	return infos, nil
}

// effectiveActivity returns the timestamp used to order a session: the last
// event's CapturedAt, or the file ModTime when no event carried a timestamp.
func effectiveActivity(info SessionInfo) time.Time {
	if !info.LastActivity.IsZero() {
		return info.LastActivity
	}
	return info.ModTime
}

// resolveTitle applies the fallback chain to produce a non-blank title for a
// session with at least one valid event. Returns "" only when sr has no valid
// events (count == 0).
//
// Chain (first non-empty wins):
//  1. transcript aiTitle (last ai-title record)
//  2. transcript lastPrompt (last last-prompt record)
//  3. first UserPromptSubmit prompt captured during the session scan
//  4. "project · shortid" where project = filepath.Base(cwd) or "unknown"
func (s *Store) resolveTitle(id string, sr scanResult) string {
	if sr.count == 0 {
		return ""
	}

	// Attempt transcript-derived fields if a path was captured.
	if sr.transcriptPath != "" {
		tr := s.cachedTranscript(sr.transcriptPath)
		if tr.aiTitle != "" {
			return tr.aiTitle
		}
		if tr.lastPrompt != "" {
			return tr.lastPrompt
		}
	}

	// First UserPromptSubmit prompt from session events.
	if sr.firstUserPrompt != "" {
		return sr.firstUserPrompt
	}

	// Final rung: "project · shortid".
	project := "unknown"
	if sr.cwd != "" {
		project = filepath.Base(sr.cwd)
	}
	shortID := id
	if len(shortID) > 7 {
		shortID = shortID[:7]
	}
	return project + " · " + shortID
}

// cachedTranscript returns a transcriptResult for path, using the mtime cache
// to avoid re-reading unchanged files. Thread-safe.
func (s *Store) cachedTranscript(path string) transcriptResult {
	fi, err := os.Stat(path)
	if err != nil {
		// File missing or unreadable — return empty without caching.
		return transcriptResult{}
	}
	key := transcriptCacheKey{path: path, mtime: fi.ModTime()}

	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()

	if cached, ok := s.transcriptCache[key]; ok {
		return cached
	}
	result := readTranscript(path)
	s.transcriptCache[key] = result
	return result
}

// scanResult holds all fields extracted from a single pass over a session file.
type scanResult struct {
	id              string    // session ID from the first valid event
	count           int64     // number of valid events
	last            int64     // max Seq observed
	startedAt       time.Time // CapturedAt of the first valid event; zero when absent
	lastActivity    time.Time // CapturedAt of the last valid event; zero when absent
	cwd             string    // first non-empty cwd (event.CWD preferred, Raw.cwd fallback)
	transcriptPath  string    // last non-empty transcript_path from Raw
	firstUserPrompt string    // prompt from the first UserPromptSubmit event
}

// scanSession performs a single pass over a session JSONL file, extracting the
// fields needed by Sessions() and fileFor(). Any malformed line is skipped; a
// missing file is not an error for fileFor's prime path (it returns err only
// when os.Open fails for a reason other than NotExist would need special
// handling — callers should be prepared for a zero scanResult).
func scanSession(path string) (scanResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return scanResult{}, err
	}
	defer f.Close()

	var sr scanResult
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if sr.id == "" {
			sr.id = ev.SessionID
		}
		if sr.count == 0 && !ev.CapturedAt.IsZero() {
			sr.startedAt = ev.CapturedAt
		}
		if !ev.CapturedAt.IsZero() {
			sr.lastActivity = ev.CapturedAt
		}
		sr.count++
		if ev.Seq > sr.last {
			sr.last = ev.Seq
		}

		// CWD: prefer promoted field, fall back to Raw.
		if sr.cwd == "" {
			if ev.CWD != "" {
				sr.cwd = ev.CWD
			} else if cwd := rawStringField(ev.Raw, "cwd"); cwd != "" {
				sr.cwd = cwd
			}
		}

		// transcript_path: last non-empty wins (defensive: it's stable across events).
		if tp := rawStringField(ev.Raw, "transcript_path"); tp != "" {
			sr.transcriptPath = tp
		}

		// First UserPromptSubmit prompt.
		if sr.firstUserPrompt == "" && ev.HookEvent == "UserPromptSubmit" {
			if p := rawStringField(ev.Raw, "prompt"); p != "" {
				sr.firstUserPrompt = p
			}
		}
	}
	return sr, sc.Err()
}

// rawStringField extracts a top-level string field from a JSON object without
// fully unmarshaling it. Returns "" for any error or absent/non-string field.
func rawStringField(raw json.RawMessage, field string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return ""
	}
	v, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return s
}

// Delete removes a single session's JSONL file from disk and drops any
// in-process writer state for it. Because the daemon holds a live append
// handle per session, an out-of-process unlink would leak writes to the freed
// inode; doing the delete in-process lets us Close() and forget the handle
// under s.mu before unlinking, so a delete is consistent even while capture is
// live. Re-capturing the same id afterward opens a fresh file with a reset
// seq.
//
// The id must satisfy paths.SafeSessionID — anything else is rejected with
// ErrInvalidSessionID without touching disk. Deleting a session whose file is
// already absent is not an error (idempotent).
func (s *Store) Delete(sessionID string) error {
	if !paths.SafeSessionID(sessionID) {
		return ErrInvalidSessionID
	}
	path := filepath.Join(s.dir, paths.SessionFilename(sessionID))

	// Capture the transcript path (if any) before we remove the file so we can
	// evict its cache entries afterward. Best-effort: a scan failure just means
	// no eviction.
	transcriptPath := ""
	if sr, err := scanSession(path); err == nil {
		transcriptPath = sr.transcriptPath
	}

	// Close the live handle, drop writer state, and unlink — all under s.mu so
	// a concurrent Append cannot recreate the file between the unlink and the
	// map cleanup.
	s.mu.Lock()
	if f, ok := s.files[sessionID]; ok {
		_ = f.Close()
		delete(s.files, sessionID)
	}
	delete(s.seq, sessionID)
	err := os.Remove(path)
	s.mu.Unlock()
	if err != nil && !os.IsNotExist(err) {
		return err
	}

	if transcriptPath != "" {
		s.transcriptMu.Lock()
		for k := range s.transcriptCache {
			if k.path == transcriptPath {
				delete(s.transcriptCache, k)
			}
		}
		s.transcriptMu.Unlock()
	}
	return nil
}

// Close flushes and closes all open session handles.
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var firstErr error
	for id, f := range s.files {
		if err := f.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
		delete(s.files, id)
	}
	return firstErr
}
