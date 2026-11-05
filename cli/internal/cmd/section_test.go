package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

var laterNow = time.Date(2026, 11, 4, 8, 30, 0, 0, time.UTC)

// at runs the CLI on dir with the clock set to laterNow.
func at(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, now: laterNow}, args...)
}

func sectionSlugs(t *testing.T, dir string) []string {
	t.Helper()
	j := decode(t, runIn(t, dir, "ls", "--json").stdout)
	var out []string
	for _, s := range j["sections"].([]any) {
		out = append(out, s.(map[string]any)["slug"].(string))
	}
	return out
}

func readFile(t *testing.T, dir, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
	must(t, err)
	return string(b)
}

func TestSectionAdd(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "section", "add", "Internal Processes", "--description", "How work gets done.")
	wantOK(t, r)
	if r.stdout != "Added section internal-processes (Internal Processes).\n" {
		t.Errorf("stdout %q", r.stdout)
	}
	if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"technical", "team", "internal-processes"}) {
		t.Errorf("new sections go last: %v", got)
	}
	f := readFile(t, dir, "sections/internal-processes/_section.md")
	for _, want := range []string{"order: 2\n", "created: 2026-11-04T08:30:00Z\n", "updated: 2026-11-04T08:30:00Z\n", "How work gets done."} {
		if !strings.Contains(f, want) {
			t.Errorf("%q not in\n%s", want, f)
		}
	}
	if runtime.GOOS != "windows" {
		for _, p := range []string{"sections/internal-processes", "sections/internal-processes/_section.md"} {
			fi, _ := os.Stat(filepath.Join(dir, p))
			want := os.FileMode(0o600)
			if fi.IsDir() {
				want = 0o700
			}
			if fi.Mode().Perm() != want {
				t.Errorf("%s: mode %v, want %v", p, fi.Mode().Perm(), want)
			}
		}
	}
	// The same title again gets a distinct name.
	wantOK(t, at(t, dir, "section", "add", "Internal Processes"))
	if got := sectionSlugs(t, dir); got[len(got)-1] != "internal-processes-2" {
		t.Errorf("%v", got)
	}
	// Words without quotes make one title.
	wantOK(t, at(t, dir, "section", "add", "Customer", "support"))
	if got := sectionSlugs(t, dir); got[len(got)-1] != "customer-support" {
		t.Errorf("%v", got)
	}
}

func TestSectionAddJSON(t *testing.T) {
	dir := newFixture(t)
	j := decode(t, at(t, dir, "section", "add", "Tools", "--json").stdout)
	if !reflect.DeepEqual(keys(j), []string{"section"}) {
		t.Fatalf("%v", keys(j))
	}
	s := j["section"].(map[string]any)
	if s["slug"] != "tools" || s["order"] != 2.0 || s["title"] != "Tools" {
		t.Errorf("%v", s)
	}
	if !reflect.DeepEqual(keys(s), []string{"counts", "description", "order", "slug", "title"}) {
		t.Errorf("%v", keys(s))
	}
}

func TestSectionAddInvalid(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"blank title", []string{"  "}, "is empty"},
		{"title too long", []string{strings.Repeat("a", 81)}, "too long"},
		{"control character", []string{"a\x1b[31mb"}, "control characters"},
		{"description too long", []string{"Ok", "--description", strings.Repeat("d", 2001)}, "too long"},
		{"no title", nil, "requires at least 1 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := sectionSlugs(t, dir)
			r := at(t, dir, append([]string{"section", "add"}, tt.args...)...)
			wantFail(t, r, ExitUsage, tt.want)
			if !reflect.DeepEqual(sectionSlugs(t, dir), before) {
				t.Error("a failed add changed the notebook")
			}
		})
	}
}

func TestSectionRename(t *testing.T) {
	dir := newFixture(t)
	before := readFile(t, dir, "sections/team/_section.md")
	r := at(t, dir, "section", "rename", "team", "People", "&", "Culture")
	wantOK(t, r)
	if r.stdout != "Renamed section team to People & Culture.\n" {
		t.Errorf("stdout %q", r.stdout)
	}
	after := readFile(t, dir, "sections/team/_section.md")
	if !strings.Contains(after, "People & Culture") || !strings.Contains(after, "created: 2026-11-03T12:00:00Z") || !strings.Contains(after, "updated: 2026-11-04T08:30:00Z") {
		t.Errorf("before:\n%s\nafter:\n%s", before, after)
	}
	if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"technical", "team"}) {
		t.Errorf("the slug must not change: %v", got)
	}
	// Topics and journal links survive.
	wantOK(t, run(t, dir, "show", "team/rituals"))
	j := decode(t, at(t, dir, "section", "rename", "team", "Team", "--json").stdout)
	s := j["section"].(map[string]any)
	if s["slug"] != "team" || s["title"] != "Team" || s["counts"].(map[string]any)["total"] != 2.0 {
		t.Errorf("%v", s)
	}
}

func TestSectionRenameErrors(t *testing.T) {
	dir := newFixture(t)
	wantFail(t, at(t, dir, "section", "rename", "nope", "X"), ExitNotFound, `section "nope" does not exist`)
	wantFail(t, at(t, dir, "section", "rename", "../x", "X"), ExitUsage, "invalid section")
	wantFail(t, at(t, dir, "section", "rename", "team", "   "), ExitUsage, "is empty")
	wantFail(t, at(t, dir, "section", "rename", "team", strings.Repeat("x", 81)), ExitUsage, "too long")
	wantFail(t, at(t, dir, "section", "rename", "team"), ExitUsage, "requires at least 2 arg")
	if !strings.Contains(readFile(t, dir, "sections/team/_section.md"), "title: \"Team\"") {
		t.Error("a failed rename changed the file")
	}
}

func TestSectionRm(t *testing.T) {
	t.Run("refuses without --yes when not on a terminal", func(t *testing.T) {
		dir := newFixture(t)
		r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: "y\n"}, "section", "rm", "team")
		wantFail(t, r, ExitUsage, "needs confirmation")
		if got := sectionSlugs(t, dir); len(got) != 2 {
			t.Errorf("nothing may be deleted: %v", got)
		}
	})
	t.Run("--yes deletes, clears journal references", func(t *testing.T) {
		dir := newFixture(t)
		r := at(t, dir, "section", "rm", "technical", "--yes")
		wantOK(t, r)
		if r.stdout != "Deleted section technical. Topics deleted: 3. Journal entries unlinked: 1.\n" {
			t.Errorf("stdout %q", r.stdout)
		}
		if _, err := os.Stat(filepath.Join(dir, "sections", "technical")); !os.IsNotExist(err) {
			t.Errorf("section directory still exists: %v", err)
		}
		if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"team"}) {
			t.Errorf("%v", got)
		}
		j := decode(t, run(t, dir, "journal", "--json").stdout)
		for _, e := range j["entries"].([]any) {
			m := e.(map[string]any)
			if m["section"] == "technical" || m["topic"] != "" && strings.HasPrefix(m["topic"].(string), "technical/") {
				t.Errorf("entry still linked: %v", m)
			}
		}
		// The files themselves no longer name the section.
		_ = filepath.WalkDir(filepath.Join(dir, "journal"), func(p string, d os.DirEntry, err error) error {
			must(t, err)
			if !d.IsDir() {
				if b, _ := os.ReadFile(p); strings.Contains(string(b), "technical") {
					t.Errorf("%s still mentions the section", p)
				}
			}
			return nil
		})
	})
	t.Run("terminal: prompt states what is deleted, y confirms", func(t *testing.T) {
		dir := newFixture(t)
		r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: "y\n", interactive: true}, "section", "rm", "technical")
		wantOK(t, r)
		want := "Delete section technical (Technical)? Topics deleted: 3. Journal entries that lose their link: 1. This cannot be undone. [y/N] "
		if r.stderr != want {
			t.Errorf("prompt %q, want %q", r.stderr, want)
		}
		if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"team"}) {
			t.Errorf("%v", got)
		}
	})
	t.Run("terminal: anything but yes cancels", func(t *testing.T) {
		dir := newFixture(t)
		r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: "n\n", interactive: true}, "section", "rm", "technical")
		if r.code != ExitFailure || !strings.Contains(r.stderr, "cancelled") {
			t.Errorf("%d %q", r.code, r.stderr)
		}
		if got := sectionSlugs(t, dir); len(got) != 2 {
			t.Errorf("nothing may be deleted: %v", got)
		}
	})
	t.Run("json", func(t *testing.T) {
		dir := newFixture(t)
		j := decode(t, at(t, dir, "section", "rm", "team", "-y", "--json").stdout)
		d := j["deleted"].(map[string]any)
		if !reflect.DeepEqual(keys(d), []string{"journal_entries_unlinked", "section", "topics"}) || d["section"] != "team" || d["topics"] != 2.0 || d["journal_entries_unlinked"] != 1.0 {
			t.Errorf("%v", j)
		}
	})
	t.Run("refuses a section with files that are not valid topics", func(t *testing.T) {
		dir := newFixture(t)
		must(t, os.WriteFile(filepath.Join(dir, "sections", "technical", "broken.md"), []byte("SECRET-CONTENT"), 0o600))
		must(t, os.WriteFile(filepath.Join(dir, "sections", "technical", "notes.txt"), []byte("x"), 0o600))
		for _, o := range []opts{
			{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: "y\n", interactive: true},
			{env: map[string]string{"BEARINGS_NOTEBOOK": dir}},
		} {
			args := []string{"section", "rm", "technical"}
			if !o.interactive {
				args = append(args, "--yes")
			}
			r := runOpts(t, o, args...)
			wantFail(t, r, ExitNotebook, "broken.md, notes.txt")
			if strings.Contains(r.stderr+r.stdout, "SECRET-CONTENT") || strings.Contains(r.stderr, "[y/N]") {
				t.Errorf("must not leak content or ask: %q", r.stderr)
			}
		}
		if got := sectionSlugs(t, dir); len(got) != 2 {
			t.Errorf("nothing may be deleted: %v", got)
		}
		if _, err := os.Stat(filepath.Join(dir, "sections", "technical", "backtesting.md")); err != nil {
			t.Error(err)
		}
	})
	t.Run("invalid and missing", func(t *testing.T) {
		dir := newFixture(t)
		wantFail(t, at(t, dir, "section", "rm", "nope", "--yes"), ExitNotFound, "does not exist")
		wantFail(t, at(t, dir, "section", "rm", "..", "--yes"), ExitUsage, "invalid section")
		wantFail(t, at(t, dir, "section", "rm", "--yes"), ExitUsage, "accepts 1 arg")
	})
}

func TestSectionMv(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "section", "mv", "team", "--up")
	wantOK(t, r)
	if r.stdout != "Moved section team up.\n" {
		t.Errorf("%q", r.stdout)
	}
	if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"team", "technical"}) {
		t.Errorf("%v", got)
	}
	r = at(t, dir, "section", "mv", "team", "--up")
	wantOK(t, r)
	if r.stdout != "Section team is already the first.\n" {
		t.Errorf("%q", r.stdout)
	}
	r = at(t, dir, "section", "mv", "team", "--down")
	wantOK(t, r)
	if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"technical", "team"}) {
		t.Errorf("%v", got)
	}
	r = at(t, dir, "section", "mv", "team", "--down")
	wantOK(t, r)
	if r.stdout != "Section team is already the last.\n" {
		t.Errorf("%q", r.stdout)
	}
	j := decode(t, at(t, dir, "section", "mv", "technical", "--down", "--json").stdout)
	if j["moved"] != true || !reflect.DeepEqual(j["order"], []any{"team", "technical"}) || !reflect.DeepEqual(keys(j), []string{"moved", "order", "section"}) {
		t.Errorf("%v", j)
	}
	wantFail(t, at(t, dir, "section", "mv", "team"), ExitUsage, "exactly one of --up and --down")
	wantFail(t, at(t, dir, "section", "mv", "team", "--up", "--down"), ExitUsage, "exactly one of --up and --down")
	wantFail(t, at(t, dir, "section", "mv", "nope", "--up"), ExitNotFound, "does not exist")
}

func TestSectionMvAddsNewSectionsLast(t *testing.T) {
	dir := newFixture(t)
	wantOK(t, at(t, dir, "section", "mv", "team", "--up"))
	wantOK(t, at(t, dir, "section", "add", "Tools"))
	if got := sectionSlugs(t, dir); !reflect.DeepEqual(got, []string{"team", "technical", "tools"}) {
		t.Errorf("%v", got)
	}
}

func TestSectionLsAndGroup(t *testing.T) {
	dir := newFixture(t)
	if a, b := runIn(t, dir, "section", "ls"), runIn(t, dir, "ls"); a.stdout != b.stdout || a.code != 0 {
		t.Errorf("section ls differs from ls:\n%s\n%s", a.stdout, b.stdout)
	}
	wantFail(t, runIn(t, dir, "section", "bogus"), ExitUsage, `unknown command "bogus"`)
	r := runIn(t, dir, "section")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "add") || !strings.Contains(r.stdout, "rename") {
		t.Errorf("help should list subcommands: %s", r.stdout)
	}
}
