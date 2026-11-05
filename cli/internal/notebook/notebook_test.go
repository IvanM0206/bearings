package notebook

import (
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)

type harness struct {
	nb       *Notebook
	dir      string
	now      time.Time
	mu       sync.Mutex
	problems []Problem
}

func (h *harness) codes() []Code {
	h.mu.Lock()
	defer h.mu.Unlock()
	var out []Code
	for _, p := range h.problems {
		out = append(out, CodeOf(p.Err))
	}
	return out
}

func (h *harness) opts() *Options {
	return &Options{
		Now: func() time.Time { return h.now },
		OnProblem: func(p Problem) {
			h.mu.Lock()
			defer h.mu.Unlock()
			h.problems = append(h.problems, p)
		},
	}
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{dir: filepath.Join(t.TempDir(), "nb"), now: t0}
	nb, err := Init(h.dir, Config{Format: 1, Name: "Test"}, h.opts())
	if err != nil {
		t.Fatal(err)
	}
	h.nb = nb
	return h
}

func (h *harness) path(rel string) string { return filepath.Join(h.nb.Root(), filepath.FromSlash(rel)) }

func (h *harness) write(t *testing.T, rel, content string) {
	t.Helper()
	p := h.path(rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func must[T any](t *testing.T) func(T, error) T {
	return func(v T, err error) T {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
}

func ptr(i int) *int { return &i }

func TestInitAndOpen(t *testing.T) {
	h := newHarness(t)
	data, err := os.ReadFile(h.path("bearings.yaml"))
	if err != nil || string(data) != "format: 1\nname: \"Test\"\n" {
		t.Fatalf("%q %v", data, err)
	}
	if _, err := Open(h.dir, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(h.dir, Config{Format: 1}, nil); CodeOf(err) != CodeNotEmpty {
		t.Fatalf("init over a notebook: %v", err)
	}
	empty := filepath.Join(t.TempDir(), "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Init(empty, Config{Format: 1}, nil); err != nil {
		t.Fatalf("init into empty dir: %v", err)
	}
	if _, err := Open(t.TempDir(), nil); CodeOf(err) != CodeNotNotebook {
		t.Fatalf("open non-notebook: %v", err)
	}
	if _, err := Open(filepath.Join(t.TempDir(), "missing"), nil); CodeOf(err) != CodeNotNotebook {
		t.Fatalf("open missing: %v", err)
	}
	bad := t.TempDir()
	if err := os.WriteFile(filepath.Join(bad, "bearings.yaml"), []byte("format: 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(bad, nil); CodeOf(err) != CodeUnsupportedFormat {
		t.Fatalf("open format 2: %v", err)
	}
}

func TestModes(t *testing.T) {
	if os.PathSeparator == '\\' {
		t.Skip("POSIX modes")
	}
	h := newHarness(t)
	sec := must[Section](t)(h.nb.AddSection(NewSection{Title: "Technical"}))
	must[Topic](t)(h.nb.AddTopic(NewTopic{Section: sec.Slug, Title: "Backtesting"}))
	must[Entry](t)(h.nb.AddEntry(NewEntry{Body: "x"}))
	err := filepath.WalkDir(h.nb.Root(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			return err
		}
		want := fs.FileMode(0o600)
		if fi.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", p, fi.Mode().Perm(), want)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestSectionsAndTopics(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	tech := must[Section](t)(nb.AddSection(NewSection{Title: "Técnico", Description: "Stack\n"}))
	team := must[Section](t)(nb.AddSection(NewSection{Title: "Team", Order: ptr(0)}))
	if tech.Slug != "tecnico" || tech.Order != 0 || team.Order != 0 {
		t.Fatalf("%+v %+v", tech, team)
	}
	again := must[Section](t)(nb.AddSection(NewSection{Title: "Tecnico"}))
	if again.Slug != "tecnico-2" || again.Order != 1 {
		t.Fatalf("%+v", again)
	}
	if _, err := nb.AddSection(NewSection{Slug: "tecnico", Title: "x"}); CodeOf(err) != CodeExists {
		t.Fatal(err)
	}
	if _, err := nb.AddSection(NewSection{Slug: "../x", Title: "x"}); CodeOf(err) != CodeInvalidSlug {
		t.Fatal(err)
	}
	if _, err := nb.AddSection(NewSection{Title: "\x1b[31m"}); CodeOf(err) != CodeControlChar {
		t.Fatal(err)
	}
	secs := must[[]Section](t)(nb.Sections())
	if len(secs) != 3 || secs[0].Slug != "team" || secs[1].Slug != "tecnico" || secs[2].Slug != "tecnico-2" {
		t.Fatalf("order: %+v", secs)
	}

	a := must[Topic](t)(nb.AddTopic(NewTopic{Section: "tecnico", Title: "Arquitectura", Type: TypeRole}))
	b := must[Topic](t)(nb.AddTopic(NewTopic{Section: "tecnico", Title: "Backtesting", Order: ptr(0)}))
	if a.Status != StatusPending || b.Type != TypePersonal || a.Order != 0 {
		t.Fatalf("%+v %+v", a, b)
	}
	if _, err := nb.AddTopic(NewTopic{Section: "missing", Title: "x"}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.AddTopic(NewTopic{Section: "tecnico", Title: "x", Status: "done"}); CodeOf(err) != CodeInvalidValue {
		t.Fatal(err)
	}
	if _, err := nb.AddTopic(NewTopic{Section: "tecnico", Title: "x", Resources: []Resource{{URL: "ftp://x"}}}); CodeOf(err) != CodeInvalidValue {
		t.Fatal(err)
	}
	ts := must[[]Topic](t)(nb.Topics("tecnico"))
	if len(ts) != 2 || ts[0].Slug != "arquitectura" || ts[1].Slug != "backtesting" {
		t.Fatalf("topic order: %+v", ts)
	}

	// Update with conflict detection.
	h.now = t0.Add(time.Hour)
	a.Status, a.Notes = StatusInProgress, "notes"
	a2 := must[Topic](t)(nb.UpdateTopic(a))
	if !a2.Updated.Equal(t0.Add(time.Hour)) || !a2.Created.Equal(t0) {
		t.Fatalf("%+v", a2)
	}
	if _, err := nb.UpdateTopic(a); CodeOf(err) != CodeConflict {
		t.Fatalf("stale update: %v", err)
	}
	a2.Version = Version{}
	if _, err := nb.UpdateTopic(a2); CodeOf(err) != CodeConflict {
		t.Fatalf("update without version: %v", err)
	}

	got := must[Topic](t)(nb.Topic(TopicRef{"tecnico", "arquitectura"}))
	if got.Notes != "notes" || got.Status != StatusInProgress {
		t.Fatalf("%+v", got)
	}
	tech = must[Section](t)(nb.Section("tecnico"))
	tech.Title = "Technical"
	must[Section](t)(nb.UpdateSection(tech))
	if _, err := nb.UpdateSection(tech); CodeOf(err) != CodeConflict {
		t.Fatal(err)
	}

	filtered := must[[]Topic](t)(nb.FindTopics(TopicFilter{Type: TypeRole}))
	if len(filtered) != 1 || filtered[0].Slug != "arquitectura" {
		t.Fatalf("%+v", filtered)
	}
	path := must[[]RoleStep](t)(nb.RolePath())
	if len(path) != 1 || path[0].Step != 1 || path[0].SectionTitle != "Technical" {
		t.Fatalf("%+v", path)
	}
	p := must[Progress](t)(nb.Progress())
	if p.All.Total != 2 || p.All.InProgress != 1 || p.Role.Total != 1 || p.Personal.Pending != 1 || len(p.Sections) != 3 {
		t.Fatalf("%+v", p)
	}
}

func TestRolePathOrdering(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "b", Title: "B", Order: ptr(1)}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "a", Title: "A", Order: ptr(2)}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "c", Title: "C", Order: ptr(1)}))
	for _, x := range []struct {
		sec, slug string
		order     int
		typ       TopicType
	}{{"a", "z", 0, TypeRole}, {"a", "y", 0, TypeRole}, {"b", "x", 5, TypeRole}, {"b", "w", 6, TypePersonal}, {"c", "v", 0, TypeRole}} {
		must[Topic](t)(nb.AddTopic(NewTopic{Section: x.sec, Slug: x.slug, Title: x.slug, Order: ptr(x.order), Type: x.typ}))
	}
	var got []string
	for _, s := range must[[]RoleStep](t)(nb.RolePath()) {
		got = append(got, s.Topic.Ref().String())
	}
	if strings.Join(got, ",") != "b/x,c/v,a/y,a/z" {
		t.Fatal(got)
	}
}

func TestJournal(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "BT"}))
	d1, d2 := Date{2026, 11, 1}, Date{2026, 11, 3}
	e1 := must[Entry](t)(nb.AddEntry(NewEntry{Date: d1, Body: "first"}))
	h.now = t0.Add(time.Minute)
	e2 := must[Entry](t)(nb.AddEntry(NewEntry{Date: d2, Topic: TopicRef{"tech", "bt"}, Body: "second"}))
	h.now = t0.Add(2 * time.Minute)
	e3 := must[Entry](t)(nb.AddEntry(NewEntry{Date: d2, Section: "tech", Body: "third"}))
	if e2.Section != "tech" {
		t.Fatalf("topic must set section: %+v", e2)
	}
	if _, err := os.Stat(h.path("journal/2026/11/2026-11-03-" + e2.ID + ".md")); err != nil {
		t.Fatal(err)
	}
	all := must[[]Entry](t)(nb.Journal(JournalFilter{}))
	if len(all) != 3 || all[0].ID != e3.ID || all[1].ID != e2.ID || all[2].ID != e1.ID {
		t.Fatalf("journal order: %+v", all)
	}
	if got := must[[]Entry](t)(nb.Journal(JournalFilter{From: d2})); len(got) != 2 {
		t.Fatal(got)
	}
	if got := must[[]Entry](t)(nb.Journal(JournalFilter{To: d1})); len(got) != 1 {
		t.Fatal(got)
	}
	if got := must[[]Entry](t)(nb.Journal(JournalFilter{Section: "tech"})); len(got) != 2 {
		t.Fatal(got)
	}
	if got := must[[]Entry](t)(nb.Journal(JournalFilter{Topic: TopicRef{"tech", "bt"}})); len(got) != 1 {
		t.Fatal(got)
	}
	if got := must[[]Entry](t)(nb.Journal(JournalFilter{Limit: 1})); len(got) != 1 || got[0].ID != e3.ID {
		t.Fatal(got)
	}

	// Validation of new entries.
	if _, err := nb.AddEntry(NewEntry{Body: "   "}); CodeOf(err) != CodeEmpty {
		t.Fatal(err)
	}
	if _, err := nb.AddEntry(NewEntry{Body: "x", Section: "nope"}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.AddEntry(NewEntry{Body: "x", Topic: TopicRef{"tech", "nope"}}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.AddEntry(NewEntry{Body: "x", Section: "tech", Topic: TopicRef{"other", "bt"}}); CodeOf(err) != CodeTopicSectionMismatch {
		t.Fatal(err)
	}
	if _, err := nb.AddEntry(NewEntry{Body: "x", Date: Date{1969, 1, 1}}); CodeOf(err) != CodeInvalidDate {
		t.Fatal(err)
	}

	// Update and delete.
	e1.Body = "first, edited"
	e1 = must[Entry](t)(nb.UpdateEntry(e1))
	if got := must[Entry](t)(nb.Entry(e1.ID)); got.Body != "first, edited" {
		t.Fatal(got)
	}
	stale := e1
	stale.Version = versionOf([]byte("other"))
	if _, err := nb.UpdateEntry(stale); CodeOf(err) != CodeConflict {
		t.Fatal(err)
	}
	if err := nb.DeleteEntry(e1.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := nb.Entry(e1.ID); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.Entry("../../x"); CodeOf(err) != CodeInvalidID {
		t.Fatal(err)
	}
}

func TestDanglingReferences(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	h.write(t, "journal/2026/11/2026-11-03-aaaaaaaaaa.md",
		"---\ndate: 2026-11-03\nsection: gone\ntopic: gone/x\ncreated: 2026-11-03T00:00:00Z\nupdated: 2026-11-03T00:00:00Z\n---\nx\n")
	h.write(t, "journal/2026/11/2026-11-03-bbbbbbbbbb.md",
		"---\ndate: 2026-11-03\nsection: tech\ntopic: tech/gone\ncreated: 2026-11-03T00:00:00Z\nupdated: 2026-11-03T00:00:00Z\n---\nx\n")
	es := must[[]Entry](t)(nb.Journal(JournalFilter{}))
	if len(es) != 2 {
		t.Fatal(es)
	}
	for _, e := range es {
		switch e.ID {
		case "aaaaaaaaaa":
			if e.Section != "" || !e.Topic.IsZero() {
				t.Fatalf("not unlinked: %+v", e)
			}
		case "bbbbbbbbbb":
			if e.Section != "tech" || !e.Topic.IsZero() {
				t.Fatalf("not unlinked: %+v", e)
			}
		}
	}
	if n := len(h.codes()); n != 2 {
		t.Fatalf("expected 2 warnings, got %v", h.codes())
	}
}

func TestMoveAndDeleteUpdateReferences(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "team", Title: "Team"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "BT"}))
	e := must[Entry](t)(nb.AddEntry(NewEntry{Topic: TopicRef{"tech", "bt"}, Body: "x"}))

	moved := must[Topic](t)(nb.MoveTopic(TopicRef{"tech", "bt"}, TopicRef{"team", "backtest"}))
	if moved.Ref().String() != "team/backtest" {
		t.Fatal(moved)
	}
	if _, err := nb.Topic(TopicRef{"tech", "bt"}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if got := must[Entry](t)(nb.Entry(e.ID)); got.Topic.String() != "team/backtest" || got.Section != "team" {
		t.Fatalf("%+v", got)
	}
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "x", Title: "X"}))
	if _, err := nb.MoveTopic(TopicRef{"tech", "x"}, TopicRef{"team", "backtest"}); CodeOf(err) != CodeExists {
		t.Fatal(err)
	}
	if _, err := nb.MoveTopic(TopicRef{"tech", "x"}, TopicRef{"nope", "x"}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}

	must[Section](t)(nb.MoveSection("team", "people"))
	if got := must[Entry](t)(nb.Entry(e.ID)); got.Topic.String() != "people/backtest" || got.Section != "people" {
		t.Fatalf("%+v", got)
	}
	if _, err := nb.MoveSection("people", "tech"); CodeOf(err) != CodeExists {
		t.Fatal(err)
	}
	if _, err := nb.MoveSection("people", "../x"); CodeOf(err) != CodeInvalidSlug {
		t.Fatal(err)
	}

	if err := nb.DeleteTopic(TopicRef{"people", "backtest"}); err != nil {
		t.Fatal(err)
	}
	if got := must[Entry](t)(nb.Entry(e.ID)); !got.Topic.IsZero() || got.Section != "people" {
		t.Fatalf("%+v", got)
	}
	if err := nb.DeleteSection("people"); err != nil {
		t.Fatal(err)
	}
	if got := must[Entry](t)(nb.Entry(e.ID)); got.Section != "" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Lstat(h.path("sections/people")); !os.IsNotExist(err) {
		t.Fatalf("section dir still there: %v", err)
	}
	// Unknown files inside a deleted section are kept.
	h.write(t, "sections/tech/README.txt", "keep")
	if err := nb.DeleteSection("tech"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.path("sections/tech/README.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(h.path("sections/tech/x.md")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestInvalidFilesAreSkipped(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "ok", Title: "OK"}))
	h.write(t, "sections/tech/bad.md", "---\ntitle: &a x\n---\n")
	h.write(t, "sections/orphan/topic.md", validTopic)
	h.write(t, "sections/emptydir/README.md", "not a section")
	h.write(t, "sections/broken/_section.md", "nope")
	h.write(t, "README.md", "ignored")
	h.write(t, ".git/config", "ignored")
	h.write(t, "journal/2026/11/notes.txt", "ignored")
	h.write(t, "journal/2026/12/2026-11-03-aaaaaaaaaa.md", validEntry)
	h.write(t, "sections/tech/.ok.md.swp", "ignored")

	secs := must[[]Section](t)(nb.Sections())
	if len(secs) != 1 || secs[0].Slug != "tech" {
		t.Fatalf("%+v", secs)
	}
	ts := must[[]Topic](t)(nb.Topics("tech"))
	if len(ts) != 1 {
		t.Fatalf("%+v", ts)
	}
	must[[]Entry](t)(nb.Journal(JournalFilter{}))
	got := map[Code]int{}
	for _, c := range h.codes() {
		got[c]++
	}
	if got[CodeSectionMissing] != 1 || got[CodeYAMLAnchor] != 1 || got[CodeMissingFrontmatter] != 1 || got[CodePathMismatch] != 1 || len(h.codes()) != 4 {
		t.Fatalf("problems: %v", h.codes())
	}
	if _, err := nb.Topics("orphan"); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.Topic(TopicRef{"orphan", "topic"}); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
}

func TestSearch(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Técnico", Description: "Arquitectura del bot"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "risk", Title: "Gestión de riesgo", Notes: strings.Repeat("filler ", 20) + "STOP-LOSS rules\nnext"}))
	must[Entry](t)(nb.AddEntry(NewEntry{Body: "Learned about the stop-loss today"}))
	hits := must[[]SearchHit](t)(nb.Search("stop-loss", SearchOptions{}))
	if len(hits) != 2 || hits[0].Kind != HitTopic || hits[1].Kind != HitEntry || hits[0].Field != "body" {
		t.Fatalf("%+v", hits)
	}
	if !strings.HasPrefix(hits[0].Snippet, "…") || !strings.Contains(hits[0].Snippet, "STOP-LOSS rules next") {
		t.Fatalf("snippet %q", hits[0].Snippet)
	}
	if hits := must[[]SearchHit](t)(nb.Search("gestion", SearchOptions{})); len(hits) != 1 || hits[0].Field != "title" {
		t.Fatalf("%+v", hits)
	}
	if hits := must[[]SearchHit](t)(nb.Search("TECNICO", SearchOptions{})); len(hits) != 1 || hits[0].Kind != HitSection {
		t.Fatalf("%+v", hits)
	}
	if hits := must[[]SearchHit](t)(nb.Search("o", SearchOptions{Limit: 2})); len(hits) != 2 {
		t.Fatalf("%+v", hits)
	}
	for _, q := range []string{"", "  ", strings.Repeat("q", MaxQueryLen+1), "a\x1bb"} {
		if _, err := nb.Search(q, SearchOptions{}); CodeOf(err) != CodeInvalidQuery {
			t.Fatalf("%q: %v", q, err)
		}
	}
}

func TestSearchResources(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "Backtesting", Notes: "plain notes",
		Resources: []Resource{{URL: "https://example.com/wiki/quantlib", Title: "Candle Handbook"}}}))
	for _, q := range []string{"handbook", "QUANTLIB"} {
		hits := must[[]SearchHit](t)(nb.Search(q, SearchOptions{}))
		if len(hits) != 1 || hits[0].Kind != HitTopic || hits[0].Field != "resources" || hits[0].Snippet == "" {
			t.Fatalf("%q: %+v", q, hits)
		}
	}
	// Title and body matches win over resources.
	if hits := must[[]SearchHit](t)(nb.Search("backtesting", SearchOptions{})); len(hits) != 1 || hits[0].Field != "title" {
		t.Fatalf("%+v", hits)
	}
}

func TestSourceEditing(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	tp := must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "BT"}))
	src := must[Source](t)(nb.TopicSource(tp.Ref()))
	if src.Path != "sections/tech/bt.md" {
		t.Fatal(src.Path)
	}
	h.now = t0.Add(time.Hour)
	edited := bytes.Replace(src.Data, []byte("status: pending"), []byte("status: mastered"), 1)
	edited = append(edited, []byte("New notes\r\n")...)
	v, err := nb.ReplaceSource(src, edited)
	if err != nil {
		t.Fatal(err)
	}
	got := v.(Topic)
	if got.Status != StatusMastered || got.Notes != "New notes" || !got.Updated.Equal(t0.Add(time.Hour)) {
		t.Fatalf("%+v", got)
	}
	// Same source again: the file changed since it was read.
	if _, err := nb.ReplaceSource(src, edited); CodeOf(err) != CodeConflict {
		t.Fatal(err)
	}
	src = must[Source](t)(nb.TopicSource(tp.Ref()))
	if _, err := nb.ReplaceSource(src, []byte("---\ntitle: &x y\n---\n")); CodeOf(err) != CodeYAMLAnchor {
		t.Fatal(err)
	}
	if after := must[Source](t)(nb.TopicSource(tp.Ref())); !bytes.Equal(after.Data, src.Data) {
		t.Fatal("invalid edit modified the file")
	}
	e := must[Entry](t)(nb.AddEntry(NewEntry{Date: Date{2026, 11, 3}, Body: "x"}))
	esrc := must[Source](t)(nb.EntrySource(e.ID))
	moved := bytes.Replace(esrc.Data, []byte("date: 2026-11-03"), []byte("date: 2026-11-04"), 1)
	if _, err := nb.ReplaceSource(esrc, moved); CodeOf(err) != CodePathMismatch {
		t.Fatal(err)
	}
	bad := Source{Path: "../etc/passwd"}
	if _, err := nb.ReplaceSource(bad, []byte("x")); err == nil {
		t.Fatal("expected error")
	}
}

func TestConfigUpdate(t *testing.T) {
	h := newHarness(t)
	c := must[Config](t)(h.nb.Config())
	c.Language = "es"
	must[Config](t)(h.nb.UpdateConfig(c))
	if _, err := h.nb.UpdateConfig(c); CodeOf(err) != CodeConflict {
		t.Fatal(err)
	}
	c = must[Config](t)(h.nb.Config())
	c.Language = "fr"
	if _, err := h.nb.UpdateConfig(c); CodeOf(err) != CodeInvalidValue {
		t.Fatal(err)
	}
}

func TestConcurrentWrites(t *testing.T) {
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	var wg sync.WaitGroup
	errs := make(chan error, 20)
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := h.nb.AddEntry(NewEntry{Section: "tech", Body: "concurrent"})
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if es := must[[]Entry](t)(h.nb.Journal(JournalFilter{})); len(es) != 20 {
		t.Fatal(len(es))
	}
}

func TestMoveTopicToOtherSectionGoesLast(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "a", Title: "A"}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "b", Title: "B"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "a", Slug: "one", Title: "One"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "b", Slug: "x", Title: "X"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "b", Slug: "y", Title: "Y"}))
	moved := must[Topic](t)(nb.MoveTopic(TopicRef{"a", "one"}, TopicRef{"b", "one"}))
	if moved.Order != 2 {
		t.Fatalf("order %d, want 2", moved.Order)
	}
}

func orderOf(t *testing.T, nb *Notebook, section string) []string {
	t.Helper()
	ts := must[[]Topic](t)(nb.Topics(section))
	var out []string
	for _, x := range ts {
		out = append(out, x.Slug)
	}
	return out
}

func TestReorderTopic(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "a", Title: "A"}))
	for _, s := range []string{"one", "two", "three"} {
		must[Topic](t)(nb.AddTopic(NewTopic{Section: "a", Slug: s, Title: s}))
	}
	h.now = h.now.Add(time.Hour)
	moved, err := nb.ReorderTopic(TopicRef{"a", "three"}, -1)
	if err != nil || !moved {
		t.Fatal(moved, err)
	}
	if got := strings.Join(orderOf(t, nb, "a"), ","); got != "one,three,two" {
		t.Fatal(got)
	}
	untouched := must[Topic](t)(nb.Topic(TopicRef{"a", "one"}))
	if !untouched.Updated.Equal(t0) {
		t.Errorf("a topic that did not change must keep `updated`: %v", untouched.Updated)
	}
	if moved, _ := nb.ReorderTopic(TopicRef{"a", "one"}, -1); moved {
		t.Error("first topic moved up")
	}
	if moved, _ := nb.ReorderTopic(TopicRef{"a", "two"}, 1); moved {
		t.Error("last topic moved down")
	}
	if _, err := nb.ReorderTopic(TopicRef{"a", "nope"}, 1); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
	if _, err := nb.ReorderTopic(TopicRef{"a", "../x"}, 1); CodeOf(err) != CodeInvalidRef {
		t.Fatal(err)
	}
}

func TestReorderWithEqualOrders(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "a", Title: "A", Order: ptr(5)}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "b", Title: "B", Order: ptr(5)}))
	must[Section](t)(nb.AddSection(NewSection{Slug: "c", Title: "C", Order: ptr(5)}))
	// Sorted by slug: a, b, c. Move c up.
	moved, err := nb.ReorderSection("c", -1)
	if err != nil || !moved {
		t.Fatal(moved, err)
	}
	var got []string
	for _, s := range must[[]Section](t)(nb.Sections()) {
		got = append(got, s.Slug)
	}
	if strings.Join(got, ",") != "a,c,b" {
		t.Fatal(got)
	}
	if _, err := nb.ReorderSection("nope", 1); CodeOf(err) != CodeNotFound {
		t.Fatal(err)
	}
}

func TestReorderTopicWithTiesElsewhere(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "a", Title: "A"}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "a", Slug: "a1", Title: "1", Order: ptr(0)}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "a", Slug: "b2", Title: "2", Order: ptr(0)}))
	must[Topic](t)(nb.AddTopic(NewTopic{Section: "a", Slug: "c3", Title: "3", Order: ptr(1)}))
	if _, err := nb.ReorderTopic(TopicRef{"a", "c3"}, -1); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(orderOf(t, nb, "a"), ","); got != "a1,c3,b2" {
		t.Fatal(got)
	}
}
