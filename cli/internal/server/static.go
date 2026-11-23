package server

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
)

// The compiled web interface lives in webdist/. It is generated from web/
// (React + TypeScript) by `go generate ./internal/server`, which needs
// Node.js; the output is committed so that building bearings needs Go only.
// CI regenerates it and fails when the committed copy is out of date.
//go:generate go run ./gen ../../../web webdist

//go:embed all:webdist
var webdistFS embed.FS

// contentTypes is a fixed table: the platform's MIME database is not used,
// so that responses are the same everywhere (and on Windows, where the
// registry can be changed by other programs).
var contentTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".mjs":         "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".json":        "application/json; charset=utf-8",
	".webmanifest": "application/manifest+json; charset=utf-8",
	".txt":         "text/plain; charset=utf-8",
	".svg":         "image/svg+xml",
	".png":         "image/png",
	".ico":         "image/x-icon",
	".woff2":       "font/woff2",
	".woff":        "font/woff",
	".map":         "application/json; charset=utf-8",
}

func staticFS() fs.FS {
	sub, err := fs.Sub(webdistFS, "webdist")
	if err != nil {
		panic(err) // the directory is embedded at build time
	}
	return sub
}

// serveStatic serves the embedded interface: files by exact path, and
// index.html for unknown paths when a browser asks for a page (single-page
// app routes). There are no directory listings and no dot files.
func (s *Server) serveStatic(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/")
	if name == "" {
		name = "index.html"
	}
	files := staticFS()
	data, ok := readStatic(files, name)
	if !ok {
		if !strings.Contains(r.Header.Get("Accept"), "text/html") {
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
		name = "index.html"
		if data, ok = readStatic(files, name); !ok {
			writeError(w, http.StatusNotFound, "not_found", "Not found.")
			return
		}
	}
	ct, known := contentTypes[strings.ToLower(path.Ext(name))]
	if !known {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("Content-Length", strconv.Itoa(len(data)))
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodGet {
		_, _ = w.Write(data) //nolint:gosec // G705: embedded build output with a fixed Content-Type table and nosniff, not user input
	}
}

// readStatic reads a regular, non-hidden file from the embedded tree.
func readStatic(files fs.FS, name string) ([]byte, bool) {
	if !fs.ValidPath(name) || strings.ContainsRune(name, '\\') {
		return nil, false
	}
	for _, seg := range strings.Split(name, "/") {
		if strings.HasPrefix(seg, ".") {
			return nil, false
		}
	}
	info, err := fs.Stat(files, name)
	if err != nil || !info.Mode().IsRegular() {
		return nil, false
	}
	data, err := fs.ReadFile(files, name)
	if err != nil {
		return nil, false
	}
	return data, true
}
