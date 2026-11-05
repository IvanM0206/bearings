package cmd

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEditTopic(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'New notes.\n\n- one\n- two\n' > "$last"`)
	before := readFile(t, dir, "sections/team/who-is-who.md")
	r, tmp := runEditing(t, dir, f, "edit", "team/who-is-who")
	wantOK(t, r)
	if r.stdout != "Saved the notes of team/who-is-who.\n" {
		t.Errorf("%q", r.stdout)
	}
	if f.initial() != "See the org chart." {
		t.Errorf("the editor must start from the current notes, got %q", f.initial())
	}
	rec := f.record()
	if filepath.Base(rec["file"]) != "notes.md" || rec["args"] != "2" {
		t.Errorf("%v", rec)
	}
	if l := leftovers(t, tmp, ""); len(l) != 0 {
		t.Errorf("temp files left: %v", l)
	}
	tp := showTopic(t, dir, "team/who-is-who")
	if tp["notes"] != "New notes.\n\n- one\n- two" || tp["updated"] != "2026-11-04T08:30:00Z" || tp["created"] != "2026-11-03T12:00:00Z" || tp["status"] != "mastered" {
		t.Errorf("%v", tp)
	}
	after := readFile(t, dir, "sections/team/who-is-who.md")
	if !strings.HasPrefix(after, "---\ntitle:") || !strings.HasSuffix(after, "\n- two\n") || after == before {
		t.Errorf("%s", after)
	}
}

func TestEditTopicJSON(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'Changed.' > "$last"`)
	r, _ := runEditing(t, dir, f, "edit", "team/rituals", "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if !reflect.DeepEqual(keys(j), []string{"changed", "topic"}) || j["changed"] != true || j["topic"].(map[string]any)["notes"] != "Changed." {
		t.Errorf("%v", j)
	}
}

func TestEditTopicUnchangedDoesNotWrite(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `:`) // leaves the file as it was
	before := readFile(t, dir, "sections/technical/risk.md")
	r, _ := runEditing(t, dir, f, "edit", "technical/risk")
	wantOK(t, r)
	if r.stdout != "No changes.\n" || readFile(t, dir, "sections/technical/risk.md") != before {
		t.Errorf("%q", r.stdout)
	}
	// Only a change in line ends or trailing blanks is not a change either.
	f2 := newFakeEditor(t, `printf 'Limits are reviewed weekly.\r\n\r\n' > "$last"`)
	r, _ = runEditing(t, dir, f2, "edit", "technical/risk", "--json")
	if decode(t, r.stdout)["changed"] != false || readFile(t, dir, "sections/technical/risk.md") != before {
		t.Errorf("%s", r.stdout)
	}
}

func TestEditTopicClearNotes(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `: > "$last"`)
	r, _ := runEditing(t, dir, f, "edit", "technical/risk")
	wantOK(t, r)
	if showTopic(t, dir, "technical/risk")["notes"] != "" {
		t.Error("notes not cleared")
	}
}

func TestEditTopicRejectedContent(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"too large for a topic", `head -c 300000 /dev/zero | tr '\0' 'a' > "$last"`, "too long"},
		{"larger than any file", `head -c 1100000 /dev/zero | tr '\0' 'a' > "$last"`, "larger than a notebook file"},
		{"not UTF-8", `printf 'caf\351\n' > "$last"`, "not valid UTF-8"},
		{"NUL byte", `printf 'a\000b' > "$last"`, "not valid UTF-8"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := newFixture(t)
			before := readFile(t, dir, "sections/technical/risk.md")
			f := newFakeEditor(t, tt.body)
			r, tmp := runEditing(t, dir, f, "edit", "technical/risk")
			wantFail(t, r, ExitUsage, tt.want)
			if readFile(t, dir, "sections/technical/risk.md") != before {
				t.Error("the file must not change")
			}
			if l := leftovers(t, tmp, "bearings-edit-"); len(l) != 0 {
				t.Errorf("temp files left: %v", l)
			}
		})
	}
}

func TestEditTopicConflictKeepsEditInRecoveryFile(t *testing.T) {
	dir := newFixture(t)
	target := filepath.Join(dir, "sections", "technical", "risk.md")
	// While the "user" edits, someone else changes the topic file.
	f := newFakeEditor(t, `printf 'extra: oops\n' >> "`+target+`"; sed -i.bak 's/weekly/daily/' "`+target+`" 2>/dev/null; rm -f "`+target+`.bak"; printf 'My careful notes.\n' > "$last"`)
	r, tmp := runEditing(t, dir, f, "edit", "technical/risk")
	wantFail(t, r, ExitFailure, "the file changed while it was being edited")
	if !strings.Contains(r.stderr, "sections/technical/risk.md") {
		t.Errorf("the message should name the file: %s", r.stderr)
	}
	// The concurrent change is still there; ours did not overwrite it.
	got := readFile(t, dir, "sections/technical/risk.md")
	if !strings.Contains(got, "extra: oops") || strings.Contains(got, "My careful notes.") {
		t.Errorf("the other writer's version must win:\n%s", got)
	}
	rec := leftovers(t, tmp, "bearings-recovery-")
	if len(rec) != 1 {
		t.Fatalf("recovery dirs: %v\n%s", rec, r.stderr)
	}
	path := filepath.Join(rec[0], "notes.md")
	if !strings.Contains(r.stderr, "your text was saved to "+path) {
		t.Errorf("stderr must tell where the text is: %s", r.stderr)
	}
	b, err := os.ReadFile(path)
	must(t, err)
	if string(b) != "My careful notes.\n" {
		t.Errorf("recovery file: %q", b)
	}
	if l := leftovers(t, tmp, "bearings-edit-"); len(l) != 0 {
		t.Errorf("edit temp dir left: %v", l)
	}
}

func TestEditTopicRemovedWhileEditing(t *testing.T) {
	dir := newFixture(t)
	target := filepath.Join(dir, "sections", "technical", "risk.md")
	f := newFakeEditor(t, `rm "`+target+`"; printf 'Notes.\n' > "$last"`)
	r, tmp := runEditing(t, dir, f, "edit", "technical/risk")
	if r.code == 0 {
		t.Fatalf("must fail: %s", r.stdout)
	}
	if _, err := os.Stat(target); err == nil {
		t.Error("the file must not be recreated")
	}
	if len(leftovers(t, tmp, "bearings-recovery-")) != 1 {
		t.Error("expected a recovery file")
	}
}

func TestEditJournal(t *testing.T) {
	dir := newFixture(t)
	es := entriesOf2(t, dir, "--from", "2026-10-20", "--to", "2026-10-20")
	id := es[0]["id"].(string)
	f := newFakeEditor(t, `printf 'First day: got access to the wiki and the chat.\n' > "$last"`)
	r, _ := runEditing(t, dir, f, "edit", "--journal", id)
	wantOK(t, r)
	if r.stdout != "Saved journal entry "+id+".\n" {
		t.Errorf("%q", r.stdout)
	}
	if f.initial() != "First day: got access to the wiki." || filepath.Base(f.record()["file"]) != "entry.md" {
		t.Errorf("%q %v", f.initial(), f.record())
	}
	got := entriesOf2(t, dir, "--from", "2026-10-20", "--to", "2026-10-20")[0]
	if got["body"] != "First day: got access to the wiki and the chat." || got["created"] != es[0]["created"] || got["updated"] != "2026-11-04T08:30:00Z" || got["date"] != "2026-10-20" {
		t.Errorf("%v", got)
	}

	// Clearing the text is refused; the entry stays.
	f2 := newFakeEditor(t, `: > "$last"`)
	r, _ = runEditing(t, dir, f2, "edit", "--journal", id)
	wantFail(t, r, ExitFailure, "nothing was saved")
	if entriesOf2(t, dir, "--from", "2026-10-20", "--to", "2026-10-20")[0]["body"] != got["body"] {
		t.Error("entry changed")
	}
	// Unchanged text.
	f3 := newFakeEditor(t, `:`)
	r, _ = runEditing(t, dir, f3, "edit", "--journal", id, "--json")
	if decode(t, r.stdout)["changed"] != false {
		t.Errorf("%s", r.stdout)
	}
}

func TestEditJournalConflict(t *testing.T) {
	dir := newFixture(t)
	es := entriesOf2(t, dir, "--from", "2026-10-20", "--to", "2026-10-20")
	id := es[0]["id"].(string)
	target := filepath.Join(dir, "journal", "2026", "10", "2026-10-20-"+id+".md")
	f := newFakeEditor(t, `printf 'Added elsewhere.\n' >> "`+target+`"; printf 'Rewritten.\n' > "$last"`)
	r, tmp := runEditing(t, dir, f, "edit", "--journal", id)
	wantFail(t, r, ExitFailure, "changed while it was being edited")
	if !strings.Contains(readFile(t, dir, "journal/2026/10/2026-10-20-"+id+".md"), "Added elsewhere.") {
		t.Error("the other writer's change was lost")
	}
	rec := leftovers(t, tmp, "bearings-recovery-")
	if len(rec) != 1 {
		t.Fatalf("%v", rec)
	}
	if b, _ := os.ReadFile(filepath.Join(rec[0], "entry.md")); string(b) != "Rewritten.\n" {
		t.Errorf("%q", b)
	}
}

func TestEditErrors(t *testing.T) {
	dir := newFixture(t)
	f := newFakeEditor(t, `printf 'x' > "$last"`)
	tests := []struct {
		name     string
		args     []string
		wantCode int
		want     string
	}{
		{"nothing given", nil, ExitUsage, "either a topic"},
		{"both given", []string{"team/rituals", "--journal", "0123456789"}, ExitUsage, "either a topic"},
		{"bad ref", []string{"rituals"}, ExitUsage, "invalid topic reference"},
		{"missing topic", []string{"team/nope"}, ExitNotFound, "does not exist"},
		{"missing section", []string{"nope/x"}, ExitNotFound, "does not exist"},
		{"bad entry id", []string{"--journal", "../../x"}, ExitUsage, "invalid journal entry id"},
		{"empty entry id", []string{"--journal", ""}, ExitUsage, "invalid journal entry id"},
		{"unknown entry", []string{"--journal", "0123456789"}, ExitNotFound, "does not exist"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, _ := runEditing(t, dir, f, append([]string{"edit"}, tt.args...)...)
			wantFail(t, r, tt.wantCode, tt.want)
		})
	}
	if _, err := os.Stat(filepath.Join(f.logDir, "record")); err == nil {
		t.Error("the editor must not start for invalid arguments")
	}
}

func TestEditWithoutEditor(t *testing.T) {
	dir := newFixture(t)
	r := run(t, dir, "edit", "team/rituals")
	wantFail(t, r, ExitUsage, "no editor configured: set $VISUAL or $EDITOR")
}

func TestEditorValueIsNotRunByAShell(t *testing.T) {
	dir := newFixture(t)
	marker := filepath.Join(t.TempDir(), "pwned")
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	// With a shell, this would create the marker file.
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir, "EDITOR": "true; touch " + marker}}, "edit", "team/rituals")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the editor command was interpreted by a shell")
	}
	if r.code == 0 && !strings.Contains(r.stdout, "No changes") {
		t.Errorf("%d %s %s", r.code, r.stdout, r.stderr)
	}
}
