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
//
// Phase 8 additions:
//   - selBand / selBandFocused: subtle background band for the selected row
//     (never reverse-video — fixes the "random white block" defect).
//   - accent / accentDim: info-color accent for the caret and left bar when the
//     pane is focused / unfocused.
//   - ellipsis: dim marker for truncated values.
package tui

import "github.com/charmbracelet/lipgloss"

// ansiWidth returns the visible (ANSI-escape-stripped) width of s using
// lipgloss's ANSI-aware measurement. This is the authoritative width for any
// string that may contain terminal escape sequences.
func ansiWidth(s string) int {
	return lipgloss.Width(s)
}

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

	// selection is kept for backward-compatibility but is now unused for row
	// highlighting (replaced by selBand/selBandFocused + accent bar).
	// Set to a no-op style to ensure legacy callers don't emit reverse-video.
	selection lipgloss.Style

	// header styles pane title rows.
	header lipgloss.Style

	// chip styles small status chips / badges (e.g. "↓ N new", "● live").
	chip lipgloss.Style

	// selBand is the background applied to the selected row when the owning
	// pane is NOT focused ("where you'll land when you tab here" — dimmer).
	selBand lipgloss.Style

	// selBandFocused is the background applied to the selected row when the
	// owning pane IS focused — brighter than selBand.
	selBandFocused lipgloss.Style

	// accent is the caret / left-border color when the pane is focused.
	// Uses the info color (bright-blue / dark-blue).
	accent lipgloss.Style

	// accentDim is the caret color when the pane is unfocused (grey).
	accentDim lipgloss.Style

	// accentError is the left-border color for error-row selection (red).
	accentError lipgloss.Style

	// ellipsis styles the truncation marker (…) — dim, muted.
	ellipsis lipgloss.Style

	// pathColor styles path values in the events TARGET column.
	pathColor lipgloss.Style
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
		// selection is intentionally a no-op — reverse-video removed in Phase 8.
		selection: lipgloss.NewStyle(),
		header:    lipgloss.NewStyle().Bold(true),
		chip: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "4",
			Dark:  "12",
		}),
		// Subtle background band for selected rows (never reverse-video).
		selBand: lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{
			Light: "254", // very light grey in light mode
			Dark:  "235", // #1d2530 approximated to ANSI 235
		}),
		selBandFocused: lipgloss.NewStyle().Background(lipgloss.AdaptiveColor{
			Light: "252", // slightly stronger in light mode
			Dark:  "236", // #23303f approximated to ANSI 236
		}),
		// Caret / accent when pane is focused: info blue.
		accent: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "4",
			Dark:  "12",
		}),
		// Caret when pane is unfocused: muted grey.
		accentDim: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "7",
			Dark:  "8",
		}),
		// Left-border for error-row selection: red.
		accentError: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "1",
			Dark:  "9",
		}),
		// Truncation marker: same as muted.
		ellipsis: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
			Light: "7",
			Dark:  "8",
		}),
		// File paths in TARGET column: info blue (same as accent).
		pathColor: lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{
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
		success:        plain,
		failure:        plain,
		running:        plain,
		info:           plain,
		muted:          plain,
		selection:      plain,
		header:         plain,
		chip:           plain,
		selBand:        plain,
		selBandFocused: plain,
		accent:         plain,
		accentDim:      plain,
		accentError:    plain,
		ellipsis:       plain,
		pathColor:      plain,
	}
}

// themeFor returns the appropriate token set given the noColor flag.
func themeFor(noColor bool) tokens {
	if noColor {
		return noColorTheme()
	}
	return defaultTheme()
}
