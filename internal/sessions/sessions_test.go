package sessions

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sessionsDirFor returns a sessionsDir resolver that uses the given temp dir.
func sessionsDirFor(dir string) func() (string, error) {
	return func() (string, error) { return dir, nil }
}

// sessionsDirErr returns a resolver that always errors.
func sessionsDirErr(msg string) func() (string, error) {
	return func() (string, error) { return "", errors.New(msg) }
}

// populate creates named files in dir with the given content.
func populate(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("populate mkdir: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("populate write %s: %v", name, err)
		}
	}
}

// TestClear_EmptyDir exits 0 and reports zero files deleted.
func TestClear_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	out := &bytes.Buffer{}
	code := run([]string{"--yes"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	got := out.String()
	if !strings.Contains(got, "deleted 0 session files") {
		t.Fatalf("output=%q, want 'deleted 0 session files'", got)
	}
}

// TestClear_YesFlag deletes all *.jsonl, leaves non-jsonl, prints summary.
func TestClear_YesFlag(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{
		"a.jsonl":   `{"id":"a"}`,
		"b.jsonl":   `{"id":"b"}`,
		"keep.txt":  "keep me",
		"notes.log": "also keep",
	})

	out := &bytes.Buffer{}
	code := run([]string{"--yes"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}

	// JSONL files must be gone.
	for _, f := range []string{"a.jsonl", "b.jsonl"} {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s still exists", f)
		}
	}
	// Non-JSONL files must remain.
	for _, f := range []string{"keep.txt", "notes.log"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s should still exist: %v", f, err)
		}
	}

	got := out.String()
	if !strings.Contains(got, "deleted 2 session files") {
		t.Fatalf("output=%q, want 'deleted 2 session files'", got)
	}
}

// TestClear_ShortYesFlag tests that -y is equivalent to --yes.
func TestClear_ShortYesFlag(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"a.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{"-y"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0 for -y", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.jsonl")); !os.IsNotExist(err) {
		t.Error("a.jsonl should be deleted with -y")
	}
}

// TestClear_DryRun lists files, deletes nothing, exits 0.
func TestClear_DryRun(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{
		"sess1.jsonl": `{}`,
		"sess2.jsonl": `{}`,
		"keep.txt":    "keep",
	})

	out := &bytes.Buffer{}
	code := run([]string{"--dry-run"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}

	// Both JSONL filenames must appear in output.
	got := out.String()
	if !strings.Contains(got, "sess1.jsonl") {
		t.Errorf("output=%q, want sess1.jsonl listed", got)
	}
	if !strings.Contains(got, "sess2.jsonl") {
		t.Errorf("output=%q, want sess2.jsonl listed", got)
	}
	// Non-JSONL must not appear.
	if strings.Contains(got, "keep.txt") {
		t.Errorf("output=%q, keep.txt should not appear in dry-run", got)
	}
	// Summary must mention 'would delete'.
	if !strings.Contains(got, "would delete 2 session files") {
		t.Errorf("output=%q, want 'would delete 2 session files'", got)
	}

	// Files must still exist.
	for _, f := range []string{"sess1.jsonl", "sess2.jsonl", "keep.txt"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s should still exist after dry-run: %v", f, err)
		}
	}
}

// TestClear_InteractiveAccept_y confirms 'y' deletes and exits 0.
func TestClear_InteractiveAccept_y(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"sess.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{}, sessionsDirFor(dir), strings.NewReader("y\n"), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "sess.jsonl")); !os.IsNotExist(err) {
		t.Error("sess.jsonl should be deleted after 'y'")
	}
}

// TestClear_InteractiveAccept_yes confirms 'yes' (case-insensitive) deletes and exits 0.
func TestClear_InteractiveAccept_yes(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"sess.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{}, sessionsDirFor(dir), strings.NewReader("YES\n"), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}
}

// TestClear_InteractiveReject_n confirms 'n' aborts with exit 2, no deletions.
func TestClear_InteractiveReject_n(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"sess.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{}, sessionsDirFor(dir), strings.NewReader("n\n"), out)
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "sess.jsonl")); err != nil {
		t.Error("sess.jsonl should NOT be deleted after 'n'")
	}
}

// TestClear_InteractiveReject_empty confirms empty input aborts with exit 2.
func TestClear_InteractiveReject_empty(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"sess.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{}, sessionsDirFor(dir), strings.NewReader("\n"), out)
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
}

// TestClear_InteractiveReject_garbage confirms unrecognised input aborts.
func TestClear_InteractiveReject_garbage(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"sess.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{}, sessionsDirFor(dir), strings.NewReader("maybe\n"), out)
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "sess.jsonl")); err != nil {
		t.Error("sess.jsonl should NOT be deleted after garbage input")
	}
}

// TestClear_NonJSONLFilesUntouched ensures subdirs and non-jsonl files survive.
func TestClear_NonJSONLFilesUntouched(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{
		"a.jsonl":        `{}`,
		"notes.txt":      "notes",
		"subdir/b.jsonl": `{}`, // inside a subdir — should NOT be deleted
	})

	out := &bytes.Buffer{}
	code := run([]string{"--yes"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 0 {
		t.Fatalf("code=%d, want 0", code)
	}

	// Top-level JSONL is deleted.
	if _, err := os.Stat(filepath.Join(dir, "a.jsonl")); !os.IsNotExist(err) {
		t.Error("a.jsonl should be deleted")
	}
	// notes.txt survives.
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Error("notes.txt should still exist")
	}
	// Subdir JSONL survives (we only delete direct children).
	if _, err := os.Stat(filepath.Join(dir, "subdir", "b.jsonl")); err != nil {
		t.Error("subdir/b.jsonl should still exist — only top-level files are deleted")
	}
	// Summary should mention 1 deleted (only a.jsonl).
	if !strings.Contains(out.String(), "deleted 1 session files") {
		t.Errorf("output=%q, want 'deleted 1 session files'", out.String())
	}
}

// TestClear_BadFlag exits 2 for unrecognised flags, deletes nothing.
func TestClear_BadFlag(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{"a.jsonl": `{}`})
	out := &bytes.Buffer{}
	code := run([]string{"--bogus"}, sessionsDirFor(dir), strings.NewReader(""), out)
	if code != 2 {
		t.Fatalf("code=%d, want 2", code)
	}
	if _, err := os.Stat(filepath.Join(dir, "a.jsonl")); err != nil {
		t.Error("a.jsonl should NOT be deleted after bad flag")
	}
}

// TestClear_SessionsDirError exits 1 when the sessions dir can't be resolved.
func TestClear_SessionsDirError(t *testing.T) {
	out := &bytes.Buffer{}
	code := run([]string{"--yes"}, sessionsDirErr("no such dir"), strings.NewReader(""), out)
	if code != 1 {
		t.Fatalf("code=%d, want 1", code)
	}
}

// TestClear_PartialFailure simulates one unremovable file: exits 1, removes the rest.
func TestClear_PartialFailure(t *testing.T) {
	dir := t.TempDir()
	populate(t, dir, map[string]string{
		"a.jsonl": `{}`,
		"b.jsonl": `{}`,
		"c.jsonl": `{}`,
	})

	// Inject a remover that always fails on b.jsonl.
	failOn := filepath.Join(dir, "b.jsonl")
	remover := func(path string) error {
		if path == failOn {
			return errors.New("permission denied")
		}
		return os.Remove(path)
	}

	out := &bytes.Buffer{}
	code := runWithRemover([]string{"--yes"}, sessionsDirFor(dir), strings.NewReader(""), out, remover)
	if code != 1 {
		t.Fatalf("code=%d, want 1 (partial failure)", code)
	}

	// a.jsonl and c.jsonl are deleted; b.jsonl remains.
	if _, err := os.Stat(filepath.Join(dir, "a.jsonl")); !os.IsNotExist(err) {
		t.Error("a.jsonl should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "c.jsonl")); !os.IsNotExist(err) {
		t.Error("c.jsonl should be deleted")
	}
	if _, err := os.Stat(filepath.Join(dir, "b.jsonl")); err != nil {
		t.Error("b.jsonl should still exist (removal failed)")
	}
}

// TestFormatSize verifies size formatting thresholds.
func TestFormatSize(t *testing.T) {
	cases := []struct {
		bytes int64
		want  string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1023, "1023 B"},
		{1024 * 1024, "1.0 MB"},
		{12897484, "12.3 MB"}, // 12.3 * 1024 * 1024 rounded
		{1024, "1.0 KB"},
		{512 * 1024, "512.0 KB"},
	}
	for _, tc := range cases {
		got := formatSize(tc.bytes)
		if got != tc.want {
			t.Errorf("formatSize(%d)=%q, want %q", tc.bytes, got, tc.want)
		}
	}
}
