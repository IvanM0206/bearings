package notebook

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func snapshotFixture(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	must[Section](t)(h.nb.AddSection(NewSection{Slug: "tech", Title: "Tech"}))
	must[Topic](t)(h.nb.AddTopic(NewTopic{Section: "tech", Slug: "build", Title: "Build"}))
	d, _ := ParseDate("2026-11-03")
	must[Entry](t)(h.nb.AddEntry(NewEntry{Date: d, Body: "Hello."}))
	return h
}

func TestSnapshotReadsOnlyLayoutPaths(t *testing.T) {
	h := snapshotFixture(t)
	// Things that are not part of the layout are never read.
	h.write(t, ".git/config", "[core]\n")
	h.write(t, "README.md", "readme")
	h.write(t, "sections/tech/notes.txt", "stray")
	h.write(t, "sections/tech/.build.md.swp", "swap")
	h.write(t, "sections/Bad Slug/_section.md", validSection)
	h.write(t, "sections/file", "not a directory")
	h.write(t, "journal/2026/11/readme.md", "stray")
	h.write(t, "journal/notes/x.md", "stray")
	files := must[map[string][]byte](t)(h.nb.Snapshot(0))
	var got []string
	for p := range files {
		got = append(got, p)
	}
	sort.Strings(got)
	entries := must[[]Entry](t)(h.nb.Journal(JournalFilter{}))
	want := []string{
		"bearings.yaml",
		entryRel(entries[0].Date, entries[0].ID),
		"sections/tech/_section.md",
		"sections/tech/build.md",
	}
	sort.Strings(want)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	raw, err := os.ReadFile(h.path("sections/tech/build.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(files["sections/tech/build.md"]) != string(raw) {
		t.Fatal("snapshot must return the raw bytes")
	}
	if _, err := ParseTree(files); err != nil {
		t.Fatalf("a valid notebook must validate: %v", err)
	}
}

func TestSnapshotFailsClosed(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, h *harness)
		code  Code
		path  string
	}{
		{"symlinked topic", func(t *testing.T, h *harness) {
			symlinkOrSkip(t, filepath.Join(outside(t), "secret.md"), h.path("sections/tech/secret.md"))
		}, CodeSymlink, "sections/tech/secret.md"},
		{"symlinked section dir", func(t *testing.T, h *harness) {
			symlinkOrSkip(t, outside(t), h.path("sections/linked"))
		}, CodeSymlink, "sections/linked"},
		{"symlinked journal month", func(t *testing.T, h *harness) {
			symlinkOrSkip(t, outside(t), h.path("journal/2025/01"))
		}, CodeSymlink, "journal/2025/01"},
		{"symlinked config", func(t *testing.T, h *harness) {
			p := h.path("bearings.yaml")
			ext := filepath.Join(t.TempDir(), "bearings.yaml")
			if err := os.Rename(p, ext); err != nil {
				t.Fatal(err)
			}
			symlinkOrSkip(t, ext, p)
		}, CodeSymlink, "bearings.yaml"},
		{"directory at a topic path", func(t *testing.T, h *harness) {
			if err := os.Mkdir(h.path("sections/tech/dir.md"), 0o700); err != nil {
				t.Fatal(err)
			}
		}, CodeNotRegular, "sections/tech/dir.md"},
		{"journal path mismatch", func(t *testing.T, h *harness) {
			h.write(t, "journal/2026/12/2026-11-03-0123456789.md", "x")
		}, CodePathMismatch, "journal/2026/12/2026-11-03-0123456789.md"},
		{"oversized file", func(t *testing.T, h *harness) {
			h.write(t, "sections/tech/big.md", string(make([]byte, MaxFileSize+1)))
		}, CodeLimit, "sections/tech/big.md"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := snapshotFixture(t)
			c.setup(t, h)
			_, err := h.nb.Snapshot(0)
			var e *Error
			if !errors.As(err, &e) || e.Code != c.code || e.Path != c.path {
				t.Fatalf("got %v, want %s in %s", err, c.code, c.path)
			}
		})
	}
}

func TestSnapshotTotalLimit(t *testing.T) {
	h := snapshotFixture(t)
	files := must[map[string][]byte](t)(h.nb.Snapshot(0))
	var total int64
	for _, d := range files {
		total += int64(len(d))
	}
	if _, err := h.nb.Snapshot(total); err != nil {
		t.Fatalf("exactly at the limit: %v", err)
	}
	_, err := h.nb.Snapshot(total - 1)
	var e *Error
	if !errors.As(err, &e) || e.Code != CodeLimit || e.Limit != LimitTotalSize || e.Max != total-1 {
		t.Fatalf("got %v", err)
	}
}

func TestCheckNewTarget(t *testing.T) {
	base := t.TempDir()
	if err := CheckNewTarget(filepath.Join(base, "missing", "nb")); err != nil {
		t.Fatalf("missing: %v", err)
	}
	empty := filepath.Join(base, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CheckNewTarget(empty); err != nil {
		t.Fatalf("empty: %v", err)
	}
	full := filepath.Join(base, "full")
	if err := os.MkdirAll(filepath.Join(full, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := CheckNewTarget(full); CodeOf(err) != CodeNotEmpty {
		t.Fatalf("non-empty: %v", err)
	}
	file := filepath.Join(base, "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckNewTarget(file); CodeOf(err) != CodeNotEmpty {
		t.Fatalf("file: %v", err)
	}
	link := filepath.Join(base, "link")
	symlinkOrSkip(t, empty, link)
	if err := CheckNewTarget(link); CodeOf(err) != CodeSymlink {
		t.Fatalf("symlink: %v", err)
	}
	if err := CheckNewTarget(""); err == nil {
		t.Fatal("empty path accepted")
	}
	if _, err := os.Stat(filepath.Join(base, "missing")); !os.IsNotExist(err) {
		t.Fatal("CheckNewTarget must not create anything")
	}
}
