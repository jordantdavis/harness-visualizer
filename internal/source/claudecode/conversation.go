// internal/source/claudecode/conversation.go

// Package claudecode adapts Claude Code's on-disk transcript format into the
// harness-agnostic model.Turn type. It is the ONLY package that knows the
// Claude Code transcript schema; everything upstream consumes model.Turn. All
// parsing is defensive — a missing, unreadable, or foreign file yields no turns
// and no error, so the timeline degrades gracefully to operations-only.
package claudecode

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"

	"jordandavis.dev/harness-visualizer/internal/model"
)

const (
	scanBufInit = 64 * 1024
	scanBufMax  = 16 * 1024 * 1024
)

// ReadConversation parses path (a Claude Code transcript JSONL) into ordered
// conversation turns. Returns (nil, nil) when the file is absent. Malformed
// lines and unrecognised record types are skipped.
func ReadConversation(path string) ([]model.Turn, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var turns []model.Turn
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec record
		if json.Unmarshal(line, &rec) != nil {
			continue
		}
		if rec.Type != "user" && rec.Type != "assistant" {
			continue
		}
		turn, ok := rec.toTurn()
		if ok {
			turns = append(turns, turn)
		}
	}
	return turns, sc.Err()
}

// record is one transcript line. content is either a JSON string (user) or an
// array of typed blocks (assistant); rawContent defers that decision.
type record struct {
	Type      string `json:"type"`
	Timestamp string `json:"timestamp"`
	Message   struct {
		Role       string          `json:"role"`
		RawContent json.RawMessage `json:"content"`
	} `json:"message"`
}

type block struct {
	Type     string `json:"type"`
	Text     string `json:"text"`
	Thinking string `json:"thinking"`
	ID       string `json:"id"`
}

func (r record) toTurn() (model.Turn, bool) {
	t := model.Turn{Role: r.Type}
	if ts, err := time.Parse(time.RFC3339, r.Timestamp); err == nil {
		t.At = ts
	}

	// content as a plain string.
	var s string
	if json.Unmarshal(r.Message.RawContent, &s) == nil {
		t.Text = strings.TrimSpace(s)
		return t, t.Text != ""
	}

	// content as an array of blocks.
	var blocks []block
	if json.Unmarshal(r.Message.RawContent, &blocks) != nil {
		return model.Turn{}, false
	}
	var texts, thinks []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if b.Text != "" {
				texts = append(texts, b.Text)
			}
		case "thinking":
			if b.Thinking != "" {
				thinks = append(thinks, b.Thinking)
			}
		case "tool_use":
			if b.ID != "" {
				t.ToolRefs = append(t.ToolRefs, b.ID) // referenced, not emitted as a turn
			}
		}
	}
	t.Text = strings.TrimSpace(strings.Join(texts, "\n"))
	t.Thinking = strings.TrimSpace(strings.Join(thinks, "\n"))
	// Keep the turn if it has prose, thinking, or tool references.
	return t, t.Text != "" || t.Thinking != "" || len(t.ToolRefs) > 0
}
