package event

import (
	"encoding/json"
	"testing"
)

func TestParseExtractsCanonicalFields(t *testing.T) {
	raw := []byte(`{
		"hook_event_name": "PreToolUse",
		"session_id": "abc-123",
		"cwd": "/home/u/proj",
		"tool_name": "Bash",
		"tool_input": {"command": "git status"}
	}`)

	ev, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if ev.HookEvent != "PreToolUse" {
		t.Errorf("HookEvent = %q, want %q", ev.HookEvent, "PreToolUse")
	}
	if ev.SessionID != "abc-123" {
		t.Errorf("SessionID = %q, want %q", ev.SessionID, "abc-123")
	}
	if ev.CWD != "/home/u/proj" {
		t.Errorf("CWD = %q, want %q", ev.CWD, "/home/u/proj")
	}
	if ev.ToolName != "Bash" {
		t.Errorf("ToolName = %q, want %q", ev.ToolName, "Bash")
	}
}

func TestParsePreservesRawVerbatim(t *testing.T) {
	raw := []byte(`{"hook_event_name":"Stop","extra":{"deep":[1,2,3]}}`)
	ev, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	var got, want map[string]any
	if err := json.Unmarshal(ev.Raw, &got); err != nil {
		t.Fatalf("Raw is not valid JSON: %v", err)
	}
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatalf("input is not valid JSON: %v", err)
	}
	gb, _ := json.Marshal(got)
	wb, _ := json.Marshal(want)
	if string(gb) != string(wb) {
		t.Errorf("Raw = %s, want %s", gb, wb)
	}
}

func TestParseMissingFieldsAreEmptyNotError(t *testing.T) {
	ev, err := Parse([]byte(`{"hook_event_name":"Notification"}`))
	if err != nil {
		t.Fatalf("Parse returned error: %v", err)
	}
	if ev.SessionID != "" || ev.CWD != "" || ev.ToolName != "" {
		t.Errorf("expected empty optional fields, got %+v", ev)
	}
}

func TestParseInvalidJSONErrors(t *testing.T) {
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}
