// Package sessions implements `hv sessions clear`: delete all captured
// session JSONL files from the sessions directory.
//
// Only *.jsonl files at the top level of SessionsDir are removed; the
// directory itself and any non-.jsonl files are left intact. If the daemon
// is running when this command executes, in-flight writes to a deleted inode
// are lost — stop the daemon first to be safe.
package sessions

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"jordandavis.dev/harness-visualizer/internal/paths"
)

// Run is the CLI entrypoint for `hv sessions`. args is everything after
// "sessions" on the command line. The only currently supported sub-subcommand
// is "clear".
func Run(args []string) int {
	const sessUsage = "usage: hv sessions <subcommand>\n\nSubcommands:\n  clear    delete all captured session JSONL files\n"
	if len(args) == 0 {
		fmt.Fprint(os.Stdout, sessUsage)
		return 0
	}
	sub, rest := args[0], args[1:]
	switch sub {
	case "--help", "-h", "help":
		fmt.Fprint(os.Stdout, sessUsage)
		return 0
	case "clear":
		return run(rest, paths.SessionsDir, os.Stdin, os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "sessions: unknown subcommand %q\n\n%s", sub, sessUsage)
		return 2
	}
}

// run is the testable core. sessionsDir resolves the target directory;
// stdin drives the interactive prompt; out receives all human-readable output.
// Errors are written to os.Stderr.
func run(args []string, sessionsDir func() (string, error), stdin io.Reader, out io.Writer) int {
	return runWithRemover(args, sessionsDir, stdin, out, os.Remove)
}

// runWithRemover is the fully-injectable core, also used by tests to simulate
// removal failures.
func runWithRemover(
	args []string,
	sessionsDir func() (string, error),
	stdin io.Reader,
	out io.Writer,
	remove func(string) error,
) int {
	var flagYes, flagDryRun bool

	for _, a := range args {
		switch a {
		case "--yes", "-y":
			flagYes = true
		case "--dry-run":
			flagDryRun = true
		case "--help", "-h":
			fmt.Fprint(out, usage())
			return 0
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "sessions clear: unknown flag %q\n\n", a)
				fmt.Fprint(os.Stderr, usage())
				return 2
			}
		}
	}

	dir, err := sessionsDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "sessions clear: resolve sessions dir: "+err.Error())
		return 1
	}

	files, totalSize, err := collectJSONL(dir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sessions clear: scan sessions dir: "+err.Error())
		return 1
	}

	if flagDryRun {
		for _, f := range files {
			fmt.Fprintln(out, f)
		}
		fmt.Fprintf(out, "would delete %d session files (%s)\n", len(files), formatSize(totalSize))
		return 0
	}

	if !flagYes {
		fmt.Fprintf(out, "Delete %d sessions (%s)? [y/N] ", len(files), formatSize(totalSize))
		answer, _ := bufio.NewReader(stdin).ReadString('\n')
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(os.Stderr, "sessions clear: aborted")
			return 2
		}
	}

	var failed int
	for _, f := range files {
		if err := remove(f); err != nil {
			fmt.Fprintf(os.Stderr, "sessions clear: remove %s: %v\n", filepath.Base(f), err)
			failed++
		}
	}

	deleted := len(files) - failed
	fmt.Fprintf(out, "deleted %d session files (%s)\n", deleted, formatSize(totalSize))

	if failed > 0 {
		return 1
	}
	return 0
}

// collectJSONL returns the absolute paths and total byte size of all *.jsonl
// files that are direct children of dir (non-recursive).
func collectJSONL(dir string) ([]string, int64, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, 0, err
	}
	var files []string
	var total int64
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue // skip unreadable entries
		}
		files = append(files, filepath.Join(dir, e.Name()))
		total += info.Size()
	}
	return files, total, nil
}

// formatSize renders a byte count as a human-readable string:
//   - < 1 KB  → "N B"
//   - < 1 MB  → "N.N KB"  (one decimal)
//   - >= 1 MB → "N.N MB"  (one decimal)
func formatSize(b int64) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case b >= mb:
		return fmt.Sprintf("%.1f MB", float64(b)/mb)
	case b >= kb:
		return fmt.Sprintf("%.1f KB", float64(b)/kb)
	default:
		return fmt.Sprintf("%d B", b)
	}
}

// usage returns the help text for `hv sessions clear`.
func usage() string {
	return `usage: hv sessions clear [--yes|-y] [--dry-run]

Delete all captured session JSONL files from the sessions directory.

Flags:
  --yes, -y     skip the confirmation prompt
  --dry-run     list files and total size; delete nothing

Exit codes:
  0  success (including an already-empty directory)
  1  partial or total failure (some files could not be deleted)
  2  bad flags or prompt aborted

Daemon caveat:
  If the daemon is running when this command executes, in-flight writes to
  a just-deleted inode are lost silently. Stop the daemon before clearing to
  be sure all data is cleanly removed.
`
}
