// internal/model/detail.go
package model

import (
	"encoding/json"

	"jordandavis.dev/harness-visualizer/internal/event"
	"jordandavis.dev/harness-visualizer/internal/source/claudecode/hooks"
)

// OperationDetail is the heavy, lazily-fetched payload for one operation. Only
// one of Diff / (Command,Output) is populated, selected by DetailKind. RawPre /
// RawPost are the verbatim hook payloads — the always-available escape hatch.
type OperationDetail struct {
	ID         string          `json:"id"`
	Tool       string          `json:"tool"`
	DetailKind string          `json:"detail_kind"` // "diff" | "output" | "generic"
	FilePath   string          `json:"file_path,omitempty"`
	Diff       []DiffOp        `json:"diff,omitempty"`
	Command    string          `json:"command,omitempty"`
	Output     string          `json:"output,omitempty"`
	ExitCode   *int            `json:"exit_code,omitempty"`
	RawPre     json.RawMessage `json:"raw_pre,omitempty"`
	RawPost    json.RawMessage `json:"raw_post,omitempty"`
}

// BuildOperationDetail shapes the detail payload from a Pre event and its
// optional Post. post may be nil for a still-running operation.
func BuildOperationDetail(pre, post *event.Event) OperationDetail {
	d := OperationDetail{
		ID:         hooks.ToolUseID(pre.Raw),
		Tool:       pre.ToolName,
		DetailKind: "generic",
		RawPre:     pre.Raw,
	}
	if post != nil {
		d.RawPost = post.Raw
	}

	switch pre.ToolName {
	case "Edit", "Write", "MultiEdit":
		var in struct {
			FilePath  string `json:"file_path"`
			OldString string `json:"old_string"`
			NewString string `json:"new_string"`
		}
		if parseToolInput(pre.Raw, &in) && (in.OldString != "" || in.NewString != "") {
			d.DetailKind = "diff"
			d.FilePath = in.FilePath
			d.Diff = DiffLines(in.OldString, in.NewString)
		}
	case "Bash":
		var in struct {
			Command string `json:"command"`
		}
		if parseToolInput(pre.Raw, &in) {
			d.Command = in.Command
		}
		if post != nil {
			var resp struct {
				ToolResponse struct {
					ExitCode *int   `json:"exit_code"`
					Stdout   string `json:"stdout"`
					Stderr   string `json:"stderr"`
				} `json:"tool_response"`
			}
			if json.Unmarshal(post.Raw, &resp) == nil {
				d.ExitCode = resp.ToolResponse.ExitCode
				d.Output = resp.ToolResponse.Stdout + resp.ToolResponse.Stderr
			}
		}
		d.DetailKind = "output"
	}
	return d
}

// parseToolInput unmarshals the tool_input object from a hook payload into v.
func parseToolInput(raw json.RawMessage, v any) bool {
	var w struct {
		ToolInput json.RawMessage `json:"tool_input"`
	}
	if json.Unmarshal(raw, &w) != nil || len(w.ToolInput) == 0 {
		return false
	}
	return json.Unmarshal(w.ToolInput, v) == nil
}
