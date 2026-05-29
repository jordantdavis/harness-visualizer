// Command cchv is the Claude Code Harness Visualizer. The single binary plays
// three roles, selected by subcommand:
//
//	cchv hook     forward one hook payload (stdin) to the daemon; also the
//	              bare/default invocation Claude Code calls per hook
//	cchv daemon   long-running HTTP server + SSE hub (--foreground for dev)
//	cchv tui      the bubbletea reality-viewer
//
// Each role lives in its own package exposing Run(args []string) int; main is
// pure dispatch so the hook critical path stays tiny.
package main

import (
	"fmt"
	"os"

	"jordandavis.dev/cc-harness-visualizer/internal/client"
	"jordandavis.dev/cc-harness-visualizer/internal/daemon"
	"jordandavis.dev/cc-harness-visualizer/internal/tui"
)

func main() {
	// Bare invocation (no subcommand) is the hook forwarder: Claude Code
	// runs `cchv` per hook, and the hook path must never fail.
	if len(os.Args) < 2 {
		os.Exit(client.Run(nil))
	}

	cmd, rest := os.Args[1], os.Args[2:]
	switch cmd {
	case "hook":
		os.Exit(client.Run(rest))
	case "daemon":
		os.Exit(daemon.Run(rest))
	case "tui":
		os.Exit(tui.Run(rest))
	case "-h", "--help", "help":
		usage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "cchv: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `cchv — Claude Code Harness Visualizer

usage:
  cchv hook       forward a hook payload from stdin to the daemon (default)
  cchv daemon     run the capture daemon (--foreground, --port)
  cchv tui        open the terminal viewer
`)
}
