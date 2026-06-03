// Package daemon (internal test) drives the start/stop/restart/status verbs
// against injected fakes — no real forking, signaling, or HTTP — mirroring the
// client_test / sessions_test style.
package daemon

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// fixture builds a lifecycle wired to controllable fakes plus a temp runtime
// dir holding the pid/port files. Callers tweak the fake funcs before invoking
// a verb.
type fixture struct {
	lc       *lifecycle
	out      *bytes.Buffer
	errw     *bytes.Buffer
	pidPath  string
	portPath string

	// Controllable state.
	healthyVal bool
	aliveVal   bool
	signals    []syscall.Signal
	spawned    int
	servePort  int
	serveCode  int
	serveCalls int
}

// newFixture wires a lifecycle to fakes. A port file recording port 7842 is
// written by default so recordedAddr resolves to an address; tests that model
// a never-started or stopped daemon call removePort.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	f := &fixture{
		out:       &bytes.Buffer{},
		errw:      &bytes.Buffer{},
		pidPath:   filepath.Join(dir, "daemon.pid"),
		portPath:  filepath.Join(dir, "daemon.port"),
		serveCode: 0,
	}
	f.lc = &lifecycle{
		pidFile:       func() (string, error) { return f.pidPath, nil },
		portFile:      func() (string, error) { return f.portPath, nil },
		healthy:       func(string) bool { return f.healthyVal },
		spawn:         func() { f.spawned++ },
		signal:        func(pid int, sig syscall.Signal) error { f.signals = append(f.signals, sig); return nil },
		alive:         func(int) bool { return f.aliveVal },
		serve:         func(port int) int { f.serveCalls++; f.servePort = port; return f.serveCode },
		out:           f.out,
		errw:          f.errw,
		pollInterval:  time.Millisecond,
		stopTimeout:   20 * time.Millisecond,
		startDeadline: 20 * time.Millisecond,
	}
	f.writePort(t, "7842")
	return f
}

// writePid drops a pidfile with the given pid.
func (f *fixture) writePid(t *testing.T, pid int) {
	t.Helper()
	if err := os.WriteFile(f.pidPath, []byte(strconv.Itoa(pid)), 0o644); err != nil {
		t.Fatalf("write pidfile: %v", err)
	}
}

// writePort drops a portfile, so recordedAddr resolves to 127.0.0.1:<port>.
func (f *fixture) writePort(t *testing.T, port string) {
	t.Helper()
	if err := os.WriteFile(f.portPath, []byte(port), 0o644); err != nil {
		t.Fatalf("write portfile: %v", err)
	}
}

// removePort deletes the portfile, modeling a daemon that never started or was
// cleanly stopped (recordedAddr then returns ok=false).
func (f *fixture) removePort(t *testing.T) {
	t.Helper()
	if err := os.Remove(f.portPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove portfile: %v", err)
	}
}

// --- dispatch ---

func TestDispatchBarePrintsUsageNonZero(t *testing.T) {
	f := newFixture(t)
	if code := f.lc.dispatch(nil); code != 2 {
		t.Fatalf("bare daemon exit = %d, want 2", code)
	}
	if !strings.Contains(f.errw.String(), "usage: hv daemon") {
		t.Errorf("bare daemon missing usage on stderr: %q", f.errw.String())
	}
}

func TestDispatchHelpExitsZero(t *testing.T) {
	f := newFixture(t)
	if code := f.lc.dispatch([]string{"--help"}); code != 0 {
		t.Fatalf("daemon --help exit = %d, want 0", code)
	}
	if !strings.Contains(f.out.String(), "usage: hv daemon") {
		t.Errorf("daemon --help missing usage on stdout: %q", f.out.String())
	}
}

func TestDispatchUnknownVerbExitsTwo(t *testing.T) {
	f := newFixture(t)
	if code := f.lc.dispatch([]string{"bogus"}); code != 2 {
		t.Fatalf("daemon bogus exit = %d, want 2", code)
	}
	if !strings.Contains(f.errw.String(), `unknown command "bogus"`) {
		t.Errorf("missing unknown-command message: %q", f.errw.String())
	}
}

// --- start ---

func TestStartRefusesWhenHealthy(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	code := f.lc.dispatch([]string{"start"})
	if code != 1 {
		t.Fatalf("start (already healthy) exit = %d, want 1", code)
	}
	if f.serveCalls != 0 {
		t.Error("start must not invoke serve when a daemon is already healthy")
	}
	if !strings.Contains(f.out.String(), "already running") {
		t.Errorf("missing 'already running' message: %q", f.out.String())
	}
}

func TestStartServesWhenNotHealthy(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.serveCode = 0
	code := f.lc.dispatch([]string{"start", "--port=9999"})
	if code != 0 {
		t.Fatalf("start exit = %d, want 0", code)
	}
	if f.serveCalls != 1 {
		t.Fatalf("serve called %d times, want 1", f.serveCalls)
	}
	if f.servePort != 9999 {
		t.Errorf("serve port = %d, want 9999", f.servePort)
	}
}

func TestStartForegroundFlagAccepted(t *testing.T) {
	f := newFixture(t)
	if code := f.lc.dispatch([]string{"start", "--foreground=true", "--port=7842"}); code != 0 {
		t.Fatalf("start --foreground exit = %d, want 0", code)
	}
	if f.serveCalls != 1 {
		t.Errorf("serve not called; --foreground may have broken parsing")
	}
}

// TestStartRefusesWhenExactPortHealthy covers the case with no port file but a
// foreign daemon already on the requested port: the exact-port check refuses.
func TestStartRefusesWhenExactPortHealthy(t *testing.T) {
	f := newFixture(t)
	f.removePort(t)     // no recorded instance
	f.healthyVal = true // something answers on the bind port
	code := f.lc.dispatch([]string{"start", "--port=7842"})
	if code != 1 {
		t.Fatalf("start (exact port busy) exit = %d, want 1", code)
	}
	if f.serveCalls != 0 {
		t.Error("start must not serve when the requested port is already healthy")
	}
}

// TestStartEphemeralSkipsRefuse verifies --port 0 never triggers the refuse
// path even if some daemon is healthy elsewhere: an ephemeral port can't
// conflict, so we always serve.
func TestStartEphemeralSkipsRefuse(t *testing.T) {
	f := newFixture(t)
	f.removePort(t)     // no recorded instance
	f.healthyVal = true // a foreign daemon exists, but on a fixed port
	code := f.lc.dispatch([]string{"start", "--port=0"})
	if code != 0 {
		t.Fatalf("start --port=0 exit = %d, want 0", code)
	}
	if f.serveCalls != 1 || f.servePort != 0 {
		t.Errorf("serve calls=%d port=%d, want 1 and 0", f.serveCalls, f.servePort)
	}
}

func TestStartBadFlagExitsOne(t *testing.T) {
	f := newFixture(t)
	if code := f.lc.dispatch([]string{"start", "--nope"}); code != 1 {
		t.Fatalf("start --nope exit = %d, want 1", code)
	}
	if f.serveCalls != 0 {
		t.Error("serve must not run when flag parsing fails")
	}
}

// --- status ---

func TestStatusHealthyWithPid(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	f.writePid(t, 4242)
	code := f.lc.dispatch([]string{"status"})
	if code != 0 {
		t.Fatalf("status (healthy) exit = %d, want 0", code)
	}
	got := f.out.String()
	if !strings.Contains(got, "pid 4242") || !strings.Contains(got, "http://127.0.0.1:7842/") {
		t.Errorf("status output missing pid/url: %q", got)
	}
}

func TestStatusHealthyNoPid(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	code := f.lc.dispatch([]string{"status"})
	if code != 0 {
		t.Fatalf("status exit = %d, want 0", code)
	}
	if !strings.Contains(f.out.String(), "daemon running at") {
		t.Errorf("status output unexpected: %q", f.out.String())
	}
}

func TestStatusNotRunning(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.writePid(t, 4242) // stale pidfile must not register as running
	code := f.lc.dispatch([]string{"status"})
	if code != 1 {
		t.Fatalf("status (stale pidfile, not healthy) exit = %d, want 1", code)
	}
	if !strings.Contains(f.out.String(), "daemon not running") {
		t.Errorf("status output unexpected: %q", f.out.String())
	}
}

// TestStatusNoPortFile verifies that a missing port file reports "not running"
// without ever probing the default port (no foreign-daemon false positive).
func TestStatusNoPortFile(t *testing.T) {
	f := newFixture(t)
	f.removePort(t)
	f.healthyVal = true // would be a false positive if recordedAddr fell back
	code := f.lc.dispatch([]string{"status"})
	if code != 1 {
		t.Fatalf("status (no port file) exit = %d, want 1", code)
	}
}

// --- stop ---

func TestStopNotRunningRefuses(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.aliveVal = false
	code := f.lc.dispatch([]string{"stop"})
	if code != 1 {
		t.Fatalf("stop (nothing running) exit = %d, want 1", code)
	}
	if !strings.Contains(f.out.String(), "daemon not running") {
		t.Errorf("stop output unexpected: %q", f.out.String())
	}
	if len(f.signals) != 0 {
		t.Errorf("stop must not signal when nothing is running, sent %v", f.signals)
	}
}

func TestStopStalePidfileCleanedAndRefuses(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.aliveVal = false // pid in file is dead
	f.writePid(t, 9999)
	code := f.lc.dispatch([]string{"stop"})
	if code != 1 {
		t.Fatalf("stop (stale pidfile) exit = %d, want 1", code)
	}
	if _, err := os.Stat(f.pidPath); !os.IsNotExist(err) {
		t.Error("stale pidfile should have been removed")
	}
	if len(f.signals) != 0 {
		t.Errorf("stop must not signal a dead pid, sent %v", f.signals)
	}
}

func TestStopGraceful(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	f.aliveVal = true
	f.writePid(t, 4242)
	// SIGTERM makes the process exit and stop answering health.
	f.lc.signal = func(pid int, sig syscall.Signal) error {
		f.signals = append(f.signals, sig)
		if sig == syscall.SIGTERM {
			f.aliveVal = false
			f.healthyVal = false
		}
		return nil
	}
	code := f.lc.dispatch([]string{"stop"})
	if code != 0 {
		t.Fatalf("stop (graceful) exit = %d, want 0", code)
	}
	if len(f.signals) != 1 || f.signals[0] != syscall.SIGTERM {
		t.Errorf("signals = %v, want [SIGTERM]", f.signals)
	}
	if !strings.Contains(f.out.String(), "daemon stopped") {
		t.Errorf("stop output unexpected: %q", f.out.String())
	}
	if _, err := os.Stat(f.pidPath); !os.IsNotExist(err) {
		t.Error("pidfile should be removed after stop")
	}
}

func TestStopSigkillFallback(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	f.aliveVal = true // never dies on SIGTERM
	f.writePid(t, 4242)
	code := f.lc.dispatch([]string{"stop"})
	if code != 0 {
		t.Fatalf("stop (sigkill fallback) exit = %d, want 0", code)
	}
	if len(f.signals) != 2 || f.signals[0] != syscall.SIGTERM || f.signals[1] != syscall.SIGKILL {
		t.Errorf("signals = %v, want [SIGTERM SIGKILL]", f.signals)
	}
	if !strings.Contains(f.out.String(), "forced") {
		t.Errorf("expected 'forced' in output: %q", f.out.String())
	}
}

// --- restart ---

func TestRestartFromStoppedSpawnsAndWaitsHealthy(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.aliveVal = false
	// Spawning the new daemon writes its port file and becomes healthy.
	f.lc.spawn = func() { f.spawned++; f.writePort(t, "7842"); f.healthyVal = true }
	code := f.lc.dispatch([]string{"restart"})
	if code != 0 {
		t.Fatalf("restart exit = %d, want 0", code)
	}
	if f.spawned != 1 {
		t.Errorf("spawn called %d times, want 1", f.spawned)
	}
	if !strings.Contains(f.out.String(), "daemon restarted at") {
		t.Errorf("restart output unexpected: %q", f.out.String())
	}
}

func TestRestartFailsWhenNeverHealthy(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = false
	f.aliveVal = false
	f.lc.spawn = func() { f.spawned++ } // stays unhealthy
	code := f.lc.dispatch([]string{"restart"})
	if code != 1 {
		t.Fatalf("restart (never healthy) exit = %d, want 1", code)
	}
	if f.spawned != 1 {
		t.Errorf("spawn called %d times, want 1", f.spawned)
	}
	if !strings.Contains(f.errw.String(), "did not become healthy") {
		t.Errorf("missing failure message: %q", f.errw.String())
	}
}

func TestRestartStopsRunningDaemonFirst(t *testing.T) {
	f := newFixture(t)
	f.healthyVal = true
	f.aliveVal = true
	f.writePid(t, 4242)
	f.lc.signal = func(pid int, sig syscall.Signal) error {
		f.signals = append(f.signals, sig)
		if sig == syscall.SIGTERM {
			f.aliveVal = false
			f.healthyVal = false
		}
		return nil
	}
	f.lc.spawn = func() { f.spawned++; f.writePort(t, "7842"); f.healthyVal = true }
	code := f.lc.dispatch([]string{"restart"})
	if code != 0 {
		t.Fatalf("restart (running) exit = %d, want 0", code)
	}
	if len(f.signals) == 0 || f.signals[0] != syscall.SIGTERM {
		t.Errorf("restart should SIGTERM the old daemon first, signals = %v", f.signals)
	}
	if f.spawned != 1 {
		t.Errorf("spawn called %d times, want 1", f.spawned)
	}
}
