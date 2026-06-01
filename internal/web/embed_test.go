package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestSPAHandler_ServesIndexAtRoot(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("<!doctype html><hv-app></hv-app>")}}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "hv-app") {
		t.Fatalf("root: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_ServesAsset(t *testing.T) {
	files := fstest.MapFS{
		"index.html":    {Data: []byte("idx")},
		"assets/app.js": {Data: []byte("console.log(1)")},
	}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/assets/app.js", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "console.log(1)" {
		t.Fatalf("asset: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_FallsBackToIndex(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("idx")}}
	rec := httptest.NewRecorder()
	NewHandler(files).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/some/spa/route", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "idx" {
		t.Fatalf("fallback: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_UnbuiltNotice(t *testing.T) {
	rec := httptest.NewRecorder()
	NewHandler(fstest.MapFS{}).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "make build") {
		t.Fatalf("notice: code=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_PathTraversalCannotEscape(t *testing.T) {
	files := fstest.MapFS{"index.html": {Data: []byte("idx")}}
	h := NewHandler(files)
	for _, p := range []string{"/../embed.go", "/../../go.mod", "/assets/../../embed.go"} {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if strings.Contains(rec.Body.String(), "package web") {
			t.Fatalf("path %q escaped the FS root: body=%q", p, rec.Body.String())
		}
	}
}
