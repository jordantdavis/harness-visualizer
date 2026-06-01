// internal/source/claudecode/hooks/hooks_test.go
package hooks

import (
	"encoding/json"
	"testing"
)

func raw(s string) json.RawMessage { return json.RawMessage(s) }

func TestToolUseID(t *testing.T) {
	cases := []struct {
		name string
		raw  json.RawMessage
		want string
	}{
		{"present", raw(`{"tool_use_id":"abc"}`), "abc"},
		{"absent", raw(`{"other":1}`), ""},
		{"empty raw", raw(``), ""},
		{"malformed", raw(`not json`), ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ToolUseID(c.raw); got != c.want {
				t.Fatalf("ToolUseID = %q, want %q", got, c.want)
			}
		})
	}
}

func TestSubagentID(t *testing.T) {
	if got := SubagentID(raw(`{"subagent_id":"sa-1"}`)); got != "sa-1" {
		t.Fatalf("SubagentID = %q, want sa-1", got)
	}
	if got := SubagentID(raw(`{}`)); got != "" {
		t.Fatalf("SubagentID absent = %q, want \"\"", got)
	}
	if got := SubagentID(raw(`not json`)); got != "" {
		t.Fatalf("SubagentID malformed = %q, want \"\"", got)
	}
}

func TestSubagentTarget(t *testing.T) {
	if got := SubagentTarget(raw(`{"subagent_type":"engineer"}`)); got != "engineer" {
		t.Fatalf("subagent_type = %q, want engineer", got)
	}
	if got := SubagentTarget(raw(`{"description":"do a thing"}`)); got != "do a thing" {
		t.Fatalf("description fallback = %q, want \"do a thing\"", got)
	}
	if got := SubagentTarget(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestSubagentHasError(t *testing.T) {
	if !SubagentHasError(raw(`{"error":"boom"}`)) {
		t.Fatal("error field present should return true")
	}
	if !SubagentHasError(raw(`{"error":{"message":"x"}}`)) {
		t.Fatal("non-empty error object should return true")
	}
	if SubagentHasError(raw(`{"error":""}`)) {
		t.Fatal("empty error string should return false")
	}
	if SubagentHasError(raw(`{}`)) {
		t.Fatal("absent error should return false")
	}
	if SubagentHasError(raw(`not json`)) {
		t.Fatal("malformed should return false")
	}
}

func TestCompactID(t *testing.T) {
	if got := CompactID(raw(`{"compact_id":"c-1"}`)); got != "c-1" {
		t.Fatalf("CompactID = %q, want c-1", got)
	}
	if got := CompactID(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestCompactTarget(t *testing.T) {
	if got := CompactTarget(raw(`{"trigger":"auto"}`)); got != "auto" {
		t.Fatalf("trigger = %q, want auto", got)
	}
	if got := CompactTarget(raw(`{"reason":"manual"}`)); got != "manual" {
		t.Fatalf("reason fallback = %q, want manual", got)
	}
	if got := CompactTarget(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}

func TestPostToolUseFailureMessage(t *testing.T) {
	if got := PostToolUseFailureMessage(raw(`{"error":"boom"}`)); got != "boom" {
		t.Fatalf("error string = %q, want boom", got)
	}
	if got := PostToolUseFailureMessage(raw(`{"error":{"message":"oops"}}`)); got != "oops" {
		t.Fatalf("error.message = %q, want oops", got)
	}
	if got := PostToolUseFailureMessage(raw(`{}`)); got != "" {
		t.Fatalf("absent = %q, want \"\"", got)
	}
}
