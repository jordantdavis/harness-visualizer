// Package web embeds the built Lit single-page app (internal/web/dist) and
// serves it as an http.Handler with SPA fallback. The bundle is embedded at
// compile time; a fresh checkout that has not run `make build` embeds only a
// placeholder, and the handler serves a "run make build" notice instead of the
// app. The embed path is internal/web/dist (not the top-level web/dist) because
// go:embed patterns cannot traverse upward out of the source file's directory.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var distFS embed.FS

// Handler returns the production handler serving the embedded dist tree.
func Handler() http.Handler {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		// Unreachable: dist is embedded at compile time. Serve the notice.
		return NewHandler(emptyFS{})
	}
	return NewHandler(sub)
}

// NewHandler builds an SPA handler over files. A request that names an existing
// file is served directly; anything else falls back to index.html. When
// index.html is absent (unbuilt checkout), a plain-text notice is served.
func NewHandler(files fs.FS) http.Handler {
	return &spaHandler{files: files}
}

type spaHandler struct{ files fs.FS }

func (h *spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" {
		name = "index.html"
	}
	if h.serveFile(w, r, name) {
		return
	}
	// SPA fallback: serve index.html for unknown (client-routed) paths.
	if h.serveFile(w, r, "index.html") {
		return
	}
	// Unbuilt checkout: helpful notice instead of a bare 404.
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("cchv web bundle not built. Run `make build` to build and embed the frontend.\n"))
}

// serveFile serves name from the FS if it exists and is a regular file,
// returning true when it handled the response.
func (h *spaHandler) serveFile(w http.ResponseWriter, r *http.Request, name string) bool {
	f, err := h.files.Open(name)
	if err != nil {
		return false
	}
	st, serr := f.Stat()
	_ = f.Close()
	if serr != nil || st.IsDir() {
		return false
	}
	http.ServeFileFS(w, r, h.files, name)
	return true
}

// emptyFS is a zero-file FS used only on the unreachable fs.Sub error path.
type emptyFS struct{}

func (emptyFS) Open(string) (fs.File, error) { return nil, fs.ErrNotExist }
