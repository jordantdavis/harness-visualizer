// Package tui — run.go
//
// Run is the entrypoint for `cchv tui`. It parses flags, respects NO_COLOR,
// and either starts the bubbletea UI or runs plain line-per-event mode.
package tui

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run is the entrypoint for `cchv tui`. args may include:
//
//	--plain         line-per-event mode (no bubbletea UI; pipe/screen-reader safe)
//	--no-animation  disable animated indicators (static heartbeat, no blinking)
//
// NO_COLOR env var is also respected: suppresses all ANSI color and implies
// --no-animation.
//
// Returns 0 on clean exit, 1 on error.
func Run(args []string) int {
	fs := flag.NewFlagSet("cchv tui", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	plain := fs.Bool("plain", false, "line-per-event mode (screen-reader / pipe / bug-report friendly)")
	noAnim := fs.Bool("no-animation", false, "disable animated indicators (reduced motion)")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return 0
		}
		return 2
	}

	noColor := os.Getenv("NO_COLOR") != ""
	reducedMotion := *noAnim || noColor

	c := NewHTTPClient()

	if *plain {
		return runPlainMain(c)
	}

	m := newModel(c, noColor, reducedMotion)

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
