// Package serve implements `hv serve`: ensure the daemon is running, then
// open the system browser at the daemon's embedded web UI. It is a thin
// launcher, not a second server — the daemon already serves the UI at /.
package serve

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	"jordandavis.dev/harness-visualizer/internal/client"
)

// Run is the CLI entrypoint for `hv serve`. args is currently unused.
func Run(args []string) int {
	_ = args
	return run(client.EnsureDaemon, openBrowser, os.Stdout)
}

// run is the testable core. ensure returns the daemon "host:port"; open is
// invoked with the UI URL. An open failure is non-fatal — the URL is printed
// so the user can open it manually.
func run(ensure func() (string, error), open func(string) error, out io.Writer) int {
	addr, err := ensure()
	if err != nil {
		fmt.Fprintln(os.Stderr, "serve: "+err.Error())
		return 1
	}
	url := "http://" + addr + "/"
	fmt.Fprintf(out, "hv serve: %s\n", url)
	if err := open(url); err != nil {
		fmt.Fprintln(os.Stderr, "serve: open browser: "+err.Error())
	}
	return 0
}

// openBrowser opens url in the system default browser.
func openBrowser(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}
