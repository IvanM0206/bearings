package server

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"
)

// start runs the server on its real listener and returns a client that
// does not follow redirects.
func (e *testEnv) start(t *testing.T) (*http.Client, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- e.srv.Serve(ctx) }()
	t.Cleanup(cancel)
	c := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return c, cancel, done
}

func TestTokenFlow(t *testing.T) {
	e := newEnv(t, false)
	c, _, _ := e.start(t)
	base := e.srv.BaseURL()

	type reply struct {
		StatusCode int
		Header     http.Header
		cookies    []*http.Cookie
	}
	get := func(target string) reply {
		t.Helper()
		resp, err := c.Get(base + target)
		must(t, err)
		defer func() { _ = resp.Body.Close() }()
		_, _ = io.Copy(io.Discard, resp.Body)
		return reply{resp.StatusCode, resp.Header, resp.Cookies()}
	}

	// The printed URL carries the token on the dedicated path.
	if !strings.HasPrefix(e.srv.URL(), "http://127.0.0.1:"+strconv.Itoa(e.port)+"/auth?token=") {
		t.Fatalf("URL %q", e.srv.URL())
	}

	for _, tt := range []struct{ name, query string }{
		{"missing", ""},
		{"empty", "?token="},
		{"wrong", "?token=nope"},
		{"prefix of the right one", "?token=" + e.srv.token[:10]},
		{"right one plus a suffix", "?token=" + e.srv.token + "x"},
		{"in another parameter", "?t=" + e.srv.token},
	} {
		resp := get("/auth" + tt.query)
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s: status %d", tt.name, resp.StatusCode)
		}
		if resp.Header.Get("Set-Cookie") != "" {
			t.Errorf("%s: cookie set", tt.name)
		}
	}

	resp := get("/auth?token=" + e.srv.token)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/" {
		t.Errorf("Location %q: the token must not stay in the URL", loc)
	}
	cs := resp.cookies
	if len(cs) != 1 {
		t.Fatalf("cookies: %v", cs)
	}
	ck := cs[0]
	if ck.Name != "bearings_session_"+strconv.Itoa(e.port) {
		t.Errorf("cookie name %q lacks the port", ck.Name)
	}
	if !ck.HttpOnly || ck.SameSite != http.SameSiteStrictMode || ck.Path != "/" {
		t.Errorf("cookie attributes: %+v", ck)
	}
	if !ck.Expires.IsZero() || ck.MaxAge != 0 {
		t.Errorf("cookie is not a session cookie: %+v", ck)
	}
	if !strings.Contains(resp.Header.Get("Set-Cookie"), "SameSite=Strict") || !strings.Contains(resp.Header.Get("Set-Cookie"), "HttpOnly") {
		t.Errorf("Set-Cookie %q", resp.Header.Get("Set-Cookie"))
	}
	if resp.Header.Get("Cache-Control") != "no-store" {
		t.Error("the auth response must not be cached")
	}
	if ck.Domain != "" {
		t.Errorf("cookie has a Domain: %q", ck.Domain)
	}

	// The cookie opens the API; without it the API is closed.
	if resp := get("/api/v1/info"); resp.StatusCode != 401 {
		t.Errorf("no cookie: %d", resp.StatusCode)
	}
	req, _ := http.NewRequest("GET", base+"/api/v1/info", nil)
	req.AddCookie(ck)
	resp2, err := c.Do(req)
	must(t, err)
	body, _ := io.ReadAll(resp2.Body)
	_ = resp2.Body.Close()
	if resp2.StatusCode != 200 || !strings.Contains(string(body), "Example role") {
		t.Errorf("with cookie: %d %s", resp2.StatusCode, body)
	}

	// POST to /auth is refused.
	pr, err := c.Post(base+"/auth?token="+e.srv.token, "application/json", strings.NewReader("{}"))
	must(t, err)
	_ = pr.Body.Close()
	if pr.StatusCode != 405 {
		t.Errorf("POST /auth: %d", pr.StatusCode)
	}
}

// Following the redirect with a cookie jar, as a browser does, ends on the
// page without the token and with a working session.
func TestBrowserLikeSession(t *testing.T) {
	e := newEnv(t, false)
	_, _, _ = e.start(t)
	jar, err := cookiejar.New(nil)
	must(t, err)
	c := &http.Client{Jar: jar, Timeout: 5 * time.Second}
	resp, err := c.Get(e.srv.URL())
	must(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || resp.Request.URL.RawQuery != "" || resp.Request.URL.Path != "/" {
		t.Fatalf("landed on %s (%d)", resp.Request.URL, resp.StatusCode)
	}
	if !strings.Contains(string(body), "Bearings") {
		t.Errorf("body %q", body)
	}
	resp, err = c.Get(e.srv.BaseURL() + "/api/v1/info")
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Errorf("API after sign-in: %d", resp.StatusCode)
	}
	// A mutation through the real server needs the origin too.
	req, _ := http.NewRequest("POST", e.srv.BaseURL()+"/api/v1/sections", strings.NewReader(`{"title":"Real"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err = c.Do(req)
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Errorf("mutation without Origin: %d", resp.StatusCode)
	}
	req, _ = http.NewRequest("POST", e.srv.BaseURL()+"/api/v1/sections", strings.NewReader(`{"title":"Real"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", e.origin())
	resp, err = c.Do(req)
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != 201 {
		t.Errorf("mutation with Origin: %d", resp.StatusCode)
	}
	// A request to the real listener with a foreign Host is refused.
	req, _ = http.NewRequest("GET", e.srv.BaseURL()+"/api/v1/info", nil)
	req.Host = "evil.example"
	resp, err = c.Do(req)
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMisdirectedRequest {
		t.Errorf("foreign host on a real connection: %d", resp.StatusCode)
	}
}

func TestTwoInstancesHaveDifferentCookies(t *testing.T) {
	a, b := newEnv(t, false), newEnv(t, false)
	if a.srv.cookie == b.srv.cookie {
		t.Errorf("both use %q", a.srv.cookie)
	}
	// With the same fixed random source the tokens match, but a cookie of
	// one is never read as a session of the other.
	r := a.newReq("GET", "/api/v1/info", nil)
	r.Header.Del("Cookie")
	r.AddCookie(&http.Cookie{Name: b.srv.cookie, Value: a.srv.token}) //nolint:gosec // G124: test request cookie
	if rec := a.serve(r); rec.Code != 401 {
		t.Errorf("foreign cookie accepted: %d", rec.Code)
	}
}

func TestTokenGeneration(t *testing.T) {
	_, nb := buildNotebook(t)
	mk := func() *Server {
		ln, err := Listen(0)
		must(t, err)
		t.Cleanup(func() { _ = ln.Close() })
		s, err := New(ln, Config{Notebook: nb})
		must(t, err)
		return s
	}
	a, b := mk(), mk()
	if a.token == b.token {
		t.Error("two servers share a token")
	}
	// 32 random bytes in base64url without padding: 43 characters.
	if len(a.token) < 43 || strings.ContainsAny(a.token, "+/=") {
		t.Errorf("token %q is not 32 bytes of base64url", a.token)
	}
	if _, err := url.Parse(a.URL()); err != nil {
		t.Error(err)
	}
	// A failing random source is an error, never a weak token.
	ln, err := Listen(0)
	must(t, err)
	defer func() { _ = ln.Close() }()
	if _, err := New(ln, Config{Notebook: nb, Rand: strings.NewReader("short")}); err == nil {
		t.Error("short random source accepted")
	}
	if _, err := New(ln, Config{}); err == nil {
		t.Error("missing notebook accepted")
	}
}

func TestTokenEqual(t *testing.T) {
	for _, tt := range []struct {
		a, b string
		want bool
	}{
		{"abc", "abc", true},
		{"abc", "abd", false},
		{"abc", "abcd", false},
		{"", "", true},
		{"", "a", false},
	} {
		if got := tokenEqual(tt.a, tt.b); got != tt.want {
			t.Errorf("tokenEqual(%q, %q) = %v", tt.a, tt.b, got)
		}
	}
}

// Tokens are compared with crypto/subtle, never with == or bytes.Equal.
func TestTokensAreComparedInConstantTime(t *testing.T) {
	src := readFile(t, "server.go")
	if !strings.Contains(src, "subtle.ConstantTimeCompare") {
		t.Fatal("server.go does not use subtle.ConstantTimeCompare")
	}
	for _, f := range []string{"middleware.go", "server.go", "api.go"} {
		for i, line := range strings.Split(readFile(t, f), "\n") {
			if strings.Contains(line, "s.token") && (strings.Contains(line, "== s.token") || strings.Contains(line, "s.token ==") || strings.Contains(line, "s.token !=")) {
				t.Errorf("%s:%d compares the token with ==: %s", f, i+1, line)
			}
		}
	}
	entries, err := os.ReadDir(".")
	must(t, err)
	for _, en := range entries {
		if strings.HasSuffix(en.Name(), ".go") && !strings.HasSuffix(en.Name(), "_test.go") {
			if strings.Contains(readFile(t, en.Name()), "bytes.Equal") {
				t.Errorf("%s uses bytes.Equal", en.Name())
			}
		}
	}
}

func TestGracefulShutdown(t *testing.T) {
	e := newEnv(t, false)
	c, cancel, done := e.start(t)
	resp, err := c.Get(e.srv.BaseURL() + "/")
	must(t, err)
	_ = resp.Body.Close()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("Serve returned %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Serve did not stop")
	}
	if resp, err := c.Get(e.srv.BaseURL() + "/"); err == nil {
		_ = resp.Body.Close()
		t.Error("server still answers after shutdown")
	}
}
