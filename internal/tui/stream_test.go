package tui

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// drainStream collects all StreamEvents from ch until it closes.
func drainStream(ch <-chan StreamEvent) []StreamEvent {
	var out []StreamEvent
	for se := range ch {
		out = append(out, se)
	}
	return out
}

func TestReadSSEStreamParsesDataFrames(t *testing.T) {
	input := "" +
		"id: 1\ndata: {\"seq\":1,\"hook_event\":\"PreToolUse\",\"session_id\":\"s1\",\"tool_name\":\"Bash\"}\n\n" +
		": ping\n\n" +
		"id: 2\ndata: {\"seq\":2,\"hook_event\":\"PostToolUse\",\"session_id\":\"s1\"}\n\n"

	ch := make(chan StreamEvent, 8)
	readSSEStream(strings.NewReader(input), ch)
	got := drainStream(ch)

	// Expect: 2 parsed events, then a terminal Err frame.
	if len(got) != 3 {
		t.Fatalf("got %d frames, want 3 (2 events + EOF): %+v", len(got), got)
	}
	if got[0].Err != nil || got[0].Event == nil {
		t.Fatalf("frame0 should be an event, got %+v", got[0])
	}
	if got[0].Event.HookEvent != "PreToolUse" || got[0].Event.Seq != 1 {
		t.Errorf("frame0 event = %+v, want PreToolUse seq 1", got[0].Event)
	}
	if got[1].Event == nil || got[1].Event.HookEvent != "PostToolUse" {
		t.Errorf("frame1 event = %+v, want PostToolUse", got[1].Event)
	}
	if got[2].Err == nil {
		t.Errorf("final frame should carry a terminating Err, got %+v", got[2])
	}
}

func TestReadSSEStreamMalformedFrameDegrades(t *testing.T) {
	input := "" +
		"data: {not valid json}\n\n" +
		"data: {\"seq\":5,\"hook_event\":\"Stop\",\"session_id\":\"s1\"}\n\n"

	ch := make(chan StreamEvent, 8)
	readSSEStream(strings.NewReader(input), ch)
	got := drainStream(ch)

	// Malformed frame must NOT terminate the stream nor surface as Err — it
	// degrades to a placeholder event so the row renders without crashing.
	if len(got) != 3 {
		t.Fatalf("got %d frames, want 3 (degraded + event + EOF): %+v", len(got), got)
	}
	if got[0].Err != nil {
		t.Errorf("malformed frame should degrade, not error: %+v", got[0])
	}
	if got[0].Event == nil {
		t.Fatalf("malformed frame should yield a degraded event, got nil")
	}
	if got[0].Event.HookEvent == "" {
		t.Errorf("degraded event should have a non-empty marker HookEvent")
	}
	if got[1].Event == nil || got[1].Event.Seq != 5 {
		t.Errorf("frame after malformed should parse normally, got %+v", got[1])
	}
}

func TestHTTPClientStreamReceivesEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		w.WriteHeader(http.StatusOK)
		fl.Flush()
		fmt.Fprint(w, "id: 1\ndata: {\"seq\":1,\"hook_event\":\"PreToolUse\",\"session_id\":\"s1\"}\n\n")
		fl.Flush()
		// Close after one event by returning from the handler.
	}))
	defer srv.Close()

	c := newHTTPClientAt(srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, err := c.Stream(ctx, "")
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}

	se := <-ch
	if se.Err != nil || se.Event == nil {
		t.Fatalf("first frame should be an event, got %+v", se)
	}
	if se.Event.Seq != 1 || se.Event.HookEvent != "PreToolUse" {
		t.Errorf("event = %+v, want seq 1 PreToolUse", se.Event)
	}
}

func TestFakeClientStreamReturnsChannel(t *testing.T) {
	fake := &FakeClient{}
	ch, err := fake.Stream(context.Background(), "")
	if err != nil {
		t.Fatalf("Stream() error: %v", err)
	}
	if ch == nil {
		t.Fatal("Stream() returned nil channel")
	}
}
