package cli

import (
	"github.com/spf13/cobra"

	"jordandavis.dev/harness-visualizer/internal/client"
	"jordandavis.dev/harness-visualizer/internal/daemon"
	"jordandavis.dev/harness-visualizer/internal/serve"
	"jordandavis.dev/harness-visualizer/internal/sessions"
)

func init() {
	// Default the seams to the real role-package entrypoints. Tests reassign
	// these before building the root command to inject fakes.
	hookRun = client.Run
	daemonRun = daemon.Run
	serveRun = serve.Run
	sessionsRun = sessions.Run
}

// newRootCmd builds a fresh root command with all subcommands attached. A fresh
// tree per call keeps tests isolated (cobra commands carry mutable parse state).
//
// SilenceUsage/SilenceErrors are on so cobra never prints usage or the error
// string on failure — our exit-code contract owns that. Bare `hv` still prints
// help and exits 0 (cobra runs HelpFunc when a parent command has no Run).
func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hv",
		Short: "Claude Code Harness Visualizer",
		Long: "hv — Claude Code Harness Visualizer\n\n" +
			"Capture every Claude Code hook event and see what the harness actually\n" +
			"does: which hooks fire, in what order, with what data, and how long\n" +
			"tools take. Config is intent; the captured event stream is reality.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newHookCmd(),
		newDaemonCmd(),
		newServeCmd(),
		newSessionsCmd(),
		newVersionCmd(),
	)

	return root
}
