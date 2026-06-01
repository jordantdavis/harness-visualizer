package paths

import (
	"path/filepath"
	"testing"
)

func TestDataDirHonorsOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HV_DATA_DIR", tmp)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir error: %v", err)
	}
	if dir != tmp {
		t.Errorf("DataDir = %q, want %q", dir, tmp)
	}
}

func TestDataDirFallsBackToXDG(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HV_DATA_DIR", "")
	t.Setenv("XDG_DATA_HOME", tmp)

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("DataDir error: %v", err)
	}
	want := filepath.Join(tmp, "hv")
	if dir != want {
		t.Errorf("DataDir = %q, want %q", dir, want)
	}
}

func TestSessionsDirIsUnderDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("HV_DATA_DIR", tmp)

	dir, err := SessionsDir()
	if err != nil {
		t.Fatalf("SessionsDir error: %v", err)
	}
	want := filepath.Join(tmp, "sessions")
	if dir != want {
		t.Errorf("SessionsDir = %q, want %q", dir, want)
	}
}

func TestRuntimeDirHonorsXDGRuntimeDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)
	t.Setenv("HV_DATA_DIR", "")

	dir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir error: %v", err)
	}
	want := filepath.Join(tmp, "hv")
	if dir != want {
		t.Errorf("RuntimeDir = %q, want %q", dir, want)
	}
}

func TestRuntimeDirFallsBackToDataDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("HV_DATA_DIR", tmp)

	dir, err := RuntimeDir()
	if err != nil {
		t.Fatalf("RuntimeDir error: %v", err)
	}
	if dir != tmp {
		t.Errorf("RuntimeDir = %q, want %q", dir, tmp)
	}
}

func TestSessionFilenameSanitizes(t *testing.T) {
	cases := map[string]string{
		"abc-123":     "abc-123.jsonl",
		"a/b/../c":    "a_b____c.jsonl", // 8 chars: a / b / . . / c → 4 replacements between b and c
		"weird id!@#": "weird_id___.jsonl",
		"":            "_.jsonl",
	}
	for in, want := range cases {
		if got := SessionFilename(in); got != want {
			t.Errorf("SessionFilename(%q) = %q, want %q", in, got, want)
		}
	}
}
