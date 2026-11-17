package server

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// concrete fills the parameters of a route pattern with valid values.
func concrete(pattern string) string {
	r := strings.NewReplacer("{slug}", "technical", "{section}", "technical", "{topic}", "backtesting", "{id}", "0123456789")
	return r.Replace(pattern)
}

func TestListenAddress(t *testing.T) {
	ln, err := Listen(0)
	must(t, err)
	defer func() { _ = ln.Close() }()
	addr := ln.Addr().String()
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("listening on %s", addr)
	}
	if ln.Addr().Network() != "tcp" {
		t.Fatal(ln.Addr().Network())
	}
	for _, p := range []int{-1, 65536} {
		if l, err := Listen(p); err == nil {
			_ = l.Close()
			t.Errorf("Listen(%d) succeeded", p)
		}
	}
	// New refuses a listener that is not on 127.0.0.1 only.
	_, nb := buildNotebook(t)
	wide, err := netListenAll()
	if err != nil {
		t.Skipf("cannot listen on all interfaces here: %v", err)
	}
	defer func() { _ = wide.Close() }()
	if _, err := New(wide, Config{Notebook: nb}); err == nil {
		t.Error("New accepted a listener on 0.0.0.0")
	}
}

func TestHostHeader(t *testing.T) {
	e := newEnv(t, false)
	p := e.port
	tests := []struct {
		name string
		host string
		want int
	}{
		{"ip", fmt.Sprintf("127.0.0.1:%d", p), http.StatusOK},
		{"localhost", fmt.Sprintf("localhost:%d", p), http.StatusOK},
		{"evil", fmt.Sprintf("evil.com:%d", p), http.StatusMisdirectedRequest},
		{"evil no port", "evil.com", http.StatusMisdirectedRequest},
		{"other port", fmt.Sprintf("127.0.0.1:%d", p+1), http.StatusMisdirectedRequest},
		{"no port", "127.0.0.1", http.StatusMisdirectedRequest},
		{"localhost no port", "localhost", http.StatusMisdirectedRequest},
		{"missing", "", http.StatusMisdirectedRequest},
		{"suffix attack", fmt.Sprintf("127.0.0.1:%d.evil.com", p), http.StatusMisdirectedRequest},
		{"prefix attack", fmt.Sprintf("evil.com:%d@127.0.0.1:%d", p, p), http.StatusMisdirectedRequest},
		{"ipv6", fmt.Sprintf("[::1]:%d", p), http.StatusMisdirectedRequest},
		{"upper", fmt.Sprintf("LOCALHOST:%d", p), http.StatusMisdirectedRequest},
		{"dns name of loopback", fmt.Sprintf("localtest.me:%d", p), http.StatusMisdirectedRequest},
		{"trailing dot", fmt.Sprintf("localhost.:%d", p), http.StatusMisdirectedRequest},
	}
	for _, tt := range tests {
		for _, target := range []string{"/api/v1/info", "/", "/auth?token=x", "/missing"} {
			t.Run(tt.name+target, func(t *testing.T) {
				rec := e.do("GET", target, nil, withHost(tt.host), withAccept("text/html"))
				if tt.want == http.StatusOK {
					if rec.Code == http.StatusMisdirectedRequest {
						t.Fatalf("host %q refused", tt.host)
					}
					return
				}
				if rec.Code != tt.want {
					t.Fatalf("host %q on %s: %d, want %d", tt.host, target, rec.Code, tt.want)
				}
				assertSecurityHeaders(t, rec.Header())
				if strings.Contains(rec.Body.String(), "Example role") {
					t.Fatal("data served to a foreign host")
				}
			})
		}
	}
}

func TestSecFetchSite(t *testing.T) {
	e := newEnv(t, false)
	for v, want := range map[string]int{
		"cross-site": 403, "same-site": 403, "bogus": 403,
		"same-origin": 200, "none": 200, "": 200,
	} {
		rec := e.do("GET", "/api/v1/info", nil, withHeader("Sec-Fetch-Site", v))
		if rec.Code != want {
			t.Errorf("Sec-Fetch-Site %q: %d, want %d", v, rec.Code, want)
		}
	}
	rec := e.do("POST", "/api/v1/sections", map[string]any{"title": "X"}, withHeader("Sec-Fetch-Site", "cross-site"))
	if rec.Code != 403 {
		t.Errorf("cross-site mutation: %d", rec.Code)
	}
	assertNoSection(t, e, "x")
}

func assertNoSection(t *testing.T, e *testEnv, slug string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(e.dir, "sections", slug)); err == nil {
		t.Errorf("section %q was created", slug)
	}
}

func TestAPIRequiresCookie(t *testing.T) {
	e := newEnv(t, false)
	for _, rt := range e.srv.routes {
		path := concrete(rt.pattern)
		for name, o := range map[string]opt{
			"none":  withoutCookie(),
			"wrong": withHeader("Cookie", e.srv.cookie+"=wrong"),
			"other": withHeader("Cookie", "other="+e.srv.token),
			"empty": withHeader("Cookie", e.srv.cookie+"="),
		} {
			rec := e.do(rt.method, path, map[string]any{}, o)
			if rec.Code != http.StatusUnauthorized {
				t.Errorf("%s %s (%s cookie): %d, want 401", rt.method, path, name, rec.Code)
			}
			if got := errCode(t, rec); got != "unauthorized" {
				t.Errorf("%s %s: code %q", rt.method, path, got)
			}
		}
	}
	// Unknown API paths do not reveal anything before authentication.
	if rec := e.do("GET", "/api/v1/nope", nil, withoutCookie()); rec.Code != 401 {
		t.Errorf("unknown path without cookie: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/nope", nil); rec.Code != 404 {
		t.Errorf("unknown path with cookie: %d", rec.Code)
	}
	// The token itself in a header or query is not a credential.
	if rec := e.do("GET", "/api/v1/info?token="+e.srv.token, nil, withoutCookie()); rec.Code != 401 {
		t.Errorf("token in query: %d", rec.Code)
	}
	if rec := e.do("GET", "/api/v1/info", nil, withoutCookie(), withHeader("Authorization", "Bearer "+e.srv.token)); rec.Code != 401 {
		t.Errorf("token in header: %d", rec.Code)
	}
}

func TestMutationOrigin(t *testing.T) {
	e := newEnv(t, false)
	p := e.port
	tests := []struct {
		name   string
		origin *string
		ok     bool
	}{
		{"missing", nil, false},
		{"empty", ptr(""), false},
		{"null", ptr("null"), false},
		{"evil", ptr("http://evil.com"), false},
		{"evil with port", ptr(fmt.Sprintf("http://evil.com:%d", p)), false},
		{"other port", ptr(fmt.Sprintf("http://127.0.0.1:%d", p+1)), false},
		{"https", ptr(fmt.Sprintf("https://127.0.0.1:%d", p)), false},
		{"trailing slash", ptr(fmt.Sprintf("http://127.0.0.1:%d/", p)), false},
		{"ipv6", ptr(fmt.Sprintf("http://[::1]:%d", p)), false},
		{"ip", ptr(fmt.Sprintf("http://127.0.0.1:%d", p)), true},
		{"localhost", ptr(fmt.Sprintf("http://localhost:%d", p)), true},
	}
	for _, rt := range e.srv.routes {
		if !rt.write {
			continue
		}
		for _, tt := range tests {
			t.Run(rt.method+" "+rt.pattern+" "+tt.name, func(t *testing.T) {
				o := withoutHeader("Origin")
				if tt.origin != nil {
					o = withHeader("Origin", *tt.origin)
				}
				rec := e.do(rt.method, concrete(rt.pattern), map[string]any{}, o)
				got := rec.Code == http.StatusForbidden && errCode(t, rec) == "forbidden_origin"
				if tt.ok == got {
					t.Fatalf("origin %v: status %d code %s, allowed=%v", tt.origin, rec.Code, rec.Body.String(), tt.ok)
				}
			})
		}
	}
}

func ptr(s string) *string { return &s }

func TestContentType(t *testing.T) {
	e := newEnv(t, false)
	body := `{"title":"Ops"}`
	tests := []struct {
		name string
		ct   *string
		want int
	}{
		{"json", ptr("application/json"), 201},
		{"json charset", ptr("application/json; charset=utf-8"), 201},
		{"json upper", ptr("Application/JSON"), 201},
		{"missing", nil, 415},
		{"text", ptr("text/plain"), 415},
		{"form", ptr("application/x-www-form-urlencoded"), 415},
		{"multipart", ptr("multipart/form-data; boundary=x"), 415},
		{"json-ish", ptr("application/jsonp"), 415},
		{"garbage", ptr("%%%"), 415},
	}
	for i, tt := range tests {
		o := withoutHeader("Content-Type")
		if tt.ct != nil {
			o = withType(*tt.ct)
		}
		slug := fmt.Sprintf(`{"title":"Ops %d"}`, i)
		_ = body
		rec := e.do("POST", "/api/v1/sections", slug, o)
		if rec.Code != tt.want {
			t.Errorf("%s: %d, want %d (%s)", tt.name, rec.Code, tt.want, rec.Body.String())
		}
	}
	// Bodiless mutations need no content type.
	rec := e.do("POST", "/api/v1/topics/technical/risk/advance", nil)
	if rec.Code != 200 {
		t.Errorf("bodiless advance: %d %s", rec.Code, rec.Body.String())
	}
}

func TestBodyLimit(t *testing.T) {
	e := newEnv(t, false)
	big := `{"title":"X","description":"` + strings.Repeat("a", MaxBodyBytes+10) + `"}`
	// Declared length too large: refused before reading.
	rec := e.do("POST", "/api/v1/sections", big)
	if rec.Code != http.StatusRequestEntityTooLarge || errCode(t, rec) != "payload_too_large" {
		t.Errorf("declared size: %d %s", rec.Code, rec.Body.String())
	}
	// Unknown length (chunked): stopped while reading.
	rec = e.do("POST", "/api/v1/sections", big, withContentLength(-1))
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("streamed size: %d %s", rec.Code, rec.Body.String())
	}
	// Just under the limit is read (and then rejected by the notebook's own
	// limit on the description, not by the server).
	ok := `{"title":"X","description":"` + strings.Repeat("a", 10<<10) + `"}`
	if rec := e.do("POST", "/api/v1/sections", ok); rec.Code == http.StatusRequestEntityTooLarge {
		t.Errorf("small body refused: %s", rec.Body.String())
	}
	assertNoSection(t, e, "x")
}

func TestStrictJSON(t *testing.T) {
	e := newEnv(t, false)
	tests := []struct {
		name string
		body string
		want int
		code string
	}{
		{"unknown field", `{"title":"A","extra":1}`, 400, "invalid_json"},
		{"unknown nested", `{"title":"A","slug":"a","order":1,"foo":{}}`, 400, "invalid_json"},
		{"trailing object", `{"title":"A"}{"title":"B"}`, 400, "invalid_json"},
		{"trailing garbage", `{"title":"A"} x`, 400, "invalid_json"},
		{"truncated", `{"title":`, 400, "invalid_json"},
		{"empty", ``, 400, "invalid_json"},
		{"array", `[]`, 400, "invalid_json"},
		{"wrong type", `{"title":5}`, 400, "invalid_json"},
		{"null body", `null`, 422, "empty"},
		{"ok", `{"title":"Fine"}`, 201, ""},
	}
	for _, tt := range tests {
		rec := e.do("POST", "/api/v1/sections", tt.body, withContentLength(int64(len(tt.body)+boolInt(tt.body == ""))))
		if tt.body == "" {
			// An empty body has no content type requirement; it must still fail.
			rec = e.do("POST", "/api/v1/sections", nil)
		}
		if rec.Code != tt.want {
			t.Errorf("%s: %d, want %d: %s", tt.name, rec.Code, tt.want, rec.Body.String())
			continue
		}
		if tt.code != "" && errCode(t, rec) != tt.code {
			t.Errorf("%s: code %q, want %q", tt.name, errCode(t, rec), tt.code)
		}
	}
	// Trailing whitespace is fine.
	if rec := e.do("POST", "/api/v1/sections", "{\"title\":\"Spaces\"}\n  \n"); rec.Code != 201 {
		t.Errorf("trailing whitespace: %d", rec.Code)
	}
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func TestReadOnlyBlocksEveryWriteRoute(t *testing.T) {
	e := newEnv(t, true)
	before := snapshotDir(t, e.dir)
	writes := 0
	for _, rt := range e.srv.routes {
		if rt.method == http.MethodGet {
			if rt.write {
				t.Errorf("GET route %s marked as write", rt.pattern)
			}
			continue
		}
		writes++
		if !rt.write {
			t.Errorf("%s %s changes state but is not marked as a write route", rt.method, rt.pattern)
		}
		rec := e.do(rt.method, concrete(rt.pattern), map[string]any{"title": "x", "version": "0-00", "status": "mastered", "delta": 1, "to": "x", "body": "x"})
		if rec.Code != http.StatusForbidden || errCode(t, rec) != "read_only" {
			t.Errorf("%s %s in read-only mode: %d %s", rt.method, rt.pattern, rec.Code, rec.Body.String())
		}
	}
	if writes < 15 {
		t.Errorf("only %d write routes found", writes)
	}
	if after := snapshotDir(t, e.dir); after != before {
		t.Error("read-only server changed the notebook")
	}
	info := e.json("GET", "/api/v1/info", nil, 200)
	if info["read_only"] != true {
		t.Errorf("info.read_only = %v", info["read_only"])
	}
	// Reads still work.
	e.json("GET", "/api/v1/outline", nil, 200)
	rw := newEnv(t, false)
	if rw.json("GET", "/api/v1/info", nil, 200)["read_only"] != false {
		t.Error("read-write server reports read_only")
	}
}

// snapshotDir returns a string describing every file of dir with its content.
func snapshotDir(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	must(t, filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		if d.IsDir() {
			b.WriteString("D " + rel + "\n")
			return nil
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		b.WriteString("F " + rel + "\n" + string(data) + "\n")
		return nil
	}))
	return b.String()
}

func TestRouteTable(t *testing.T) {
	e := newEnv(t, false)
	seen := map[string]bool{}
	for _, rt := range e.srv.routes {
		key := rt.method + " " + rt.pattern
		if seen[key] {
			t.Errorf("duplicate route %s", key)
		}
		seen[key] = true
		if !strings.HasPrefix(rt.pattern, "/api/v1/") {
			t.Errorf("route %s outside /api/v1/", key)
		}
		if rt.method != http.MethodGet && !rt.write {
			t.Errorf("%s is a mutation without the write flag", key)
		}
		if rt.handler == nil {
			t.Errorf("%s has no handler", key)
		}
	}
	// Wrong method on a known path: 405 with Allow, and still behind auth.
	rec := e.do("PUT", "/api/v1/sections", map[string]any{})
	if rec.Code != 405 || rec.Header().Get("Allow") == "" {
		t.Errorf("PUT /sections: %d allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

var securityHeaders = map[string]string{
	"Content-Security-Policy":      contentSecurityPolicy,
	"X-Content-Type-Options":       "nosniff",
	"Referrer-Policy":              "no-referrer",
	"X-Frame-Options":              "DENY",
	"Cross-Origin-Opener-Policy":   "same-origin",
	"Cross-Origin-Resource-Policy": "same-origin",
}

func assertSecurityHeaders(t *testing.T, h http.Header) {
	t.Helper()
	for k, v := range securityHeaders {
		if got := h.Get(k); got != v {
			t.Errorf("%s = %q, want %q", k, got, v)
		}
	}
	if h.Get("Permissions-Policy") == "" {
		t.Error("Permissions-Policy missing")
	}
	for k := range h {
		if strings.HasPrefix(strings.ToLower(k), "access-control-") {
			t.Errorf("CORS header %s set", k)
		}
	}
}

func TestSecurityHeadersOnEveryResponse(t *testing.T) {
	e := newEnv(t, false)
	csp := contentSecurityPolicy
	for _, bad := range []string{"unsafe-inline", "unsafe-eval", "*", "http:", "https:"} {
		if strings.Contains(csp, bad) {
			t.Errorf("CSP contains %q", bad)
		}
	}
	for _, want := range []string{"default-src 'none'", "script-src 'self'", "style-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "form-action 'self'", "connect-src 'self'"} {
		if !strings.Contains(csp, want) {
			t.Errorf("CSP lacks %q", want)
		}
	}
	tests := []struct {
		name   string
		method string
		target string
		opts   []opt
		body   any
	}{
		{"api ok", "GET", "/api/v1/info", nil, nil},
		{"api list", "GET", "/api/v1/outline", nil, nil},
		{"api 401", "GET", "/api/v1/info", []opt{withoutCookie()}, nil},
		{"api 404", "GET", "/api/v1/nope", nil, nil},
		{"api 405", "PUT", "/api/v1/sections", nil, map[string]any{}},
		{"api 400", "GET", "/api/v1/sections/BAD", nil, nil},
		{"api 403 origin", "POST", "/api/v1/sections", []opt{withoutHeader("Origin")}, map[string]any{}},
		{"api 415", "POST", "/api/v1/sections", []opt{withType("text/plain")}, "x"},
		{"api 201", "POST", "/api/v1/sections", nil, map[string]any{"title": "Headers"}},
		{"api 204", "DELETE", "/api/v1/sections/headers", nil, nil},
		{"static index", "GET", "/", nil, nil},
		{"static asset 404", "GET", "/nothing.js", nil, nil},
		{"static spa", "GET", "/some/route", []opt{withAccept("text/html")}, nil},
		{"static 405", "POST", "/", []opt{withoutHeader("Origin")}, nil},
		{"auth ok", "GET", "/auth?token=" + e.srv.token, nil, nil},
		{"auth bad", "GET", "/auth?token=nope", nil, nil},
		{"auth 405", "POST", "/auth", nil, nil},
		{"options", "OPTIONS", "/api/v1/info", nil, nil},
		{"options static", "OPTIONS", "/", nil, nil},
		{"bad host", "GET", "/", []opt{withHost("evil.com")}, nil},
		{"cross-site", "GET", "/", []opt{withHeader("Sec-Fetch-Site", "cross-site")}, nil},
		{"bad path", "GET", "/api/v1/sections/..%2fx", nil, nil},
		{"with foreign origin", "GET", "/api/v1/info", []opt{withHeader("Origin", "http://evil.com")}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := e.do(tt.method, tt.target, tt.body, tt.opts...)
			assertSecurityHeaders(t, rec.Header())
			if strings.HasPrefix(tt.target, "/api/") && rec.Header().Get("Cache-Control") != "no-store" {
				t.Errorf("API response without no-store: %q", rec.Header().Get("Cache-Control"))
			}
		})
	}
	// OPTIONS never succeeds.
	for _, target := range []string{"/", "/api/v1/info", "/auth", "/x"} {
		if rec := e.do("OPTIONS", target, nil); rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("OPTIONS %s: %d", target, rec.Code)
		}
	}
}

func TestPathTraversalInPaths(t *testing.T) {
	e := newEnv(t, false)
	before := snapshotDir(t, e.dir)
	targets := []string{
		"/api/v1/sections/../../etc/passwd",
		"/api/v1/sections/..%2f..%2fetc%2fpasswd",
		"/api/v1/sections/%2e%2e",
		"/api/v1/sections/%2e%2e%2f",
		"/api/v1/sections/..%5c..%5cwindows",
		"/api/v1/sections/..\\x",
		"/api/v1/sections/C:%5Cwindows",
		"/api/v1/sections/%2Fetc%2Fpasswd",
		"/api/v1/sections//etc/passwd",
		"/api/v1/sections/technical/",
		"/api/v1/sections/technical/../team",
		"/api/v1/sections/tech%00",
		"/api/v1/sections/%00",
		"/api/v1/sections/Technical",
		"/api/v1/sections/con",
		"/api/v1/topics/technical/..%2f..%2fbearings.yaml",
		"/api/v1/topics/technical/../../bearings.yaml",
		"/api/v1/topics/..%2fteam/rituals",
		"/api/v1/topics/technical/backtesting.md",
		"/api/v1/topics/%2e%2e/%2e%2e",
		"/api/v1/topics//risk",
		"/api/v1/journal/..%2f..%2fbearings",
		"/api/v1/journal/../../x",
		"/api/v1/journal/2026-11-03-0123456789",
		"/api/v1/journal/%2e%2e",
	}
	for _, target := range targets {
		for _, method := range []string{"GET", "DELETE", "PATCH"} {
			rec := e.do(method, target, map[string]any{"version": "0-00"})
			if rec.Code == 200 || rec.Code == 201 || rec.Code == 204 || rec.Code >= 500 {
				t.Errorf("%s %s: status %d %s", method, target, rec.Code, rec.Body.String())
			}
			body := rec.Body.String()
			if strings.Contains(body, "root:") || strings.Contains(body, e.dir) {
				t.Errorf("%s %s leaked: %s", method, target, body)
			}
		}
	}
	if after := snapshotDir(t, e.dir); after != before {
		t.Error("path traversal attempts changed the notebook")
	}
}

func TestPathTraversalInBodies(t *testing.T) {
	e := newEnv(t, false)
	parent := filepath.Dir(e.dir)
	before := snapshotDir(t, e.dir)
	bad := []string{"../x", "..", "a/b", `a\b`, "/etc/passwd", "C:\\x", "x/../y", "%2e%2e", "a\x00b", ".hidden", "UP"}
	var cases []struct {
		method, target string
		body           map[string]any
	}
	for _, v := range bad {
		cases = append(cases,
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/sections", map[string]any{"title": "T", "slug": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/sections/technical/move", map[string]any{"to": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/sections/technical/topics", map[string]any{"title": "T", "slug": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/topics/technical/risk/move", map[string]any{"to": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/topics/technical/risk/move", map[string]any{"to": "technical/" + v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/topics/technical/risk/move", map[string]any{"to": v + "/risk"}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/journal", map[string]any{"body": "x", "section": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/journal", map[string]any{"body": "x", "topic": v}},
			struct {
				method, target string
				body           map[string]any
			}{"POST", "/api/v1/journal", map[string]any{"body": "x", "date": v}},
		)
	}
	for _, c := range cases {
		rec := e.do(c.method, c.target, c.body)
		if rec.Code != 400 && rec.Code != 422 && !(rec.Code == 404 && strings.Contains(fmt.Sprint(c.body), "a/b")) {
			t.Errorf("%s %s %v: status %d %s", c.method, c.target, c.body, rec.Code, rec.Body.String())
		}
	}
	if after := snapshotDir(t, e.dir); after != before {
		t.Error("invalid names changed the notebook")
	}
	entries, _ := os.ReadDir(parent)
	if len(entries) != 1 {
		t.Errorf("files appeared next to the notebook: %v", entries)
	}
}

// The error responses never carry file contents, file names or absolute
// paths, even when notebook files are invalid.
func TestErrorsNeverLeakContent(t *testing.T) {
	const marker = "SECRETMARKER9f3a"
	dir, nb := buildNotebook(t)
	write := func(rel, content string) {
		p := filepath.Join(dir, filepath.FromSlash(rel))
		must(t, os.MkdirAll(filepath.Dir(p), 0o700))
		must(t, os.WriteFile(p, []byte(content), 0o600))
	}
	write("sections/technical/leaky.md", "---\ntitle: Leaky\n"+marker+"_key: 1\n---\nbody "+marker+"\n")
	write("sections/technical/wrongtype.md", "---\ntitle: "+marker+"\nstatus: "+marker+"\ntype: role\norder: x"+marker+"\ncreated: 2026-11-01T12:00:00Z\nupdated: 2026-11-01T12:00:00Z\n---\nbody "+marker+"\n")
	write("sections/technical/notutf8.md", "---\ntitle: a\n---\n\xff\xfe"+marker+"\n")
	write("sections/"+marker+"bad/_section.md", "not frontmatter "+marker)
	write("journal/2026/11/2026-11-03-zzzzzzzzzz.md", "---\ndate: nope "+marker+"\n---\n"+marker)
	write("journal/2026/11/2026-11-03-0123456789.md", "---\ndate: 2026-11-03\n"+marker+": 1\n---\n"+marker)
	e := newEnvFor(t, dir, nb, false)

	targets := []string{
		"/api/v1/info", "/api/v1/sections", "/api/v1/outline", "/api/v1/sections/technical", "/api/v1/sections/technical/topics",
		"/api/v1/topics/technical/leaky", "/api/v1/topics/technical/wrongtype", "/api/v1/topics/technical/notutf8",
		"/api/v1/topics/technical/missing", "/api/v1/journal", "/api/v1/journal/0123456789", "/api/v1/journal/zzzzzzzzzz",
		"/api/v1/role-path", "/api/v1/progress", "/api/v1/search?q=" + marker, "/api/v1/search?q=leaky",
		"/api/v1/search?q=%00", "/api/v1/nope", "/api/v1/sections/" + marker,
	}
	check := func(what, body string) {
		t.Helper()
		if strings.Contains(body, marker) {
			t.Errorf("%s leaked file content: %s", what, body)
		}
		if strings.Contains(body, dir) || strings.Contains(body, filepath.ToSlash(dir)) {
			t.Errorf("%s leaked an absolute path: %s", what, body)
		}
	}
	for _, target := range targets {
		rec := e.do("GET", target, nil)
		check("GET "+target, rec.Body.String())
		if rec.Code >= 400 {
			if errCode(t, rec) == "" {
				t.Errorf("GET %s: error without a code: %s", target, rec.Body.String())
			}
		}
	}
	// Writes that hit the invalid files.
	for _, c := range []struct {
		method, target string
		body           any
	}{
		{"PATCH", "/api/v1/topics/technical/leaky", map[string]any{"version": "1-" + strings.Repeat("0", 64), "title": "x"}},
		{"POST", "/api/v1/topics/technical/leaky/advance", nil},
		{"DELETE", "/api/v1/topics/technical/wrongtype", nil},
		{"POST", "/api/v1/topics/technical/leaky/move", map[string]any{"to": "team/leaky"}},
		{"DELETE", "/api/v1/sections/technical", nil},
		{"PATCH", "/api/v1/journal/0123456789", map[string]any{"version": "1-" + strings.Repeat("0", 64), "body": "x"}},
		{"DELETE", "/api/v1/journal/0123456789", nil},
		{"POST", "/api/v1/sections/technical/reorder", map[string]any{"delta": 1}},
	} {
		rec := e.do(c.method, c.target, c.body)
		check(c.method+" "+c.target, rec.Body.String())
	}
	check("logs", e.logs.String())
}

func TestStaticHasNoInlineCode(t *testing.T) {
	data := readFile(t, "webdist/index.html")
	for _, re := range []string{`(?i)<script(\s[^>]*)?>\s*[^<\s]`, `(?i)<style`, `(?i)\sstyle\s*=`, `(?i)\son[a-z]+\s*=`, `(?i)javascript:`} {
		if regexp.MustCompile(re).MatchString(data) {
			t.Errorf("index.html matches %s", re)
		}
	}
}
