package tui

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run is the entrypoint for `cchv tui`. args may include flags in future
// phases; currently unused. Returns 0 on clean exit, 1 on error.
//
// Run auto-discovers the daemon port from paths.PortFile and connects via HTTP.
// It respects the NO_COLOR environment variable for color-free rendering.
func Run(args []string) int {
	noColor := os.Getenv("NO_COLOR") != ""

	client := NewHTTPClient()
	m := newModel(client, noColor)

	opts := []tea.ProgramOption{tea.WithAltScreen()}
	if noColor {
		opts = append(opts, tea.WithoutRenderer())
	}

	p := tea.NewProgram(m, opts...)
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "cchv tui: %v\n", err)
		return 1
	}
	return 0
}
