package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

// topicRefs lists the refs of a section in order, through the CLI.
func lsRefs(t *testing.T, dir, section string) []string {
	t.Helper()
	j := decode(t, runIn(t, dir, "ls", section, "--json").stdout)
	var out []string
	for _, x := range j["topics"].([]any) {
		out = append(out, x.(map[string]any)["ref"].(string))
	}
	return out
}

func showTopic(t *testing.T, dir, ref string) map[string]any {
	t.Helper()
	r := runIn(t, dir, "show", ref, "--json")
	wantOK(t, r)
	return decode(t, r.stdout)["topic"].(map[string]any)
}

func TestTopicAdd(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "topic", "add", "team", "On-call", "handover")
	wantOK(t, r)
	if r.stdout != "Added topic team/on-call-handover (On-call handover), personal, pending.\n" {
		t.Errorf("stdout %q", r.stdout)
	}
	if got := lsRefs(t, dir, "team"); !reflect.DeepEqual(got, []string{"team/rituals", "team/who-is-who", "team/on-call-handover"}) {
		t.Errorf("new topics go last: %v", got)
	}
	f := readFile(t, dir, "sections/team/on-call-handover.md")
	for _, want := range []string{"status: pending\n", "type: personal\n", "order: 2\n", "created: 2026-11-04T08:30:00Z\n", "updated: 2026-11-04T08:30:00Z\n"} {
		if !strings.Contains(f, want) {
			t.Errorf("%q not in\n%s", want, f)
		}
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(dir, "sections/team/on-call-handover.md"))
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("mode %v", fi.Mode().Perm())
		}
	}

	r = at(t, dir, "topic", "add", "team", "Rituals", "--type", "role", "--status", "in-progress", "--json")
	wantOK(t, r)
	tp := decode(t, r.stdout)["topic"].(map[string]any)
	if tp["ref"] != "team/rituals-2" || tp["type"] != "role" || tp["status"] != "in-progress" || tp["order"] != 3.0 || tp["section_title"] != "Team" {
		t.Errorf("%v", tp)
	}
	if !reflect.DeepEqual(keys(tp), keys(showTopic(t, dir, "team/rituals-2"))) {
		t.Errorf("add --json must have the same topic shape as show --json")
	}
}

func TestTopicAddInvalid(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{"unknown section", []string{"nope", "X"}, ExitNotFound, "does not exist"},
		{"bad section", []string{"../x", "X"}, ExitUsage, "invalid section"},
		{"blank title", []string{"team", " "}, ExitUsage, "is empty"},
		{"long title", []string{"team", strings.Repeat("x", 161)}, ExitUsage, "too long"},
		{"control characters", []string{"team", "a\nb"}, ExitUsage, "control characters"},
		{"bad type", []string{"team", "X", "--type", "other"}, ExitUsage, "invalid type"},
		{"empty type", []string{"team", "X", "--type", ""}, ExitUsage, "invalid type"},
		{"bad status", []string{"team", "X", "--status", "done"}, ExitUsage, "invalid status"},
		{"missing title", []string{"team"}, ExitUsage, "requires at least 2 arg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wantFail(t, at(t, dir, append([]string{"topic", "add"}, tt.args...)...), tt.wantCode, tt.want)
			if got := lsRefs(t, dir, "team"); len(got) != 2 {
				t.Errorf("a failed add changed the notebook: %v", got)
			}
		})
	}
}

func TestTopicEdit(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "topic", "edit", "technical/scratch", "--title", "Shortcuts I use", "--type", "role", "--clear-suggested")
	wantOK(t, r)
	if r.stdout != "Updated topic technical/scratch.\n" {
		t.Errorf("%q", r.stdout)
	}
	tp := showTopic(t, dir, "technical/scratch")
	if tp["title"] != "Shortcuts I use" || tp["type"] != "role" || tp["suggested"] != false {
		t.Errorf("%v", tp)
	}
	if tp["created"] != "2026-11-03T12:00:00Z" || tp["updated"] != "2026-11-04T08:30:00Z" {
		t.Errorf("created must be kept and updated set: %v", tp)
	}
	if tp["notes"] != "Secret handshake." || tp["order"] != 2.0 || tp["slug"] != "scratch" {
		t.Errorf("other fields must be kept: %v", tp)
	}
}

func TestTopicEditResources(t *testing.T) {
	dir := newFixture(t)
	wantOK(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/rituals", "--resource-title", "Rituals page"))
	wantOK(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "http://example.com/second"))
	res := showTopic(t, dir, "team/rituals")["resources"].([]any)
	want := []any{
		map[string]any{"url": "https://example.com/rituals", "title": "Rituals page"},
		map[string]any{"url": "http://example.com/second", "title": ""},
	}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("%v", res)
	}

	// Removing needs --yes outside a terminal.
	r := at(t, dir, "topic", "edit", "team/rituals", "--remove-resource", "http://example.com/second")
	wantFail(t, r, ExitUsage, "needs confirmation")
	if len(showTopic(t, dir, "team/rituals")["resources"].([]any)) != 2 {
		t.Fatal("resource removed without confirmation")
	}
	// On a terminal, the prompt says what goes away; "n" cancels.
	o := opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, interactive: true, stdin: "n\n"}
	r = runOpts(t, o, "topic", "edit", "team/rituals", "--remove-resource", "http://example.com/second")
	if r.code != ExitFailure || !strings.Contains(r.stderr, "Remove from topic team/rituals the resource http://example.com/second? Resources removed: 1.") {
		t.Errorf("%d %q", r.code, r.stderr)
	}
	if len(showTopic(t, dir, "team/rituals")["resources"].([]any)) != 2 {
		t.Fatal("resource removed after answering no")
	}
	o.stdin = "yes\n"
	wantOK(t, runOpts(t, o, "topic", "edit", "team/rituals", "--remove-resource", "http://example.com/second"))
	wantOK(t, at(t, dir, "topic", "edit", "team/rituals", "--remove-resource", "https://example.com/rituals", "--yes"))
	if res := showTopic(t, dir, "team/rituals")["resources"].([]any); len(res) != 0 {
		t.Errorf("%v", res)
	}
}

func TestTopicEditResourceURLValidation(t *testing.T) {
	dir := newFixture(t)
	for _, u := range []string{
		"javascript:alert(1)", "ftp://example.com/x", "file:///etc/passwd", "https://user:pw@example.com/x",
		"https://user@example.com/", "example.com/x", "https://", "https://exa mple.com", "HTTPS://example.com",
		"https://example.com/\x1b[31m", "https://example.com/" + strings.Repeat("a", 2048), "data:text/html,hi",
	} {
		r := at(t, dir, "topic", "edit", "team/rituals", "--add-resource", u)
		wantFail(t, r, ExitUsage, "invalid URL")
	}
	if len(showTopic(t, dir, "team/rituals")["resources"].([]any)) != 0 {
		t.Error("an invalid URL was stored")
	}
	wantOK(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/a"))
	wantFail(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/a"), ExitUsage, "already has the resource")
	wantFail(t, at(t, dir, "topic", "edit", "team/rituals", "--remove-resource", "https://example.com/zzz", "--yes"), ExitUsage, "no resource with the URL")
	wantFail(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/b", "--resource-title", strings.Repeat("t", 161)), ExitUsage, "too long")
}

func TestTopicEditResourceLimit(t *testing.T) {
	dir := newFixture(t)
	for i := 0; i < 50; i++ {
		wantOK(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/r"+strings.Repeat("x", i)))
	}
	wantFail(t, at(t, dir, "topic", "edit", "team/rituals", "--add-resource", "https://example.com/one-more"), ExitUsage, "too long")
}

func TestTopicEditErrors(t *testing.T) {
	dir := newFixture(t)
	tests := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{"nothing to change", []string{"team/rituals"}, ExitUsage, "nothing to change"},
		{"resource title alone", []string{"team/rituals", "--resource-title", "x"}, ExitUsage, "--resource-title needs --add-resource"},
		{"bad type", []string{"team/rituals", "--type", "x"}, ExitUsage, "invalid type"},
		{"blank title", []string{"team/rituals", "--title", " "}, ExitUsage, "is empty"},
		{"long title", []string{"team/rituals", "--title", strings.Repeat("x", 161)}, ExitUsage, "too long"},
		{"bad ref", []string{"rituals", "--title", "x"}, ExitUsage, "invalid topic reference"},
		{"traversal ref", []string{"../x/y", "--title", "x"}, ExitUsage, "invalid topic reference"},
		{"missing topic", []string{"team/nope", "--title", "x"}, ExitNotFound, "does not exist"},
		{"missing section", []string{"nope/x", "--title", "x"}, ExitNotFound, "does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			before := readFile(t, dir, "sections/team/rituals.md")
			wantFail(t, at(t, dir, append([]string{"topic", "edit"}, tt.args...)...), tt.wantCode, tt.want)
			if readFile(t, dir, "sections/team/rituals.md") != before {
				t.Error("a failed edit changed the file")
			}
		})
	}
}

func TestTopicRm(t *testing.T) {
	t.Run("non-terminal without --yes refuses", func(t *testing.T) {
		dir := newFixture(t)
		wantFail(t, runIn(t, dir, "topic", "rm", "technical/backtesting"), ExitUsage, "needs confirmation")
		wantOK(t, runIn(t, dir, "show", "technical/backtesting"))
	})
	t.Run("--yes deletes and unlinks the journal", func(t *testing.T) {
		dir := newFixture(t)
		r := at(t, dir, "topic", "rm", "technical/backtesting", "--yes")
		wantOK(t, r)
		if r.stdout != "Deleted topic technical/backtesting. Resources deleted: 2. Journal entries unlinked: 1.\n" {
			t.Errorf("%q", r.stdout)
		}
		wantFail(t, run(t, dir, "show", "technical/backtesting"), ExitNotFound, "does not exist")
		if got := lsRefs(t, dir, "technical"); !reflect.DeepEqual(got, []string{"technical/risk", "technical/scratch"}) {
			t.Errorf("%v", got)
		}
		// The entry stays, linked to the section only.
		j := decode(t, run(t, dir, "journal", "--section", "technical", "--json").stdout)
		es := j["entries"].([]any)
		if len(es) != 1 || es[0].(map[string]any)["topic"] != "" || es[0].(map[string]any)["section"] != "technical" {
			t.Errorf("%v", es)
		}
	})
	t.Run("terminal prompt", func(t *testing.T) {
		dir := newFixture(t)
		o := opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, interactive: true, stdin: "\n"}
		r := runOpts(t, o, "topic", "rm", "technical/backtesting")
		want := "Delete topic technical/backtesting (Backtesting)? Resources deleted: 2. Journal entries that lose their link to it: 1. This cannot be undone. [y/N] "
		if r.code != ExitFailure || r.stderr[:len(want)] != want {
			t.Errorf("%d %q", r.code, r.stderr)
		}
		wantOK(t, run(t, dir, "show", "technical/backtesting"))
		o.stdin = "y\n"
		wantOK(t, runOpts(t, o, "topic", "rm", "technical/backtesting"))
		wantFail(t, run(t, dir, "show", "technical/backtesting"), ExitNotFound, "does not exist")
	})
	t.Run("json and errors", func(t *testing.T) {
		dir := newFixture(t)
		j := decode(t, at(t, dir, "topic", "rm", "team/rituals", "-y", "--json").stdout)
		d := j["deleted"].(map[string]any)
		if d["topic"] != "team/rituals" || d["resources"] != 0.0 || d["journal_entries_unlinked"] != 0.0 {
			t.Errorf("%v", j)
		}
		wantFail(t, at(t, dir, "topic", "rm", "team/rituals", "-y"), ExitNotFound, "does not exist")
		wantFail(t, at(t, dir, "topic", "rm", "team", "-y"), ExitUsage, "invalid topic reference")
	})
}

func TestTopicMvUpDown(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "topic", "mv", "technical/scratch", "--up")
	wantOK(t, r)
	if r.stdout != "Moved topic technical/scratch up.\n" {
		t.Errorf("%q", r.stdout)
	}
	if got := lsRefs(t, dir, "technical"); !reflect.DeepEqual(got, []string{"technical/backtesting", "technical/scratch", "technical/risk"}) {
		t.Errorf("%v", got)
	}
	wantOK(t, at(t, dir, "topic", "mv", "technical/scratch", "--up"))
	r = at(t, dir, "topic", "mv", "technical/scratch", "--up")
	if r.stdout != "Topic technical/scratch is already the first.\n" {
		t.Errorf("%q", r.stdout)
	}
	wantOK(t, at(t, dir, "topic", "mv", "technical/scratch", "--down"))
	if got := lsRefs(t, dir, "technical"); !reflect.DeepEqual(got, []string{"technical/backtesting", "technical/scratch", "technical/risk"}) {
		t.Errorf("%v", got)
	}
	wantOK(t, at(t, dir, "topic", "mv", "technical/risk", "--up"))
	r = at(t, dir, "topic", "mv", "technical/scratch", "--down")
	if r.stdout != "Topic technical/scratch is already the last.\n" {
		t.Errorf("%q", r.stdout)
	}
	j := decode(t, at(t, dir, "topic", "mv", "technical/backtesting", "--down", "--json").stdout)
	if j["moved"] != true || !reflect.DeepEqual(keys(j), []string{"moved", "order", "topic"}) ||
		!reflect.DeepEqual(j["order"], []any{"technical/risk", "technical/backtesting", "technical/scratch"}) {
		t.Errorf("%v", j)
	}
	// A new topic still goes last after reordering.
	wantOK(t, at(t, dir, "topic", "add", "technical", "Newest"))
	if got := lsRefs(t, dir, "technical"); got[len(got)-1] != "technical/newest" {
		t.Errorf("%v", got)
	}
}

func TestTopicMvToSection(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "topic", "mv", "technical/backtesting", "--to-section", "team")
	wantOK(t, r)
	if r.stdout != "Moved topic technical/backtesting to team/backtesting.\n" {
		t.Errorf("%q", r.stdout)
	}
	if got := lsRefs(t, dir, "team"); !reflect.DeepEqual(got, []string{"team/rituals", "team/who-is-who", "team/backtesting"}) {
		t.Errorf("moved topics go last: %v", got)
	}
	if got := lsRefs(t, dir, "technical"); !reflect.DeepEqual(got, []string{"technical/risk", "technical/scratch"}) {
		t.Errorf("%v", got)
	}
	// Content kept; journal entry follows.
	tp := showTopic(t, dir, "team/backtesting")
	if tp["notes"] != "Download the sample data first.\nThen run the simulator." || len(tp["resources"].([]any)) != 2 || tp["created"] != "2026-11-03T12:00:00Z" {
		t.Errorf("%v", tp)
	}
	j := decode(t, run(t, dir, "show", "team/backtesting", "--json").stdout)
	if len(j["journal"].([]any)) != 1 {
		t.Errorf("the entry must follow the topic: %v", j["journal"])
	}
	// Errors.
	wantFail(t, at(t, dir, "topic", "mv", "team/backtesting", "--to-section", "team"), ExitUsage, "already in section team")
	wantFail(t, at(t, dir, "topic", "mv", "team/backtesting", "--to-section", "nope"), ExitNotFound, "does not exist")
	wantFail(t, at(t, dir, "topic", "mv", "team/backtesting", "--to-section", "../x"), ExitUsage, "invalid section")
	wantOK(t, at(t, dir, "topic", "add", "technical", "Backtesting"))
	wantFail(t, at(t, dir, "topic", "mv", "technical/backtesting", "--to-section", "team"), ExitUsage, "topic team/backtesting already exists")
	if _, err := os.Stat(filepath.Join(dir, "sections/technical/backtesting.md")); err != nil {
		t.Error("a refused move must leave the source in place")
	}
}

func TestTopicMvNeedsOneTarget(t *testing.T) {
	dir := newFixture(t)
	wantFail(t, at(t, dir, "topic", "mv", "team/rituals"), ExitUsage, "exactly one of --up, --down and --to-section")
	wantFail(t, at(t, dir, "topic", "mv", "team/rituals", "--up", "--down"), ExitUsage, "exactly one")
	wantFail(t, at(t, dir, "topic", "mv", "team/rituals", "--up", "--to-section", "technical"), ExitUsage, "exactly one")
	wantFail(t, at(t, dir, "topic", "mv", "team/nope", "--up"), ExitNotFound, "does not exist")
}

func TestTopicLs(t *testing.T) {
	dir := newFixture(t)
	if a, b := runIn(t, dir, "topic", "ls", "technical"), runIn(t, dir, "ls", "technical"); a.code != 0 || a.stdout != b.stdout {
		t.Errorf("topic ls differs from ls:\n%s\n%s", a.stdout, b.stdout)
	}
	r := runIn(t, dir, "topic", "ls", "technical", "--status", "mastered", "--type", "role")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "risk") || strings.Contains(r.stdout, "scratch") {
		t.Errorf("%s", r.stdout)
	}
	wantFail(t, runIn(t, dir, "topic", "ls"), ExitUsage, "accepts 1 arg")
}

func TestAdvance(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "advance", "team/rituals")
	wantOK(t, r)
	if r.stdout != "team/rituals: pending -> in progress.\n" {
		t.Errorf("%q", r.stdout)
	}
	tp := showTopic(t, dir, "team/rituals")
	if tp["status"] != "in-progress" || tp["updated"] != "2026-11-04T08:30:00Z" || tp["created"] != "2026-11-03T12:00:00Z" {
		t.Errorf("%v", tp)
	}
	r = at(t, dir, "advance", "team/rituals")
	if r.stdout != "team/rituals: in progress -> mastered.\n" {
		t.Errorf("%q", r.stdout)
	}
	// Mastered stays mastered and the file is not rewritten.
	before := readFile(t, dir, "sections/team/rituals.md")
	r = runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, now: laterNow.Add(48 * 3600e9)}, "advance", "team/rituals")
	wantOK(t, r)
	if r.stdout != "team/rituals is already mastered.\n" || readFile(t, dir, "sections/team/rituals.md") != before {
		t.Errorf("%q", r.stdout)
	}
	j := decode(t, at(t, dir, "advance", "team/rituals", "--json").stdout)
	if j["changed"] != false || j["previous_status"] != "mastered" || !reflect.DeepEqual(keys(j), []string{"changed", "previous_status", "topic"}) {
		t.Errorf("%v", j)
	}
	wantFail(t, at(t, dir, "advance", "team/nope"), ExitNotFound, "does not exist")
	wantFail(t, at(t, dir, "advance", "bad"), ExitUsage, "invalid topic reference")
	wantFail(t, at(t, dir, "advance"), ExitUsage, "accepts 1 arg")
}

func TestStatus(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "status", "team/who-is-who", "pending")
	wantOK(t, r)
	if r.stdout != "team/who-is-who: mastered -> pending.\n" {
		t.Errorf("%q", r.stdout)
	}
	if showTopic(t, dir, "team/who-is-who")["status"] != "pending" {
		t.Error("status not set")
	}
	r = at(t, dir, "status", "team/who-is-who", "pending")
	if r.stdout != "team/who-is-who is already pending.\n" {
		t.Errorf("%q", r.stdout)
	}
	j := decode(t, at(t, dir, "status", "team/who-is-who", "in-progress", "--json").stdout)
	if j["changed"] != true || j["previous_status"] != "pending" || j["topic"].(map[string]any)["status"] != "in-progress" {
		t.Errorf("%v", j)
	}
	wantFail(t, at(t, dir, "status", "team/who-is-who", "done"), ExitUsage, "invalid status")
	wantFail(t, at(t, dir, "status", "team/who-is-who", ""), ExitUsage, "invalid status")
	wantFail(t, at(t, dir, "status", "team/nope", "pending"), ExitNotFound, "does not exist")
	wantFail(t, at(t, dir, "status", "team/who-is-who"), ExitUsage, "accepts 2 arg")
}
