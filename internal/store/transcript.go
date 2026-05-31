package store

import (
	"bufio"
	"encoding/json"
	"os"
)

// transcriptResult holds the fields extracted from a Claude Code transcript
// JSONL file. All fields are empty when the file is missing, unreadable, or
// contains no matching records.
type transcriptResult struct {
	// aiTitle is the aiTitle field of the last "ai-title" record found.
	aiTitle string
	// lastPrompt is the lastPrompt field of the last "last-prompt" record found.
	lastPrompt string
}

// readTranscript opens path, scans it line-by-line, and returns the last
// "ai-title" and "last-prompt" records. Any error (missing file, unreadable,
// malformed JSON, absent fields) is swallowed — the caller falls through to
// the next rung of the title fallback chain. This function is intentionally
// defensive because it reads Claude Code's internal, undocumented format.
func readTranscript(path string) transcriptResult {
	f, err := os.Open(path)
	if err != nil {
		return transcriptResult{}
	}
	defer f.Close()

	var res transcriptResult
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, scanBufInit), scanBufMax)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var rec struct {
			Type       string `json:"type"`
			AiTitle    string `json:"aiTitle"`
			LastPrompt string `json:"lastPrompt"`
		}
		// Unmarshal errors (garbage lines, arrays, primitives) are silently skipped.
		if err := json.Unmarshal(line, &rec); err != nil {
			continue
		}
		switch rec.Type {
		case "ai-title":
			if rec.AiTitle != "" {
				res.aiTitle = rec.AiTitle
			}
		case "last-prompt":
			if rec.LastPrompt != "" {
				res.lastPrompt = rec.LastPrompt
			}
		}
	}
	// Scanner errors (e.g. line too long) are ignored — partial results are fine.
	return res
}
