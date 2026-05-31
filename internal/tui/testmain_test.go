package tui

// testmain_test.go — package-level test initialization.
//
// Forces TrueColor rendering for all tui tests so that color-asserting tests
// work deterministically in headless/CI environments (which default to no-color).
// Tests that verify noColor suppression do so via model.noColor=true, not via
// the terminal profile — noColorTheme() produces zero-styled tokens that emit no
// ANSI regardless of the color profile.

import (
	"os"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
)

func TestMain(m *testing.M) {
	// Force TrueColor so lipgloss.Style.Render produces ANSI output in tests.
	lipgloss.SetColorProfile(termenv.TrueColor)
	os.Exit(m.Run())
}
