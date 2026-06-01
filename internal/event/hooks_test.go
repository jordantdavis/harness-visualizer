package event

import "testing"

func TestHooksRegistryAllNamesPresent(t *testing.T) {
	want := []string{
		"PermissionRequest", "PermissionDenied",
		"InstructionsLoaded",
		"ConfigChange",
		"CwdChanged",
		"TaskCreated", "TaskCompleted",
		"UserPromptExpansion",
		"WorktreeRemove",
		"StopFailure",
	}
	got := map[string]bool{}
	for _, h := range Hooks {
		got[h.Name] = true
	}
	for _, n := range want {
		if !got[n] {
			t.Errorf("registry missing hook %q", n)
		}
	}
	if len(Hooks) != len(want) {
		t.Errorf("registry length = %d, want %d", len(Hooks), len(want))
	}
}

func TestHooksRegistryEntriesComplete(t *testing.T) {
	validSeverity := map[string]bool{"info": true, "warn": true, "error": true, "dim": true}
	validLane := map[Lane]bool{
		LanePermission: true, LaneInstructions: true, LaneConfig: true,
		LaneCwd: true, LaneTask: true, LaneExpansion: true,
		LaneWorktree: true, LaneStopFailure: true,
	}
	for _, h := range Hooks {
		if h.Glyph == "" {
			t.Errorf("%s: empty glyph", h.Name)
		}
		if h.Label == "" || len(h.Label) > 16 {
			t.Errorf("%s: bad label %q (len=%d)", h.Name, h.Label, len(h.Label))
		}
		if !validLane[h.Lane] {
			t.Errorf("%s: bad lane %q", h.Name, h.Lane)
		}
		if !validSeverity[h.Severity] {
			t.Errorf("%s: bad severity %q", h.Name, h.Severity)
		}
	}
}

func TestLookup(t *testing.T) {
	if _, ok := Lookup("PermissionRequest"); !ok {
		t.Error("Lookup(PermissionRequest) should succeed")
	}
	if _, ok := Lookup("PreToolUse"); ok {
		t.Error("Lookup(PreToolUse) should fail — paired hooks are not in the registry")
	}
	if _, ok := Lookup(""); ok {
		t.Error("Lookup(empty) should fail")
	}
}
