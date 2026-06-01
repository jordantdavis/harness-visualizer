package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAppendAssignsMonotonicSeqPerSession(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 3; i++ {
		ev := &event.Event{SessionID: "s1", HookEvent: "PreToolUse"}
		if err := s.Append(ev); err != nil {
			t.Fatalf("Append: %v", err)
		}
		if ev.Seq != int64(i+1) {
			t.Errorf("event %d Seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
	ev := &event.Event{SessionID: "s2", HookEvent: "Stop"}
	if err := s.Append(ev); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if ev.Seq != 1 {
		t.Errorf("new session Seq = %d, want 1", ev.Seq)
	}
}

func TestAppendWritesOneJSONLineToCorrectFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if err := s.Append(&event.Event{SessionID: "abc", HookEvent: "Stop"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "abc.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if n := countLines(data); n != 1 {
		t.Errorf("line count = %d, want 1", n)
	}
	var ev event.Event
	if err := json.Unmarshal(data[:len(data)-1], &ev); err != nil {
		t.Fatalf("stored line is not valid JSON: %v", err)
	}
	if ev.HookEvent != "Stop" || ev.Seq != 1 {
		t.Errorf("stored event = %+v, want HookEvent=Stop Seq=1", ev)
	}
}

func TestAppendIsConcurrencySafe(t *testing.T) {
	s := newTestStore(t)
	var wg sync.WaitGroup
	const n = 50
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.Append(&event.Event{SessionID: "race", HookEvent: "PreToolUse"})
		}()
	}
	wg.Wait()
	evs, err := s.Read("race", 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(evs) != n {
		t.Fatalf("got %d events, want %d", len(evs), n)
	}
	seen := make(map[int64]bool, n)
	for _, ev := range evs {
		if ev.Seq < 1 || ev.Seq > n {
			t.Errorf("Seq %d out of range", ev.Seq)
		}
		if seen[ev.Seq] {
			t.Errorf("duplicate Seq %d", ev.Seq)
		}
		seen[ev.Seq] = true
	}
}

func countLines(b []byte) int {
	n := 0
	for _, c := range b {
		if c == '\n' {
			n++
		}
	}
	return n
}

func TestSeqResumesFromExistingFile(t *testing.T) {
	dir := t.TempDir()
	s1, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_ = s1.Append(&event.Event{SessionID: "resume", HookEvent: "A"})
	_ = s1.Append(&event.Event{SessionID: "resume", HookEvent: "B"})
	if err := s1.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()
	ev := &event.Event{SessionID: "resume", HookEvent: "C"}
	if err := s2.Append(ev); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if ev.Seq != 3 {
		t.Errorf("resumed Seq = %d, want 3", ev.Seq)
	}
}

func TestReadReturnsAllEventsInOrder(t *testing.T) {
	s := newTestStore(t)
	for _, h := range []string{"A", "B", "C"} {
		if err := s.Append(&event.Event{SessionID: "r", HookEvent: h}); err != nil {
			t.Fatalf("Append: %v", err)
		}
	}
	evs, err := s.Read("r", 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(evs) != 3 {
		t.Fatalf("len = %d, want 3", len(evs))
	}
	for i, want := range []string{"A", "B", "C"} {
		if evs[i].HookEvent != want {
			t.Errorf("evs[%d].HookEvent = %q, want %q", i, evs[i].HookEvent, want)
		}
		if evs[i].Seq != int64(i+1) {
			t.Errorf("evs[%d].Seq = %d, want %d", i, evs[i].Seq, i+1)
		}
	}
}

func TestReadSinceCursorFiltersBySeq(t *testing.T) {
	s := newTestStore(t)
	for i := 0; i < 5; i++ {
		_ = s.Append(&event.Event{SessionID: "r", HookEvent: "X"})
	}
	evs, err := s.Read("r", 3)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("len = %d, want 2", len(evs))
	}
	if evs[0].Seq != 4 || evs[1].Seq != 5 {
		t.Errorf("got Seqs %d,%d want 4,5", evs[0].Seq, evs[1].Seq)
	}
}

func TestReadMissingSessionReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	evs, err := s.Read("nope", 0)
	if err != nil {
		t.Fatalf("Read of missing session should not error, got %v", err)
	}
	if len(evs) != 0 {
		t.Errorf("len = %d, want 0", len(evs))
	}
}

func TestReadSkipsMalformedLines(t *testing.T) {
	dir := t.TempDir()
	s, _ := New(dir)
	defer s.Close()
	_ = s.Append(&event.Event{SessionID: "m", HookEvent: "good1"})
	f, _ := os.OpenFile(filepath.Join(dir, "m.jsonl"), os.O_WRONLY|os.O_APPEND, 0o644)
	_, _ = f.WriteString("this is not json\n")
	_ = f.Close()
	_ = s.Append(&event.Event{SessionID: "m", HookEvent: "good2"})
	evs, err := s.Read("m", 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(evs) != 2 {
		t.Fatalf("len = %d, want 2 (malformed line skipped)", len(evs))
	}
}

func TestSessionsListsAllWithCountsAndLastSeq(t *testing.T) {
	s := newTestStore(t)
	_ = s.Append(&event.Event{SessionID: "alpha", HookEvent: "A"})
	_ = s.Append(&event.Event{SessionID: "alpha", HookEvent: "B"})
	_ = s.Append(&event.Event{SessionID: "beta", HookEvent: "C"})
	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	byID := map[string]SessionInfo{}
	for _, in := range infos {
		byID[in.ID] = in
	}
	if len(byID) != 2 {
		t.Fatalf("got %d sessions, want 2", len(byID))
	}
	if byID["alpha"].EventCount != 2 || byID["alpha"].LastSeq != 2 {
		t.Errorf("alpha = %+v, want EventCount=2 LastSeq=2", byID["alpha"])
	}
	if byID["beta"].EventCount != 1 || byID["beta"].LastSeq != 1 {
		t.Errorf("beta = %+v, want EventCount=1 LastSeq=1", byID["beta"])
	}
}

func TestSessionsEmptyDirReturnsEmpty(t *testing.T) {
	s := newTestStore(t)
	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 0 {
		t.Errorf("got %d, want 0", len(infos))
	}
}

func TestSessionsReportsTrueSessionID(t *testing.T) {
	s := newTestStore(t)
	// An id with a char that sanitize() rewrites in the filename.
	_ = s.Append(&event.Event{SessionID: "a/b", HookEvent: "X"})
	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	if infos[0].ID != "a/b" {
		t.Errorf("ID = %q, want %q (true session id, not sanitized stem)", infos[0].ID, "a/b")
	}
}
