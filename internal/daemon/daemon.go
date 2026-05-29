// Package daemon implements the cchv HTTP daemon: it receives hook events via
// POST, persists them through a per-session writer goroutine, and fans them
// out live over SSE. A single Server value is the testable unit; Run is the
// thin CLI entrypoint.
package daemon

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"jordandavis.dev/cc-harness-visualizer/internal/event"
	"jordandavis.dev/cc-harness-visualizer/internal/paths"
	"jordandavis.dev/cc-harness-visualizer/internal/store"
)

const (
	defaultPort      = 7842
	maxBodyBytes     = 10 * 1024 * 1024 // 10 MB
	writerBuf        = 256               // per-session inbound channel capacity
	subBuf           = 64               // per-SSE-subscriber outbound channel capacity
	heartbeatPeriod  = 15 * time.Second
	writerFlushDelay = 250 * time.Millisecond
)

// ---- Hub ----

// subscriber represents one SSE client. filter is the session ID to watch, or
// "" to watch all sessions.
type subscriber struct {
	filter string
	ch     chan *event.Event
}

// hub fans out events to all active SSE subscribers. It is safe for
// concurrent use.
type hub struct {
	mu   sync.RWMutex
	subs map[*subscriber]struct{}
}

func newHub() *hub { return &hub{subs: make(map[*subscriber]struct{})} }

// subscribe registers sub and returns a cleanup func.
func (h *hub) subscribe(sub *subscriber) func() {
	h.mu.Lock()
	h.subs[sub] = struct{}{}
	h.mu.Unlock()
	return func() {
		h.mu.Lock()
		delete(h.subs, sub)
		h.mu.Unlock()
	}
}

// publish sends ev to every matching subscriber. Slow subscribers are dropped
// (non-blocking send).
func (h *hub) publish(ev *event.Event) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for sub := range h.subs {
		if sub.filter != "" && sub.filter != ev.SessionID {
			continue
		}
		select {
		case sub.ch <- ev:
		default:
			// Drop rather than block the writer.
		}
	}
}

// ---- Per-session writer ----

// sessionWriter serialises Append calls for one session via a buffered channel
// so POST handlers return immediately.
type sessionWriter struct {
	ch   chan *event.Event
	done chan struct{}
}

// newSessionWriter starts the drain goroutine and returns the writer.
func newSessionWriter(st *store.Store, h *hub) *sessionWriter {
	sw := &sessionWriter{
		ch:   make(chan *event.Event, writerBuf),
		done: make(chan struct{}),
	}
	go sw.drain(st, h)
	return sw
}

// send queues ev for writing; non-blocking (drops if buffer full, which
// should not happen in practice given writerBuf = 256).
func (sw *sessionWriter) send(ev *event.Event) {
	select {
	case sw.ch <- ev:
	default:
	}
}

// drain processes the channel until closed, calling store.Append and then
// publishing to the hub after Seq is assigned.
func (sw *sessionWriter) drain(st *store.Store, h *hub) {
	defer close(sw.done)
	for ev := range sw.ch {
		if err := st.Append(ev); err == nil {
			h.publish(ev)
		}
	}
}

// close shuts down the writer goroutine and waits for it to finish.
func (sw *sessionWriter) close() {
	close(sw.ch)
	<-sw.done
}

// ---- Server ----

// Server is the testable HTTP server. Create with NewServer; call
// ListenAndServe to bind a port; call Shutdown to stop cleanly.
type Server struct {
	st      *store.Store
	hub     *hub
	mux     *http.ServeMux
	writers sync.Map // sessionID -> *sessionWriter

	httpSrv  *http.Server
	portFile string
	pidFile  string

	shutdownOnce sync.Once
}

// NewServer constructs a Server backed by st. The caller owns st's lifetime;
// Shutdown drains internal state but does NOT call st.Close() — the caller
// must do that.
func NewServer(st *store.Store) *Server {
	s := &Server{
		st:  st,
		hub: newHub(),
		mux: http.NewServeMux(),
	}
	s.mux.HandleFunc("/healthz", s.handleHealthz)
	s.mux.HandleFunc("/events", s.handleEvents)
	s.mux.HandleFunc("/sessions", s.handleSessions)
	s.mux.HandleFunc("/sessions/", s.handleSessionEvents)
	s.mux.HandleFunc("/stream", s.handleStream)
	return s
}

// ServeHTTP implements http.Handler so Server can be used with httptest.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe binds to addr (e.g. "127.0.0.1:7842"), writes the actual
// port to portFile and the pid to pidFile, and starts serving in the
// background. It returns the actual addr string ("host:port"). The caller
// must later call Shutdown.
func (s *Server) ListenAndServe(addr, portFile, pidFile string) (string, error) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return "", err
	}
	actualAddr := ln.Addr().String()
	_, portStr, _ := net.SplitHostPort(actualAddr)

	// Write runtime files.
	if portFile != "" {
		if werr := os.WriteFile(portFile, []byte(portStr), 0o644); werr != nil {
			_ = ln.Close()
			return "", fmt.Errorf("write port file: %w", werr)
		}
	}
	if pidFile != "" {
		if werr := os.WriteFile(pidFile, []byte(strconv.Itoa(os.Getpid())), 0o644); werr != nil {
			_ = ln.Close()
			return "", fmt.Errorf("write pid file: %w", werr)
		}
	}

	s.portFile = portFile
	s.pidFile = pidFile

	s.httpSrv = &http.Server{Handler: s.mux}
	go func() { _ = s.httpSrv.Serve(ln) }()
	return actualAddr, nil
}

// Shutdown drains all session writers, closes the HTTP server gracefully, and
// removes runtime files. Safe to call multiple times.
func (s *Server) Shutdown() {
	s.shutdownOnce.Do(func() {
		// Stop HTTP server first so no new events arrive.
		if s.httpSrv != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.httpSrv.Shutdown(ctx)
		}

		// Drain all session writers.
		s.writers.Range(func(_, v any) bool {
			v.(*sessionWriter).close()
			return true
		})

		// Remove runtime files.
		if s.portFile != "" {
			_ = os.Remove(s.portFile)
		}
		if s.pidFile != "" {
			_ = os.Remove(s.pidFile)
		}
	})
}

// writerFor returns the sessionWriter for sessionID, creating one if needed.
func (s *Server) writerFor(sessionID string) *sessionWriter {
	if v, ok := s.writers.Load(sessionID); ok {
		return v.(*sessionWriter)
	}
	sw := newSessionWriter(s.st, s.hub)
	actual, loaded := s.writers.LoadOrStore(sessionID, sw)
	if loaded {
		// Another goroutine beat us; shut down the one we just created.
		sw.close()
		return actual.(*sessionWriter)
	}
	return sw
}

// ---- Handlers ----

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBodyBytes)

	var ev event.Event
	if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Non-blocking: push onto session's writer channel and return immediately.
	s.writerFor(ev.SessionID).send(&ev)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	infos, err := s.st.Sessions()
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if infos == nil {
		infos = []store.SessionInfo{} // ensure JSON array, not null
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infos)
}

// handleSessionEvents handles GET /sessions/{id}/events?since=<seq>.
func (s *Server) handleSessionEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// Extract {id} from path: /sessions/<id>/events
	path := strings.TrimPrefix(r.URL.Path, "/sessions/")
	path = strings.TrimSuffix(path, "/events")
	sessionID := path

	var since int64
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if v, err := strconv.ParseInt(sinceStr, 10, 64); err == nil {
			since = v
		}
	}

	evs, err := s.st.Read(sessionID, since)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if evs == nil {
		evs = []*event.Event{} // ensure JSON array, not null
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(evs)
}

func (s *Server) handleStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	sessionFilter := r.URL.Query().Get("session")

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	sub := &subscriber{filter: sessionFilter, ch: make(chan *event.Event, subBuf)}
	unsub := s.hub.subscribe(sub)
	defer unsub()

	ticker := time.NewTicker(heartbeatPeriod)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case ev := <-sub.ch:
			line, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			fmt.Fprintf(w, "id: %d\ndata: %s\n\n", ev.Seq, line)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		}
	}
}

// ---- Run entrypoint ----

// Run is the CLI entrypoint for `cchv daemon`. args should not include the
// subcommand name. Returns an OS exit code (0 = success).
//
// Flags:
//
//	--foreground  (bool, default true) run in foreground; ignored — always serves in-process
//	--port        (int,  default 7842) preferred listen port; falls back to :0 on EADDRINUSE
func Run(args []string) int {
	fs := flag.NewFlagSet("daemon", flag.ContinueOnError)
	foreground := fs.Bool("foreground", true, "run in foreground (always true; detach is the hook CLI's job)")
	port := fs.Int("port", defaultPort, "preferred listen port")
	if err := fs.Parse(args); err != nil {
		fmt.Fprintln(os.Stderr, "daemon: "+err.Error())
		return 1
	}
	_ = foreground // detach is the hook CLI's responsibility

	sessDir, err := paths.SessionsDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon: sessions dir: "+err.Error())
		return 1
	}
	st, err := store.New(sessDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon: store: "+err.Error())
		return 1
	}
	defer st.Close()

	portFile, err := paths.PortFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon: port file: "+err.Error())
		return 1
	}
	pidFile, err := paths.PidFile()
	if err != nil {
		fmt.Fprintln(os.Stderr, "daemon: pid file: "+err.Error())
		return 1
	}

	srv := NewServer(st)

	// Try preferred port; fall back to ephemeral on EADDRINUSE.
	preferredAddr := fmt.Sprintf("127.0.0.1:%d", *port)
	addr, err := srv.ListenAndServe(preferredAddr, portFile, pidFile)
	if err != nil {
		if isAddrInUse(err) {
			addr, err = srv.ListenAndServe("127.0.0.1:0", portFile, pidFile)
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "daemon: listen: "+err.Error())
			return 1
		}
	}

	fmt.Fprintf(os.Stdout, "cchv daemon listening on %s\n", addr)

	// Block until SIGINT or SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	srv.Shutdown()
	_ = st.Close()
	return 0
}

// isAddrInUse returns true when err wraps EADDRINUSE.
func isAddrInUse(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "address already in use") ||
		strings.Contains(err.Error(), "bind: address already in use") ||
		strings.Contains(err.Error(), "listen tcp")
}
