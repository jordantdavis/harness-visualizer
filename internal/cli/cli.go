// Package cli owns all Cobra wiring for the hv command tree. The four role
// packages (client, daemon, serve, sessions) keep their Run(args []string) int
// shape as a stable, testable seam; this package builds the cobra.Command tree
// on top and translates between cobra's error-based control flow and our
// exit-code contract.
//
// The hook critical path is protected by an outer layer here: cli.Execute wraps
// the whole cobra Execute in panic recovery + always-exit-0, but only when the
// invocation is `hv hook`. That keeps the per-event hot path as safe as the
// hand-rolled dispatch it replaces, while letting normal commands surface real
// exit codes.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// Seam variables: the role-package entrypoints, injectable so cli tests can
// exercise the command tree (flag wiring, exit-code translation, hook safety)
// without real side effects (network POSTs, daemon spawns, file deletion).
// Production wiring points them at the real implementations in root.go's init.
var (
	hookRun     func(args []string) int
	daemonRun   func(args []string) int
	serveRun    func(args []string) int
	sessionsRun func(args []string) int
)

// exitError carries a specific process exit code out through cobra's RunE,
// which only knows about (error != nil). runRoot unwraps it back into the code.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// codeErr converts a role package's int exit code into the error a RunE must
// return: nil for success (0), an exitError carrying the code otherwise.
func codeErr(code int) error {
	if code == 0 {
		return nil
	}
	return &exitError{code: code}
}

// Execute is the process entrypoint, called from main. It builds the root
// command, runs it against os.Args, and returns the process exit code.
func Execute() int {
	args := os.Args[1:]
	return runRoot(newRootCmd(), args, isHookInvocation(args))
}

// isHookInvocation reports whether args select the hook subcommand. The hook
// path gets the always-exit-0 + panic-recover treatment; nothing else does.
func isHookInvocation(args []string) bool {
	return len(args) >= 1 && args[0] == "hook"
}

// runRoot executes root against args and maps cobra's result to an exit code.
// It is shared by Execute and the cli tests so both go through identical
// exit-code and hook-safety logic.
//
//   - isHook=true: any panic or error is swallowed and the code forced to 0,
//     preserving the "hook never fails" contract even against cobra-internal
//     failures (flag parsing, init panics) that occur before client.Run.
//   - isHook=false: an exitError's carried code is returned verbatim; any other
//     cobra error (unknown command, bad flag) maps to 2, matching the historical
//     "unknown command exits 2" behavior.
func runRoot(root *cobra.Command, args []string, isHook bool) (code int) {
	if isHook {
		defer func() {
			_ = recover()
			code = 0
		}()
	}

	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		if isHook {
			return 0
		}
		var ee *exitError
		if errors.As(err, &ee) {
			// Role package already emitted its own diagnostics; just carry the code.
			return ee.code
		}
		// Genuine cobra error (unknown command/flag). SilenceErrors keeps the
		// hook path silent, so surface it ourselves for normal commands.
		fmt.Fprintln(root.ErrOrStderr(), "hv: "+err.Error())
		return 2
	}
	return code
}
