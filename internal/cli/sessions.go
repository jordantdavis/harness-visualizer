package cli

import "github.com/spf13/cobra"

// newSessionsCmd builds the `hv sessions` parent and its `clear` child. The
// parent has no Run, so bare `hv sessions` prints help and exits 0. The clear
// child's flags are defined in cobra and forwarded as a reconstructed arg slice
// (prefixed with the "clear" subcommand) into sessions.Run, preserving that
// package's existing seam and exit codes (0 success, 1 failure, 2 bad flags or
// aborted prompt).
func newSessionsCmd() *cobra.Command {
	sessionsCmd := &cobra.Command{
		Use:           "sessions",
		Short:         "Manage captured sessions",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	var (
		yes    bool
		dryRun bool
	)
	clearCmd := &cobra.Command{
		Use:           "clear",
		Short:         "Delete all captured session JSONL files",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fwd := []string{"clear"}
			if yes {
				fwd = append(fwd, "--yes")
			}
			if dryRun {
				fwd = append(fwd, "--dry-run")
			}
			return codeErr(sessionsRun(fwd))
		},
	}
	clearCmd.Flags().BoolVarP(&yes, "yes", "y", false, "skip the confirmation prompt")
	clearCmd.Flags().BoolVar(&dryRun, "dry-run", false, "list files and total size; delete nothing")

	sessionsCmd.AddCommand(clearCmd)
	return sessionsCmd
}
