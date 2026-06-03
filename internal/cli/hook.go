package cli

import (
	"io"

	"github.com/spf13/cobra"
)

// newHookCmd builds the `hv hook` subcommand: the per-event critical path that
// Claude Code runs for every hook. It forwards stdin to the daemon and always
// exits 0, within a ~100ms budget, with panic recovery — guarantees enforced
// inside client.Run and reinforced by the outer recover in runRoot.
//
// DisableFlagParsing keeps cobra's hands off the args entirely: `hv hook
// --anything` reaches client.Run as-is (client.Run ignores args) and cobra
// never errors or prints, identical to the pre-cobra bare dispatch. Out/Err are
// pointed at io.Discard as belt-and-suspenders so nothing can leak to stdout.
func newHookCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "hook",
		Short:              "Forward a hook payload from stdin to the daemon",
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetOut(io.Discard)
			cmd.SetErr(io.Discard)
			return codeErr(hookRun(args))
		},
	}
}
