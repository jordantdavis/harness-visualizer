// Package client implements the hook-forwarder CLI critical path. It reads the
// Claude Code hook payload from stdin, enriches it into a canonical Event, and
// POSTs it to the local daemon — all under a hard time budget and with a
// guarantee of always exiting 0 so the hook never blocks or breaks Claude Code.
package client

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"syscall"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/paths"
)

const (
	stdinLimit  = 10 << 20 // 10 MB — safety cap on hook payload size
	totalBudget = 100 * time.Millisecond
	postBudget  = 50 * time.Millisecond
	defaultPort = 7842
)

// debug logs only when CCHV_DEBUG is non-empty.
var debug = log.New(io.Discard, "[cchv] ", log.LstdFlags)

func init() {
	if os.Getenv("CCHV_DEBUG") != "" {
		debug.SetOutput(os.Stderr)
	}
}

// Run is the exported entrypoint for `cchv hook` (and the bare default
// invocation). It wires real stdin, stdout, the port-file-resolved daemon
// address, and the real daemon auto-spawn function, then delegates to run.
// It always returns 0 — a panic in run is caught here before it can surface
// as a hook failure.
func Run(args []string) (exitCode int) {
	defer func() {
		if r := recover(); r != nil {
			debug.Printf("recovered panic: %v", r)
			exitCode = 0
		}
	}()

	return run(os.Stdin, resolveAddr(), io.Discard, spawnDaemon)
}

// run is the testable core: read stdin (bounded), parse + enrich the event,
// POST to addr, and return an exit code (always 0). spawnFn is called when
// the POST fails with a connection-refused error, allowing tests to inject a
// no-op instead of forking a real process.
//
// Contract:
//   - stdin is read up to stdinLimit bytes; excess is discarded.
//   - The whole operation is bounded by totalBudget via context.WithTimeout.
//   - All errors are swallowed; nothing is written to stdout.
//   - spawnFn is called at most once, synchronously, before returning.
func run(stdin io.Reader, addr string, stdout io.Writer, spawnFn func()) int {
	ctx, cancel := context.WithTimeout(context.Background(), totalBudget)
	defer cancel()

	raw, err := io.ReadAll(io.LimitReader(stdin, stdinLimit))
	if err != nil {
		debug.Printf("read stdin: %v", err)
		return 0
	}

	ev, err := event.Parse(raw)
	if err != nil {
		// Non-JSON payload: send a best-effort event with raw bytes preserved.
		debug.Printf("event.Parse: %v — sending best-effort event", err)
		ev = &event.Event{Raw: json.RawMessage(raw)}
	}

	ev.ID = newID()
	ev.CapturedAt = time.Now()
	// Seq is intentionally left zero; the daemon assigns it.

	body, err := json.Marshal(ev)
	if err != nil {
		debug.Printf("json.Marshal: %v", err)
		return 0
	}

	postCtx, postCancel := context.WithTimeout(ctx, postBudget)
	defer postCancel()

	url := "http://" + addr + "/events"
	req, err := http.NewRequestWithContext(postCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		debug.Printf("http.NewRequest: %v", err)
		return 0
	}
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Transport: &http.Transport{
			// Dial 127.0.0.1 directly — no DNS, no localhost resolver edge cases.
			DisableKeepAlives: true,
		},
	}

	resp, err := client.Do(req)
	if err != nil {
		if isConnRefused(err) {
			debug.Printf("connection refused — spawning daemon")
			spawnFn()
		} else {
			debug.Printf("POST /events: %v", err)
		}
		return 0
	}
	resp.Body.Close()
	debug.Printf("POST /events → %d", resp.StatusCode)

	return 0
}

// resolveAddr reads the port file and returns "127.0.0.1:<port>". Falls back
// to the default port when the file is missing, empty, or unreadable.
func resolveAddr() string {
	if pf, err := paths.PortFile(); err == nil {
		if data, err := os.ReadFile(pf); err == nil {
			if port := strings.TrimSpace(string(data)); port != "" {
				return "127.0.0.1:" + port
			}
		}
	}
	return fmt.Sprintf("127.0.0.1:%d", defaultPort)
}

// newID returns a 16-byte random hex string used as the event ID.
// Uses crypto/rand — no third-party dependencies required.
func newID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Extremely unlikely; fall back to a timestamp-based ID.
		return fmt.Sprintf("%016x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b[:])
}

// isConnRefused reports whether err represents a connection-refused OS error,
// which signals the daemon is not running.
func isConnRefused(err error) bool {
	if err == nil {
		return false
	}
	// Fast path: string check covers all platforms.
	if strings.Contains(err.Error(), "connection refused") {
		return true
	}
	// Unwrap to syscall.Errno for platforms where the message may differ.
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.ECONNREFUSED
	}
	return false
}

// spawnDaemon forks `cchv daemon` fully detached: new session (Setsid),
// stdout/stderr redirected to the runtime-dir daemon log, no Wait.
// The current event is dropped; the next hook invocation (ms later) will
// find the daemon running.
func spawnDaemon() {
	exe, err := os.Executable()
	if err != nil {
		debug.Printf("os.Executable: %v", err)
		return
	}

	logPath := daemonLogPath()
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		debug.Printf("open daemon log %s: %v", logPath, err)
		// Fall through — still try to spawn even without a log file.
		logFile = nil
	}

	// Build the command manually so we can set SysProcAttr without importing
	// os/exec at package level in a way that pulls in extra deps.
	procAttr := &os.ProcAttr{
		Files: procFiles(logFile),
		Sys:   &syscall.SysProcAttr{Setsid: true},
	}

	proc, err := os.StartProcess(exe, []string{exe, "daemon"}, procAttr)
	if err != nil {
		debug.Printf("spawn daemon: %v", err)
		if logFile != nil {
			logFile.Close()
		}
		return
	}
	// Fire and forget — do not Wait.
	_ = proc
	debug.Printf("spawned daemon pid=%d", proc.Pid)
	// logFile stays open in the child process via inherited fd; we can close
	// our copy.
	if logFile != nil {
		logFile.Close()
	}
}

// daemonLogPath returns the path to write daemon stdout/stderr.
// Falls back gracefully to /dev/null if paths resolution fails.
func daemonLogPath() string {
	dir, err := paths.RuntimeDir()
	if err != nil {
		return os.DevNull
	}
	return dir + "/daemon.log"
}
