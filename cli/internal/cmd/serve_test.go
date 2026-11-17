package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// serveRun starts `bearings serve` in memory and returns once the address
// has been printed.
type serveRun struct {
	cancel context.CancelFunc
	done   chan int
	out    *lockedBuffer
	errb   *lockedBuffer
	opened chan string
}

func startServe(t *testing.T, dir, lang string, args ...string) *serveRun {
	t.Helper()
	return startServeWith(t, dir, lang, nil, args...)
}

func startServeWith(t *testing.T, dir, lang string, openErr error, args ...string) *serveRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	r := &serveRun{cancel: cancel, done: make(chan int, 1), out: &lockedBuffer{}, errb: &lockedBuffer{}, opened: make(chan string, 4)}
	var once sync.Once
	go func() {
		code := Execute(append([]string{"--lang", lang, "serve", "--notebook", dir}, args...), Env{
			Stdout: r.out, Stderr: r.errb, Context: ctx,
			Getenv: func(string) string { return "" },
			Rand:   bytes.NewReader(bytes.Repeat([]byte{7}, 32)),
			OpenURL: func(u string) error {
				r.opened <- u
				return openErr
			},
		})
		once.Do(func() { r.done <- code })
	}()
	t.Cleanup(cancel)
	deadline := time.Now().Add(5 * time.Second)
	for !strings.Contains(r.out.String(), "\n") {
		if time.Now().After(deadline) {
			select {
			case code := <-r.done:
				t.Fatalf("serve exited with %d: %s", code, r.errb.String())
			default:
			}
			t.Fatal("serve did not print an address")
		}
		time.Sleep(5 * time.Millisecond)
	}
	return r
}

func (r *serveRun) stop(t *testing.T) int {
	t.Helper()
	r.cancel()
	select {
	case code := <-r.done:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("serve did not stop")
		return -1
	}
}

var urlRe = regexp.MustCompile(`^http://127\.0\.0\.1:(\d+)/auth\?token=([A-Za-z0-9_-]{43})\n$`)

func TestServeCommand(t *testing.T) {
	dir := newFixture(t)
	r := startServe(t, dir, "en")
	m := urlRe.FindStringSubmatch(r.out.String())
	if m == nil {
		t.Fatalf("stdout %q is not the address", r.out.String())
	}
	url := strings.TrimSuffix(r.out.String(), "\n")
	select {
	case got := <-r.opened:
		if got != url {
			t.Errorf("opened %q, printed %q", got, url)
		}
	case <-time.After(2 * time.Second):
		t.Error("the browser was not opened")
	}
	// The server answers: the token logs in and the API shows the notebook.
	c := &http.Client{Transport: &http.Transport{DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }, Timeout: 5 * time.Second}
	resp, err := c.Get(url)
	must(t, err)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther || len(resp.Cookies()) != 1 {
		t.Fatalf("auth: %d %v", resp.StatusCode, resp.Cookies())
	}
	req, _ := http.NewRequest("GET", "http://127.0.0.1:"+m[1]+"/api/v1/info", nil)
	req.AddCookie(resp.Cookies()[0])
	resp, err = c.Do(req)
	must(t, err)
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != 200 || !strings.Contains(string(body), `"read_only":false`) {
		t.Errorf("info: %d %s", resp.StatusCode, body)
	}
	if code := r.stop(t); code != ExitOK {
		t.Errorf("exit %d: %s", code, r.errb.String())
	}
	if !strings.Contains(r.errb.String(), "Server stopped.") {
		t.Errorf("stderr: %q", r.errb.String())
	}
}

func TestServeOutputInBothLanguages(t *testing.T) {
	dir := goldenFixture(t)
	for _, lang := range []string{"en", "es"} {
		t.Run(lang, func(t *testing.T) {
			r := startServe(t, dir, lang, "--read-only", "--no-open")
			code := r.stop(t)
			out := urlRe.ReplaceAllString(r.out.String(), "http://127.0.0.1:PORT/auth?token=TOKEN\n")
			errText := r.errb.String()
			got := goldenText(code, out, errText)
			path := filepath.Join("testdata", "golden", lang, "serve.golden")
			if *updateGolden {
				must(t, os.MkdirAll(filepath.Dir(path), 0o755))
				must(t, os.WriteFile(path, []byte(got), 0o644))
				return
			}
			want, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("%v (run: go test ./internal/cmd -run Serve -update)", err)
			}
			if string(want) != got {
				t.Errorf("output differs from %s\n--- want\n%s\n--- got\n%s", path, want, got)
			}
			select {
			case u := <-r.opened:
				t.Errorf("--no-open still opened %q", u)
			default:
			}
		})
	}
}

// A browser that cannot be opened does not stop the server: the address is
// printed anyway and a hint says so.
func TestServeOpenFailureIsNotFatal(t *testing.T) {
	for _, lang := range []string{"en", "es"} {
		r := startServeWith(t, newFixture(t), lang, errors.New("no browser"))
		if !urlRe.MatchString(r.out.String()) {
			t.Errorf("address not printed: %q", r.out.String())
		}
		deadline := time.Now().Add(2 * time.Second)
		want := map[string]string{"en": "Could not open the browser", "es": "No se pudo abrir el navegador"}[lang]
		for !strings.Contains(r.errb.String(), want) && time.Now().Before(deadline) {
			time.Sleep(5 * time.Millisecond)
		}
		if !strings.Contains(r.errb.String(), want) {
			t.Errorf("%s: stderr %q", lang, r.errb.String())
		}
		if code := r.stop(t); code != ExitOK {
			t.Errorf("exit %d", code)
		}
	}
}

func TestServeErrors(t *testing.T) {
	dir := newFixture(t)
	empty := t.TempDir()
	tests := []struct {
		name string
		args []string
		code int
	}{
		{"port not a number", []string{"serve", "--port", "abc", "--notebook", dir}, ExitUsage},
		{"port negative", []string{"serve", "--port", "-1", "--notebook", dir}, ExitUsage},
		{"port too big", []string{"serve", "--port", "70000", "--notebook", dir}, ExitUsage},
		{"no notebook", []string{"serve", "--notebook", empty}, ExitNotebook},
		{"extra argument", []string{"serve", "extra", "--notebook", dir}, ExitUsage},
		{"no flag to change the address", []string{"serve", "--host", "0.0.0.0", "--notebook", dir}, ExitUsage},
		{"no flag to change the address (addr)", []string{"serve", "--addr", "0.0.0.0:80", "--notebook", dir}, ExitUsage},
	}
	for _, tt := range tests {
		for _, lang := range []string{"en", "es"} {
			res := runCLIArgs(t, append([]string{"--lang", lang}, tt.args...))
			if res.code != tt.code {
				t.Errorf("%s (%s): exit %d, want %d: %s", tt.name, lang, res.code, tt.code, res.stderr)
			}
			if res.stdout != "" {
				t.Errorf("%s: stdout %q", tt.name, res.stdout)
			}
		}
	}
}

func TestServePortInUse(t *testing.T) {
	dir := newFixture(t)
	r := startServe(t, dir, "en", "--no-open")
	m := urlRe.FindStringSubmatch(r.out.String())
	res := runCLIArgs(t, []string{"serve", "--no-open", "--port", m[1], "--notebook", dir})
	if res.code != ExitFailure || !strings.Contains(res.stderr, "cannot listen on 127.0.0.1") {
		t.Errorf("exit %d: %s", res.code, res.stderr)
	}
	r.stop(t)
}

func runCLIArgs(t *testing.T, args []string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, Env{Stdout: &out, Stderr: &errb, Getenv: func(string) string { return "" }, OpenURL: func(string) error { return nil }})
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}
