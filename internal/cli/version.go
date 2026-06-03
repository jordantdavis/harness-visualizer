package cli

import (
	"fmt"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// newVersionCmd builds `hv version`. It reads build metadata via
// runtime/debug.ReadBuildInfo, which works for both `go install` and `go build`
// without ldflags wiring.
func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print version information",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), versionString(debug.ReadBuildInfo))
			return nil
		},
	}
}

// versionString renders the version line, e.g.
//
//	hv 0.0.0-dev (commit abc1234, built 2026-06-02T10:00:00Z)
//
// It appends "+dirty" to the version when built from a modified tree. readInfo
// is injected so tests can drive it deterministically.
func versionString(readInfo func() (*debug.BuildInfo, bool)) string {
	version := "0.0.0-dev"
	commit := "unknown"
	built := "unknown"
	dirty := false

	if info, ok := readInfo(); ok && info != nil {
		if v := info.Main.Version; v != "" && v != "(devel)" {
			version = v
		}
		for _, s := range info.Settings {
			switch s.Key {
			case "vcs.revision":
				commit = shortCommit(s.Value)
			case "vcs.time":
				if s.Value != "" {
					built = s.Value
				}
			case "vcs.modified":
				dirty = s.Value == "true"
			}
		}
	}

	if dirty {
		version += "+dirty"
	}
	return fmt.Sprintf("hv %s (commit %s, built %s)", version, commit, built)
}

// shortCommit truncates a git revision to its 7-character short form.
func shortCommit(rev string) string {
	if rev == "" {
		return "unknown"
	}
	if len(rev) > 7 {
		return rev[:7]
	}
	return rev
}
