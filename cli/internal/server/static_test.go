package server

import (
	"strings"
	"testing"
)

func TestStatic(t *testing.T) {
	e := newEnv(t, false)
	index := readFile(t, "webdist/index.html")

	tests := []struct {
		name   string
		method string
		target string
		accept string
		want   int
		ctype  string
		body   bool
	}{
		{"root", "GET", "/", "", 200, "text/html; charset=utf-8", true},
		{"index file", "GET", "/index.html", "", 200, "text/html; charset=utf-8", true},
		{"head", "HEAD", "/", "", 200, "text/html; charset=utf-8", false},
		{"spa route", "GET", "/topics/technical/risk", "text/html,application/xhtml+xml", 200, "text/html; charset=utf-8", true},
		{"unknown asset", "GET", "/assets/app.js", "*/*", 404, "application/json; charset=utf-8", true},
		{"unknown, json", "GET", "/whatever", "application/json", 404, "application/json; charset=utf-8", true},
		{"directory", "GET", "/webdist", "text/html", 200, "text/html; charset=utf-8", true}, // falls back to the app, no listing
		{"dot file", "GET", "/.gitkeep", "", 404, "", true},
		{"hidden dir", "GET", "/.git/config", "", 404, "", true},
		{"dot dot", "GET", "/../go.mod", "", 404, "", true},
		{"encoded dot dot", "GET", "/%2e%2e/go.mod", "", 404, "", true},
		{"backslash", "GET", "/..%5cgo.mod", "", 404, "", true},
		{"post", "POST", "/", "", 405, "application/json; charset=utf-8", true},
		{"delete", "DELETE", "/index.html", "", 405, "application/json; charset=utf-8", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var o []opt
			if tt.accept != "" {
				o = append(o, withAccept(tt.accept))
			}
			rec := e.do(tt.method, tt.target, nil, append(o, withoutHeader("Origin"))...)
			if tt.want == 200 && tt.name == "directory" {
				// The app shell is served, never a file listing.
				if strings.Contains(rec.Body.String(), "index.html</a>") {
					t.Fatal("directory listing")
				}
			}
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body)
			}
			assertSecurityHeaders(t, rec.Header())
			if tt.ctype != "" && rec.Header().Get("Content-Type") != tt.ctype {
				t.Errorf("Content-Type %q, want %q", rec.Header().Get("Content-Type"), tt.ctype)
			}
			if tt.want == 200 && tt.method == "GET" && rec.Body.String() != index {
				t.Error("unexpected body")
			}
			if !tt.body && rec.Body.Len() != 0 {
				t.Error("HEAD returned a body")
			}
		})
	}
	// Static files carry no notebook data, so they do not need the cookie.
	if rec := e.do("GET", "/", nil, withoutCookie()); rec.Code != 200 {
		t.Errorf("index without cookie: %d", rec.Code)
	}
	if strings.Contains(index, "Example role") {
		t.Error("index.html holds notebook data")
	}
}

func TestContentTypesAreFixed(t *testing.T) {
	for ext, want := range map[string]string{
		".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8", ".svg": "image/svg+xml",
		".woff2": "font/woff2", ".json": "application/json; charset=utf-8",
	} {
		if contentTypes[ext] != want {
			t.Errorf("%s: %q", ext, contentTypes[ext])
		}
	}
}
