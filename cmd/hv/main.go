// Command hv is the Claude Code Harness Visualizer. The single binary plays
// several roles, selected by subcommand:
//
//	hv hook          forward one hook payload (stdin) to the daemon
//	hv daemon        manage the capture daemon: start | stop | restart | status
//	hv serve         ensure the daemon is up and open the web UI in a browser
//	hv sessions      manage captured sessions (see: hv sessions clear)
//	hv version       print version information
//	hv completion    generate shell completion scripts
//
// All CLI wiring lives in internal/cli (Cobra); main stays pure dispatch so the
// hook critical path's safety contract is owned in one place.
package main

import (
	"os"

	"jordandavis.dev/harness-visualizer/internal/cli"
)

func main() { os.Exit(cli.Execute()) }
