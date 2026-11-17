package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

var fixtureNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

// fixedRand yields the same 32 bytes every time: a fixed token for tests.
type fixedRand struct{}

func (fixedRand) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i*7 + 1)
	}
	return len(p), nil
}

// logBuffer collects log output safely.
type logBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (l *logBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.Write(p)
}

func (l *logBuffer) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.b.String()
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func intp(n int) *int { return &n }

// buildNotebook makes a small invented notebook:
//
//	technical: backtesting (role, in-progress), risk (role, mastered), scratch (personal, pending)
//	team:      rituals (role, pending)
//	journal:   2026-11-03 x2, 2026-10-20
func buildNotebook(t *testing.T) (string, *notebook.Notebook) {
	t.Helper()
	tmp, err := filepath.EvalSymlinks(t.TempDir())
	must(t, err)
	dir := filepath.Join(tmp, "nb")
	nb, err := notebook.Init(dir, notebook.Config{Format: 1, Name: "Example role", Language: "en"},
		&notebook.Options{Now: func() time.Time { return fixtureNow }})
	must(t, err)
	_, err = nb.AddSection(notebook.NewSection{Slug: "technical", Title: "Technical", Order: intp(0), Description: "Systems and tools."})
	must(t, err)
	_, err = nb.AddSection(notebook.NewSection{Slug: "team", Title: "Team", Order: intp(1)})
	must(t, err)
	for _, in := range []notebook.NewTopic{
		{Section: "technical", Slug: "backtesting", Title: "Backtesting", Status: notebook.StatusInProgress, Type: notebook.TypeRole, Order: intp(0),
			Notes: "Download the sample data first.", Resources: []notebook.Resource{{URL: "https://example.com/wiki", Title: "Guide"}}},
		{Section: "technical", Slug: "risk", Title: "Risk limits", Status: notebook.StatusMastered, Type: notebook.TypeRole, Order: intp(1)},
		{Section: "technical", Slug: "scratch", Title: "My shortcuts", Status: notebook.StatusPending, Type: notebook.TypePersonal, Suggested: true, Order: intp(2)},
		{Section: "team", Slug: "rituals", Title: "Team rituals", Status: notebook.StatusPending, Type: notebook.TypeRole, Order: intp(0)},
	} {
		_, err := nb.AddTopic(in)
		must(t, err)
	}
	d1, _ := notebook.ParseDate("2026-11-03")
	d0, _ := notebook.ParseDate("2026-10-20")
	for _, e := range []notebook.NewEntry{
		{Date: d1, Topic: notebook.TopicRef{Section: "technical", Topic: "backtesting"}, Body: "The simulator skips days without data."},
		{Date: d1, Section: "team", Body: "Standup is at ten."},
		{Date: d0, Body: "First day: got access to the wiki."},
	} {
		_, err := nb.AddEntry(e)
		must(t, err)
	}
	return dir, nb
}

// testEnv is a server on a real loopback listener, driven mostly through
// its handler.
type testEnv struct {
	t    *testing.T
	dir  string
	nb   *notebook.Notebook
	srv  *Server
	logs *logBuffer
	h    http.Handler
	port int
}

func newEnv(t *testing.T, readOnly bool) *testEnv {
	t.Helper()
	dir, nb := buildNotebook(t)
	return newEnvFor(t, dir, nb, readOnly)
}

func newEnvFor(t *testing.T, dir string, nb *notebook.Notebook, readOnly bool) *testEnv {
	t.Helper()
	ln, err := Listen(0)
	must(t, err)
	t.Cleanup(func() { _ = ln.Close() })
	logs := &logBuffer{}
	srv, err := New(ln, Config{Notebook: nb, ReadOnly: readOnly, Rand: fixedRand{}, Logger: newTestLogger(logs)})
	must(t, err)
	return &testEnv{t: t, dir: dir, nb: nb, srv: srv, logs: logs, h: srv.Handler(), port: srv.Port()}
}

func (e *testEnv) host() string   { return fmt.Sprintf("127.0.0.1:%d", e.port) }
func (e *testEnv) origin() string { return fmt.Sprintf("http://127.0.0.1:%d", e.port) }

// opt changes a request before it is sent.
type opt func(*http.Request)

func withHost(h string) opt         { return func(r *http.Request) { r.Host = h } }
func withHeader(k, v string) opt    { return func(r *http.Request) { r.Header.Set(k, v) } }
func withoutHeader(k string) opt    { return func(r *http.Request) { r.Header.Del(k) } }
func withoutCookie() opt            { return withoutHeader("Cookie") }
func withAccept(a string) opt       { return withHeader("Accept", a) }
func withType(ct string) opt        { return withHeader("Content-Type", ct) }
func withContentLength(n int64) opt { return func(r *http.Request) { r.ContentLength = n } }

// newReq builds an authenticated, same-origin request. body may be nil, a
// string or a value to encode as JSON.
func (e *testEnv) newReq(method, target string, body any, opts ...opt) *http.Request {
	e.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	case []byte:
		rd = bytes.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		must(e.t, err)
		rd = bytes.NewReader(raw)
	}
	r := httptest.NewRequest(method, target, rd)
	r.Host = e.host()
	r.AddCookie(&http.Cookie{Name: e.srv.cookie, Value: e.srv.token}) //nolint:gosec // G124: test request cookie
	if rd != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if isMutation(method) {
		r.Header.Set("Origin", e.origin())
	}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (e *testEnv) do(method, target string, body any, opts ...opt) *httptest.ResponseRecorder {
	e.t.Helper()
	return e.serve(e.newReq(method, target, body, opts...))
}

func (e *testEnv) serve(r *http.Request) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.h.ServeHTTP(rec, r)
	return rec
}

// json runs a request, checks the status and decodes the JSON object.
func (e *testEnv) json(method, target string, body any, want int) map[string]any {
	e.t.Helper()
	rec := e.do(method, target, body)
	if rec.Code != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, target, rec.Code, want, rec.Body.String())
	}
	var m map[string]any
	if rec.Body.Len() > 0 {
		if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
			e.t.Fatalf("%s %s: not JSON: %v\n%s", method, target, err, rec.Body.String())
		}
	}
	return m
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var b errorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &b); err != nil {
		t.Fatalf("not an error body: %v\n%s", err, rec.Body.String())
	}
	return b.Error.Code
}

func str(m map[string]any, k string) string {
	s, _ := m[k].(string)
	return s
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	return string(b)
}

func newTestLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func netListenAll() (net.Listener, error) {
	return net.Listen("tcp4", "0.0.0.0:0") //nolint:gosec // G102: the test checks that New refuses such a listener
}

func jsonUnmarshal(b []byte, v any) error { return json.Unmarshal(b, v) }
