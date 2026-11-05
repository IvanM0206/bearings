package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

func entriesOf2(t *testing.T, dir string, args ...string) []map[string]any {
	t.Helper()
	j := decode(t, runIn(t, dir, append([]string{"journal", "--json"}, args...)...).stdout)
	var out []map[string]any
	for _, e := range j["entries"].([]any) {
		out = append(out, e.(map[string]any))
	}
	return out
}

func TestLogText(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "log", "Learned", "how", "deploys", "work.")
	wantOK(t, r)
	es := entriesOf2(t, dir, "--from", "2026-11-04")
	if len(es) != 1 || es[0]["body"] != "Learned how deploys work." || es[0]["date"] != "2026-11-04" || es[0]["section"] != "" {
		t.Fatalf("%v", es)
	}
	id := es[0]["id"].(string)
	if r.stdout != "Added journal entry "+id+" (2026-11-04).\n" {
		t.Errorf("%q", r.stdout)
	}
	rel := "journal/2026/11/2026-11-04-" + id + ".md"
	f := readFile(t, dir, rel)
	if !strings.Contains(f, "created: 2026-11-04T08:30:00Z\n") || !strings.HasSuffix(f, "\nLearned how deploys work.\n") {
		t.Errorf("%s", f)
	}
	if runtime.GOOS != "windows" {
		for p, want := range map[string]os.FileMode{rel: 0o600, filepath.Dir(rel): 0o700, "journal/2026": 0o700} {
			fi, err := os.Stat(filepath.Join(dir, p))
			must(t, err)
			if fi.Mode().Perm() != want {
				t.Errorf("%s: mode %v, want %v", p, fi.Mode().Perm(), want)
			}
		}
	}
}

func TestLogLinks(t *testing.T) {
	dir := newFixture(t)
	r := at(t, dir, "log", "-t", "technical/risk", "Limits again.")
	wantOK(t, r)
	if !strings.HasSuffix(r.stdout, "Linked to technical/risk.\n") {
		t.Errorf("%q", r.stdout)
	}
	wantOK(t, at(t, dir, "log", "-s", "team", "-t", "team/rituals", "Both given.", "--date", "2026-10-31"))
	wantOK(t, at(t, dir, "log", "-s", "team", "Section only."))
	j := decode(t, at(t, dir, "log", "-s", "technical", "--date", "2026-11-01", "With json", "--json").stdout)
	e := j["entry"].(map[string]any)
	if !reflect.DeepEqual(keys(e), []string{"body", "created", "date", "id", "section", "topic", "updated"}) || e["section"] != "technical" || e["topic"] != "" || e["date"] != "2026-11-01" {
		t.Errorf("%v", e)
	}
	got := map[string]string{}
	for _, x := range entriesOf2(t, dir) {
		got[x["body"].(string)] = x["section"].(string) + "|" + x["topic"].(string)
	}
	want := map[string]string{
		"Limits again.": "technical|technical/risk", "Both given.": "team|team/rituals", "Section only.": "team|",
		"With json": "technical|",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%q: got %q, want %q", k, got[k], v)
		}
	}
	// Shows up under the topic.
	if j := decode(t, run(t, dir, "show", "technical/risk", "--json").stdout); len(j["journal"].([]any)) != 1 {
		t.Errorf("%v", j["journal"])
	}
}

func TestLogErrors(t *testing.T) {
	dir := newFixture(t)
	long := strings.Repeat("x", 64<<10+1)
	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantCode int
		want     string
	}{
		{"empty text", []string{"   "}, "", ExitUsage, "entry is empty"},
		{"empty stdin", []string{"-"}, " \n\n", ExitUsage, "entry is empty"},
		{"stdin too long", []string{"-"}, long, ExitUsage, "too long"},
		{"argument too long", []string{long}, "", ExitUsage, "too long"},
		{"stdin not utf8", []string{"-"}, "ok \xff\xfe", ExitUsage, "not valid UTF-8"},
		{"stdin NUL", []string{"-"}, "a\x00b", ExitUsage, "not valid UTF-8"},
		{"bad date", []string{"--date", "2026-02-30", "x"}, "", ExitUsage, "invalid date for --date"},
		{"date format", []string{"--date", "yesterday", "x"}, "", ExitUsage, "invalid date for --date"},
		{"date too old", []string{"--date", "1969-12-31", "x"}, "", ExitUsage, "invalid date"},
		{"unknown section", []string{"-s", "nope", "x"}, "", ExitNotFound, `section "nope" does not exist`},
		{"bad section", []string{"-s", "../x", "x"}, "", ExitUsage, "invalid section"},
		{"unknown topic", []string{"-t", "team/nope", "x"}, "", ExitNotFound, `topic "team/nope" does not exist`},
		{"bad topic", []string{"-t", "team", "x"}, "", ExitUsage, "invalid topic reference"},
		{"topic traversal", []string{"-t", "../../etc/passwd", "x"}, "", ExitUsage, "invalid topic reference"},
		{"section and topic disagree", []string{"-s", "technical", "-t", "team/rituals", "x"}, "", ExitUsage, "topic team/rituals is not in section technical"},
		{"no editor", nil, "", ExitUsage, "no editor configured"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: tt.stdin}, append([]string{"log"}, tt.args...)...)
			wantFail(t, r, tt.wantCode, tt.want)
			if n := len(entriesOf2(t, dir)); n != 3 {
				t.Errorf("a failed log changed the journal: %d entries", n)
			}
		})
	}
}

func TestLogStdin(t *testing.T) {
	dir := newFixture(t)
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}, stdin: "Line one.\n\nLine two with ünïcode.\n", now: laterNow}, "log", "-", "-s", "team")
	wantOK(t, r)
	es := entriesOf2(t, dir, "--from", "2026-11-04")
	if len(es) != 1 || es[0]["body"] != "Line one.\n\nLine two with ünïcode." {
		t.Errorf("%v", es)
	}
}

func TestLogEditor(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'Met the on-call lead.\nShe shared the runbook.\n' > "$last"`)
	r, tmp := runEditing(t, dir, f, "log", "-t", "team/rituals")
	wantOK(t, r)
	rec := f.record()
	if rec["args"] != "2" || rec["first"] != "--wait" {
		t.Errorf("the temp path must be the last argument, after the editor's own: %v", rec)
	}
	if filepath.Base(rec["file"]) != "entry.md" {
		t.Errorf("%v", rec)
	}
	if runtime.GOOS != "windows" && (rec["filemode"] != "600" || rec["dirmode"] != "700") {
		t.Errorf("temp modes: file %s, dir %s", rec["filemode"], rec["dirmode"])
	}
	if f.initial() != "" {
		t.Errorf("a new entry starts from an empty file, got %q", f.initial())
	}
	if _, err := os.Stat(filepath.Dir(rec["file"])); !os.IsNotExist(err) {
		t.Errorf("temp directory not removed: %v", err)
	}
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
	es := entriesOf2(t, dir, "--from", "2026-11-04")
	if len(es) != 1 || es[0]["body"] != "Met the on-call lead.\nShe shared the runbook." || es[0]["topic"] != "team/rituals" {
		t.Errorf("%v", es)
	}
}

func TestLogEditorEmptyAborts(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf '  \n\n' > "$last"`)
	r, tmp := runEditing(t, dir, f, "log")
	wantFail(t, r, ExitFailure, "the text is empty; nothing was saved")
	if n := len(entriesOf2(t, dir)); n != 3 {
		t.Errorf("%d entries", n)
	}
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
}

func TestLogEditorFailure(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'half written' > "$last"; exit 3`)
	r, tmp := runEditing(t, dir, f, "log")
	wantFail(t, r, ExitFailure, "did not finish successfully")
	if n := len(entriesOf2(t, dir)); n != 3 {
		t.Errorf("an editor that fails must not save: %d entries", n)
	}
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left after a failed editor: %v", l)
	}
}

func TestLogEditorCannotStart(t *testing.T) {
	dir := newFixture(t)
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir, "EDITOR": "/nonexistent/editor --x"}}, "log")
	wantFail(t, r, ExitFailure, `cannot start the editor "editor"`)
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
}

func TestLogEditorKilledBySignal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX signals")
	}
	dir := newFixture(t)
	// The editor asks its parent (this process) to terminate. The command
	// handles the signal: it stops the editor and cleans up.
	f := newFakeEditor(t, `printf 'unsaved' > "$last"; kill -TERM $PPID; exec sleep 30`)
	r, tmp := runEditing(t, dir, f, "log")
	wantFail(t, r, ExitFailure, "did not finish successfully")
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left after SIGTERM: %v", l)
	}
	if n := len(entriesOf2(t, dir)); n != 3 {
		t.Errorf("%d entries", n)
	}
}

func TestLogEditorChecksRefsFirst(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'x' > "$last"`)
	r, _ := runEditing(t, dir, f, "log", "-s", "nope")
	wantFail(t, r, ExitNotFound, "does not exist")
	if _, err := os.Stat(filepath.Join(f.logDir, "record")); err == nil {
		t.Error("the editor must not start when the references are invalid")
	}
}

func TestLogEditorInvalidContentKeepsRecovery(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `head -c 70000 /dev/zero | tr '\0' 'a' > "$last"`)
	r, tmp := runEditing(t, dir, f, "log")
	wantFail(t, r, ExitUsage, "too long")
	_ = tmp
	if n := len(entriesOf2(t, dir)); n != 3 {
		t.Errorf("%d entries", n)
	}
	rec := leftovers(t, tmp, "bearings-recovery-")
	if len(rec) != 1 {
		t.Fatalf("expected a recovery directory, got %v; stderr %s", rec, r.stderr)
	}
	if !strings.Contains(r.stderr, filepath.Join(rec[0], "entry.md")) {
		t.Errorf("stderr must name the recovery file: %s", r.stderr)
	}
	b, err := os.ReadFile(filepath.Join(rec[0], "entry.md"))
	must(t, err)
	if len(b) != 70000 {
		t.Errorf("recovery file has %d bytes", len(b))
	}
	if runtime.GOOS != "windows" {
		fi, _ := os.Stat(filepath.Join(rec[0], "entry.md"))
		di, _ := os.Stat(rec[0])
		if fi.Mode().Perm() != 0o600 || di.Mode().Perm() != 0o700 {
			t.Errorf("recovery modes: %v %v", fi.Mode().Perm(), di.Mode().Perm())
		}
	}
}
