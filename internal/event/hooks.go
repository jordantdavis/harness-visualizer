// Package event — hook registry: typed metadata for the non-pairable hook
// events that get rendered as standalone lane rows in the TUI / web UI.
// Paired hooks (PreToolUse/PostToolUse, SubagentStart/SubagentStop,
// PreCompact/PostCompact, etc.) are deliberately NOT in this registry —
// they flow through the Operation model.
package event

// Lane groups related hook events for client-side categorization.
type Lane string

const (
	LanePermission   Lane = "permission"
	LaneInstructions Lane = "instructions"
	LaneConfig       Lane = "config"
	LaneCwd          Lane = "cwd"
	LaneTask         Lane = "task"
	LaneExpansion    Lane = "expansion"
	LaneMessage      Lane = "message"
	LaneWorktree     Lane = "worktree"
	LaneStopFailure  Lane = "stop_failure"
)

// HookMeta is the per-hook rendering metadata both clients share.
type HookMeta struct {
	Name     string `json:"name"`
	Glyph    string `json:"glyph"`    // single display cell
	Label    string `json:"label"`    // ≤16 chars
	Lane     Lane   `json:"lane"`
	Severity string `json:"severity"` // info | warn | error | dim
}

// Hooks is the canonical list of non-pairable hooks recognized by hv.
var Hooks = []HookMeta{
	{"PermissionRequest", "🔒", "Permission", LanePermission, "warn"},
	{"PermissionDenied", "🚫", "PermDenied", LanePermission, "error"},
	{"InstructionsLoaded", "📄", "Instructions", LaneInstructions, "info"},
	{"ConfigChange", "⚙", "Config", LaneConfig, "info"},
	{"CwdChanged", "📁", "Cwd", LaneCwd, "info"},
	{"TaskCreated", "☐", "TaskCreate", LaneTask, "info"},
	{"TaskCompleted", "☑", "TaskDone", LaneTask, "info"},
	{"UserPromptExpansion", "🪄", "Expansion", LaneExpansion, "info"},
	{"MessageDisplay", "💬", "Message", LaneMessage, "dim"},
	{"WorktreeRemove", "🪵", "WorktreeRm", LaneWorktree, "info"},
	{"StopFailure", "⚠", "StopFailure", LaneStopFailure, "error"},
}

// Lookup returns the HookMeta for a hook event name. The second result is
// false when name is not in the registry — clients should fall back to a
// generic rendering.
func Lookup(name string) (HookMeta, bool) {
	for _, h := range Hooks {
		if h.Name == name {
			return h, true
		}
	}
	return HookMeta{}, false
}
