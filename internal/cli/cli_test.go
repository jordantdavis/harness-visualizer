package cli

import (
	"bytes"
	"runtime/debug"
	"strings"
	"testing"
)

// runCLI builds a fresh root command, swaps in fake role-package seams that
// record their args, runs it against args, and returns the exit code plus
// captured stdout/stderr. It exercises the real flag wiring and exit-code
// translation without any real side effects.
func runCLI(t *testing.T, args ...string) (code int, stdout, stderr string, calls *fakeCalls) {
	t.Helper()
	calls = installFakes(t)

	root := newRootCmd()
	var out, errBuf bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errBuf)

	code = runRoot(root, args, isHookInvocation(args))
	return code, out.String(), errBuf.String(), calls
}

// fakeCalls records what each seam received and what it should return.
type fakeCalls struct {
	hookArgs     []string
	hookCalled   bool
	daemonArgs   []string
	daemonCalled bool
	serveArgs    []string
	serveCalled  bool
	sessArgs     []string
	sessCalled   bool

	hookCode, daemonCode, serveCode, sessCode int
}

// installFakes points the package seams at recorders for the duration of the
// test, restoring the real entrypoints afterward.
func installFakes(t *testing.T) *fakeCalls {
	t.Helper()
	c := &fakeCalls{}

	origHook, origDaemon, origServe, origSess := hookRun, daemonRun, serveRun, sessionsRun
	t.Cleanup(func() {
		hookRun, daemonRun, serveRun, sessionsRun = origHook, origDaemon, origServe, origSess
	})

	hookRun = func(args []string) int { c.hookCalled = true; c.hookArgs = args; return c.hookCode }
	daemonRun = func(args []string) int { c.daemonCalled = true; c.daemonArgs = args; return c.daemonCode }
	serveRun = func(args []string) int { c.serveCalled = true; c.serveArgs = args; return c.serveCode }
	sessionsRun = func(args []string) int { c.sessCalled = true; c.sessArgs = args; return c.sessCode }
	return c
}

func TestBarePrintsHelpAndExitsZero(t *testing.T) {
	code, stdout, _, _ := runCLI(t)
	if code != 0 {
		t.Fatalf("bare hv: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Harness Visualizer") {
		t.Fatalf("bare hv stdout missing help banner:\n%s", stdout)
	}
	// Help lists the subcommands.
	for _, want := range []string{"hook", "daemon", "serve", "sessions", "version"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("help missing subcommand %q", want)
		}
	}
}

func TestHelpFlagExitsZero(t *testing.T) {
	code, stdout, _, _ := runCLI(t, "--help")
	if code != 0 {
		t.Fatalf("hv --help: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("hv --help missing usage:\n%s", stdout)
	}
}

func TestVersionPrintsExpectedFormat(t *testing.T) {
	code, stdout, _, _ := runCLI(t, "version")
	if code != 0 {
		t.Fatalf("hv version: exit code = %d, want 0", code)
	}
	if !strings.HasPrefix(stdout, "hv ") {
		t.Fatalf("version output should start with 'hv ', got: %q", stdout)
	}
	if !strings.Contains(stdout, "commit ") || !strings.Contains(stdout, "built ") {
		t.Fatalf("version output missing commit/built fields: %q", stdout)
	}
}

func TestDaemonFlagPropagates(t *testing.T) {
	code, _, _, calls := runCLI(t, "daemon", "--port=8080")
	if code != 0 {
		t.Fatalf("hv daemon --port=8080: exit code = %d, want 0", code)
	}
	if !calls.daemonCalled {
		t.Fatal("daemon.Run was not called")
	}
	joined := strings.Join(calls.daemonArgs, " ")
	if !strings.Contains(joined, "--port=8080") {
		t.Fatalf("daemon args missing port: %v", calls.daemonArgs)
	}
}

func TestDaemonDefaultPort(t *testing.T) {
	_, _, _, calls := runCLI(t, "daemon")
	if !calls.daemonCalled {
		t.Fatal("daemon.Run was not called")
	}
	if !strings.Contains(strings.Join(calls.daemonArgs, " "), "--port=7842") {
		t.Fatalf("daemon default port not forwarded: %v", calls.daemonArgs)
	}
}

func TestDaemonExitCodePropagates(t *testing.T) {
	calls := installFakes(t)
	calls.daemonCode = 1
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if code := runRoot(root, []string{"daemon"}, false); code != 1 {
		t.Fatalf("daemon exit code = %d, want 1 (propagated from daemon.Run)", code)
	}
}

func TestSessionsClearDryRunDelegates(t *testing.T) {
	code, _, _, calls := runCLI(t, "sessions", "clear", "--dry-run")
	if code != 0 {
		t.Fatalf("sessions clear --dry-run: exit code = %d, want 0", code)
	}
	if !calls.sessCalled {
		t.Fatal("sessions.Run was not called")
	}
	want := []string{"clear", "--dry-run"}
	if strings.Join(calls.sessArgs, " ") != strings.Join(want, " ") {
		t.Fatalf("sessions args = %v, want %v", calls.sessArgs, want)
	}
}

func TestSessionsClearYesShortFlag(t *testing.T) {
	_, _, _, calls := runCLI(t, "sessions", "clear", "-y")
	if got := strings.Join(calls.sessArgs, " "); got != "clear --yes" {
		t.Fatalf("sessions args = %q, want %q", got, "clear --yes")
	}
}

func TestSessionsClearExitCodePropagates(t *testing.T) {
	calls := installFakes(t)
	calls.sessCode = 2 // e.g. aborted prompt
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if code := runRoot(root, []string{"sessions", "clear"}, false); code != 2 {
		t.Fatalf("sessions clear exit code = %d, want 2 (propagated)", code)
	}
}

func TestServeNoFlags(t *testing.T) {
	code, _, _, calls := runCLI(t, "serve")
	if code != 0 {
		t.Fatalf("hv serve: exit code = %d, want 0", code)
	}
	if !calls.serveCalled {
		t.Fatal("serve.Run was not called")
	}
}

func TestUnknownSubcommandExitsTwo(t *testing.T) {
	code, _, stderr, _ := runCLI(t, "bogus")
	if code != 2 {
		t.Fatalf("unknown subcommand: exit code = %d, want 2", code)
	}
	if !strings.Contains(stderr, "bogus") {
		t.Fatalf("unknown subcommand should report the command on stderr, got: %q", stderr)
	}
}

func TestUnknownTopLevelFlagExitsNonZero(t *testing.T) {
	code, _, _, _ := runCLI(t, "--bogus-flag")
	if code == 0 {
		t.Fatalf("unknown top-level flag: exit code = 0, want non-zero")
	}
}

// TestHookContractBogusFlag locks in the hook safety contract: a hook
// invocation with a bogus flag still exits 0 and writes nothing to stdout.
func TestHookContractBogusFlag(t *testing.T) {
	code, stdout, stderr, calls := runCLI(t, "hook", "--bogus-flag")
	if code != 0 {
		t.Fatalf("hv hook --bogus-flag: exit code = %d, want 0", code)
	}
	if stdout != "" {
		t.Fatalf("hv hook wrote to stdout: %q", stdout)
	}
	if stderr != "" {
		t.Fatalf("hv hook wrote to stderr: %q", stderr)
	}
	if !calls.hookCalled {
		t.Fatal("hook seam (client.Run) was not reached")
	}
	// DisableFlagParsing means the bogus flag reaches the seam verbatim.
	if strings.Join(calls.hookArgs, " ") != "--bogus-flag" {
		t.Fatalf("hook args = %v, want [--bogus-flag]", calls.hookArgs)
	}
}

// TestHookPanicStillExitsZero verifies the outer recover keeps the hook path at
// exit 0 even if something below cobra panics.
func TestHookPanicStillExitsZero(t *testing.T) {
	installFakes(t)
	hookRun = func(args []string) int { panic("boom") }
	root := newRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if code := runRoot(root, []string{"hook"}, true); code != 0 {
		t.Fatalf("panicking hook: exit code = %d, want 0", code)
	}
}

func TestCompletionCommandAvailable(t *testing.T) {
	code, stdout, _, _ := runCLI(t, "completion", "bash")
	if code != 0 {
		t.Fatalf("hv completion bash: exit code = %d, want 0", code)
	}
	if !strings.Contains(stdout, "bash completion") && !strings.Contains(stdout, "complete ") {
		t.Fatalf("completion output doesn't look like a bash script:\n%s", stdout[:min(200, len(stdout))])
	}
}

func TestVersionStringDirty(t *testing.T) {
	fake := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "(devel)"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "abc1234567890"},
				{Key: "vcs.time", Value: "2026-06-02T10:00:00Z"},
				{Key: "vcs.modified", Value: "true"},
			},
		}, true
	}
	got := versionString(fake)
	want := "hv 0.0.0-dev+dirty (commit abc1234, built 2026-06-02T10:00:00Z)"
	if got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionStringClean(t *testing.T) {
	fake := func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v1.2.3"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "deadbeefcafe"},
				{Key: "vcs.time", Value: "2026-06-02T10:00:00Z"},
				{Key: "vcs.modified", Value: "false"},
			},
		}, true
	}
	got := versionString(fake)
	want := "hv v1.2.3 (commit deadbee, built 2026-06-02T10:00:00Z)"
	if got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}

func TestVersionStringNoBuildInfo(t *testing.T) {
	got := versionString(func() (*debug.BuildInfo, bool) { return nil, false })
	want := "hv 0.0.0-dev (commit unknown, built unknown)"
	if got != want {
		t.Fatalf("versionString() = %q, want %q", got, want)
	}
}
