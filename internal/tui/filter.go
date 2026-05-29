// Package tui — filter.go
//
// Filter grammar: space-separated tokens, ANDed together.
//
//	tool:<v>        substring match vs Event.ToolName (case-insensitive)
//	hook:<v>        substring match vs Event.HookEvent (case-insensitive)
//	status:<v>      ok | error | running | info — matches derived status
//	dur:OP<v>       compare op duration; OP ∈ > >= < <=; <v> is a Go duration
//	path:<v>        substring match vs targetGist (the row's target path/text)
//	<bare text>     case-insensitive fuzzy subsequence over hook+tool+gist
//
// Tokens with an unrecognised key: fall back to bare-text treatment.
// Malformed dur: token: treated as never-matching (no crash).
package tui

import (
	"strings"
	"time"
	"unicode"
)

// filterToken is one parsed token from the filter query.
type filterToken struct {
	key   string // "tool", "hook", "status", "dur", "path", or "" (bare text)
	value string // lowercased value

	// for dur: tokens only
	durOp  string        // ">", ">=", "<", "<="
	durVal time.Duration // parsed duration
	durOK  bool          // false means the duration parse failed → never match
}

// parsedFilter is the result of parsing a user query string.
type parsedFilter struct {
	raw    string        // original query string
	tokens []filterToken // AND-ed conditions
}

// IsEmpty reports whether the filter has no conditions (matches everything).
func (f parsedFilter) IsEmpty() bool { return len(f.tokens) == 0 }

// parseFilter parses a filter query string into a parsedFilter.
// It splits on whitespace and classifies each token as key:value or bare text.
func parseFilter(query string) parsedFilter {
	query = strings.TrimSpace(query)
	if query == "" {
		return parsedFilter{raw: query}
	}

	fields := strings.FieldsFunc(query, unicode.IsSpace)
	tokens := make([]filterToken, 0, len(fields))

	for _, f := range fields {
		tok := parseToken(f)
		tokens = append(tokens, tok)
	}
	return parsedFilter{raw: query, tokens: tokens}
}

// parseToken classifies and parses one whitespace-delimited token.
func parseToken(s string) filterToken {
	idx := strings.IndexByte(s, ':')
	if idx <= 0 {
		// Bare text.
		return filterToken{value: strings.ToLower(s)}
	}
	key := strings.ToLower(s[:idx])
	val := s[idx+1:]

	switch key {
	case "tool", "hook", "status", "path":
		return filterToken{key: key, value: strings.ToLower(val)}
	case "dur":
		return parseDurToken(val)
	default:
		// Unknown key: treat whole thing as bare text.
		return filterToken{value: strings.ToLower(s)}
	}
}

// parseDurToken parses "dur:" values like ">500ms", "<=2s", "<1s", ">=100ms".
func parseDurToken(val string) filterToken {
	tok := filterToken{key: "dur"}
	ops := []string{">=", "<=", ">", "<"}
	for _, op := range ops {
		if strings.HasPrefix(val, op) {
			d, err := time.ParseDuration(val[len(op):])
			if err != nil {
				// Bad parse — never match.
				tok.durOp = op
				tok.durOK = false
				return tok
			}
			tok.durOp = op
			tok.durVal = d
			tok.durOK = true
			return tok
		}
	}
	// No recognisable op — never match.
	tok.durOK = false
	return tok
}

// matchEvent reports whether dr passes all conditions in f.
// An empty filter matches every row.
func matchEvent(f parsedFilter, dr displayRow) bool {
	for _, tok := range f.tokens {
		if !matchToken(tok, dr) {
			return false
		}
	}
	return true
}

// matchToken checks a single filter token against a display row.
func matchToken(tok filterToken, dr displayRow) bool {
	ev := dr.Pre // primary event for inspection

	switch tok.key {
	case "tool":
		return strings.Contains(strings.ToLower(ev.ToolName), tok.value)

	case "hook":
		return strings.Contains(strings.ToLower(ev.HookEvent), tok.value)

	case "status":
		return matchStatus(tok.value, dr.EffectiveStatus())

	case "dur":
		if !tok.durOK {
			return false
		}
		// Events with no known duration never match.
		if !dr.IsPair || dr.Duration == 0 {
			return false
		}
		return matchDurOp(tok.durOp, dr.Duration, tok.durVal)

	case "path":
		gist := strings.ToLower(targetGist(ev))
		return strings.Contains(gist, tok.value)

	default:
		// Bare text: fuzzy subsequence over combined text.
		combined := strings.ToLower(ev.HookEvent + " " + ev.ToolName + " " + targetGist(ev))
		return fuzzyMatch(tok.value, combined)
	}
}

// matchStatus maps a status keyword to an eventStatus and compares.
func matchStatus(keyword string, s eventStatus) bool {
	switch keyword {
	case "ok":
		return s == statusOK
	case "error":
		return s == statusError
	case "running":
		return s == statusRunning
	case "info", "neutral":
		return s == statusNeutral
	default:
		return false
	}
}

// matchDurOp applies the comparison operator.
func matchDurOp(op string, actual, threshold time.Duration) bool {
	switch op {
	case ">":
		return actual > threshold
	case ">=":
		return actual >= threshold
	case "<":
		return actual < threshold
	case "<=":
		return actual <= threshold
	default:
		return false
	}
}

// fuzzyMatch returns true when all runes in needle appear in haystack in order
// (case-insensitive subsequence match). Needle and haystack need not be
// pre-lowercased; the comparison is done case-insensitively.
func fuzzyMatch(needle, haystack string) bool {
	if needle == "" {
		return true
	}
	nr := []rune(strings.ToLower(needle))
	hr := []rune(strings.ToLower(haystack))
	ni := 0
	for hi := 0; hi < len(hr) && ni < len(nr); hi++ {
		if nr[ni] == hr[hi] {
			ni++
		}
	}
	return ni == len(nr)
}
