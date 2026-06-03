package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// defaultPort mirrors daemon's unexported default so cobra can advertise it in
// --help and completions. The daemon package remains the source of truth at
// runtime; this is just the surfaced default.
const defaultPort = 7842

// newDaemonCmd builds `hv daemon`. Cobra owns the flag definitions (for unified
// help + completions); the parsed values are forwarded as a reconstructed arg
// slice into daemon.Run, which re-parses them. This deliberate duplication
// keeps the daemon package's existing Run(args) seam and its tests untouched.
//
// --foreground is preserved purely for back-compat and remains a no-op (detach
// is the hook CLI's job), matching the pre-cobra behavior.
func newDaemonCmd() *cobra.Command {
	var (
		port       int
		foreground bool
	)
	cmd := &cobra.Command{
		Use:           "daemon",
		Short:         "Run the capture daemon (HTTP server + SSE hub)",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			fwd := []string{
				fmt.Sprintf("--port=%d", port),
				fmt.Sprintf("--foreground=%t", foreground),
			}
			return codeErr(daemonRun(fwd))
		},
	}
	cmd.Flags().IntVar(&port, "port", defaultPort, "preferred listen port")
	cmd.Flags().BoolVar(&foreground, "foreground", true, "run in foreground (ignored; kept for back-compat)")
	return cmd
}
