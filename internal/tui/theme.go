// Package tui — theme.go
//
// Semantic color tokens for the TUI. All styles reference these tokens so that
// color is applied in exactly one place and can be swapped cleanly (NO_COLOR,
// reduced-motion, adaptive light/dark).
//
// Token design:
//   - Target the 16-color ANSI palette so user terminal themes are respected.
//   - Use lipgloss AdaptiveColor for light/dark variants.
//   - noColorTheme() returns zero-styled tokens — no ANSI escapes, no color, no
//     bold; only glyphs + text tags convey meaning (designed first, not last).
//   - Color is always redundant with glyph + label (color-blind safe).
package tui

import "github.com/charmbracelet/lipgloss"

// tokens holds the semantic color styles for the TUI.
// Each field is a lipgloss.Style; callers call .Render(s) to apply styling.
type tokens struct {
	// success styles successful outcomes (PostToolUse exit 0, ✔).
	// Adaptive: dark=bright-green (10), light=dark-green (2).
	success lipgloss.Style

	// failure styles error outcomes (PostToolUse exit ≠0, ✘).
	// Adaptive: dark=bright-red (9), light=dark-red (1).
	failure lipgloss.Style

	// running styles in-progress events (PreToolUse, ▶).
	// Adaptive: dark=bright-yellow (11), light=dark-yellow (3).
	running lipgloss.Style

	// info styles informational / neutral text.
	// Adaptive: dark=bright-blue (12), light=dark-blue (4).
	info lipgloss.Style

	// muted styles secondary text (timestamps, separators).
	// Adaptive: dark=bright-black/grey (8), light=dark-grey (7 or none).
	muted lipgloss.Style

	// selection styles the selected / focused row (reverse-video).
	selection lipgloss.Style

	// header styles pane title rows.
	header lipgloss.Style

	// chip styles small status chips / badges (e.g. "↓ N new", "● live").
	chip lipgloss.Style
}

// defaultTheme returns the adaptive color token set targeting the 16-color ANSI
// palette. Light/dark adaptive via lipgloss.AdaptiveColor.
func defaultTheme() tokens {
	return tokens{
		success: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "2",  // dark-green
			Dark:  "10", // bright-green
		}),
		failure: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "1", // dark-red
			Dark:  "9", // bright-red
		}),
		running: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "3",  // dark-yellow
			Dark:  "11", // bright-yellow
		}),
		info: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "4",  // dark-blue
			Dark:  "12", // bright-blue
		}),
		muted: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "7", // light-grey
			Dark:  "8", // dark-grey / bright-black
		}),
		selection: lipgloss.NewStyle().Reverse(true),
		header:    lipgloss.NewStyle().Bold(true),
		chip: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "4",
			Dark:  "12",
		}),
	}
}

// noColorTheme returns zero-styled tokens that produce no ANSI output. When
// NO_COLOR is active or the terminal is dumb, all styling is suppressed so
// glyphs + text tags carry the full signal.
func noColorTheme() tokens {
	plain := lipgloss.NewStyle() // zero style: no color, no bold, no decoration
	return tokens{
		success:   plain,
		failure:   plain,
		running:   plain,
		info:      plain,
		muted:     plain,
		selection: plain,
		header:    plain,
		chip:      plain,
	}
}

// themeFor returns the appropriate token set given the noColor flag.
func themeFor(noColor bool) tokens {
	if noColor {
		return noColorTheme()
	}
	return defaultTheme()
}
