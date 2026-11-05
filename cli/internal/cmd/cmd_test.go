package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func wantOK(t *testing.T, r result) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
}

func wantFail(t *testing.T, r result, code int, substr string) {
	t.Helper()
	if r.code != code {
		t.Fatalf("exit %d, want %d; stderr: %s", r.code, code, r.stderr)
	}
	if r.stdout != "" {
		t.Errorf("stdout must be empty on failure, got %q", r.stdout)
	}
	if !strings.Contains(r.stderr, substr) {
		t.Errorf("stderr %q does not contain %q", r.stderr, substr)
	}
}

func TestLsSections(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "ls")
	wantOK(t, r)
	want := "SECTION    TITLE      MASTERED\ntechnical  Technical  1/3\nteam       Team       1/2\n"
	if r.stdout != want {
		t.Errorf("got\n%s\nwant\n%s", r.stdout, want)
	}

	j := decode(t, run(t, dir, "ls", "--json").stdout)
	if !reflect.DeepEqual(keys(j), []string{"sections"}) {
		t.Fatalf("keys %v", keys(j))
	}
	secs := j["sections"].([]any)
	if len(secs) != 2 {
		t.Fatalf("%v", secs)
	}
	first := secs[0].(map[string]any)
	if !reflect.DeepEqual(keys(first), []string{"counts", "description", "order", "slug", "title"}) {
		t.Errorf("section keys %v", keys(first))
	}
	counts := first["counts"].(map[string]any)
	if !reflect.DeepEqual(keys(counts), []string{"in_progress", "mastered", "pending", "total"}) {
		t.Errorf("counts keys %v", keys(counts))
	}
	if first["slug"] != "technical" || counts["total"] != 3.0 || counts["mastered"] != 1.0 || counts["in_progress"] != 1.0 || counts["pending"] != 1.0 {
		t.Errorf("%v", first)
	}
}

func TestLsEmptyNotebook(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	_, err := notebook.Init(dir, notebook.Config{Format: 1}, nil)
	must(t, err)
	r := run(t, dir, "ls")
	wantOK(t, r)
	if r.stdout != "No sections yet.\n" {
		t.Errorf("%q", r.stdout)
	}
	j := decode(t, run(t, dir, "ls", "--json").stdout)
	if s, ok := j["sections"].([]any); !ok || len(s) != 0 {
		t.Errorf("sections must be an empty array, got %#v", j["sections"])
	}
}

func TestLsTopics(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "ls", "technical")
	wantOK(t, r)
	for _, want := range []string{"Technical (technical)", "Systems and tools.", "backtesting", "in progress", "mastered", "personal", "yes"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in\n%s", want, r.stdout)
		}
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"all", []string{"ls", "technical", "--json"}, []string{"technical/backtesting", "technical/risk", "technical/scratch"}},
		{"status", []string{"ls", "technical", "--status", "mastered", "--json"}, []string{"technical/risk"}},
		{"type", []string{"ls", "technical", "--type", "personal", "--json"}, []string{"technical/scratch"}},
		{"both", []string{"ls", "technical", "--type", "role", "--status", "in-progress", "--json"}, []string{"technical/backtesting"}},
		{"none", []string{"ls", "team", "--type", "personal", "--json"}, nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, dir, tc.args...)
			wantOK(t, r)
			j := decode(t, r.stdout)
			if !reflect.DeepEqual(keys(j), []string{"filters", "section", "topics"}) {
				t.Fatalf("keys %v", keys(j))
			}
			var got []string
			for _, x := range j["topics"].([]any) {
				m := x.(map[string]any)
				if !reflect.DeepEqual(keys(m), []string{"order", "ref", "section", "slug", "status", "suggested", "title", "type"}) {
					t.Errorf("topic keys %v", keys(m))
				}
				got = append(got, m["ref"].(string))
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}

	// Counts describe the whole section even when the list is filtered.
	j := decode(t, run(t, dir, "ls", "technical", "--status", "mastered", "--json").stdout)
	if c := j["section"].(map[string]any)["counts"].(map[string]any); c["total"] != 3.0 {
		t.Errorf("counts %v", c)
	}
	if f := j["filters"].(map[string]any); f["status"] != "mastered" || f["type"] != "" {
		t.Errorf("filters %v", f)
	}

	r = run(t, dir, "ls", "team", "--type", "personal")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "No topics.") {
		t.Errorf("%q", r.stdout)
	}
}

func TestLsErrors(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"unknown section", []string{"ls", "nope"}, ExitNotFound, `section "nope" does not exist`},
		{"invalid section", []string{"ls", "Not A Slug"}, ExitUsage, "invalid section"},
		{"traversal section", []string{"ls", "../etc"}, ExitUsage, "invalid section"},
		{"absolute section", []string{"ls", "/etc"}, ExitUsage, "invalid section"},
		{"bad status", []string{"ls", "technical", "--status", "done"}, ExitUsage, `invalid status "done"`},
		{"bad type", []string{"ls", "technical", "--type", "x"}, ExitUsage, `invalid type "x"`},
		{"filter without section", []string{"ls", "--status", "pending"}, ExitUsage, "need a section"},
		{"too many args", []string{"ls", "a", "b"}, ExitUsage, "accepts at most 1"},
		{"unknown flag", []string{"ls", "--nope"}, ExitUsage, "unknown flag"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { wantFail(t, run(t, dir, tc.args...), tc.code, tc.msg) })
	}
}

func TestShow(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "show", "technical/backtesting")
	wantOK(t, r)
	for _, want := range []string{
		"Backtesting", "technical/backtesting  -  Technical", "in progress", "role", "Suggested: no",
		"Backtesting guide  https://example.com/wiki/backtesting", "  - https://example.com/plain",
		"  Download the sample data first.", "  Then run the simulator.",
		"Journal entries (1)", "2026-11-03", "    The simulator skips days without data.",
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in\n%s", want, r.stdout)
		}
	}
	if strings.Contains(r.stdout, "Standup") {
		t.Error("entries of other topics must not be listed")
	}

	j := decode(t, run(t, dir, "show", "technical/backtesting", "--json").stdout)
	if !reflect.DeepEqual(keys(j), []string{"journal", "topic"}) {
		t.Fatalf("keys %v", keys(j))
	}
	tp := j["topic"].(map[string]any)
	wantKeys := []string{"created", "notes", "order", "ref", "resources", "section", "section_title", "slug", "status", "suggested", "title", "type", "updated"}
	if !reflect.DeepEqual(keys(tp), wantKeys) {
		t.Errorf("topic keys %v", keys(tp))
	}
	if tp["ref"] != "technical/backtesting" || tp["status"] != "in-progress" || tp["created"] != "2026-11-03T12:00:00Z" || tp["section_title"] != "Technical" {
		t.Errorf("%v", tp)
	}
	res := tp["resources"].([]any)
	if len(res) != 2 || !reflect.DeepEqual(keys(res[0].(map[string]any)), []string{"title", "url"}) || res[1].(map[string]any)["title"] != "" {
		t.Errorf("resources %v", res)
	}
	es := j["journal"].([]any)
	if len(es) != 1 {
		t.Fatalf("journal %v", es)
	}
	e := es[0].(map[string]any)
	if !reflect.DeepEqual(keys(e), []string{"body", "created", "date", "id", "section", "topic", "updated"}) || e["topic"] != "technical/backtesting" || e["section"] != "technical" {
		t.Errorf("entry %v", e)
	}

	// A topic without notes, resources or entries.
	r = run(t, dir, "show", "team/rituals")
	wantOK(t, r)
	if strings.Count(r.stdout, "(none)") != 3 {
		t.Errorf("expected three (none) markers:\n%s", r.stdout)
	}
	j = decode(t, run(t, dir, "show", "team/rituals", "--json").stdout)
	if len(j["journal"].([]any)) != 0 || len(j["topic"].(map[string]any)["resources"].([]any)) != 0 {
		t.Errorf("empty lists must be [] not null: %v", j)
	}
}

func TestShowErrors(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"missing arg", []string{"show"}, ExitUsage, "accepts 1 arg"},
		{"no slash", []string{"show", "backtesting"}, ExitUsage, "invalid topic reference"},
		{"too many parts", []string{"show", "a/b/c"}, ExitUsage, "invalid topic reference"},
		{"traversal", []string{"show", "../etc/passwd"}, ExitUsage, "invalid topic reference"},
		{"traversal in topic", []string{"show", "technical/../../x"}, ExitUsage, "invalid topic reference"},
		{"backslash", []string{"show", `technical\risk`}, ExitUsage, "invalid topic reference"},
		{"uppercase", []string{"show", "Technical/Risk"}, ExitUsage, "invalid topic reference"},
		{"unknown topic", []string{"show", "technical/nope"}, ExitNotFound, `topic "technical/nope" does not exist`},
		{"unknown section", []string{"show", "nope/risk"}, ExitNotFound, "does not exist"},
		{"control characters", []string{"show", "tech\x1b[31m/x"}, ExitUsage, "invalid topic reference"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, dir, tc.args...)
			wantFail(t, r, tc.code, tc.msg)
			if strings.Contains(r.stderr, "\x1b") {
				t.Errorf("escape sequence echoed in error: %q", r.stderr)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	dir := newFixture(t)
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	_, err = nb.AddTopic(notebook.NewTopic{Section: "team", Slug: "cafe", Title: "Café etiquette", Type: notebook.TypeRole})
	must(t, err)

	tests := []struct {
		name  string
		query string
		in    string // expected "found in" label, "" = no result
		where string
	}{
		{"section title", "TECHNICAL", "section title", "technical"},
		{"section description", "systems and", "section description", "technical"},
		{"topic title", "risk limits", "topic title", "technical/risk"},
		{"topic notes", "reviewed weekly", "topic notes", "technical/risk"},
		{"resource title", "backtesting guide", "topic resources", "technical/backtesting"},
		{"resource url", "example.com/plain", "topic resources", "technical/backtesting"},
		{"journal", "access to the wiki", "journal entry", "2026-10-20"},
		{"accent and case insensitive", "CAFE", "topic title", "team/cafe"},
		{"no match", "zebra", "", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := run(t, dir, "search", tc.query)
			wantOK(t, r)
			if tc.in == "" {
				if !strings.Contains(r.stdout, `Nothing found for "zebra"`) {
					t.Errorf("%q", r.stdout)
				}
				return
			}
			if !strings.Contains(r.stdout, tc.in) || !strings.Contains(r.stdout, tc.where) {
				t.Errorf("want %q at %q in\n%s", tc.in, tc.where, r.stdout)
			}
		})
	}

	// Multiple words form one query, and the JSON shape is stable.
	r := run(t, dir, "search", "--json", "simulator", "skips")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if !reflect.DeepEqual(keys(j), []string{"hits", "query"}) || j["query"] != "simulator skips" {
		t.Fatalf("%v", j)
	}
	hits := j["hits"].([]any)
	if len(hits) != 1 {
		t.Fatalf("%v", hits)
	}
	h := hits[0].(map[string]any)
	if !reflect.DeepEqual(keys(h), []string{"date", "entry_id", "field", "kind", "section", "snippet", "title", "topic"}) {
		t.Errorf("hit keys %v", keys(h))
	}
	if h["kind"] != "entry" || h["field"] != "body" || h["date"] != "2026-11-03" || h["topic"] != "technical/backtesting" || h["entry_id"] == "" {
		t.Errorf("%v", h)
	}

	j = decode(t, run(t, dir, "search", "--json", "zebra").stdout)
	if hs, ok := j["hits"].([]any); !ok || len(hs) != 0 {
		t.Errorf("hits must be [] not null: %#v", j["hits"])
	}

	r = run(t, dir, "search", "--json", "--limit", "1", "the")
	wantOK(t, r)
	if n := len(decode(t, r.stdout)["hits"].([]any)); n != 1 {
		t.Errorf("limit not applied: %d hits", n)
	}
}

func TestSearchErrors(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"no query", []string{"search"}, ExitUsage, "requires at least 1 arg"},
		{"blank", []string{"search", "   "}, ExitUsage, "invalid search"},
		{"too long", []string{"search", strings.Repeat("q", notebook.MaxQueryLen+1)}, ExitUsage, "invalid search"},
		{"control character", []string{"search", "a\x1b[2Jb"}, ExitUsage, "invalid search"},
		{"negative limit", []string{"search", "x", "--limit", "-1"}, ExitUsage, "--limit"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { wantFail(t, run(t, dir, tc.args...), tc.code, tc.msg) })
	}
}

func TestJournal(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "journal")
	wantOK(t, r)
	i1, i2, i3 := strings.Index(r.stdout, "2026-11-03"), strings.Index(r.stdout, "Standup"), strings.Index(r.stdout, "2026-10-20")
	if i1 != 0 || i2 < 0 || i3 < i2 {
		t.Errorf("entries must be grouped by date, newest first:\n%s", r.stdout)
	}
	if strings.Count(r.stdout, "2026-11-03") != 1 {
		t.Errorf("date heading must appear once:\n%s", r.stdout)
	}

	ids := func(args ...string) []string {
		t.Helper()
		r := run(t, dir, append([]string{"journal", "--json"}, args...)...)
		wantOK(t, r)
		j := decode(t, r.stdout)
		if !reflect.DeepEqual(keys(j), []string{"entries"}) {
			t.Fatalf("keys %v", keys(j))
		}
		var out []string
		for _, e := range j["entries"].([]any) {
			out = append(out, e.(map[string]any)["body"].(string))
		}
		return out
	}
	all := ids()
	if len(all) != 3 || all[2] != "First day: got access to the wiki." {
		t.Errorf("%v", all)
	}
	tests := []struct {
		name string
		args []string
		want int
	}{
		{"from", []string{"--from", "2026-11-01"}, 2},
		{"from inclusive", []string{"--from", "2026-10-20"}, 3},
		{"to", []string{"--to", "2026-10-31"}, 1},
		{"to inclusive", []string{"--to", "2026-10-20"}, 1},
		{"range", []string{"--from", "2026-10-21", "--to", "2026-11-02"}, 0},
		{"same day", []string{"--from", "2026-11-03", "--to", "2026-11-03"}, 2},
		{"section", []string{"-s", "technical"}, 1},
		{"section long", []string{"--section", "team"}, 1},
		{"limit", []string{"--limit", "2"}, 2},
		{"all combined", []string{"-s", "team", "--from", "2026-11-03", "--limit", "5"}, 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ids(tc.args...); len(got) != tc.want {
				t.Errorf("got %d entries %v, want %d", len(got), got, tc.want)
			}
		})
	}

	r = run(t, dir, "journal", "--to", "2000-01-01")
	wantOK(t, r)
	if r.stdout != "No journal entries.\n" {
		t.Errorf("%q", r.stdout)
	}
	if e := decode(t, run(t, dir, "journal", "--to", "2000-01-01", "--json").stdout)["entries"]; len(e.([]any)) != 0 {
		t.Errorf("entries must be [] not null")
	}
}

func TestJournalErrors(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name string
		args []string
		code int
		msg  string
	}{
		{"bad from", []string{"journal", "--from", "yesterday"}, ExitUsage, "invalid date for --from"},
		{"impossible date", []string{"journal", "--from", "2026-02-30"}, ExitUsage, "invalid date for --from"},
		{"bad month", []string{"journal", "--to", "2026-13-01"}, ExitUsage, "invalid date for --to"},
		{"short date", []string{"journal", "--to", "2026-1-1"}, ExitUsage, "invalid date for --to"},
		{"before 1970", []string{"journal", "--from", "1969-12-31"}, ExitUsage, "invalid date"},
		{"time included", []string{"journal", "--from", "2026-11-03T10:00:00Z"}, ExitUsage, "invalid date"},
		{"from after to", []string{"journal", "--from", "2026-11-03", "--to", "2026-11-02"}, ExitUsage, "--from must not be after --to"},
		{"unknown section", []string{"journal", "-s", "nope"}, ExitNotFound, "does not exist"},
		{"invalid section", []string{"journal", "-s", "../x"}, ExitUsage, "invalid section"},
		{"negative limit", []string{"journal", "--limit", "-3"}, ExitUsage, "--limit"},
		{"stray arg", []string{"journal", "extra"}, ExitUsage, "unknown command"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { wantFail(t, run(t, dir, tc.args...), tc.code, tc.msg) })
	}
}

func TestProgress(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "progress")
	wantOK(t, r)
	want := `Progress

Technical   [######--------------]  1/3   33%
Team        [##########----------]  1/2   50%

All topics  [########------------]  2/5   40%
Role path   [##########----------]  2/4   50%
Personal    [--------------------]  0/1    0%

Suggested topics not reviewed yet: 1
Journal entries: 3
`
	if r.stdout != want {
		t.Errorf("got\n%s\nwant\n%s", r.stdout, want)
	}
	// Bars are as wide in every row.
	var width int
	for _, l := range strings.Split(r.stdout, "\n") {
		if i := strings.Index(l, "["); i >= 0 {
			w := strings.Index(l, "]") - i
			if width == 0 {
				width = w
			}
			if w != width {
				t.Errorf("bar width changes: %q", l)
			}
		}
	}

	j := decode(t, run(t, dir, "progress", "--json").stdout)
	if !reflect.DeepEqual(keys(j), []string{"all", "journal_entries", "personal", "role", "sections", "suggested"}) {
		t.Fatalf("keys %v", keys(j))
	}
	if j["journal_entries"] != 3.0 || j["suggested"] != 1.0 {
		t.Errorf("%v", j)
	}
	if role := j["role"].(map[string]any); role["total"] != 4.0 || role["mastered"] != 2.0 {
		t.Errorf("role %v", role)
	}
	s := j["sections"].([]any)[0].(map[string]any)
	if !reflect.DeepEqual(keys(s), []string{"all", "role", "slug", "title"}) {
		t.Errorf("section keys %v", keys(s))
	}
	if s["role"].(map[string]any)["total"] != 2.0 || s["all"].(map[string]any)["total"] != 3.0 {
		t.Errorf("%v", s)
	}
}

func TestProgressEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	_, err := notebook.Init(dir, notebook.Config{Format: 1}, nil)
	must(t, err)
	r := run(t, dir, "progress")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "[--------------------]  0/0    0%") {
		t.Errorf("%s", r.stdout)
	}
	j := decode(t, run(t, dir, "progress", "--json").stdout)
	if len(j["sections"].([]any)) != 0 {
		t.Errorf("sections must be []")
	}
}

func TestPath(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "path")
	wantOK(t, r)
	want := `Role path (2 of 4 mastered)

Technical
    1. Backtesting  (in progress)
  ✓ 2. Risk limits  (mastered)

Team
    3. Team rituals  (pending)
  ✓ 4. Who is who  (mastered)
`
	if r.stdout != want {
		t.Errorf("got\n%s\nwant\n%s", r.stdout, want)
	}
	if strings.Contains(r.stdout, "private shortcuts") {
		t.Error("personal topics must not be on the role path")
	}

	j := decode(t, run(t, dir, "path", "--json").stdout)
	if !reflect.DeepEqual(keys(j), []string{"mastered", "steps", "total"}) || j["total"] != 4.0 || j["mastered"] != 2.0 {
		t.Fatalf("%v", j)
	}
	steps := j["steps"].([]any)
	st := steps[2].(map[string]any)
	if !reflect.DeepEqual(keys(st), []string{"ref", "section", "section_title", "slug", "status", "step", "suggested", "title"}) {
		t.Errorf("step keys %v", keys(st))
	}
	if st["step"] != 3.0 || st["ref"] != "team/rituals" || st["status"] != "pending" {
		t.Errorf("%v", st)
	}
}

func TestPathNumbersAlignPastNine(t *testing.T) {
	dir := newFixture(t)
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	for _, s := range []string{"a", "b", "c", "d", "e", "f", "g"} {
		_, err := nb.AddTopic(notebook.NewTopic{Section: "team", Slug: "t-" + s, Title: "Topic " + s, Type: notebook.TypeRole})
		must(t, err)
	}
	r := run(t, dir, "path")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "   1. Backtesting") || !strings.Contains(r.stdout, "  11. Topic g") {
		t.Errorf("step numbers not aligned:\n%s", r.stdout)
	}
}

func TestPathMarkdown(t *testing.T) {
	dir := newFixture(t)
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	_, err = nb.AddTopic(notebook.NewTopic{Section: "team", Slug: "odd", Title: "Odd [title]", Type: notebook.TypeRole,
		Resources: []notebook.Resource{{URL: "https://example.com/a(b)c%20d", Title: "Link [x]"}}})
	must(t, err)

	r := run(t, dir, "path", "--md")
	wantOK(t, r)
	for _, want := range []string{
		"# Role path: Example role\n", "## Technical\n\nSystems and tools.\n", "### 1. Backtesting\n",
		"Download the sample data first.\nThen run the simulator.", "**Resources**",
		"- [Backtesting guide](<https://example.com/wiki/backtesting>)", "- <https://example.com/plain>",
		"### 2. Risk limits", "## Team\n", "### 5. Odd [title]",
		`- [Link \[x\]](<https://example.com/a(b)c%20d>)`,
	} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("missing %q in\n%s", want, r.stdout)
		}
	}
	// Nothing personal: personal topics, status words, journal text, suggested flags.
	for _, bad := range []string{"private shortcuts", "Secret handshake", "Standup", "simulator skips", "First day", "mastered", "pending", "in progress", "(", "journal"} {
		if bad == "(" {
			continue // links legitimately contain parentheses
		}
		if strings.Contains(strings.ToLower(r.stdout), strings.ToLower(bad)) {
			t.Errorf("shareable guide leaks %q:\n%s", bad, r.stdout)
		}
	}

	wantFail(t, run(t, dir, "path", "--md", "--json"), ExitUsage, "cannot be used together")
}

func TestPathMarkdownEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nb")
	_, err := notebook.Init(dir, notebook.Config{Format: 1}, nil)
	must(t, err)
	r := run(t, dir, "path", "--md")
	wantOK(t, r)
	if !strings.HasPrefix(r.stdout, "# Role path: Bearings notebook\n") || !strings.Contains(r.stdout, "empty") {
		t.Errorf("%q", r.stdout)
	}
	r = run(t, dir, "path")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Role path (0 of 0 mastered)") {
		t.Errorf("%q", r.stdout)
	}
}

// TestHostileContent: escape sequences stored in a notebook must never reach
// the terminal, in any command, while --json keeps the data intact (escaped).
func TestHostileContent(t *testing.T) {
	dir := newFixture(t)
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	evil := "ok\x1b[2J\x1b]0;pwned\x07\x1b[31mred\u202e\u009b1m\r\nnext"
	tp, err := nb.AddTopic(notebook.NewTopic{Section: "team", Slug: "evil", Title: "Evil topic", Type: notebook.TypeRole, Notes: evil})
	if err != nil {
		t.Skipf("the notebook refuses control characters in notes (good): %v", err)
	}
	_, err = nb.AddEntry(notebook.NewEntry{Topic: tp.Ref(), Body: evil})
	must(t, err)
	_, err = nb.AddTopic(notebook.NewTopic{Section: "team", Slug: "evil2", Title: "Evil two", Type: notebook.TypeRole,
		Resources: []notebook.Resource{{URL: "https://example.com/x", Title: "res\x1b[1mtitle"}}})
	if err == nil {
		t.Log("resource title with control characters accepted by the writer")
	}
	for _, args := range [][]string{
		{"ls", "team"}, {"show", "team/evil"}, {"search", "next"}, {"search", "ok"}, {"journal"}, {"path"}, {"path", "--md"}, {"progress"}, {"ls"},
	} {
		r := run(t, dir, args...)
		wantOK(t, r)
		if strings.ContainsAny(r.stdout, "\x1b\x07\r\u009b\u202e") {
			t.Errorf("%v: control sequence reached the terminal: %q", args, r.stdout)
		}
	}
	r := run(t, dir, "show", "team/evil", "--json")
	wantOK(t, r)
	if strings.Contains(r.stdout, "\x1b") || !strings.Contains(r.stdout, `\u001b[2J`) {
		t.Errorf("JSON must escape, not drop, control characters: %q", r.stdout)
	}
	if decode(t, r.stdout)["topic"].(map[string]any)["notes"] != strings.ReplaceAll(evil, "\r\n", "\n") {
		t.Errorf("JSON notes changed")
	}
}

func TestMissingAndInvalidNotebook(t *testing.T) {
	t.Run("missing directory", func(t *testing.T) {
		r := run(t, filepath.Join(t.TempDir(), "nothing"), "ls")
		wantFail(t, r, ExitNotebook, "no notebook found at")
	})
	t.Run("directory without bearings.yaml", func(t *testing.T) {
		wantFail(t, run(t, t.TempDir(), "progress"), ExitNotebook, "no notebook found")
	})
	t.Run("file instead of directory", func(t *testing.T) {
		f := filepath.Join(t.TempDir(), "file")
		must(t, os.WriteFile(f, []byte("x"), 0o600))
		wantFail(t, run(t, f, "ls"), ExitNotebook, "no notebook found")
	})
	t.Run("unsupported format", func(t *testing.T) {
		d := t.TempDir()
		must(t, os.WriteFile(filepath.Join(d, "bearings.yaml"), []byte("format: 2\nname: SECRETNAME\n"), 0o600))
		r := run(t, d, "ls")
		wantFail(t, r, ExitNotebook, "unsupported notebook format")
		if strings.Contains(r.stderr, "SECRETNAME") {
			t.Errorf("error leaks file content: %q", r.stderr)
		}
	})
	t.Run("invalid yaml does not leak content", func(t *testing.T) {
		d := t.TempDir()
		must(t, os.WriteFile(filepath.Join(d, "bearings.yaml"), []byte("format: 1\nname: [SECRETVALUE\n"), 0o600))
		r := run(t, d, "ls")
		if r.code != ExitNotebook || strings.Contains(r.stderr, "SECRETVALUE") {
			t.Errorf("exit %d: %q", r.code, r.stderr)
		}
	})
	t.Run("symlinked bearings.yaml is refused", func(t *testing.T) {
		d := t.TempDir()
		target := filepath.Join(t.TempDir(), "real.yaml")
		must(t, os.WriteFile(target, []byte("format: 1\n"), 0o600))
		if err := os.Symlink(target, filepath.Join(d, "bearings.yaml")); err != nil {
			t.Skip("symlinks unavailable")
		}
		if r := run(t, d, "ls"); r.code == 0 {
			t.Errorf("symlinked config accepted: %q", r.stdout)
		}
	})
}

// TestInvalidFilesAreSkippedWithWarning: one broken file does not stop the
// command; it is reported on stderr (once, by path) and stdout stays clean.
func TestInvalidFilesAreSkippedWithWarning(t *testing.T) {
	dir := newFixture(t)
	bad := filepath.Join(dir, "sections", "technical", "broken.md")
	must(t, os.WriteFile(bad, []byte("---\ntitle: SECRETTITLE\nstatus: nonsense\n---\nbody SECRETBODY\n"), 0o600))
	r := run(t, dir, "ls", "technical")
	wantOK(t, r)
	if !strings.Contains(r.stderr, "warning: skipped sections/technical/broken.md") {
		t.Errorf("no warning: %q", r.stderr)
	}
	if strings.Contains(r.stderr+r.stdout, "SECRET") || strings.Contains(r.stdout, "broken") {
		t.Errorf("content leaked:\nstdout %q\nstderr %q", r.stdout, r.stderr)
	}
	if strings.Count(r.stderr, "broken.md") != 1 {
		t.Errorf("warning repeated: %q", r.stderr)
	}
}

func TestNotebookLocation(t *testing.T) {
	dir := newFixture(t)
	other := filepath.Join(t.TempDir(), "other")
	_, err := notebook.Init(other, notebook.Config{Format: 1, Name: "Other"}, nil)
	must(t, err)

	// --notebook beats BEARINGS_NOTEBOOK.
	r := runCLI(t, map[string]string{"BEARINGS_NOTEBOOK": other}, "--notebook", dir, "ls")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "technical") {
		t.Errorf("flag ignored: %q", r.stdout)
	}
	// BEARINGS_NOTEBOOK beats the default.
	r = runCLI(t, map[string]string{"BEARINGS_NOTEBOOK": other, "HOME": filepath.Dir(dir)}, "ls")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "No sections") {
		t.Errorf("env ignored: %q", r.stdout)
	}
	// Default: ~/bearings.
	home := t.TempDir()
	must(t, os.Rename(dir, filepath.Join(home, "bearings")))
	r = runCLI(t, map[string]string{"HOME": home}, "ls")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "technical") {
		t.Errorf("default location ignored: %q", r.stdout)
	}
	// No home and nothing configured.
	wantFail(t, runCLI(t, map[string]string{}, "ls"), ExitUsage, "home directory")
	// Flags can come after the subcommand.
	r = runCLI(t, map[string]string{}, "ls", "--notebook", filepath.Join(home, "bearings"), "--json")
	wantOK(t, r)
	decode(t, r.stdout)
}

func TestLanguage(t *testing.T) {
	dir := newFixture(t)
	for _, tc := range []struct {
		name string
		env  map[string]string
		args []string
		code int
	}{
		{"flag en", nil, []string{"--lang", "en", "ls"}, 0},
		{"flag es accepted", nil, []string{"--lang=es", "ls"}, 0},
		{"env es accepted", map[string]string{"BEARINGS_LANG": "es"}, []string{"ls"}, 0},
		{"locale unsupported is ignored", map[string]string{"LANG": "fr_FR.UTF-8"}, []string{"ls"}, 0},
		{"unknown flag value", nil, []string{"--lang", "fr", "ls"}, ExitUsage},
		{"empty flag value", nil, []string{"--lang=", "ls"}, ExitUsage},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"BEARINGS_NOTEBOOK": dir}
			for k, v := range tc.env {
				env[k] = v
			}
			r := runCLI(t, env, tc.args...)
			if r.code != tc.code {
				t.Fatalf("exit %d, want %d: %s", r.code, tc.code, r.stderr)
			}
			if tc.code == 0 && !strings.Contains(r.stdout, "technical") {
				t.Errorf("%q", r.stdout)
			}
		})
	}
}

func TestHelpAndUsage(t *testing.T) {
	r := runCLI(t, nil, "--help")
	wantOK(t, r)
	for _, want := range []string{"ls", "show", "search", "journal", "progress", "path", "--notebook", "--json", "--lang", "--no-color", "BEARINGS_NOTEBOOK"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("help lacks %q", want)
		}
	}
	r = runCLI(t, nil)
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Available Commands") {
		t.Errorf("no args should print help: %q", r.stdout)
	}
	wantFail(t, runCLI(t, nil, "--bogus"), ExitUsage, "unknown flag")
	wantFail(t, runCLI(t, nil, "bogus"), ExitUsage, `unknown command "bogus"`)
	r = runCLI(t, nil, "completion", "bash")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "bash completion") {
		t.Errorf("completion script missing")
	}
}

func TestNoColorInBuffers(t *testing.T) {
	dir := newFixture(t)
	for _, env := range []map[string]string{
		{"BEARINGS_NOTEBOOK": dir},
		{"BEARINGS_NOTEBOOK": dir, "NO_COLOR": "1"},
	} {
		r := runCLI(t, env, "--no-color", "progress")
		wantOK(t, r)
		if strings.Contains(r.stdout, "\x1b") {
			t.Errorf("color codes in %q", r.stdout)
		}
	}
}
