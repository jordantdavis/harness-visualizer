// Package tui — plain.go
//
// Plain line-per-event mode: screen-reader / pipe / bug-report friendly output.
//
// Usage (via --plain flag):
//
//	cchv tui --plain
//
// Output format (one line per event, no ANSI):
//
//	<HH:MM:SS> <TAG> <HOOK(15)> <TOOL(12)> <GIST>
//
// TAG is always 5 characters:
//
//	[OK]   successful PostToolUse (exit 0)
//	[ERR]  failed PostToolUse (exit ≠0)
//	[RUN]  PreToolUse (in-progress)
//	[-- ]  neutral / lifecycle events
//
// No ANSI color, no cursor control, no animation. Suitable for piping and
// screen readers.
package tui

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

// plainTag returns the fixed-width (5-char) text tag for a status in plain mode.
// This is the plain-mode equivalent of statusGlyph; always 5 bytes ASCII.
func plainTag(s eventStatus) string {
	switch s {
	case statusOK:
		return "[OK] "
	case statusError:
		return "[ERR]"
	case statusRunning:
		return "[RUN]"
	default:
		return "[-- ]"
	}
}

// plainLine formats ev as a single plain-text line with no ANSI escapes.
// Columns:
//
//	HH:MM:SS  <TAG>  <HOOK(15)>  <TOOL(12)>  <GIST>
//
// The result is trimmed of trailing whitespace. This function is pure and
// deterministic: given the same event it always returns the same string.
func plainLine(ev *event.Event) string {
	// UTC for reproducibility in non-interactive/pipe contexts.
	timeStr := ev.CapturedAt.UTC().Format("15:04:05")
	tag := plainTag(deriveStatus(ev))
	hook := padRight(ev.HookEvent, 15)
	tool := padRight(ev.ToolName, 12)
	gist := targetGist(ev)

	parts := []string{timeStr, tag, hook, tool}
	if gist != "" {
		parts = append(parts, gist)
	}
	line := strings.Join(parts, " ")
	return strings.TrimRight(line, " ")
}

// runPlain is the entrypoint for --plain mode. It prints historical events for
// the most recent session (matching the TUI's auto-selection behaviour) and
// then streams live events as plain lines until ctx is cancelled.
//
// If no sessions exist the function prints a hint and returns nil.
// Stream errors are soft — historical events are always printed first.
func runPlain(ctx context.Context, c Client, w io.Writer) error {
	sessions, err := c.Sessions()
	if err != nil {
		return fmt.Errorf("plain: list sessions: %w", err)
	}
	if len(sessions) == 0 {
		fmt.Fprintln(w, "# no sessions yet — start Claude Code with the cchv plugin installed")
		return nil
	}

	// Auto-select the first (most recent) session, same as the TUI.
	selected := sessions[0]
	printPlainHeader(w, selected)

	evs, err := c.Events(selected.ID, 0)
	if err != nil {
		return fmt.Errorf("plain: load events for %s: %w", selected.ID, err)
	}
	for _, ev := range evs {
		fmt.Fprintln(w, plainLine(ev))
	}

	// Stream live events; errors here are soft since history is already printed.
	ch, err := c.Stream(ctx, "")
	if err != nil {
		return nil
	}
	for {
		select {
		case <-ctx.Done():
			return nil
		case se, ok := <-ch:
			if !ok {
				return nil
			}
			if se.Err != nil {
				return nil
			}
			if se.Event != nil {
				fmt.Fprintln(w, plainLine(se.Event))
			}
		}
	}
}

// printPlainHeader emits a one-line comment describing the session being shown.
func printPlainHeader(w io.Writer, s store.SessionInfo) {
	fmt.Fprintf(w, "# session: %s  events: %d\n", s.ID, s.EventCount)
}

// runPlainMain is the wired entrypoint called from Run when --plain is set.
// It blocks until SIGINT or the stream closes.
func runPlainMain(c Client) int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		select {
		case <-sigCh:
			cancel()
		case <-ctx.Done():
		}
	}()

	if err := runPlain(ctx, c, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "cchv tui --plain: %v\n", err)
		return 1
	}
	return 0
}
