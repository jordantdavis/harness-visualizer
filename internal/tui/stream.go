package tui

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
)

// StreamEvent is one item delivered over the live SSE stream. Exactly one of
// Event / Err is set per value: a parsed (or degraded) event, or a terminal
// error signalling the connection ended. A malformed SSE frame degrades into
// an Event (never an Err) so a single bad payload never tears down the stream.
type StreamEvent struct {
	Event *event.Event
	Err   error
}

// maxSSELine bounds a single SSE data line, matching the daemon's 10MB body cap.
const maxSSELine = 10 * 1024 * 1024

// readSSEStream reads server-sent events from r, sending one StreamEvent per
// `data:` frame on ch. Comment frames (`: ping`) and `id:`/blank lines are
// skipped. A frame whose payload is not valid JSON is emitted as a degraded
// event. When r reaches EOF or errors, a final StreamEvent carrying Err is
// sent and ch is closed. readSSEStream is synchronous; callers that want it in
// the background should `go readSSEStream(...)`.
func readSSEStream(r io.Reader, ch chan<- StreamEvent) {
	defer close(ch)

	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), maxSSELine)
	for sc.Scan() {
		line := sc.Text()
		if !strings.HasPrefix(line, "data:") {
			// id:, comments (": ping"), and blank separators carry no payload.
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" {
			continue
		}
		var ev event.Event
		if err := json.Unmarshal([]byte(data), &ev); err != nil {
			ch <- StreamEvent{Event: degradedEvent(data)}
			continue
		}
		ch <- StreamEvent{Event: &ev}
	}
	if err := sc.Err(); err != nil {
		ch <- StreamEvent{Err: err}
		return
	}
	ch <- StreamEvent{Err: io.EOF}
}

// degradedEvent builds a placeholder event for an unparseable SSE frame so the
// TUI can render a degraded row instead of crashing. The raw bytes are kept in
// Raw for the inspector's escape hatch.
func degradedEvent(data string) *event.Event {
	return &event.Event{
		HookEvent: "(malformed)",
		Raw:       json.RawMessage(data),
	}
}

// streamHTTPClient returns an http.Client suitable for a long-lived SSE
// connection: no overall Timeout (which would abort the streaming read).
// Cancellation is driven by the request context instead.
func streamHTTPClient() *http.Client {
	return &http.Client{}
}

// Stream implements Client. It opens an SSE connection to /stream (optionally
// filtered to sessionFilter) and returns a channel of StreamEvents. The
// connection — and the background reader goroutine — live until ctx is
// cancelled or the server closes the stream, at which point a terminal Err
// frame is delivered and the channel closes.
func (c *HTTPClient) Stream(ctx context.Context, sessionFilter string) (<-chan StreamEvent, error) {
	url := c.base + "/stream"
	if sessionFilter != "" {
		url += "?session=" + sessionFilter
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("GET /stream: %w", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := streamHTTPClient().Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET /stream: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("GET /stream: status %d", resp.StatusCode)
	}

	ch := make(chan StreamEvent, streamBuf)
	go func() {
		defer resp.Body.Close()
		readSSEStream(resp.Body, ch)
	}()
	return ch, nil
}

// streamBuf buffers the SSE reader ahead of the bubbletea consumer so events
// arriving between consumer wake-ups are not lost.
const streamBuf = 256
