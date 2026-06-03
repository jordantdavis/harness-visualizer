package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// TestDeleteRemovesFileAndDropsHandle covers the happy path: the on-disk file
// is gone and the in-process writer state for the session is cleared.
func TestDeleteRemovesFileAndDropsHandle(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	if err := s.Append(&event.Event{SessionID: "s1", HookEvent: "Stop"}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	path := filepath.Join(dir, "s1.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("precondition: session file missing: %v", err)
	}

	if err := s.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("file still present after Delete (err=%v)", err)
	}

	s.mu.Lock()
	_, hasFile := s.files["s1"]
	_, hasSeq := s.seq["s1"]
	s.mu.Unlock()
	if hasFile {
		t.Errorf("writer handle not dropped from s.files")
	}
	if hasSeq {
		t.Errorf("seq counter not dropped from s.seq")
	}
}

// TestDeleteIsIdempotentOnMissingFile: deleting a never-written (but safe) id
// succeeds.
func TestDeleteIsIdempotentOnMissingFile(t *testing.T) {
	s := newTestStore(t)
	if err := s.Delete("never-existed"); err != nil {
		t.Errorf("Delete of missing session = %v, want nil", err)
	}
}

// TestDeleteRejectsUnsafeID: a traversal/non-safe id is rejected without
// touching disk.
func TestDeleteRejectsUnsafeID(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// A decoy that the *sanitized* form of "../evil" would map to, to prove we
	// never reach os.Remove for a rejected id.
	decoy := filepath.Join(dir, "___evil.jsonl")
	if err := os.WriteFile(decoy, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"", "../evil", "a/b", "..", "sess.jsonl"} {
		if err := s.Delete(id); !errors.Is(err, ErrInvalidSessionID) {
			t.Errorf("Delete(%q) = %v, want ErrInvalidSessionID", id, err)
		}
	}
	if _, err := os.Stat(decoy); err != nil {
		t.Errorf("decoy removed by a rejected delete: %v", err)
	}
}

// TestDeleteLeavesOtherSessionsWritable: deleting one session must not break
// the writer map for others, and appends to a different session keep flowing.
func TestDeleteLeavesOtherSessionsWritable(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	_ = s.Append(&event.Event{SessionID: "s1", HookEvent: "A"})
	_ = s.Append(&event.Event{SessionID: "s2", HookEvent: "A"})

	if err := s.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if err := s.Append(&event.Event{SessionID: "s2", HookEvent: "B"}); err != nil {
		t.Fatalf("Append after delete: %v", err)
	}
	got, err := s.Read("s2", 0)
	if err != nil {
		t.Fatalf("Read s2: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("s2 has %d events after deleting s1, want 2", len(got))
	}
}

// TestDeleteThenRecaptureStartsCleanFile: re-capturing a deleted id opens a
// fresh file with seq reset to 1.
func TestDeleteThenRecaptureStartsCleanFile(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	_ = s.Append(&event.Event{SessionID: "s1", HookEvent: "A"})
	_ = s.Append(&event.Event{SessionID: "s1", HookEvent: "B"})
	if err := s.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	ev := &event.Event{SessionID: "s1", HookEvent: "C"}
	if err := s.Append(ev); err != nil {
		t.Fatalf("Append after delete: %v", err)
	}
	if ev.Seq != 1 {
		t.Errorf("re-captured seq = %d, want 1 (clean file)", ev.Seq)
	}
	got, err := s.Read("s1", 0)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("re-captured session has %d events, want 1", len(got))
	}
}

// TestDeleteInvalidatesTranscriptCache: a cached transcript parse for the
// deleted session is evicted.
func TestDeleteInvalidatesTranscriptCache(t *testing.T) {
	dir := t.TempDir()
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// A real (empty) transcript file so cachedTranscript stats + caches it.
	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(transcript, []byte(""), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"transcript_path":"` + transcript + `"}`)
	if err := s.Append(&event.Event{SessionID: "s1", HookEvent: "SessionStart", Raw: raw}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Sessions() resolves the title, which populates the transcript cache.
	if _, err := s.Sessions(); err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	s.transcriptMu.Lock()
	primed := len(s.transcriptCache)
	s.transcriptMu.Unlock()
	if primed == 0 {
		t.Fatalf("precondition: transcript cache not primed")
	}

	if err := s.Delete("s1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	s.transcriptMu.Lock()
	defer s.transcriptMu.Unlock()
	for k := range s.transcriptCache {
		if k.path == transcript {
			t.Errorf("transcript cache entry for %q not evicted", transcript)
		}
	}
}
