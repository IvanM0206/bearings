package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func TestRoundTrip(t *testing.T) {
	files := fixtureFiles(t)
	data := exportBytes(t, files, Options{})
	tree, err := readBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tree.Files()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, files) {
		t.Fatal("the files read back differ from the files exported")
	}
	want := fixtureTree(t)
	if len(tree.Sections) != len(want.Sections) || len(tree.Topics) != len(want.Topics) || len(tree.Entries) != len(want.Entries) {
		t.Fatalf("%+v", SummaryOf(tree))
	}
}

func TestExportIsDeterministic(t *testing.T) {
	files := fixtureFiles(t)
	a := exportBytes(t, files, Options{})
	// A copy of the map, so iteration order differs.
	c := map[string][]byte{}
	for k, v := range files {
		c[k] = append([]byte(nil), v...)
	}
	b := exportBytes(t, c, Options{})
	if !bytes.Equal(a, b) {
		t.Fatal("two exports of the same notebook differ")
	}

	zr, err := zip.NewReader(bytes.NewReader(a), int64(len(a)))
	if err != nil {
		t.Fatal(err)
	}
	if zr.Comment != "" {
		t.Error("archive comment")
	}
	var names []string
	methods := map[uint16]int{}
	for _, f := range zr.File {
		names = append(names, f.Name)
		methods[f.Method]++
		if len(f.Extra) != 0 {
			t.Errorf("%s: extra field %x", f.Name, f.Extra)
		}
		if f.Flags != 0 {
			t.Errorf("%s: flags %#x (data descriptor or other)", f.Name, f.Flags)
		}
		if f.Mode() != 0o600 {
			t.Errorf("%s: mode %v", f.Name, f.Mode())
		}
		if f.ModifiedDate != dosEpochDate || f.ModifiedTime != dosEpochTime {
			t.Errorf("%s: modification time", f.Name)
		}
		if f.Comment != "" {
			t.Errorf("%s: comment", f.Name)
		}
	}
	if !sort.StringsAreSorted(names) || len(names) != len(files) {
		t.Errorf("entries not sorted or missing: %v", names)
	}
	if methods[zip.Deflate] == 0 || methods[zip.Store] == 0 {
		// The fixture has a highly repetitive topic, which must be stored
		// to stay under the ratio limit, and ordinary files, deflated.
		t.Errorf("methods %v", methods)
	}
}

func TestRoleOnly(t *testing.T) {
	data := exportBytes(t, fixtureFiles(t), Options{RoleOnly: true})
	tree, err := readBytes(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(tree.Entries) != 0 {
		t.Errorf("journal exported: %d entries", len(tree.Entries))
	}
	for _, tp := range tree.Topics {
		if tp.Type != notebook.TypeRole {
			t.Errorf("personal topic exported: %s", tp.Ref())
		}
	}
	if len(tree.Topics) != 2 || len(tree.Sections) != 2 {
		t.Errorf("%+v", SummaryOf(tree))
	}
	if tree.Config.Name != "Example role" || tree.Config.Language != "en" {
		t.Errorf("config %+v", tree.Config)
	}
	orig := fixtureTree(t)
	for _, tp := range tree.Topics {
		if tp.Status != notebook.StatusPending || !tp.Suggested {
			t.Errorf("%s: status %s suggested %v", tp.Ref(), tp.Status, tp.Suggested)
		}
		for _, o := range orig.Topics {
			if o.Ref() == tp.Ref() && (o.Title != tp.Title || o.Notes != tp.Notes || o.Order != tp.Order ||
				!o.Created.Equal(tp.Created) || !o.Updated.Equal(tp.Updated) || len(o.Resources) != len(tp.Resources)) {
				t.Errorf("%s: fields changed: %+v", tp.Ref(), tp)
			}
		}
	}
	if bytes.Contains(data, []byte("journal/")) || bytes.Contains(data, []byte("private")) {
		t.Error("journal or personal topic names in the archive")
	}
	p, err := Prepare(fixtureFiles(t), Options{RoleOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if s := p.Summary; s.Entries != 0 || s.Topics != 2 || s.Sections != 2 || s.Files != 5 || s.Name != "Example role" {
		t.Errorf("summary %+v", s)
	}
}

func TestPrepareRejectsInvalidFiles(t *testing.T) {
	files := fixtureFiles(t)
	files["sections/tech/build.md"] = []byte("---\ntitle: &a x\n---\n")
	_, err := Prepare(files, Options{})
	var ne *notebook.Error
	if !errors.As(err, &ne) || ne.Path != "sections/tech/build.md" {
		t.Fatalf("got %v", err)
	}
	// Unknown paths never make it into the archive.
	files = fixtureFiles(t)
	files["README.md"] = []byte("x")
	data := exportBytes(t, files, Options{})
	if bytes.Contains(data, []byte("README.md")) {
		t.Fatal("unknown path exported")
	}
	// Limits the reader enforces are enforced when writing too.
	defer func(v int) { maxEntries = v }(maxEntries)
	maxEntries = 3
	if _, err := Prepare(fixtureFiles(t), Options{}); codeOf(err) != CodeTooMany {
		t.Fatalf("got %v", err)
	}
}

func writeArchive(t *testing.T, path string, force bool, root string) error {
	t.Helper()
	p, err := Prepare(fixtureFiles(t), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return CreateFile(path, force, root, func(w io.Writer) error {
		_, err := p.WriteTo(w)
		return err
	})
}

func leftovers(t *testing.T, dir string) []string {
	t.Helper()
	m, _ := filepath.Glob(filepath.Join(dir, ".bearings-export-*"))
	return m
}

func TestCreateFile(t *testing.T) {
	root := t.TempDir() // stands for the notebook
	dir := t.TempDir()
	target := filepath.Join(dir, "out.zip")
	if err := writeArchive(t, target, false, root); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if _, err := ReadFile(target); err != nil {
		t.Fatal(err)
	}

	// No overwrite without force.
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeArchive(t, target, false, root); codeOf(err) != CodeExists {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatal("existing file changed")
	}
	// With force it is replaced.
	if err := writeArchive(t, target, true, root); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(target); string(b) == "old" {
		t.Fatal("not replaced")
	}
	// A directory is never replaced.
	if err := writeArchive(t, dir, true, root); codeOf(err) != CodeNotRegular {
		t.Fatalf("directory: %v", err)
	}
	// The destination directory must exist.
	if err := writeArchive(t, filepath.Join(dir, "missing", "x.zip"), false, root); codeOf(err) != CodeIO {
		t.Fatalf("missing dir: %v", err)
	}
	if l := leftovers(t, dir); len(l) != 0 {
		t.Fatalf("temporary files left: %v", l)
	}
}

func TestCreateFileRefusesNotebookDir(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sections", "tech"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(root, "out.zip"),
		filepath.Join(root, "sections", "tech", "out.zip"),
		filepath.Join(root, "sections", "..", "out.zip"),
	} {
		if err := writeArchive(t, p, true, root); codeOf(err) != CodeInsideRoot {
			t.Fatalf("%s: %v", p, err)
		}
	}
	// Through a symlink pointing into the notebook.
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(filepath.Join(root, "sections"), link); err != nil {
		t.Skipf("symlinks: %v", err)
	}
	if err := writeArchive(t, filepath.Join(link, "out.zip"), true, root); codeOf(err) != CodeInsideRoot {
		t.Fatalf("symlink: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(root, "*.zip")); len(m) != 0 {
		t.Fatal(m)
	}
	// A sibling whose name starts with the notebook's is not inside it.
	sib := root + "-backup"
	if err := os.Mkdir(sib, 0o700); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(sib)
	if err := writeArchive(t, filepath.Join(sib, "out.zip"), false, root); err != nil {
		t.Fatalf("sibling: %v", err)
	}
}

func TestCreateFileFailureLeavesOldFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.zip")
	if err := os.WriteFile(target, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	testHookBeforeRename = func(string) error { return errors.New("disk full") }
	defer func() { testHookBeforeRename = nil }()
	if err := writeArchive(t, target, true, t.TempDir()); codeOf(err) != CodeIO {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatal("old file changed")
	}
	// A failure while producing the archive.
	err := CreateFile(target, true, "", func(w io.Writer) error { return errors.New("boom") })
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
	if b, _ := os.ReadFile(target); string(b) != "old" {
		t.Fatal("old file changed")
	}
	if l := leftovers(t, dir); len(l) != 0 {
		t.Fatalf("temporary files left: %v", l)
	}
}
