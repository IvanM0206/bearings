package cmd

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// updateGolden rewrites the golden files instead of comparing with them:
//
//	go test ./internal/cmd -run Golden -update
var updateGolden = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// countReader is a deterministic source for entry ids: every run of the
// fixture gets the same ids in the same order.
type countReader struct{ n byte }

func (r *countReader) Read(p []byte) (int, error) {
	for i := range p {
		r.n++
		p[i] = r.n*7 + 3
	}
	return len(p), nil
}

// goldenFixture is the fixture notebook with a fixed clock and fixed ids.
func goldenFixture(t *testing.T) string {
	t.Helper()
	return buildFixture(t, &notebook.Options{
		Now:  func() time.Time { return fixtureNow },
		Rand: &countReader{},
	})
}

// goldenCase is one command whose output is pinned in each language.
type goldenCase struct {
	name string
	args []string
}

var goldenCases = []goldenCase{
	{"ls", []string{"ls"}},
	{"ls_section", []string{"ls", "technical"}},
	{"show", []string{"show", "technical/backtesting"}},
	{"search", []string{"search", "simulator"}},
	{"journal", []string{"journal"}},
	{"progress", []string{"progress"}},
	{"path", []string{"path"}},
	{"path_md", []string{"path", "--md"}},
	{"error_bad_ref", []string{"show", "backtesting"}},
}

// goldenRun runs the CLI in memory with a fixed clock and no terminal, so
// that the output has no colors and no wrapping.
func goldenRun(t *testing.T, dir, lang string, args []string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"BEARINGS_NOTEBOOK": dir}
	code := Execute(append([]string{"--lang", lang}, args...), Env{
		Stdout:      &out,
		Stderr:      &errb,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: func() (string, error) { return "", os.ErrNotExist },
		Now:         func() time.Time { return fixtureNow },
	})
	return code, out.String(), errb.String()
}

// goldenText is the pinned form of one run: the exit code, then stdout and
// stderr, each under its own heading.
func goldenText(code int, stdout, stderr string) string {
	return "exit " + strconv.Itoa(code) + "\n--- stdout\n" + stdout + "--- stderr\n" + stderr
}

func TestGolden(t *testing.T) {
	dir := goldenFixture(t)
	for _, lang := range []string{"en", "es"} {
		for _, c := range goldenCases {
			t.Run(lang+"/"+c.name, func(t *testing.T) {
				code, stdout, stderr := goldenRun(t, dir, lang, c.args)
				got := goldenText(code, stdout, stderr)
				path := filepath.Join("testdata", "golden", lang, c.name+".golden")
				if *updateGolden {
					must(t, os.MkdirAll(filepath.Dir(path), 0o755))
					must(t, os.WriteFile(path, []byte(got), 0o644))
					return
				}
				want, err := os.ReadFile(path)
				if err != nil {
					t.Fatalf("%v (run: go test ./internal/cmd -run Golden -update)", err)
				}
				if string(want) != got {
					t.Errorf("output differs from %s\n--- want\n%s\n--- got\n%s", path, want, got)
				}
			})
		}
	}
}

// TestGoldenErrorsAreOneLine checks the pinned error case: it fails with a
// usage error, writes nothing to stdout, and names the problem on stderr.
func TestGoldenErrorsAreOneLine(t *testing.T) {
	dir := goldenFixture(t)
	for _, lang := range []string{"en", "es"} {
		code, stdout, stderr := goldenRun(t, dir, lang, []string{"show", "backtesting"})
		if code != ExitUsage || stdout != "" {
			t.Errorf("%s: exit %d, stdout %q", lang, code, stdout)
		}
		if n := bytes.Count([]byte(stderr), []byte("\n")); n < 2 {
			t.Errorf("%s: expected the message and the usage hint, got %q", lang, stderr)
		}
	}
}

// TestGoldenOutputHasNoEscapes checks that the pinned output carries no
// terminal control sequences, whatever the environment says.
func TestGoldenOutputHasNoEscapes(t *testing.T) {
	dir := goldenFixture(t)
	for _, c := range goldenCases {
		_, stdout, stderr := goldenRun(t, dir, "es", c.args)
		for _, s := range []string{stdout, stderr} {
			if bytes.ContainsRune([]byte(s), 0x1b) {
				t.Errorf("%s: escape sequence in output %q", c.name, s)
			}
		}
	}
}
