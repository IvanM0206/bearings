package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func TestInitBlank(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "new", "nb")
	r := runIn(t, dir, "init", "--name", "My role", "--language", "es")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Created a notebook in ") {
		t.Errorf("stdout %q", r.stdout)
	}
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	cfg, err := nb.Config()
	must(t, err)
	if cfg.Name != "My role" || cfg.Language != "es" || cfg.Template != nil {
		t.Errorf("%+v", cfg)
	}
	secs, _ := nb.Sections()
	if len(secs) != 0 {
		t.Errorf("a blank notebook has no sections: %v", secs)
	}
}

func TestInitBlankDefaultsToUILanguage(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir, "BEARINGS_LANG": "es"}}, "init")
	wantOK(t, r)
	nb, _ := notebook.Open(dir, nil)
	cfg, _ := nb.Config()
	if cfg.Language != "es" || cfg.Name != "" {
		t.Errorf("%+v", cfg)
	}
}

func TestInitFromTemplate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	r := runIn(t, dir, "init", "--template", "trading-bots-developer", "--name", "Me")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Template trading-bots-developer: 4 sections, 13 topics.") {
		t.Errorf("stdout %q", r.stdout)
	}
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	cfg, _ := nb.Config()
	if cfg.Template != nil || cfg.Name != "Me" || cfg.Language != "en" {
		t.Errorf("%+v", cfg)
	}
	prog, err := nb.Progress()
	must(t, err)
	if prog.All.Total != 13 || prog.Suggested != 13 {
		t.Errorf("%+v", prog)
	}
	entries, _ := nb.Journal(notebook.JournalFilter{})
	if len(entries) != 0 {
		t.Errorf("a new notebook has no journal entries")
	}

	// The Spanish template keeps its language unless --language says otherwise.
	dir2 := filepath.Join(t.TempDir(), "nb")
	wantOK(t, runIn(t, dir2, "init", "--template", "trading-bots-developer-es"))
	nb2, _ := notebook.Open(dir2, nil)
	if c, _ := nb2.Config(); c.Language != "es" {
		t.Errorf("%+v", c)
	}
	dir3 := filepath.Join(t.TempDir(), "nb")
	wantOK(t, runIn(t, dir3, "init", "--template", "trading-bots-developer-es", "--language", "en"))
	nb3, _ := notebook.Open(dir3, nil)
	if c, _ := nb3.Config(); c.Language != "en" {
		t.Errorf("%+v", c)
	}
}

func TestInitJSON(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	r := runIn(t, dir, "init", "--template", "trading-bots-developer", "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if !reflect.DeepEqual(keys(j), []string{"language", "name", "path", "sections", "template", "topics"}) {
		t.Fatalf("keys %v", keys(j))
	}
	if j["template"] != "trading-bots-developer" || j["sections"] != 4.0 || j["topics"] != 13.0 {
		t.Errorf("%v", j)
	}
}

func TestInitRefusesNonEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	keep := filepath.Join(dir, "notes.txt")
	must(t, os.WriteFile(keep, []byte("mine"), 0o600))
	r := runIn(t, dir, "init")
	wantFail(t, r, ExitUsage, "is not empty")
	if b, _ := os.ReadFile(keep); string(b) != "mine" {
		t.Error("existing file touched")
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("init left files behind: %v", entries)
	}
	// An existing notebook is not overwritten either.
	nbDir := newFixture(t)
	wantFail(t, runIn(t, nbDir, "init"), ExitUsage, "is not empty")
	wantOK(t, runIn(t, nbDir, "ls"))
}

func TestInitAcceptsEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	wantOK(t, runIn(t, dir, "init"))
	if _, err := notebook.Open(dir, nil); err != nil {
		t.Fatal(err)
	}
}

func TestInitRefusesFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	must(t, os.WriteFile(f, []byte("x"), 0o600))
	wantFail(t, runIn(t, f, "init"), ExitUsage, "is not empty")
}

func TestInitModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX modes")
	}
	dir := filepath.Join(t.TempDir(), "nb")
	wantOK(t, runIn(t, dir, "init", "--template", "trading-bots-developer"))
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		must(t, err)
		fi, _ := d.Info()
		want := os.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", p, fi.Mode().Perm(), want)
		}
		return nil
	})
}

func TestInitErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"unknown template", []string{"--template", "nope"}, `unknown template "nope" (available: trading-bots-developer, trading-bots-developer-es)`},
		{"path-like template", []string{"--template", "../../etc"}, "unknown template"},
		{"unknown language", []string{"--language", "fr"}, "unknown language"},
		{"empty name", []string{"--name", "  "}, "--name must not be empty"},
		{"name too long", []string{"--name", strings.Repeat("x", 81)}, "too long"},
		{"name with control character", []string{"--name", "a\x07b"}, "control characters"},
		{"positional argument", []string{"extra"}, "unknown command"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := filepath.Join(t.TempDir(), "nb")
			r := runIn(t, dir, append([]string{"init"}, tt.args...)...)
			wantFail(t, r, ExitUsage, tt.want)
			if _, err := os.Lstat(dir); err == nil {
				t.Error("a failed init must not create the directory")
			}
		})
	}
}

func TestListTemplates(t *testing.T) {
	r := runIn(t, "/nonexistent", "init", "--list-templates")
	wantOK(t, r)
	for _, want := range []string{"ID", "trading-bots-developer", "trading-bots-developer-es", "es"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("%q not in\n%s", want, r.stdout)
		}
	}
	j := decode(t, runIn(t, "/nonexistent", "init", "--list-templates", "--json").stdout)
	ts := j["templates"].([]any)
	if len(ts) != 2 || ts[0].(map[string]any)["id"] != "trading-bots-developer" {
		t.Errorf("%v", ts)
	}
}
