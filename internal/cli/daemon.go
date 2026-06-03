package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// defaultPort mirrors daemon's unexported default so cobra can advertise it in
// --help and completions. The daemon package remains the source of truth at
// runtime; this is just the surfaced default.
const defaultPort = 7842

// newDaemonCmd builds the `hv daemon` parent and its lifecycle children
// (start/stop/restart/status). Cobra owns the flag definitions and help text
// for unified completions; each child forwards a reconstructed arg slice
// (prefixed with its verb) into daemonRun, which re-dispatches on the verb.
// This mirrors the two-level `hv sessions` wiring and keeps the daemon
// package's existing Run(args) seam and tests intact.
//
// Bare `hv daemon` (no verb) prints usage and exits non-zero: the parent's
// RunE forwards its args through, so daemonRun sees an empty verb and returns
// exit 2.
func newDaemonCmd() *cobra.Command {
	daemonCmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the capture daemon (start/stop/restart/status)",
		Long: "Manage the local capture daemon (HTTP server + SSE hub).\n\n" +
			"start runs the server in the foreground and is the process the hook\n" +
			"auto-spawn launches detached; restart bounces a running daemon and\n" +
			"returns to the shell. Unix only.",
		SilenceUsage:  true,
		SilenceErrors: true,
		// Bare `hv daemon` (and any non-subcommand args) flows here; forward to
		// daemonRun so the daemon package owns the usage text and non-zero exit.
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(daemonRun(args))
		},
	}

	var port int
	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Run the capture server in the foreground (auto-spawned by hooks)",
		Long: "Run the HTTP capture server in the foreground until SIGINT/SIGTERM.\n\n" +
			"This is the process the hook auto-spawn runs detached. It hard-refuses\n" +
			"(exit 1) if a healthy daemon already owns the port — there is no\n" +
			"ephemeral-port fallback.",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fwd := []string{"start", fmt.Sprintf("--port=%d", port)}
			return codeErr(daemonRun(fwd))
		},
	}
	startCmd.Flags().IntVar(&port, "port", defaultPort, "preferred listen port")

	stopCmd := &cobra.Command{
		Use:           "stop",
		Short:         "Stop the running daemon (SIGTERM, SIGKILL fallback)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(daemonRun([]string{"stop"}))
		},
	}

	restartCmd := &cobra.Command{
		Use:           "restart",
		Short:         "Bounce the daemon and return to the shell",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(daemonRun([]string{"restart"}))
		},
	}

	statusCmd := &cobra.Command{
		Use:           "status",
		Short:         "Report whether the daemon is running (pid, port, url)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(daemonRun([]string{"status"}))
		},
	}

	daemonCmd.AddCommand(startCmd, stopCmd, restartCmd, statusCmd)
	return daemonCmd
}
