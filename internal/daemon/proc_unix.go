//go:build !windows

package daemon

import (
	"errors"
	"os"
	"syscall"
)

// sendSignal delivers sig to pid. It is the production signal func injected
// into the lifecycle; tests substitute a fake to avoid touching real
// processes.
func sendSignal(pid int, sig syscall.Signal) error {
	p, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return p.Signal(sig)
}

// processAlive reports whether pid names a live process we could signal. It
// uses signal 0 (the standard "does this process exist?" probe): a nil error
// means alive; EPERM means it exists but is owned by someone else (still
// alive); anything else (ESRCH, process-done) means gone.
func processAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = p.Signal(syscall.Signal(0))
	if err == nil {
		return true
	}
	return errors.Is(err, syscall.EPERM)
}
