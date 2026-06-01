package tui

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	"jordandavis.dev/harness-visualizer/internal/event"
)

// stripANSIInspect removes ANSI escape sequences from s for plain-text
// measurement inside the inspector package. Defined here for non-test use;
// the test-only stripANSI in phase8_test.go is identical.
func stripANSIInspect(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\x1b' {
			inEsc = true
			continue
		}
		if inEsc {
			if r == 'm' {
				inEsc = false
			}
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// inspectLines renders the inspector body for ev into a slice of display lines,
// each at most w runes wide. It applies syntax-aware rendering for known tool
// shapes (Bash commands, Edit diffs, file paths). Falls back gracefully to
// plain formatted JSON on any parse error — never panics.
// Deprecated: prefer inspectLinesColored which accepts noColor.
func inspectLines(ev *event.Event, w int) []string {
	return inspectLinesColored(ev, w, true) // noColor=true for backward compat
}

// inspectLinesColored renders the inspector body with optional syntax coloring.
// When noColor=false, JSON keys are blue, string values green, numbers/bools
// magenta, and braces/punctuation muted. Long string values are soft-wrapped
// with a hanging indent aligned under the value start (never clipped).
func inspectLinesColored(ev *event.Event, w int, noColor bool) []string {
	if len(ev.Raw) == 0 {
		return []string{"(no payload)"}
	}

	// Best-effort parse of the outer shape.
	var outer struct {
		ToolInput    json.RawMessage `json:"tool_input"`
		ToolResponse json.RawMessage `json:"tool_response"`
	}
	if err := json.Unmarshal(ev.Raw, &outer); err != nil {
		// Malformed — fall back to flat JSON.
		return formatJSONLinesColored(ev.Raw, w, noColor)
	}

	var lines []string

	if len(outer.ToolInput) > 0 {
		if !noColor {
			tok := themeFor(false)
			lines = append(lines, tok.muted.Render(padRight("tool_input", w)))
		} else {
			lines = append(lines, padRight("tool_input", w))
		}

		var toolLines []string
		switch ev.ToolName {
		case "Bash":
			toolLines = renderBashInputColored(outer.ToolInput, w, noColor)
		case "Edit":
			toolLines = renderEditInputColored(outer.ToolInput, w, noColor)
		}
		if len(toolLines) == 0 {
			toolLines = renderGenericInputColored(outer.ToolInput, w, noColor)
		}
		lines = append(lines, toolLines...)
	}

	if len(outer.ToolResponse) > 0 {
		if !noColor {
			tok := themeFor(false)
			lines = append(lines, tok.muted.Render(padRight("tool_response", w)))
		} else {
			lines = append(lines, padRight("tool_response", w))
		}
		lines = append(lines, renderGenericInputColored(outer.ToolResponse, w, noColor)...)
	}

	if len(outer.ToolInput) == 0 && len(outer.ToolResponse) == 0 {
		// Not a tool event — render whole payload.
		lines = append(lines, formatJSONLinesColored(ev.Raw, w, noColor)...)
	}

	return lines
}

// formatJSONLinesColored pretty-prints raw JSON with optional syntax coloring.
// Object keys → info/blue, string values → success/green, numbers/bools → magenta,
// braces/punctuation → muted. Long string values are soft-wrapped.
func formatJSONLinesColored(raw json.RawMessage, w int, noColor bool) []string {
	if len(raw) == 0 {
		return []string{"(empty)"}
	}
	pretty, err := json.MarshalIndent(json.RawMessage(raw), "", "  ")
	if err != nil {
		return []string{clip("(malformed JSON: "+err.Error()+")", w)}
	}
	if noColor {
		var lines []string
		for _, line := range strings.Split(string(pretty), "\n") {
			lines = append(lines, padRight(line, w))
		}
		return lines
	}
	// Syntax-colored: tokenize line by line.
	tok := themeFor(false)
	var lines []string
	for _, line := range strings.Split(string(pretty), "\n") {
		lines = append(lines, colorJSONLine(line, w, tok)...)
	}
	return lines
}

// colorJSONLine applies syntax coloring to a single JSON line and soft-wraps
// if the visible content exceeds w. Returns one or more display lines.
func colorJSONLine(line string, w int, tok tokens) []string {
	if line == "" {
		return []string{strings.Repeat(" ", w)}
	}

	// Measure leading indent (plain spaces).
	indent := 0
	for _, ch := range line {
		if ch == ' ' {
			indent++
		} else {
			break
		}
	}
	rest := line[indent:]

	// Detect line type by content.
	var styled string

	switch {
	case rest == "{" || rest == "}" || rest == "{}" || rest == "}," ||
		rest == "[" || rest == "]" || rest == "]," || rest == "[]":
		// Brace/bracket only line.
		styled = strings.Repeat(" ", indent) + tok.muted.Render(rest)

	case strings.HasPrefix(rest, `"`) && strings.Contains(rest, `": `):
		// Key-value pair: "key": value
		colIdx := strings.Index(rest, `": `)
		if colIdx >= 0 {
			keyPart := rest[:colIdx+2] // includes the quote and colon
			valuePart := rest[colIdx+2:]
			styledKey := tok.info.Render(keyPart)
			styledVal := colorJSONValue(valuePart, tok)
			styled = strings.Repeat(" ", indent) + styledKey + " " + styledVal
		} else {
			styled = strings.Repeat(" ", indent) + rest
		}

	case strings.HasPrefix(rest, `"`) && (strings.HasSuffix(rest, `"`) || strings.HasSuffix(rest, `",`)):
		// Bare string value (array element).
		styled = strings.Repeat(" ", indent) + tok.success.Render(rest)

	default:
		styled = strings.Repeat(" ", indent) + rest
	}

	// Soft-wrap if longer than w.
	if ansiWidth(styled) <= w {
		return []string{ansiPadRight(styled, w)}
	}

	// Wrap: break at w with hanging indent aligned under value start.
	return softWrapLine(styled, w, indent)
}

// colorJSONValue styles a JSON value fragment (the part after "key": ).
// Handles strings (green), numbers/bools/null (magenta), objects/arrays (muted).
func colorJSONValue(val string, tok tokens) string {
	v := strings.TrimSuffix(strings.TrimSuffix(val, ","), "")
	trailing := ""
	if strings.HasSuffix(val, ",") {
		trailing = tok.muted.Render(",")
	}

	switch {
	case strings.HasPrefix(v, `"`):
		return tok.success.Render(v) + trailing
	case v == "true" || v == "false" || v == "null":
		return tok.running.Render(v) + trailing // yellow for bools (readable as magenta isn't in ANSI 16)
	default:
		// Numbers and other literals.
		// Check if it starts with a digit or minus.
		if len(v) > 0 && (v[0] >= '0' && v[0] <= '9' || v[0] == '-') {
			return tok.info.Render(v) + trailing // blue for numbers
		}
		return v + trailing
	}
}

// softWrapLine wraps a potentially-styled line at w visible characters, producing
// continuation lines with a hanging indent of indentRunes spaces.
// This is used for the inspector where long values must not be clipped.
func softWrapLine(line string, w int, indentRunes int) []string {
	// We can't easily split ANSI-coded strings by visible width, so we use a
	// simple approach: strip ANSI, split the plain text by width, then re-apply
	// the dominant color to each segment. This is sufficient for inspector values.
	plain := stripANSIInspect(line)
	if utf8.RuneCountInString(plain) <= w {
		return []string{ansiPadRight(line, w)}
	}

	// Extract the dominant color from the line (the first ANSI code).
	// Re-apply it to each wrapped segment.
	dominantColor := extractDominantStyle(line)

	runes := []rune(plain)
	var result []string
	remaining := runes
	firstLine := true
	hangIndent := strings.Repeat(" ", indentRunes+2) // hanging indent under value

	for len(remaining) > 0 {
		var segment []rune
		var lineWidth int
		if firstLine {
			lineWidth = w
		} else {
			lineWidth = w - utf8.RuneCountInString(hangIndent)
		}
		if lineWidth < 1 {
			lineWidth = 1
		}

		if len(remaining) <= lineWidth {
			segment = remaining
			remaining = nil
		} else {
			segment = remaining[:lineWidth]
			remaining = remaining[lineWidth:]
		}

		var displayLine string
		if !firstLine {
			displayLine = hangIndent + string(segment)
		} else {
			displayLine = string(segment)
		}
		if dominantColor != nil {
			displayLine = dominantColor.Render(displayLine)
		}
		result = append(result, ansiPadRight(displayLine, w))
		firstLine = false
	}
	return result
}

// extractDominantStyle extracts the first lipgloss.Style's foreground from a
// styled string so continuation lines can match the color. Returns nil if none.
func extractDominantStyle(s string) *lipglossStyleRef {
	// Find first ESC[...m sequence.
	start := strings.Index(s, "\x1b[")
	if start < 0 {
		return nil
	}
	end := strings.Index(s[start:], "m")
	if end < 0 {
		return nil
	}
	code := s[start : start+end+1]
	return &lipglossStyleRef{code: code}
}

// lipglossStyleRef is a thin wrapper that re-renders a string with a captured
// ANSI escape sequence prefix.
type lipglossStyleRef struct {
	code string // e.g. "\x1b[32m"
}

func (r *lipglossStyleRef) Render(s string) string {
	return r.code + s + "\x1b[0m"
}

// renderBashInputColored renders tool_input for Bash events with optional color.
func renderBashInputColored(raw json.RawMessage, w int, noColor bool) []string {
	var inp struct {
		Command string `json:"command"`
	}
	if err := json.Unmarshal(raw, &inp); err != nil || inp.Command == "" {
		return nil
	}
	tok := themeFor(noColor)
	var lines []string
	for _, l := range strings.Split(inp.Command, "\n") {
		plain := "  $ " + l
		if noColor {
			lines = append(lines, padRight(plain, w))
		} else {
			lines = append(lines, ansiPadRight(tok.muted.Render("  $ ")+l, w))
		}
	}
	// Append remaining keys via generic renderer.
	lines = append(lines, renderGenericInputExcludingColored(raw, w, noColor, "command")...)
	return lines
}

// renderEditInputColored renders tool_input for Edit events with optional color.
// Shows file path, then before/after blocks for old_string/new_string.
func renderEditInputColored(raw json.RawMessage, w int, noColor bool) []string {
	var inp struct {
		FilePath  string `json:"file_path"`
		OldString string `json:"old_string"`
		NewString string `json:"new_string"`
	}
	if err := json.Unmarshal(raw, &inp); err != nil {
		return nil
	}
	if inp.OldString == "" && inp.NewString == "" {
		return nil
	}
	tok := themeFor(noColor)
	var lines []string
	if inp.FilePath != "" {
		plain := "  " + pathGlyph + " " + inp.FilePath
		if noColor {
			lines = append(lines, padRight(plain, w))
		} else {
			lines = append(lines, ansiPadRight(tok.pathColor.Render(plain), w))
		}
	}
	// Before block.
	if noColor {
		lines = append(lines, padRight("--- before", w))
		for _, l := range strings.Split(inp.OldString, "\n") {
			lines = append(lines, softWrapPlain("- "+l, w, 2)...)
		}
		lines = append(lines, padRight("+++ after", w))
		for _, l := range strings.Split(inp.NewString, "\n") {
			lines = append(lines, softWrapPlain("+ "+l, w, 2)...)
		}
	} else {
		lines = append(lines, ansiPadRight(tok.muted.Render("--- before"), w))
		for _, l := range strings.Split(inp.OldString, "\n") {
			styled := tok.failure.Render("- ") + l
			if ansiWidth(styled) > w {
				lines = append(lines, softWrapLine(styled, w, 2)...)
			} else {
				lines = append(lines, ansiPadRight(styled, w))
			}
		}
		lines = append(lines, ansiPadRight(tok.muted.Render("+++ after"), w))
		for _, l := range strings.Split(inp.NewString, "\n") {
			styled := tok.success.Render("+ ") + l
			if ansiWidth(styled) > w {
				lines = append(lines, softWrapLine(styled, w, 2)...)
			} else {
				lines = append(lines, ansiPadRight(styled, w))
			}
		}
	}
	// Remaining keys.
	lines = append(lines, renderGenericInputExcludingColored(raw, w, noColor, "file_path", "old_string", "new_string")...)
	return lines
}

// pathGlyph is the marker prepended to file path values.
const pathGlyph = "📄"

// renderGenericInput renders a JSON object's key-value pairs using
// renderValueAware for each string value. Numeric/bool/object values fall back
// to their JSON representation.
func renderGenericInput(raw json.RawMessage, w int) []string {
	return renderGenericInputExcluding(raw, w)
}

// renderGenericInputExcluding renders as renderGenericInput but skips the
// specified keys (already rendered by a tool-specific renderer).
func renderGenericInputExcluding(raw json.RawMessage, w int, skip ...string) []string {
	return renderGenericInputExcludingColored(raw, w, true, skip...)
}

// renderGenericInputColored renders a JSON object with optional syntax coloring.
func renderGenericInputColored(raw json.RawMessage, w int, noColor bool) []string {
	return renderGenericInputExcludingColored(raw, w, noColor)
}

// renderGenericInputExcludingColored renders JSON key-value pairs with optional
// syntax coloring, skipping specified keys.
func renderGenericInputExcludingColored(raw json.RawMessage, w int, noColor bool, skip ...string) []string {
	if len(raw) == 0 {
		return nil
	}
	skipSet := make(map[string]bool, len(skip))
	for _, k := range skip {
		skipSet[k] = true
	}

	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return formatJSONLinesColored(raw, w, noColor)
	}

	tok := themeFor(noColor)
	var lines []string
	for k, v := range m {
		if skipSet[k] {
			continue
		}

		// Attempt to get the string value for soft-wrap.
		var s string
		if err := json.Unmarshal(v, &s); err == nil {
			// String value: soft-wrap if long.
			if noColor {
				plain := fmt.Sprintf("  %s: %s", k, renderValueAware(k, s))
				lines = append(lines, softWrapPlain(plain, w, len(k)+4)...)
			} else {
				prefix := "  " + tok.info.Render(`"`+k+`"`) + ": "
				value := tok.success.Render(`"` + s + `"`)
				full := prefix + value
				if ansiWidth(full) <= w {
					lines = append(lines, ansiPadRight(full, w))
				} else {
					// Soft-wrap the value with indent under value start.
					indentW := ansiWidth(prefix)
					lines = append(lines, softWrapStyled(prefix, s, w, indentW, tok)...)
				}
			}
		} else {
			// Non-string: use compact JSON representation.
			raw2 := string(v)
			if noColor {
				lines = append(lines, padRight(fmt.Sprintf("  %s: %s", k, raw2), w))
			} else {
				keyStyled := tok.info.Render(`"` + k + `"`)
				valStyled := colorJSONValue(raw2, tok)
				lines = append(lines, ansiPadRight("  "+keyStyled+": "+valStyled, w))
			}
		}
	}
	return lines
}

// softWrapPlain wraps a plain-text string at w with a hanging indent of indentW.
func softWrapPlain(s string, w, indentW int) []string {
	runes := []rune(s)
	if len(runes) <= w {
		return []string{padRight(s, w)}
	}
	var result []string
	remaining := runes
	first := true
	hang := strings.Repeat(" ", indentW)
	for len(remaining) > 0 {
		var lineW int
		if first {
			lineW = w
		} else {
			lineW = w - indentW
		}
		if lineW < 1 {
			lineW = 1
		}
		var seg []rune
		if len(remaining) <= lineW {
			seg = remaining
			remaining = nil
		} else {
			seg = remaining[:lineW]
			remaining = remaining[lineW:]
		}
		var l string
		if !first {
			l = hang + string(seg)
		} else {
			l = string(seg)
		}
		result = append(result, padRight(l, w))
		first = false
	}
	return result
}

// softWrapStyled wraps a string value across multiple lines with a styled prefix
// on the first line and a hanging indent on continuation lines.
// prefix is the already-styled "  "key": " portion.
// value is the raw string content to wrap (the actual value without quotes).
// w is the display width; indentW is the visible width of the prefix.
func softWrapStyled(prefix, value string, w, indentW int, tok tokens) []string {
	// Available width for value on first line: w - indentW - 2 (for surrounding quotes).
	firstW := w - indentW - 2
	if firstW < 4 {
		firstW = 4
	}
	hangIndent := strings.Repeat(" ", indentW)

	vRunes := []rune(value)
	if len(vRunes) <= firstW {
		// Fits on one line.
		styled := prefix + tok.success.Render(`"`+value+`"`)
		return []string{ansiPadRight(styled, w)}
	}

	var result []string
	// First line: prefix + opening-quote + firstW chars.
	firstChunk := string(vRunes[:firstW])
	firstLine := prefix + tok.success.Render(`"`+firstChunk)
	result = append(result, ansiPadRight(firstLine, w))

	// Continuation lines.
	remaining := vRunes[firstW:]
	contW := w - indentW - 1 // -1 for closing quote space
	if contW < 4 {
		contW = 4
	}
	for len(remaining) > 0 {
		isLast := len(remaining) <= contW
		var chunk []rune
		if isLast {
			chunk = remaining
			remaining = nil
		} else {
			chunk = remaining[:contW]
			remaining = remaining[contW:]
		}
		var seg string
		if isLast {
			seg = string(chunk) + `"`
		} else {
			seg = string(chunk)
		}
		result = append(result, ansiPadRight(hangIndent+tok.success.Render(seg), w))
	}
	return result
}

// renderValueAware returns a display string for a JSON string value, applying
// light syntax awareness based on the key name and value content:
//
//   - key=="command" or key contains "cmd": rendered as a shell block ($ prefix per line).
//   - key=="file_path" or value looks like a path (starts with /): shown with path glyph.
//   - otherwise: value returned as-is.
//
// Always returns a non-empty string when value is non-empty. Never panics.
func renderValueAware(key, value string) string {
	if value == "" {
		return ""
	}

	// Shell command block: key is "command" or contains "cmd".
	lk := strings.ToLower(key)
	if lk == "command" || strings.Contains(lk, "cmd") {
		var sb strings.Builder
		lines := strings.Split(value, "\n")
		for i, l := range lines {
			if i > 0 {
				sb.WriteString("\n")
			}
			sb.WriteString("$ ")
			sb.WriteString(l)
		}
		return sb.String()
	}

	// File path: key is file_path/path, or value starts with '/'.
	if lk == "file_path" || lk == "path" || lk == "filepath" || strings.HasPrefix(value, "/") {
		return pathGlyph + " " + value
	}

	return value
}
