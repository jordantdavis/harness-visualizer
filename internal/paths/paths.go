// Package paths resolves the on-disk locations hv uses: data (session
// JSONL), runtime (port file, pidfile, daemon log), and per-session
// filenames. Resolution honors HV_DATA_DIR, then XDG_DATA_HOME, then
// ~/.local/share. Directories are created on demand.
package paths

import (
	"os"
	"path/filepath"
	"strings"
)

const appName = "hv"

// DataDir returns the base data directory, creating it if absent.
//
// Resolution order:
//  1. $HV_DATA_DIR (non-empty)
//  2. $XDG_DATA_HOME/hv
//  3. ~/.local/share/hv
func DataDir() (string, error) {
	dir := os.Getenv("HV_DATA_DIR")
	if dir == "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			dir = filepath.Join(xdg, appName)
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, ".local", "share", appName)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// SessionsDir returns the directory holding per-session JSONL files,
// creating it if absent. Always a subdirectory of DataDir.
func SessionsDir() (string, error) {
	base, err := DataDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(base, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// RuntimeDir returns the directory for transient runtime files (port file,
// pidfile). Prefers $XDG_RUNTIME_DIR/hv; falls back to DataDir.
func RuntimeDir() (string, error) {
	if xdg := os.Getenv("XDG_RUNTIME_DIR"); xdg != "" {
		dir := filepath.Join(xdg, appName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", err
		}
		return dir, nil
	}
	return DataDir()
}

// PortFile returns the path to the file holding the daemon's listen port.
func PortFile() (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.port"), nil
}

// PidFile returns the path to the daemon's pidfile.
func PidFile() (string, error) {
	dir, err := RuntimeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "daemon.pid"), nil
}

// SessionFilename returns the sanitized JSONL filename for a session id.
// Safe for use as a bare filename: no slashes, no path traversal.
func SessionFilename(sessionID string) string {
	return sanitize(sessionID) + ".jsonl"
}

// SessionFile returns the absolute path to a session's JSONL file.
func SessionFile(sessionID string) (string, error) {
	dir, err := SessionsDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, SessionFilename(sessionID)), nil
}

// SafeSessionID reports whether id is safe to use verbatim as a session
// filename stem: non-empty and composed only of the characters sanitize
// preserves (ASCII letters, digits, '-', '_'). Ids that sanitization would
// rewrite (slashes, dots, traversal sequences, unicode) are rejected so a
// mutating caller can refuse the request rather than act on a surprising,
// possibly-colliding path. Read paths that already tolerate foreign data may
// continue to use SessionFilename directly; deletes should gate on this first.
func SafeSessionID(id string) bool {
	if id == "" {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// sanitize maps a session id to a safe filename component. It keeps ASCII
// letters, digits, '-' and '_', replacing everything else with '_'. An empty
// input returns "_" so a filename always has at least one character.
func sanitize(id string) string {
	if id == "" {
		return "_"
	}
	var b strings.Builder
	b.Grow(len(id))
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}
