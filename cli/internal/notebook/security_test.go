package notebook

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func symlinkOrSkip(t *testing.T, target, link string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(link), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks not supported: %v", err)
	}
}

// outside creates a directory outside the notebook holding a secret file
// and a valid-looking section, and returns its path.
func outside(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "secret.md"), []byte(validTopic), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "_section.md"), []byte(validSection), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSymlinksAreNeverFollowed(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	ext := outside(t)
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))

	// A section directory that is a symlink to elsewhere.
	symlinkOrSkip(t, ext, h.path("sections/linked"))
	// A topic file that is a symlink to a file outside.
	symlinkOrSkip(t, filepath.Join(ext, "secret.md"), h.path("sections/tech/secret.md"))
	// A journal month directory that is a symlink.
	symlinkOrSkip(t, ext, h.path("journal/2026/11"))

	secs := must[[]Section](t)(nb.Sections())
	if len(secs) != 1 || secs[0].Slug != "tech" {
		t.Fatalf("symlinked section was read: %+v", secs)
	}
	if ts := must[[]Topic](t)(nb.Topics("tech")); len(ts) != 0 {
		t.Fatalf("symlinked topic was read: %+v", ts)
	}
	must[[]Entry](t)(nb.Journal(JournalFilter{}))
	n := 0
	for _, c := range h.codes() {
		if c == CodeSymlink {
			n++
		}
	}
	if n != 3 {
		t.Fatalf("expected 3 symlink warnings, got %v", h.codes())
	}

	// Direct access fails too.
	if _, err := nb.Topic(TopicRef{"tech", "secret"}); CodeOf(err) != CodeSymlink {
		t.Fatalf("direct read through symlink: %v", err)
	}
	if _, err := nb.Section("linked"); CodeOf(err) != CodeSymlink {
		t.Fatalf("section through symlink: %v", err)
	}
	if _, err := nb.TopicSource(TopicRef{"tech", "secret"}); CodeOf(err) != CodeSymlink {
		t.Fatal(err)
	}

	// Writes through symlinks are refused and do not touch the target.
	if _, err := nb.AddTopic(NewTopic{Section: "linked", Title: "x"}); CodeOf(err) != CodeSymlink {
		t.Fatalf("write through symlinked dir: %v", err)
	}
	if _, err := nb.AddTopic(NewTopic{Section: "tech", Slug: "secret", Title: "x"}); CodeOf(err) != CodeExists {
		t.Fatalf("overwrite of a symlink: %v", err)
	}
	if _, err := nb.AddEntry(NewEntry{Date: Date{2026, 11, 3}, Body: "x"}); CodeOf(err) != CodeSymlink {
		t.Fatalf("entry through symlinked month: %v", err)
	}
	if err := nb.DeleteTopic(TopicRef{"tech", "secret"}); CodeOf(err) != CodeSymlink {
		t.Fatalf("delete through symlink: %v", err)
	}
	if _, err := nb.MoveSection("linked", "other"); CodeOf(err) != CodeSymlink {
		t.Fatalf("move symlinked section: %v", err)
	}
	if err := nb.DeleteSection("linked"); CodeOf(err) != CodeSymlink {
		t.Fatalf("delete symlinked section: %v", err)
	}
	entries, err := os.ReadDir(ext)
	if err != nil || len(entries) != 2 {
		t.Fatalf("outside directory was modified: %v %v", entries, err)
	}
	data, _ := os.ReadFile(filepath.Join(ext, "secret.md"))
	if string(data) != validTopic {
		t.Fatal("outside file was modified")
	}
}

func TestSymlinkedTopLevelDirs(t *testing.T) {
	for _, dir := range []string{"sections", "journal"} {
		t.Run(dir, func(t *testing.T) {
			h := newHarness(t)
			ext := t.TempDir()
			symlinkOrSkip(t, ext, h.path(dir))
			_, err1 := h.nb.Sections()
			_, err2 := h.nb.Journal(JournalFilter{})
			if dir == "sections" && CodeOf(err1) != CodeSymlink {
				t.Fatalf("sections: %v", err1)
			}
			if dir == "journal" && CodeOf(err2) != CodeSymlink {
				t.Fatalf("journal: %v", err2)
			}
			if _, err := h.nb.AddSection(NewSection{Title: "x"}); dir == "sections" && CodeOf(err) != CodeSymlink {
				t.Fatalf("add section: %v", err)
			}
			if _, err := h.nb.AddEntry(NewEntry{Body: "x"}); dir == "journal" && CodeOf(err) != CodeSymlink {
				t.Fatalf("add entry: %v", err)
			}
			if entries, _ := os.ReadDir(ext); len(entries) != 0 {
				t.Fatal("wrote through a symlinked directory")
			}
		})
	}
}

func TestSymlinkedConfig(t *testing.T) {
	dir := t.TempDir()
	ext := filepath.Join(t.TempDir(), "bearings.yaml")
	if err := os.WriteFile(ext, []byte("format: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlinkOrSkip(t, ext, filepath.Join(dir, "bearings.yaml"))
	if _, err := Open(dir, nil); CodeOf(err) != CodeSymlink {
		t.Fatalf("open with symlinked config: %v", err)
	}
}

func TestSymlinkedRootIsResolvedOnce(t *testing.T) {
	h := newHarness(t)
	link := filepath.Join(t.TempDir(), "link")
	symlinkOrSkip(t, h.dir, link)
	nb, err := Open(link, nil)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(h.dir)
	if nb.Root() != want || !filepath.IsAbs(nb.Root()) {
		t.Fatalf("root %q, want %q", nb.Root(), want)
	}
}

func TestPathTraversalIsRejected(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	bad := []string{"..", "../x", "a/../../b", "/etc", "C:\\x", "a\\b", "x\x00y", "", ".", "Tech", "a/b"}
	for _, s := range bad {
		if _, err := nb.Section(s); CodeOf(err) != CodeInvalidSlug {
			t.Errorf("Section(%q): %v", s, err)
		}
		if _, err := nb.Topic(TopicRef{"tech", s}); CodeOf(err) != CodeInvalidRef {
			t.Errorf("Topic(tech/%q): %v", s, err)
		}
		if _, err := nb.AddSection(NewSection{Slug: s, Title: "x"}); s != "" && CodeOf(err) != CodeInvalidSlug {
			t.Errorf("AddSection(%q): %v", s, err)
		}
		if _, err := nb.AddTopic(NewTopic{Section: "tech", Slug: s, Title: "x"}); s != "" && CodeOf(err) != CodeInvalidSlug {
			t.Errorf("AddTopic(%q): %v", s, err)
		}
		if _, err := nb.MoveSection("tech", s); CodeOf(err) != CodeInvalidSlug {
			t.Errorf("MoveSection(%q): %v", s, err)
		}
		if _, err := nb.Journal(JournalFilter{Section: s}); s != "" && CodeOf(err) != CodeInvalidSlug {
			t.Errorf("Journal(%q): %v", s, err)
		}
	}
	// The low-level path builder refuses anything but plain relative paths.
	for _, rel := range []string{"", "/abs", "../x", "a/../b", "a//b", "./a", "a\\b", "c:x", "a\x00"} {
		if _, err := nb.fs.abs(rel); CodeOf(err) != CodeOutsideRoot {
			t.Errorf("abs(%q): %v", rel, err)
		}
	}
	if p, err := nb.fs.abs("sections/tech/_section.md"); err != nil || !strings.HasPrefix(p, nb.Root()+string(filepath.Separator)) {
		t.Fatal(p, err)
	}
}

func TestFailedWriteLeavesOldFileIntact(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	tp := must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "Original"}))
	before, err := os.ReadFile(h.path("sections/tech/bt.md"))
	if err != nil {
		t.Fatal(err)
	}
	var tmpSeen string
	testHookBeforeRename = func(tmp string) error {
		tmpSeen = tmp
		// The temporary file is complete, synced and private at this point.
		if fi, err := os.Stat(tmp); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
			t.Errorf("temp file: %v %v", fi, err)
		}
		return errors.New("disk full")
	}
	defer func() { testHookBeforeRename = nil }()
	tp.Title = "Changed"
	if _, err := nb.UpdateTopic(tp); CodeOf(err) != CodeIO {
		t.Fatalf("expected I/O error, got %v", err)
	}
	after, err := os.ReadFile(h.path("sections/tech/bt.md"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("old file changed: %v", err)
	}
	if _, err := os.Stat(tmpSeen); !os.IsNotExist(err) {
		t.Fatalf("temporary file left behind: %v", err)
	}
	entries, _ := os.ReadDir(h.path("sections/tech"))
	if len(entries) != 2 {
		t.Fatalf("unexpected files: %v", entries)
	}
	// New files are not created either.
	if _, err := nb.AddTopic(NewTopic{Section: "tech", Slug: "new", Title: "New"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := os.Stat(h.path("sections/tech/new.md")); !os.IsNotExist(err) {
		t.Fatal("partial file created")
	}
}

func TestConflictWhenFileChangedOnDisk(t *testing.T) {
	h := newHarness(t)
	nb := h.nb
	must[Section](t)(nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	tp := must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "bt", Title: "Original"}))
	// Another program edits the file after we read it.
	other := strings.Replace(string(must[Source](t)(nb.TopicSource(tp.Ref())).Data), "Original", "Theirs", 1)
	h.write(t, "sections/tech/bt.md", other)
	tp.Title = "Mine"
	if _, err := nb.UpdateTopic(tp); CodeOf(err) != CodeConflict {
		t.Fatalf("expected conflict, got %v", err)
	}
	data, _ := os.ReadFile(h.path("sections/tech/bt.md"))
	if string(data) != other {
		t.Fatal("their change was overwritten")
	}
	// The other program deletes it.
	if err := os.Remove(h.path("sections/tech/bt.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := nb.UpdateTopic(tp); CodeOf(err) != CodeConflict {
		t.Fatalf("expected conflict on deleted file, got %v", err)
	}
	// The conflict is also detected at the last moment, after the
	// temporary file is written.
	tp = must[Topic](t)(nb.AddTopic(NewTopic{Section: "tech", Slug: "late", Title: "Late"}))
	testHookBeforeRename = func(string) error {
		h.write(t, "sections/tech/late.md", other)
		return nil
	}
	defer func() { testHookBeforeRename = nil }()
	tp.Title = "Mine"
	if _, err := nb.UpdateTopic(tp); CodeOf(err) != CodeConflict {
		t.Fatalf("expected late conflict, got %v", err)
	}
}

func TestDeleteRefusesChangedFile(t *testing.T) {
	h := newHarness(t)
	s := must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	if err := h.nb.fs.removeFile(sectionRel("tech"), versionOf([]byte("stale"))); CodeOf(err) != CodeConflict {
		t.Fatal(err)
	}
	if err := h.nb.fs.removeFile(sectionRel("tech"), s.Version); err != nil {
		t.Fatal(err)
	}
}

func TestOversizedFileIsRejectedWithoutReading(t *testing.T) {
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	f, err := os.Create(h.path("sections/tech/big.md"))
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(1 << 30); err != nil { // sparse 1 GiB file
		t.Skip(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := h.nb.Topic(TopicRef{"tech", "big"}); CodeOf(err) != CodeLimit {
		t.Fatalf("expected limit error, got %v", err)
	}
}

func TestSectionLimit(t *testing.T) {
	h := newHarness(t)
	for i := 0; i < MaxSections; i++ {
		if err := os.MkdirAll(h.path(fmt.Sprintf("sections/s%d", i)), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.nb.Sections(); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	if _, err := h.nb.AddSection(NewSection{Title: "one more"}); CodeOf(err) != CodeLimit {
		t.Fatalf("add over the limit: %v", err)
	}
	if err := os.Mkdir(h.path("sections/extra"), 0o700); err != nil {
		t.Fatal(err)
	}
	_, err := h.nb.Sections()
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeLimit || e.Limit != LimitSections || e.Max != MaxSections {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestTopicLimit(t *testing.T) {
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	for i := 0; i < MaxTopicsPerSection; i++ {
		h.write(t, fmt.Sprintf("sections/tech/t%d.md", i), "")
	}
	if _, err := h.nb.AddTopic(NewTopic{Section: "tech", Title: "one more"}); CodeOf(err) != CodeLimit {
		t.Fatalf("add over the limit: %v", err)
	}
	h.write(t, "sections/tech/extra.md", "")
	_, err := h.nb.Topics("tech")
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeLimit || e.Limit != LimitTopicsPerSection {
		t.Fatalf("over the limit: %v", err)
	}
}

func TestJournalLimit(t *testing.T) {
	if testing.Short() {
		t.Skip("creates 200 001 files")
	}
	h := newHarness(t)
	dir := h.path("journal/2026/11")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < MaxJournalEntries; i++ {
		id := fmt.Sprintf("%010d", i)
		f, err := os.OpenFile(filepath.Join(dir, "2026-11-03-"+id+".md"), os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := f.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := h.nb.AddEntry(NewEntry{Body: "one more"}); CodeOf(err) != CodeLimit {
		t.Fatalf("add over the limit: %v", err)
	}
	h.write(t, "journal/2026/11/2026-11-03-zzzzzzzzzz.md", "")
	_, err := h.nb.Journal(JournalFilter{})
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeLimit || e.Limit != LimitJournalEntries {
		t.Fatalf("over the limit: %v", err)
	}
	if _, err := h.nb.Progress(); CodeOf(err) != CodeLimit {
		t.Fatalf("progress over the limit: %v", err)
	}
}

func TestFileSizeLimitOnDisk(t *testing.T) {
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	h.write(t, "sections/tech/big.md", validTopic+strings.Repeat("x", MaxFileSize))
	if _, err := h.nb.Topic(TopicRef{"tech", "big"}); CodeOf(err) != CodeLimit {
		t.Fatal(err)
	}
	h.write(t, "bearings.yaml", "format: 1\n#"+strings.Repeat("x", MaxFileSize))
	if _, err := Open(h.dir, nil); CodeOf(err) != CodeLimit {
		t.Fatal(err)
	}
}
