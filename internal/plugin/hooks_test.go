// Package plugin contains regression tests for the Claude Code plugin configuration.
package plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

// goldenHooks is the authoritative set of hook event names that must be registered.
// Adding a hook to hooks.json without updating this list, or vice-versa, is a test failure.
var goldenHooks = []string{
	// Original 9
	"Notification",
	"PostToolUse",
	"PreCompact",
	"PreToolUse",
	"SessionEnd",
	"SessionStart",
	"Stop",
	"SubagentStop",
	"UserPromptSubmit",
	// Tier 1 additions (10)
	"PermissionDenied",
	"PermissionRequest",
	"PostCompact",
	"PostToolBatch",
	"PostToolUseFailure",
	"StopFailure",
	"SubagentStart",
	"TaskCompleted",
	"TaskCreated",
	"UserPromptExpansion",
	// Tier 2 additions (4 — MessageDisplay omitted: not a real Claude Code hook event)
	"ConfigChange",
	"CwdChanged",
	"InstructionsLoaded",
	"WorktreeRemove",
	// Tier 3 additions (6)
	"Elicitation",
	"ElicitationResult",
	"FileChanged",
	"Setup",
	"TeammateIdle",
	"WorktreeCreate",
}

// canonicalCommand is the exact command string every hook entry must use.
const canonicalCommand = `"${CLAUDE_PLUGIN_ROOT}/bin/hv" hook`

// hooksJSON is the parsed top-level shape of plugin/hooks/hooks.json.
type hooksJSON struct {
	Hooks map[string][]hookEntry `json:"hooks"`
}

type hookEntry struct {
	Matcher string        `json:"matcher"`
	Hooks   []commandHook `json:"hooks"`
}

type commandHook struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Async   bool   `json:"async"`
}

// hooksJSONPath resolves plugin/hooks/hooks.json relative to this test file's
// location (internal/plugin/ → up two levels → module root → plugin/hooks/hooks.json).
func hooksJSONPath(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// internal/plugin/hooks_test.go → internal/plugin → internal → module root
	moduleRoot := filepath.Dir(filepath.Dir(filepath.Dir(file)))
	return filepath.Join(moduleRoot, "plugin", "hooks", "hooks.json")
}

func loadHooks(t *testing.T) hooksJSON {
	t.Helper()
	path := hooksJSONPath(t)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %s: %v", path, err)
	}
	var h hooksJSON
	if err := json.Unmarshal(data, &h); err != nil {
		t.Fatalf("invalid JSON in %s: %v", path, err)
	}
	return h
}

// TestHooksJSONRegisteredNames verifies that the set of top-level hook event names
// in hooks.json exactly matches the golden list — no more, no fewer.
func TestHooksJSONRegisteredNames(t *testing.T) {
	h := loadHooks(t)

	got := make([]string, 0, len(h.Hooks))
	for name := range h.Hooks {
		got = append(got, name)
	}
	sort.Strings(got)

	want := make([]string, len(goldenHooks))
	copy(want, goldenHooks)
	sort.Strings(want)

	wantSet := make(map[string]bool, len(want))
	for _, n := range want {
		wantSet[n] = true
	}
	gotSet := make(map[string]bool, len(got))
	for _, n := range got {
		gotSet[n] = true
	}

	var missing, extra []string
	for _, n := range want {
		if !gotSet[n] {
			missing = append(missing, n)
		}
	}
	for _, n := range got {
		if !wantSet[n] {
			extra = append(extra, n)
		}
	}

	if len(missing) > 0 || len(extra) > 0 {
		var sb strings.Builder
		if len(missing) > 0 {
			fmt.Fprintf(&sb, "missing hooks (in golden, not in JSON): %v\n", missing)
		}
		if len(extra) > 0 {
			fmt.Fprintf(&sb, "extra hooks (in JSON, not in golden): %v\n", extra)
		}
		t.Error(sb.String())
	}
}

// TestHooksJSONEntryShape verifies that every hook entry has the expected shape:
// one matcher group with matcher "*", one command hook with async:true and the
// canonical command string. This prevents silent changes to matcher semantics.
func TestHooksJSONEntryShape(t *testing.T) {
	h := loadHooks(t)

	for name, entries := range h.Hooks {
		t.Run(name, func(t *testing.T) {
			if len(entries) != 1 {
				t.Errorf("%s: expected 1 matcher group, got %d", name, len(entries))
				return
			}
			e := entries[0]
			if e.Matcher != "*" {
				t.Errorf("%s: matcher = %q, want %q", name, e.Matcher, "*")
			}
			if len(e.Hooks) != 1 {
				t.Errorf("%s: expected 1 command hook, got %d", name, len(e.Hooks))
				return
			}
			ch := e.Hooks[0]
			if ch.Type != "command" {
				t.Errorf("%s: hook type = %q, want %q", name, ch.Type, "command")
			}
			if ch.Command != canonicalCommand {
				t.Errorf("%s: command = %q, want %q", name, ch.Command, canonicalCommand)
			}
			if !ch.Async {
				t.Errorf("%s: async = false, want true", name)
			}
		})
	}
}
