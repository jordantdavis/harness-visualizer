// Package hooks adapts Claude Code's per-hook Raw payloads into harness-agnostic
// scalars. It is the ONLY place in hv that knows hook payload field names.
// All functions are defensive: a missing field, malformed JSON, or empty input
// yields a zero value rather than an error.
package hooks

import "encoding/json"

// ToolUseID returns the top-level "tool_use_id" from a PreToolUse/PostToolUse/
// PostToolUseFailure Raw payload, or "" if absent or malformed.
func ToolUseID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["tool_use_id"])
	})
}

// SubagentID returns the top-level "subagent_id" from a SubagentStart/SubagentStop
// Raw payload, or "" if absent.
func SubagentID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["subagent_id"])
	})
}

// SubagentTarget returns a human-readable label for a subagent operation,
// preferring "subagent_type" and falling back to "description".
func SubagentTarget(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		if s := unquote(m["subagent_type"]); s != "" {
			return s
		}
		return unquote(m["description"])
	})
}

// SubagentHasError reports whether the SubagentStop payload signals an error.
// True when "error" is a non-empty string, or an object/array of any shape.
func SubagentHasError(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	v, ok := m["error"]
	if !ok || len(v) == 0 {
		return false
	}
	var s string
	if json.Unmarshal(v, &s) == nil {
		return s != ""
	}
	return string(v) != "null"
}

// CompactID returns the top-level "compact_id" from a PreCompact/PostCompact
// Raw payload, or "" if absent.
func CompactID(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		return unquote(m["compact_id"])
	})
}

// CompactTarget returns a human-readable label for a compact operation,
// preferring "trigger" and falling back to "reason".
func CompactTarget(raw json.RawMessage) string {
	return decodeString(raw, func(m map[string]json.RawMessage) string {
		if s := unquote(m["trigger"]); s != "" {
			return s
		}
		return unquote(m["reason"])
	})
}

// PostToolUseFailureMessage returns a short failure description from a
// PostToolUseFailure payload. Tries "error" as a string first, then
// "error.message".
func PostToolUseFailureMessage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if s := unquote(m["error"]); s != "" {
		return s
	}
	if e, ok := m["error"]; ok && len(e) > 0 {
		var inner struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(e, &inner) == nil {
			return inner.Message
		}
	}
	return ""
}

// decodeString runs f over a top-level JSON object decoded from raw.
// Returns "" if raw is empty or not a JSON object.
func decodeString(raw json.RawMessage, f func(map[string]json.RawMessage) string) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	return f(m)
}

// unquote unmarshals a JSON RawMessage as a string. Returns "" on any error.
func unquote(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}
