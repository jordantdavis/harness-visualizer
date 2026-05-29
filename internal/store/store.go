// Package store persists captured events as JSONL, one file per session. The
// daemon is the only writer in production; Store is therefore optimized for a
// single in-process owner but is fully thread-safe. Append assigns a
// monotonic per-session sequence number. The directory is injected so the
// store is testable against a temp dir.
package store

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/paths"
)

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
}

// New creates a Store rooted at dir, creating dir if absent.
func New(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &Store{
		dir:   dir,
		seq:   make(map[string]int64),
		files: make(map[string]*os.File),
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
		if _, _, last, err := scanCountAndLastSeq(path); err == nil {
			s.seq[sessionID] = last
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

// SessionInfo summarizes one stored session for listing in the TUI.
type SessionInfo struct {
	// ID is the session's true SessionID (from the event payload), falling back
	// to the sanitized filename stem for an empty file.
	ID         string    `json:"id"`
	EventCount int64     `json:"event_count"`
	LastSeq    int64     `json:"last_seq"`
	ModTime    time.Time `json:"mod_time"`
}

// Sessions lists every recorded session by scanning the store directory.
// Counts and last seq are derived by reading each file; at personal-tool
// scale this is fast enough. A dedicated index is a documented deferral if
// listing ever slows.
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
		id, count, last, err := scanCountAndLastSeq(filepath.Join(s.dir, e.Name()))
		if err != nil {
			continue
		}
		if id == "" {
			id = strings.TrimSuffix(e.Name(), ".jsonl")
		}
		info := SessionInfo{ID: id, EventCount: count, LastSeq: last}
		if fi, err := e.Info(); err == nil {
			info.ModTime = fi.ModTime()
		}
		infos = append(infos, info)
	}
	return infos, nil
}

// scanCountAndLastSeq returns the session id from the first valid event, the
// valid-event count, and the max Seq in path. id is empty when the file has
// no valid events.
func scanCountAndLastSeq(path string) (id string, count int64, last int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, 0, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		var ev event.Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			continue
		}
		if id == "" {
			id = ev.SessionID
		}
		count++
		if ev.Seq > last {
			last = ev.Seq
		}
	}
	return id, count, last, sc.Err()
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
