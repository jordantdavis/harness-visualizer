// internal/source/claudecode/conversation_test.go
package claudecode

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTranscript(t *testing.T, lines string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(p, []byte(lines), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadConversation_UserAndAssistant(t *testing.T) {
	path := writeTranscript(t, ""+
		`{"type":"user","timestamp":"2026-05-31T10:00:00Z","message":{"role":"user","content":"add the api"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-05-31T10:00:05Z","message":{"role":"assistant","content":[`+
		`{"type":"thinking","thinking":"let me start"},`+
		`{"type":"text","text":"I'll start with the model package."},`+
		`{"type":"tool_use","id":"toolu_1","name":"Edit","input":{}}]}}`+"\n")

	turns, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 2 {
		t.Fatalf("got %d turns, want 2", len(turns))
	}
	if turns[0].Role != "user" || turns[0].Text != "add the api" {
		t.Fatalf("user turn wrong: %+v", turns[0])
	}
	a := turns[1]
	if a.Role != "assistant" || a.Text != "I'll start with the model package." {
		t.Fatalf("assistant text wrong: %+v", a)
	}
	if a.Thinking != "let me start" {
		t.Fatalf("assistant thinking wrong: %+v", a)
	}
	if len(a.ToolRefs) != 1 || a.ToolRefs[0] != "toolu_1" {
		t.Fatalf("assistant tool_refs wrong: %+v", a.ToolRefs)
	}
	if a.At.IsZero() {
		t.Fatal("assistant timestamp not parsed")
	}
}

func TestReadConversation_MissingFileDegrades(t *testing.T) {
	turns, err := ReadConversation("/no/such/file.jsonl")
	if err != nil || turns != nil {
		t.Fatalf("missing file should yield (nil, nil); got (%v, %v)", turns, err)
	}
}

func TestReadConversation_SkipsMalformedLines(t *testing.T) {
	path := writeTranscript(t, "not json\n"+
		`{"type":"user","timestamp":"2026-05-31T10:00:00Z","message":{"role":"user","content":"hi"}}`+"\n")
	turns, err := ReadConversation(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(turns) != 1 {
		t.Fatalf("got %d turns, want 1 (malformed line skipped)", len(turns))
	}
}
