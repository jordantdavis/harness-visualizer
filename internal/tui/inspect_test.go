package tui

import (
	"encoding/json"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// --- renderValueAware ---

func TestRenderValueAwareCommandKey(t *testing.T) {
	// "command" key with a shell string should produce a shell-block prefix.
	out := renderValueAware("command", "git status --short")
	if !strings.Contains(out, "$") {
		t.Errorf("command value should contain '$' shell prompt marker, got: %q", out)
	}
	if !strings.Contains(out, "git status --short") {
		t.Errorf("command value should contain the original string, got: %q", out)
	}
}

func TestRenderValueAwareMultilineCommand(t *testing.T) {
	// Multi-line commands: each line shown with a prompt prefix.
	out := renderValueAware("command", "git add .\ngit commit -m 'x'")
	if !strings.Contains(out, "git add .") {
		t.Errorf("multiline command: first line missing, got: %q", out)
	}
	if !strings.Contains(out, "git commit") {
		t.Errorf("multiline command: second line missing, got: %q", out)
	}
}

func TestRenderValueAwareFilePath(t *testing.T) {
	// "file_path" key with a path string should show a path marker.
	out := renderValueAware("file_path", "/home/user/project/main.go")
	if !strings.Contains(out, "/home/user/project/main.go") {
		t.Errorf("file_path value should contain the path, got: %q", out)
	}
	if out == "" {
		t.Error("file_path rendering should be non-empty")
	}
}

func TestRenderValueAwarePathLikeValue(t *testing.T) {
	// A value starting with '/' for an arbitrary key should render distinctly
	// (path detection is value-heuristic, not just key-based).
	out := renderValueAware("target", "/etc/hosts")
	if !strings.Contains(out, "/etc/hosts") {
		t.Errorf("path-like value should contain the original string, got: %q", out)
	}
}

func TestRenderValueAwarePlain(t *testing.T) {
	// A plain, non-special key+value passes through containing the value.
	out := renderValueAware("session_id", "abc-123")
	if !strings.Contains(out, "abc-123") {
		t.Errorf("plain value should contain the original string, got: %q", out)
	}
}

func TestRenderValueAwareNeverPanics(t *testing.T) {
	// Defensive: all unusual inputs must not panic.
	cases := [][2]string{
		{"", ""},
		{"command", ""},
		{"file_path", ""},
		{"", "/some/path"},
		{"key", strings.Repeat("x", 10000)},
	}
	for _, c := range cases {
		_ = renderValueAware(c[0], c[1])
	}
}

// --- inspectLines ---

func TestInspectLinesBashCommand(t *testing.T) {
	// A Bash PreToolUse event should show the command with shell framing.
	raw := json.RawMessage(`{"hook_event_name":"PreToolUse","tool_input":{"command":"ls -la"}}`)
	ev := &event.Event{
		HookEvent: "PreToolUse",
		ToolName:  "Bash",
		Raw:       raw,
	}
	lines := inspectLines(ev, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "$") {
		t.Errorf("Bash command inspector should show shell '$' marker; got:\n%s", joined)
	}
	if !strings.Contains(joined, "ls -la") {
		t.Errorf("Bash command inspector should show the command; got:\n%s", joined)
	}
}

func TestInspectLinesEditDiff(t *testing.T) {
	// An Edit event should show before/after framing for old_string→new_string.
	raw := json.RawMessage(`{
		"hook_event_name":"PreToolUse",
		"tool_input":{
			"file_path":"/app/main.go",
			"old_string":"foo",
			"new_string":"bar"
		}
	}`)
	ev := &event.Event{
		HookEvent: "PreToolUse",
		ToolName:  "Edit",
		Raw:       raw,
	}
	lines := inspectLines(ev, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "foo") {
		t.Errorf("Edit diff should show old_string; got:\n%s", joined)
	}
	if !strings.Contains(joined, "bar") {
		t.Errorf("Edit diff should show new_string; got:\n%s", joined)
	}
	// Must have some before/after marker.
	if !strings.Contains(joined, "-") && !strings.Contains(joined, "before") {
		t.Errorf("Edit diff should have before(-) marker; got:\n%s", joined)
	}
	if !strings.Contains(joined, "+") && !strings.Contains(joined, "after") {
		t.Errorf("Edit diff should have after(+) marker; got:\n%s", joined)
	}
}

func TestInspectLinesFilePath(t *testing.T) {
	// A Read event should include the file_path in the output.
	raw := json.RawMessage(`{"tool_input":{"file_path":"/src/readme.md"}}`)
	ev := &event.Event{HookEvent: "PreToolUse", ToolName: "Read", Raw: raw}
	lines := inspectLines(ev, 80)
	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "/src/readme.md") {
		t.Errorf("Read event should show file path; got:\n%s", joined)
	}
}

func TestInspectLinesNeverPanicsOnBadJSON(t *testing.T) {
	// Malformed JSON must not crash; returns at least one line.
	ev := &event.Event{
		HookEvent: "PreToolUse",
		ToolName:  "Bash",
		Raw:       json.RawMessage(`{bad json`),
	}
	lines := inspectLines(ev, 80)
	if len(lines) == 0 {
		t.Error("inspectLines should return at least one line even on bad JSON")
	}
}

func TestInspectLinesNeverPanicsOnEmptyRaw(t *testing.T) {
	ev := &event.Event{HookEvent: "Stop"}
	lines := inspectLines(ev, 80)
	if len(lines) == 0 {
		t.Error("inspectLines should return at least one line on empty Raw")
	}
}

// --- yank + toast via model ---

// captureYank is an injectable yankFn that records the last string yanked.
type captureYank struct{ last string }

func (c *captureYank) fn(s string) error {
	c.last = s
	return nil
}

// yankModel returns a model wired with events and focused on the inspector pane.
func yankModel() model {
	fake := fixtureClient()
	m := newModel(fake, false, false)
	m.sessions = fake.Sessions_
	m.selectedSession = "sess-1"
	m.events = fake.Events_["sess-1"]
	m.eventCursor = 0
	m.focusedPane = paneInspector
	m.width = 140
	m.height = 30
	m.layout = LayoutWide
	return m
}

func TestYankFnField(t *testing.T) {
	// yankFn must be a settable field on the model.
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn
	if m.yankFn == nil {
		t.Error("yankFn field should be non-nil after assignment")
	}
}

func TestYankSmallY(t *testing.T) {
	// 'y' yanks the focused value — the raw JSON of the selected event's Raw field.
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	if cap.last == "" {
		t.Error("'y' should have called yankFn with a non-empty string")
	}
	if !json.Valid([]byte(cap.last)) {
		t.Errorf("'y' yanked string should be valid JSON, got: %q", cap.last)
	}
}

func TestYankBigY(t *testing.T) {
	// 'Y' yanks the whole event Raw JSON.
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})

	if cap.last == "" {
		t.Error("'Y' should have called yankFn with a non-empty string")
	}
	if !json.Valid([]byte(cap.last)) {
		t.Errorf("'Y' yanked string should be valid JSON, got: %q", cap.last)
	}
}

func TestYankSmallYSetsStatusMsg(t *testing.T) {
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	m2 := updated.(model)

	if m2.statusMsg == "" {
		t.Error("'y' should set statusMsg (toast)")
	}
}

func TestYankBigYSetsStatusMsg(t *testing.T) {
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})
	m2 := updated.(model)

	if m2.statusMsg == "" {
		t.Error("'Y' should set statusMsg (toast)")
	}
}

func TestYankNoEventDoesNothing(t *testing.T) {
	// When there is no selected event, yank should not call yankFn.
	m := yankModel()
	m.events = nil
	cap := &captureYank{}
	m.yankFn = cap.fn

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	if cap.last != "" {
		t.Error("'y' with no event should not call yankFn")
	}
}

func TestStatusMsgAppearsInView(t *testing.T) {
	m := yankModel()
	m.statusMsg = "yanked value"

	view := m.View()
	if !strings.Contains(view, "yanked value") {
		t.Errorf("statusMsg should appear in the rendered view; got view length %d", len(view))
	}
}

func TestDefaultYankFnIsNotNil(t *testing.T) {
	// newModel should wire a non-nil default yankFn.
	m := newModel(fixtureClient(), false, false)
	if m.yankFn == nil {
		t.Error("newModel should provide a non-nil default yankFn")
	}
}

func TestYankSmallYYanksEventRaw(t *testing.T) {
	// 'y' with inspector focus should yank the event's Raw field.
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn
	ev := m.selectedEvent()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})

	if cap.last != string(ev.Raw) {
		t.Errorf("'y' should yank event.Raw; got %q, want %q", cap.last, string(ev.Raw))
	}
}

func TestYankBigYYanksPrettyRaw(t *testing.T) {
	// 'Y' yanks the pretty-printed whole event Raw JSON.
	m := yankModel()
	cap := &captureYank{}
	m.yankFn = cap.fn
	ev := m.selectedEvent()

	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")})

	// Pretty-printed should still be valid JSON and contain the same data.
	if !json.Valid([]byte(cap.last)) {
		t.Errorf("'Y' should yank valid JSON, got: %q", cap.last)
	}
	// Compare via round-trip: both should unmarshal to the same thing.
	var got, want interface{}
	json.Unmarshal([]byte(cap.last), &got)
	json.Unmarshal(ev.Raw, &want)
	gotJ, _ := json.Marshal(got)
	wantJ, _ := json.Marshal(want)
	if string(gotJ) != string(wantJ) {
		t.Errorf("'Y' yanked different JSON content; got %q, want %q", gotJ, wantJ)
	}
}
