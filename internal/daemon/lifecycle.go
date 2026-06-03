package daemon

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"

	"jordandavis.dev/harness-visualizer/internal/client"
	"jordandavis.dev/harness-visualizer/internal/paths"
)

// daemonUsage is printed by bare `hv daemon`, `hv daemon --help`, and on an
// unknown verb. It documents the start/restart asymmetry called out in the
// design: start IS the server (blocks); restart bounces it and returns.
const daemonUsage = `usage: hv daemon <command>

Manage the local capture daemon (Unix only).

Commands:
  start      run the HTTP server in the foreground (this is the process the
             hook auto-spawn runs detached). Hard-refuses (exit 1) if a healthy
             daemon already owns the port. Flags: --port (default 7842),
             --foreground (accepted, ignored).
  stop       SIGTERM the running daemon and wait for graceful exit (SIGKILL
             fallback). Hard-refuses (exit 1) if nothing is running.
  restart    stop the running daemon (tolerating "not running"), then spawn a
             fresh detached daemon and wait for it to become healthy. Unlike
             start, this returns to the shell.
  status     report running/stopped plus pid, port, and URL. Exit 0 if healthy,
             1 if not.
`

// lifecycle bundles every side-effecting dependency the daemon control verbs
// touch, so the full start/stop/restart/status logic can be driven against
// fakes + temp dirs with no real forking, signaling, or HTTP. Production wiring
// is in newLifecycle.
type lifecycle struct {
	pidFile  func() (string, error) // path to daemon.pid
	portFile func() (string, error) // path to daemon.port
	healthy  func(addr string) bool // authoritative liveness (GET /healthz)
	spawn    func()                 // fork a detached `hv daemon start`
	signal   func(pid int, sig syscall.Signal) error
	alive    func(pid int) bool // is pid a live process?
	serve    func(port int) int  // blocking foreground server (start's body)

	out  io.Writer
	errw io.Writer

	pollInterval  time.Duration // gap between liveness polls
	stopTimeout   time.Duration // SIGTERM grace before SIGKILL
	startDeadline time.Duration // how long restart waits for health
}

// newLifecycle wires the lifecycle to the real primitives. The health-check and
// spawn helpers are reused from internal/client so the lifecycle commands and
// the hook forwarder agree on what "healthy" means and how a daemon is
// launched. Address resolution, however, is port-file-only here (see
// recordedAddr) rather than client.ResolveAddr — the lifecycle must not fall
// back to probing the default port.
func newLifecycle() *lifecycle {
	return &lifecycle{
		pidFile:       paths.PidFile,
		portFile:      paths.PortFile,
		healthy:       client.DaemonHealthy,
		spawn:         client.SpawnDaemon,
		signal:        sendSignal,
		alive:         processAlive,
		serve:         serveForeground,
		out:           os.Stdout,
		errw:          os.Stderr,
		pollInterval:  50 * time.Millisecond,
		stopTimeout:   5 * time.Second,
		startDeadline: 3 * time.Second,
	}
}

// dispatch switches on the first arg (the verb) and delegates to the matching
// command, mirroring the two-level internal/sessions style. Bare `hv daemon`
// and unknown verbs print usage and exit non-zero; --help exits 0.
func (l *lifecycle) dispatch(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(l.errw, daemonUsage)
		return 2
	}
	verb, rest := args[0], args[1:]
	switch verb {
	case "--help", "-h", "help":
		fmt.Fprint(l.out, daemonUsage)
		return 0
	case "start":
		return l.start(rest)
	case "stop":
		return l.stop(false)
	case "restart":
		return l.restart()
	case "status":
		return l.status()
	default:
		fmt.Fprintf(l.errw, "daemon: unknown command %q\n\n%s", verb, daemonUsage)
		return 2
	}
}

// start runs the foreground server, refusing first if a healthy daemon already
// owns the port. --foreground is accepted for back-compat and ignored (detach
// is always the caller's job).
//
// Two refuse checks, both grounded in real ports (never the default-port
// fallback): the recorded daemon (port file), which catches an instance on a
// non-default port; and the exact port we are about to bind (skipped for the
// ephemeral :0 case, which can never conflict). If neither is healthy we serve;
// the bind itself is the final arbiter and fails hard with no :0 fallback.
func (l *lifecycle) start(args []string) int {
	fs := flag.NewFlagSet("daemon start", flag.ContinueOnError)
	fs.SetOutput(l.errw)
	port := fs.Int("port", defaultPort, "preferred listen port")
	foreground := fs.Bool("foreground", true, "run in foreground (accepted; ignored)")
	if err := fs.Parse(args); err != nil {
		return 1
	}
	_ = foreground // detach is the caller's responsibility, never ours

	if addr, ok := l.recordedAddr(); ok && l.healthy(addr) {
		fmt.Fprintf(l.out, "daemon already running at %s\n", daemonURL(addr))
		return 1
	}
	if *port != 0 {
		addr := fmt.Sprintf("127.0.0.1:%d", *port)
		if l.healthy(addr) {
			fmt.Fprintf(l.out, "daemon already running at %s\n", daemonURL(addr))
			return 1
		}
	}
	return l.serve(*port)
}

// status reports liveness using /healthz against the recorded address as the
// source of truth (a stale pidfile alone never counts as "running"; a missing
// port file means "not running"). Exit 0 if healthy, 1 if not.
func (l *lifecycle) status() int {
	addr, ok := l.recordedAddr()
	if !ok || !l.healthy(addr) {
		fmt.Fprintln(l.out, "daemon not running")
		return 1
	}
	if pid, havePid := l.readPid(); havePid {
		fmt.Fprintf(l.out, "daemon running (pid %d) at %s\n", pid, daemonURL(addr))
	} else {
		fmt.Fprintf(l.out, "daemon running at %s\n", daemonURL(addr))
	}
	return 0
}

// stop signals the running daemon to exit. In tolerant mode (used by restart) a
// missing daemon is success; otherwise it hard-refuses with exit 1. A stale
// pidfile (pid dead, nothing healthy) is cleaned up rather than signaled, and
// we never SIGKILL a pid we can't confirm is alive.
func (l *lifecycle) stop(tolerant bool) int {
	addr, haveAddr := l.recordedAddr()
	healthy := haveAddr && l.healthy(addr)
	pid, havePid := l.readPid()
	pidAlive := havePid && l.alive(pid)

	// Nothing controllable: no live pid and nothing answering health.
	if !pidAlive && !healthy {
		l.removeRuntimeFiles() // clean a stale pidfile/portfile if present
		if tolerant {
			return 0
		}
		fmt.Fprintln(l.out, "daemon not running")
		return 1
	}

	// Healthy but no live pid we can signal — refuse rather than guess.
	if !pidAlive {
		if tolerant {
			return 0
		}
		fmt.Fprintln(l.errw, "daemon: responding but pidfile is missing or stale; cannot signal")
		return 1
	}

	// Graceful stop: SIGTERM, then poll for the process to exit.
	_ = l.signal(pid, syscall.SIGTERM)
	deadline := time.Now().Add(l.stopTimeout)
	for time.Now().Before(deadline) {
		if !l.alive(pid) {
			l.removeRuntimeFiles()
			fmt.Fprintln(l.out, "daemon stopped")
			return 0
		}
		time.Sleep(l.pollInterval)
	}

	// Grace expired — force kill and clean up the runtime files ourselves
	// (the daemon never got to remove them).
	_ = l.signal(pid, syscall.SIGKILL)
	l.removeRuntimeFiles()
	fmt.Fprintln(l.out, "daemon stopped (forced)")
	return 0
}

// restart bounces the daemon: stop it tolerantly, spawn a fresh detached one,
// then poll /healthz until healthy. Unlike start it returns to the shell.
func (l *lifecycle) restart() int {
	if code := l.stop(true); code != 0 {
		// stop is invoked tolerantly here, so it only fails on a genuine
		// non-tolerant condition — defensive, should not happen.
		return code
	}

	l.spawn()

	deadline := time.Now().Add(l.startDeadline)
	for time.Now().Before(deadline) {
		time.Sleep(l.pollInterval)
		if addr, ok := l.recordedAddr(); ok && l.healthy(addr) {
			fmt.Fprintf(l.out, "daemon restarted at %s\n", daemonURL(addr))
			return 0
		}
	}
	fmt.Fprintln(l.errw, "daemon: restarted daemon did not become healthy")
	return 1
}

// recordedAddr returns the address of the daemon recorded in the port file and
// whether a port file actually exists. Unlike client.ResolveAddr it does NOT
// fall back to the default port: for lifecycle decisions, "no port file" means
// "no daemon recorded for this data dir", not "go probe 127.0.0.1:7842" (which
// could hit an unrelated daemon and produce false positives).
func (l *lifecycle) recordedAddr() (string, bool) {
	pf, err := l.portFile()
	if err != nil {
		return "", false
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return "", false
	}
	port := strings.TrimSpace(string(data))
	if port == "" {
		return "", false
	}
	return "127.0.0.1:" + port, true
}

// readPid reads and parses the pidfile. Returns ok=false when the file is
// absent, empty, unreadable, or not a positive integer.
func (l *lifecycle) readPid() (int, bool) {
	pf, err := l.pidFile()
	if err != nil {
		return 0, false
	}
	data, err := os.ReadFile(pf)
	if err != nil {
		return 0, false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return 0, false
	}
	return pid, true
}

// removeRuntimeFiles deletes the pid and port files, ignoring errors. Used to
// clear stale files when the daemon could not (crashed, or we SIGKILLed it).
func (l *lifecycle) removeRuntimeFiles() {
	if pf, err := l.pidFile(); err == nil {
		_ = os.Remove(pf)
	}
	if pf, err := l.portFile(); err == nil {
		_ = os.Remove(pf)
	}
}

// daemonURL renders the browser/health URL for a "host:port" address.
func daemonURL(addr string) string {
	return "http://" + addr + "/"
}
