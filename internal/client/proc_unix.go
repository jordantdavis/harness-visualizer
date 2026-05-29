//go:build !windows

package client

import "os"

// procFiles returns the []*os.File slice for os.ProcAttr.Files on Unix.
// stdin is /dev/null; stdout and stderr go to logFile (or /dev/null if nil).
func procFiles(logFile *os.File) []*os.File {
	devnull, _ := os.Open(os.DevNull)
	out := logFile
	if out == nil {
		out = devnull
	}
	return []*os.File{devnull, out, out}
}
