package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// result is the outcome of one CLI invocation.
type result struct {
	stdout, stderr string
	code           int
}

// runCLI runs the CLI in memory with the given environment variables.
func runCLI(t *testing.T, env map[string]string, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	code := Execute(args, Env{
		Stdout: &out,
		Stderr: &errb,
		Getenv: func(k string) string { return env[k] },
		UserHomeDir: func() (string, error) {
			if h, ok := env["HOME"]; ok {
				return h, nil
			}
			return "", os.ErrNotExist
		},
	})
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// run runs the CLI against the notebook in dir with a clean environment.
func run(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return runCLI(t, map[string]string{"BEARINGS_NOTEBOOK": dir}, args...)
}

func intp(n int) *int { return &n }

var fixtureNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

// newFixture builds a small invented notebook in a temp dir:
//
//	technical: backtesting (role, in-progress), risk (role, mastered), scratch (personal, pending, suggested)
//	team:      rituals (role, pending), who-is-who (role, mastered)
//	journal:   2026-11-03 x2 (one linked to technical/backtesting, one to team),
//	           2026-10-20 (unlinked)
func newFixture(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "nb")
	nb, err := notebook.Init(dir, notebook.Config{Format: 1, Name: "Example role", Language: "en"},
		&notebook.Options{Now: func() time.Time { return fixtureNow }})
	must(t, err)
	_, err = nb.AddSection(notebook.NewSection{Slug: "technical", Title: "Technical", Order: intp(0), Description: "Systems and tools."})
	must(t, err)
	_, err = nb.AddSection(notebook.NewSection{Slug: "team", Title: "Team", Order: intp(1)})
	must(t, err)
	add := func(in notebook.NewTopic) {
		t.Helper()
		_, err := nb.AddTopic(in)
		must(t, err)
	}
	add(notebook.NewTopic{Section: "technical", Slug: "backtesting", Title: "Backtesting", Status: notebook.StatusInProgress, Type: notebook.TypeRole,
		Order: intp(0), Notes: "Download the sample data first.\nThen run the simulator.",
		Resources: []notebook.Resource{{URL: "https://example.com/wiki/backtesting", Title: "Backtesting guide"}, {URL: "https://example.com/plain"}}})
	add(notebook.NewTopic{Section: "technical", Slug: "risk", Title: "Risk limits", Status: notebook.StatusMastered, Type: notebook.TypeRole, Order: intp(1), Notes: "Limits are reviewed weekly."})
	add(notebook.NewTopic{Section: "technical", Slug: "scratch", Title: "My private shortcuts", Status: notebook.StatusPending, Type: notebook.TypePersonal, Suggested: true, Order: intp(2), Notes: "Secret handshake."})
	add(notebook.NewTopic{Section: "team", Slug: "rituals", Title: "Team rituals", Status: notebook.StatusPending, Type: notebook.TypeRole, Order: intp(0)})
	add(notebook.NewTopic{Section: "team", Slug: "who-is-who", Title: "Who is who", Status: notebook.StatusMastered, Type: notebook.TypeRole, Order: intp(1), Notes: "See the org chart."})

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
	return dir
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// decode unmarshals JSON output into a generic value.
func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("output is not a JSON object: %v\n%s", err, s)
	}
	return m
}

// keys returns the sorted key set of a JSON object, to pin down its shape.
func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}
