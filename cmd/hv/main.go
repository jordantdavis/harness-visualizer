// Command hv is the Claude Code Harness Visualizer. The single binary plays
// three roles, selected by subcommand:
//
//	hv hook     forward one hook payload (stdin) to the daemon; also the
//	              bare/default invocation Claude Code calls per hook
//	hv daemon   long-running HTTP server + SSE hub (--foreground for dev)
//	hv tui      the bubbletea reality-viewer
//
// Each role lives in its own package exposing Run(args []string) int; main is
// pure dispatch so the hook critical path stays tiny.
package main

import (
	"fmt"
	"os"

	"jordandavis.dev/harness-visualizer/internal/client"
	"jordandavis.dev/harness-visualizer/internal/daemon"
	"jordandavis.dev/harness-visualizer/internal/serve"
	"jordandavis.dev/harness-visualizer/internal/tui"
)

func main() {
	// Bare invocation (no subcommand) is the hook forwarder: Claude Code
	// runs `hv` per hook, and the hook path must never fail.
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
	case "serve":
		os.Exit(serve.Run(rest))
	case "-h", "--help", "help":
		usage(os.Stdout)
		os.Exit(0)
	default:
		fmt.Fprintf(os.Stderr, "hv: unknown command %q\n\n", cmd)
		usage(os.Stderr)
		os.Exit(2)
	}
}

func usage(w *os.File) {
	fmt.Fprint(w, `hv — Claude Code Harness Visualizer

usage:
  hv hook       forward a hook payload from stdin to the daemon (default)
  hv daemon     run the capture daemon (--foreground, --port)
  hv tui        open the terminal viewer
  hv serve      ensure the daemon is up and open the web UI in a browser
`)
}
