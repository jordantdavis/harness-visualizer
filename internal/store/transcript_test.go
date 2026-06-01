package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// writeTranscriptLines writes JSONL lines to a temp file and returns its path.
func writeTranscriptLines(t *testing.T, lines []string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "transcript-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer f.Close()
	for _, l := range lines {
		if _, err := fmt.Fprintln(f, l); err != nil {
			t.Fatalf("write: %v", err)
		}
	}
	return f.Name()
}

// ---- readTranscript unit tests ----

func TestReadTranscriptLastAiTitleWins(t *testing.T) {
	path := writeTranscriptLines(t, []string{
		`{"type":"ai-title","aiTitle":"first title"}`,
		`{"type":"ai-title","aiTitle":"second title"}`,
		`{"type":"ai-title","aiTitle":"last title"}`,
	})
	got := readTranscript(path)
	if got.aiTitle != "last title" {
		t.Errorf("aiTitle = %q, want %q", got.aiTitle, "last title")
	}
}

func TestReadTranscriptLastPromptExtracted(t *testing.T) {
	path := writeTranscriptLines(t, []string{
		`{"type":"last-prompt","lastPrompt":"first prompt"}`,
		`{"type":"last-prompt","lastPrompt":"second prompt"}`,
	})
	got := readTranscript(path)
	if got.lastPrompt != "second prompt" {
		t.Errorf("lastPrompt = %q, want %q", got.lastPrompt, "second prompt")
	}
}

func TestReadTranscriptMissingFileReturnsEmpty(t *testing.T) {
	got := readTranscript("/nonexistent/path/no-such.jsonl")
	if got.aiTitle != "" || got.lastPrompt != "" {
		t.Errorf("expected empty result, got %+v", got)
	}
}

func TestReadTranscriptGarbageLinesSkipped(t *testing.T) {
	path := writeTranscriptLines(t, []string{
		`not json at all`,
		`{"type":"ai-title","aiTitle":"good"}`,
		`{broken`,
		`null`,
	})
	got := readTranscript(path)
	if got.aiTitle != "good" {
		t.Errorf("aiTitle = %q, want %q", got.aiTitle, "good")
	}
}

func TestReadTranscriptMalformedJSONNoPanic(t *testing.T) {
	path := writeTranscriptLines(t, []string{
		`{{{{{{{`,
		`[1,2,3]`,
		`"just a string"`,
	})
	// Must not panic; result should be empty.
	got := readTranscript(path)
	if got.aiTitle != "" || got.lastPrompt != "" {
		t.Errorf("expected empty result for all-garbage, got %+v", got)
	}
}

func TestReadTranscriptEmptyFileReturnsEmpty(t *testing.T) {
	path := writeTranscriptLines(t, nil)
	got := readTranscript(path)
	if got.aiTitle != "" || got.lastPrompt != "" {
		t.Errorf("expected empty result for empty file, got %+v", got)
	}
}

func TestReadTranscriptBothFieldsPresent(t *testing.T) {
	path := writeTranscriptLines(t, []string{
		`{"type":"last-prompt","lastPrompt":"the prompt"}`,
		`{"type":"ai-title","aiTitle":"the title"}`,
	})
	got := readTranscript(path)
	if got.aiTitle != "the title" {
		t.Errorf("aiTitle = %q, want %q", got.aiTitle, "the title")
	}
	if got.lastPrompt != "the prompt" {
		t.Errorf("lastPrompt = %q, want %q", got.lastPrompt, "the prompt")
	}
}

// ---- Fallback chain tests via Sessions() ----

// writeSessionWithRaw writes one event to a session file with a full Raw payload.
func writeSessionEvent(t *testing.T, dir, sessionID string, raw map[string]any) {
	t.Helper()
	rawBytes, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal raw: %v", err)
	}
	ev := &event.Event{
		SessionID: sessionID,
		HookEvent: "PreToolUse",
		CWD:       "/test/project",
		Raw:       json.RawMessage(rawBytes),
	}
	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()
	if err := s.Append(ev); err != nil {
		t.Fatalf("Append: %v", err)
	}
}

func TestSessionsTitleFromAiTitle(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := writeTranscriptLines(t, []string{
		`{"type":"ai-title","aiTitle":"AI Generated Title"}`,
	})
	writeSessionEvent(t, dir, "sess-aititle", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-aititle",
		"cwd":             "/test/myproject",
		"transcript_path": transcriptPath,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	if infos[0].Title != "AI Generated Title" {
		t.Errorf("Title = %q, want %q", infos[0].Title, "AI Generated Title")
	}
}

func TestSessionsTitleFallsToLastPrompt(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := writeTranscriptLines(t, []string{
		`{"type":"last-prompt","lastPrompt":"user typed this"}`,
	})
	writeSessionEvent(t, dir, "sess-lastprompt", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-lastprompt",
		"cwd":             "/test/myproject",
		"transcript_path": transcriptPath,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	if infos[0].Title != "user typed this" {
		t.Errorf("Title = %q, want %q", infos[0].Title, "user typed this")
	}
}

func TestSessionsTitleFallsToFirstUserPromptSubmit(t *testing.T) {
	dir := t.TempDir()
	// transcript has nothing useful
	transcriptPath := writeTranscriptLines(t, []string{
		`{"type":"some-other-type","value":"irrelevant"}`,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// First event: not a UserPromptSubmit
	rawFirst, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-ups",
		"cwd":             "/test/myproject",
		"transcript_path": transcriptPath,
	})
	if err := s.Append(&event.Event{
		SessionID: "sess-ups",
		HookEvent: "PreToolUse",
		CWD:       "/test/myproject",
		Raw:       json.RawMessage(rawFirst),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Second event: UserPromptSubmit with a prompt
	rawUPS, _ := json.Marshal(map[string]any{
		"hook_event_name": "UserPromptSubmit",
		"session_id":      "sess-ups",
		"cwd":             "/test/myproject",
		"transcript_path": transcriptPath,
		"prompt":          "fix the bug please",
	})
	if err := s.Append(&event.Event{
		SessionID: "sess-ups",
		HookEvent: "UserPromptSubmit",
		CWD:       "/test/myproject",
		Raw:       json.RawMessage(rawUPS),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()

	infos, err := s2.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	if infos[0].Title != "fix the bug please" {
		t.Errorf("Title = %q, want %q", infos[0].Title, "fix the bug please")
	}
}

func TestSessionsTitleFallsToProjectShortID(t *testing.T) {
	dir := t.TempDir()
	// No transcript, no UserPromptSubmit

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	rawEv, _ := json.Marshal(map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "abcdefghijk",
		"cwd":             "/home/user/my-cool-project",
	})
	if err := s.Append(&event.Event{
		SessionID: "abcdefghijk",
		HookEvent: "Stop",
		CWD:       "/home/user/my-cool-project",
		Raw:       json.RawMessage(rawEv),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()

	infos, err := s2.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	// project = "my-cool-project", shortid = "abcdefg"
	want := "my-cool-project · abcdefg"
	if infos[0].Title != want {
		t.Errorf("Title = %q, want %q", infos[0].Title, want)
	}
}

func TestSessionsTitleProjectShortIDShortSessionID(t *testing.T) {
	// Session IDs shorter than 7 chars must not panic.
	dir := t.TempDir()

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	rawEv, _ := json.Marshal(map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "abc",
		"cwd":             "/home/user/proj",
	})
	if err := s.Append(&event.Event{
		SessionID: "abc",
		HookEvent: "Stop",
		CWD:       "/home/user/proj",
		Raw:       json.RawMessage(rawEv),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()

	infos, err := s2.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	// shortid = "abc" (less than 7 chars)
	want := "proj · abc"
	if infos[0].Title != want {
		t.Errorf("Title = %q, want %q", infos[0].Title, want)
	}
}

func TestSessionsTitleUnknownCWD(t *testing.T) {
	// No CWD at all — project falls back to "unknown".
	dir := t.TempDir()

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// Raw has no cwd field; event.CWD also empty.
	rawEv, _ := json.Marshal(map[string]any{
		"hook_event_name": "Stop",
		"session_id":      "abcdefghijk",
	})
	if err := s.Append(&event.Event{
		SessionID: "abcdefghijk",
		HookEvent: "Stop",
		Raw:       json.RawMessage(rawEv),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()

	infos, err := s2.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	want := "unknown · abcdefg"
	if infos[0].Title != want {
		t.Errorf("Title = %q, want %q", infos[0].Title, want)
	}
}

func TestSessionsCWDPopulated(t *testing.T) {
	dir := t.TempDir()

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	rawEv, _ := json.Marshal(map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-cwd",
		"cwd":             "/home/user/realproject",
	})
	if err := s.Append(&event.Event{
		SessionID: "sess-cwd",
		HookEvent: "PreToolUse",
		CWD:       "/home/user/realproject",
		Raw:       json.RawMessage(rawEv),
	}); err != nil {
		t.Fatalf("Append: %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	s2, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s2.Close()

	infos, err := s2.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	if infos[0].CWD != "/home/user/realproject" {
		t.Errorf("CWD = %q, want %q", infos[0].CWD, "/home/user/realproject")
	}
}

// TestSessionsMtimeCache verifies the transcript is not re-parsed when the
// file mtime is unchanged. We make the transcript file unreadable after the
// first Sessions() call; if the cache works, the second call still returns
// the cached title without an error.
func TestSessionsMtimeCache(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := writeTranscriptLines(t, []string{
		`{"type":"ai-title","aiTitle":"Cached Title"}`,
	})
	writeSessionEvent(t, dir, "sess-cache", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-cache",
		"cwd":             "/test/cacheproject",
		"transcript_path": transcriptPath,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	// First call: populates cache.
	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions (1st): %v", err)
	}
	if len(infos) != 1 || infos[0].Title != "Cached Title" {
		t.Fatalf("1st call: unexpected infos %+v", infos)
	}

	// Make the transcript unreadable so a re-read would fail.
	if err := os.Chmod(transcriptPath, 0o000); err != nil {
		t.Fatalf("Chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(transcriptPath, 0o644) })

	// Second call: must still return "Cached Title" from cache.
	infos2, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions (2nd): %v", err)
	}
	if len(infos2) != 1 || infos2[0].Title != "Cached Title" {
		t.Errorf("2nd call: Title = %q, want %q (should be served from cache)", infos2[0].Title, "Cached Title")
	}
}

// TestSessionsMtimeCacheInvalidatedOnChange confirms that a new mtime (by
// touching the transcript) causes the cache to be re-read.
func TestSessionsMtimeCacheInvalidatedOnChange(t *testing.T) {
	dir := t.TempDir()
	transcriptFile := filepath.Join(t.TempDir(), "t.jsonl")
	if err := os.WriteFile(transcriptFile, []byte(`{"type":"ai-title","aiTitle":"Old Title"}`+"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	writeSessionEvent(t, dir, "sess-refresh", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-refresh",
		"cwd":             "/test/refreshproject",
		"transcript_path": transcriptFile,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions (1st): %v", err)
	}
	if len(infos) != 1 || infos[0].Title != "Old Title" {
		t.Fatalf("1st call: unexpected %+v", infos)
	}

	// Update the transcript with a new title (and force an mtime change).
	// Ensure a distinct mtime by sleeping 10ms.
	// Use os.WriteFile to atomically overwrite.
	// On fast filesystems mtime granularity can be 1s; we rely on chtimes to
	// be explicit about the change.
	fi, err := os.Stat(transcriptFile)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	newContent := []byte(`{"type":"ai-title","aiTitle":"New Title"}` + "\n")
	if err := os.WriteFile(transcriptFile, newContent, 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Advance mtime by 2 seconds to guarantee cache-key difference.
	newMtime := fi.ModTime().Add(2e9) // 2 seconds
	if err := os.Chtimes(transcriptFile, newMtime, newMtime); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	infos2, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions (2nd): %v", err)
	}
	if len(infos2) != 1 || infos2[0].Title != "New Title" {
		t.Errorf("2nd call: Title = %q, want %q", infos2[0].Title, "New Title")
	}
}

// TestSessionsMissingTranscriptFallsThrough verifies that a transcript_path
// pointing to a nonexistent file causes the fallback chain to proceed without
// an error being surfaced.
func TestSessionsMissingTranscriptFallsThrough(t *testing.T) {
	dir := t.TempDir()
	writeSessionEvent(t, dir, "sess-notranscript", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-notranscript",
		"cwd":             "/test/nofile",
		"transcript_path": "/nonexistent/no-such.jsonl",
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	infos, err := s.Sessions()
	if err != nil {
		t.Fatalf("Sessions: %v", err)
	}
	if len(infos) != 1 {
		t.Fatalf("got %d sessions, want 1", len(infos))
	}
	// Falls through to project·shortid fallback.
	if infos[0].Title == "" {
		t.Error("Title should not be empty; expected fallback to project·shortid")
	}
}

// TestSessionsConcurrentReadTranscriptCache checks the mtime cache is
// race-condition-free under concurrent Sessions() calls.
func TestSessionsConcurrentReadTranscriptCache(t *testing.T) {
	dir := t.TempDir()
	transcriptPath := writeTranscriptLines(t, []string{
		`{"type":"ai-title","aiTitle":"Concurrent Title"}`,
	})
	writeSessionEvent(t, dir, "sess-concurrent", map[string]any{
		"hook_event_name": "PreToolUse",
		"session_id":      "sess-concurrent",
		"cwd":             "/test/concurrentproject",
		"transcript_path": transcriptPath,
	})

	s, err := New(dir)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer s.Close()

	var wg sync.WaitGroup
	const n = 20
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			infos, err := s.Sessions()
			if err != nil {
				return
			}
			if len(infos) > 0 && infos[0].Title != "Concurrent Title" {
				t.Errorf("Title = %q, want %q", infos[0].Title, "Concurrent Title")
			}
		}()
	}
	wg.Wait()
}
