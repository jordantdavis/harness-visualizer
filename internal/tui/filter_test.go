package tui

import (
	"encoding/json"
	"testing"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// --- parseFilter ------------------------------------------------------------

func TestParseFilterEmpty(t *testing.T) {
	f := parseFilter("")
	if len(f.tokens) != 0 {
		t.Errorf("empty query: want 0 tokens, got %d", len(f.tokens))
	}
	if f.IsEmpty() {
		// empty filter matches everything
		t.Log("empty filter is empty — correct")
	}
}

func TestParseFilterToolToken(t *testing.T) {
	f := parseFilter("tool:Bash")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	tok := f.tokens[0]
	if tok.key != "tool" || tok.value != "bash" {
		t.Errorf("token = {%q, %q}, want {tool, bash}", tok.key, tok.value)
	}
}

func TestParseFilterHookToken(t *testing.T) {
	f := parseFilter("hook:PostToolUse")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	if f.tokens[0].key != "hook" || f.tokens[0].value != "posttooluse" {
		t.Errorf("hook token = %+v", f.tokens[0])
	}
}

func TestParseFilterStatusToken(t *testing.T) {
	f := parseFilter("status:error")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	if f.tokens[0].key != "status" {
		t.Errorf("key = %q, want status", f.tokens[0].key)
	}
}

func TestParseFilterDurGT(t *testing.T) {
	f := parseFilter("dur:>500ms")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	tok := f.tokens[0]
	if tok.key != "dur" {
		t.Errorf("key = %q, want dur", tok.key)
	}
	if tok.durOp != ">" {
		t.Errorf("durOp = %q, want >", tok.durOp)
	}
}

func TestParseFilterDurLTE(t *testing.T) {
	f := parseFilter("dur:<=2s")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	tok := f.tokens[0]
	if tok.durOp != "<=" {
		t.Errorf("durOp = %q, want <=", tok.durOp)
	}
}

func TestParseFilterPathToken(t *testing.T) {
	f := parseFilter("path:auth")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	if f.tokens[0].key != "path" {
		t.Errorf("key = %q, want path", f.tokens[0].key)
	}
}

func TestParseFilterBareTextFuzzy(t *testing.T) {
	f := parseFilter("bash")
	if len(f.tokens) != 1 {
		t.Fatalf("want 1 token, got %d", len(f.tokens))
	}
	if f.tokens[0].key != "" {
		t.Errorf("bare text token should have empty key, got %q", f.tokens[0].key)
	}
	if f.tokens[0].value != "bash" {
		t.Errorf("value = %q, want bash", f.tokens[0].value)
	}
}

func TestParseFilterMultipleTokens(t *testing.T) {
	f := parseFilter("tool:Bash status:error")
	if len(f.tokens) != 2 {
		t.Fatalf("want 2 tokens, got %d", len(f.tokens))
	}
}

func TestParseFilterIsEmpty(t *testing.T) {
	if !parseFilter("").IsEmpty() {
		t.Error("empty query should produce IsEmpty()==true")
	}
	if parseFilter("tool:Bash").IsEmpty() {
		t.Error("non-empty query should produce IsEmpty()==false")
	}
}

// --- matchEvent -------------------------------------------------------------

func filterEv(hook, tool string, raw string) *event.Event {
	return &event.Event{
		HookEvent:  hook,
		ToolName:   tool,
		CapturedAt: time.Date(2026, 5, 28, 10, 0, 0, 0, time.UTC),
		Raw:        json.RawMessage(raw),
	}
}

func TestMatchToolToken(t *testing.T) {
	ev := filterEv("PreToolUse", "Bash", `{"tool_input":{"command":"ls"}}`)
	dr := displayRow{Pre: ev}

	f := parseFilter("tool:bash")
	if !matchEvent(f, dr) {
		t.Error("tool:bash should match Bash tool")
	}
	f2 := parseFilter("tool:Read")
	if matchEvent(f2, dr) {
		t.Error("tool:Read should not match Bash tool")
	}
}

func TestMatchHookToken(t *testing.T) {
	ev := filterEv("PreToolUse", "Bash", `{}`)
	dr := displayRow{Pre: ev}

	if !matchEvent(parseFilter("hook:pretooluse"), dr) {
		t.Error("hook:pretooluse should match PreToolUse")
	}
	if matchEvent(parseFilter("hook:posttooluse"), dr) {
		t.Error("hook:posttooluse should not match PreToolUse")
	}
}

func TestMatchStatusTokenError(t *testing.T) {
	post := filterEv("PostToolUse", "Bash", `{"tool_response":{"exit_code":1}}`)
	dr := displayRow{Pre: post}

	if !matchEvent(parseFilter("status:error"), dr) {
		t.Error("status:error should match exit_code=1")
	}
	if matchEvent(parseFilter("status:ok"), dr) {
		t.Error("status:ok should not match exit_code=1")
	}
}

func TestMatchStatusTokenOK(t *testing.T) {
	post := filterEv("PostToolUse", "Bash", `{"tool_response":{"exit_code":0}}`)
	dr := displayRow{Pre: post}
	if !matchEvent(parseFilter("status:ok"), dr) {
		t.Error("status:ok should match exit_code=0")
	}
}

func TestMatchStatusTokenRunning(t *testing.T) {
	pre := filterEv("PreToolUse", "Bash", `{}`)
	dr := displayRow{Pre: pre}
	if !matchEvent(parseFilter("status:running"), dr) {
		t.Error("status:running should match PreToolUse (running)")
	}
}

func TestMatchDurToken(t *testing.T) {
	pre := filterEv("PreToolUse", "Bash", `{}`)
	post := filterEv("PostToolUse", "Bash", `{"tool_response":{"exit_code":0}}`)
	dr := displayRow{Pre: pre, Post: post, IsPair: true, Duration: 800 * time.Millisecond}

	if !matchEvent(parseFilter("dur:>500ms"), dr) {
		t.Error("dur:>500ms should match 800ms")
	}
	if matchEvent(parseFilter("dur:>1s"), dr) {
		t.Error("dur:>1s should not match 800ms")
	}
	if !matchEvent(parseFilter("dur:<=800ms"), dr) {
		t.Error("dur:<=800ms should match 800ms")
	}
	if !matchEvent(parseFilter("dur:<1s"), dr) {
		t.Error("dur:<1s should match 800ms")
	}
}

func TestMatchDurTokenNoDuration(t *testing.T) {
	// An unpaired event (no known duration) must not match a dur: token.
	pre := filterEv("PreToolUse", "Bash", `{}`)
	dr := displayRow{Pre: pre}
	if matchEvent(parseFilter("dur:>0ms"), dr) {
		t.Error("event with no duration should not match any dur: token")
	}
}

func TestMatchPathToken(t *testing.T) {
	pre := filterEv("PreToolUse", "Read", `{"tool_input":{"file_path":"/home/user/auth.go"}}`)
	dr := displayRow{Pre: pre}
	if !matchEvent(parseFilter("path:auth"), dr) {
		t.Error("path:auth should match /home/user/auth.go")
	}
	if matchEvent(parseFilter("path:main"), dr) {
		t.Error("path:main should not match auth.go")
	}
}

func TestMatchBareTextFuzzy(t *testing.T) {
	ev := filterEv("PreToolUse", "Bash", `{"tool_input":{"command":"git status"}}`)
	dr := displayRow{Pre: ev}
	// "gts" should fuzzy-match "git status" in the combined text
	if !matchEvent(parseFilter("gts"), dr) {
		t.Error("fuzzy: gts should match combined text containing git status")
	}
	// "zzz" should not match
	if matchEvent(parseFilter("zzz"), dr) {
		t.Error("fuzzy: zzz should not match anything in this event")
	}
}

func TestMatchMultipleTokensAND(t *testing.T) {
	ev := filterEv("PreToolUse", "Bash", `{"tool_input":{"command":"ls"}}`)
	dr := displayRow{Pre: ev}
	// Both conditions must be true.
	if !matchEvent(parseFilter("tool:bash hook:pretooluse"), dr) {
		t.Error("AND of two matching tokens should be true")
	}
	// One condition fails → false.
	if matchEvent(parseFilter("tool:bash hook:posttooluse"), dr) {
		t.Error("AND with one failing condition should be false")
	}
}

func TestMatchEmptyFilterMatchesAll(t *testing.T) {
	ev := filterEv("PreToolUse", "Bash", `{}`)
	dr := displayRow{Pre: ev}
	if !matchEvent(parseFilter(""), dr) {
		t.Error("empty filter should match everything")
	}
}

// --- fuzzyMatch helper -------------------------------------------------------

func TestFuzzyMatch(t *testing.T) {
	cases := []struct {
		needle, haystack string
		want             bool
	}{
		{"", "anything", true},
		{"abc", "aXbXc", true},
		{"abc", "abcdef", true},
		{"abc", "xbxcx", false},
		{"bash", "PreToolUse Bash git status", true},
		{"zzz", "PreToolUse Bash git status", false},
	}
	for _, tc := range cases {
		got := fuzzyMatch(tc.needle, tc.haystack)
		if got != tc.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.needle, tc.haystack, got, tc.want)
		}
	}
}
