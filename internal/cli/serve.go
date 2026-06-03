package cli

import "github.com/spf13/cobra"

// newServeCmd builds `hv serve`: ensure the daemon is up, then open the web UI
// in a browser. No flags — it forwards an empty arg slice to serve.Run.
func newServeCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "serve",
		Short:         "Ensure the daemon is up and open the web UI in a browser",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return codeErr(serveRun(nil))
		},
	}
}
