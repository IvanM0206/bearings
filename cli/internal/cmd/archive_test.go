package cmd

import (
	"archive/zip"
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/i18n"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func snapshotOf(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	nb, err := notebook.Open(dir, nil)
	must(t, err)
	files, err := nb.Snapshot(0)
	must(t, err)
	return files
}

func TestExportImportRoundTrip(t *testing.T) {
	src := newFixture(t)
	zipPath := filepath.Join(t.TempDir(), "nb.zip")
	r := run(t, src, "export", zipPath)
	wantOK(t, r)
	if r.stdout != "Exported "+zipPath+": 2 sections, 5 topics, 3 journal entries.\n" {
		t.Errorf("%q", r.stdout)
	}
	if fi, err := os.Stat(zipPath); err != nil || (runtime.GOOS != "windows" && fi.Mode().Perm() != 0o600) {
		t.Fatalf("archive mode: %v %v", fi, err)
	}

	dst := filepath.Join(t.TempDir(), "imported")
	r = runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dst}}, "import", zipPath, "--yes")
	wantOK(t, r)
	if !strings.Contains(r.stderr, "The archive holds the notebook Example role: 2 sections, 5 topics, 3 journal entries.") {
		t.Errorf("summary: %q", r.stderr)
	}
	if !strings.Contains(r.stdout, "Imported the notebook into ") {
		t.Errorf("%q", r.stdout)
	}
	if !reflect.DeepEqual(snapshotOf(t, src), snapshotOf(t, dst)) {
		t.Fatal("the imported notebook differs from the exported one")
	}
	for _, args := range [][]string{{"ls", "--json"}, {"journal", "--json"}, {"path", "--json"}} {
		if a, b := run(t, src, args...).stdout, run(t, dst, args...).stdout; a != b {
			t.Errorf("%v differs:\n%s\n%s", args, a, b)
		}
	}
	// Modes of everything written.
	err := filepath.WalkDir(dst, func(p string, d fs.DirEntry, err error) error {
		if err != nil || runtime.GOOS == "windows" {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		want := fs.FileMode(0o600)
		if d.IsDir() {
			want = 0o700
		}
		if fi.Mode().Perm() != want {
			t.Errorf("%s: mode %v, want %v", p, fi.Mode().Perm(), want)
		}
		return nil
	})
	must(t, err)
}

func TestExportIsDeterministicThroughCLI(t *testing.T) {
	src := newFixture(t)
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.zip"), filepath.Join(dir, "b.zip")
	wantOK(t, run(t, src, "export", a))
	// Stray files and a .git directory change nothing.
	must(t, os.MkdirAll(filepath.Join(src, ".git"), 0o700))
	must(t, os.WriteFile(filepath.Join(src, ".git", "config"), []byte("[core]\n"), 0o600))
	must(t, os.WriteFile(filepath.Join(src, "README.md"), []byte("x"), 0o600))
	must(t, os.WriteFile(filepath.Join(src, "sections", "technical", "notes.txt"), []byte("x"), 0o600))
	wantOK(t, run(t, src, "export", b))
	da, _ := os.ReadFile(a)
	db, _ := os.ReadFile(b)
	if !bytes.Equal(da, db) {
		t.Fatal("exports differ")
	}
	zr, err := zip.NewReader(bytes.NewReader(da), int64(len(da)))
	must(t, err)
	for _, f := range zr.File {
		info, err := notebook.ClassifyPath(f.Name)
		if err != nil || info.Kind == notebook.PathUnknown {
			t.Errorf("non-layout path exported: %q", f.Name)
		}
	}
}

func TestExportForceAndRefusals(t *testing.T) {
	src := newFixture(t)
	zipPath := filepath.Join(t.TempDir(), "nb.zip")
	must(t, os.WriteFile(zipPath, []byte("old"), 0o600))
	wantFail(t, run(t, src, "export", zipPath), ExitUsage, "already exists")
	if b, _ := os.ReadFile(zipPath); string(b) != "old" {
		t.Fatal("overwritten without --force")
	}
	wantOK(t, run(t, src, "export", zipPath, "--force"))
	if b, _ := os.ReadFile(zipPath); string(b) == "old" {
		t.Fatal("not replaced with --force")
	}

	wantFail(t, run(t, src, "export", filepath.Join(src, "inside.zip")), ExitUsage, "inside the notebook")
	wantFail(t, run(t, src, "export", filepath.Join(src, "sections", "inside.zip"), "--force"), ExitUsage, "inside the notebook")
	if m, _ := filepath.Glob(filepath.Join(src, "*.zip")); len(m) != 0 {
		t.Fatal(m)
	}
	wantFail(t, run(t, src, "export", t.TempDir(), "--force"), ExitUsage, "directory")
	wantFail(t, run(t, src, "export", filepath.Join(t.TempDir(), "missing", "x.zip")), ExitFailure, "cannot read or write the archive")
	wantFail(t, run(t, filepath.Join(t.TempDir(), "none"), "export", zipPath), ExitNotebook, "no notebook found")
}

func TestExportRefusesInvalidNotebook(t *testing.T) {
	src := newFixture(t)
	bad := filepath.Join(src, "sections", "team", "broken.md")
	must(t, os.WriteFile(bad, []byte("---\ntitle: Secret content here\nbogus: 1\n---\n"), 0o600))
	zipPath := filepath.Join(t.TempDir(), "nb.zip")
	r := run(t, src, "export", zipPath)
	wantFail(t, r, ExitNotebook, "sections/team/broken.md")
	if strings.Contains(r.stderr, "Secret content") {
		t.Fatal("the error leaks file contents")
	}
	if _, err := os.Stat(zipPath); !os.IsNotExist(err) {
		t.Fatal("an archive was written")
	}
	if m, _ := filepath.Glob(filepath.Join(filepath.Dir(zipPath), ".bearings-*")); len(m) != 0 {
		t.Fatal(m)
	}
	// A symlink at a layout path is refused too.
	must(t, os.Remove(bad))
	if err := os.Symlink(filepath.Join(src, "bearings.yaml"), filepath.Join(src, "sections", "team", "link.md")); err != nil {
		t.Skip(err)
	}
	wantFail(t, run(t, src, "export", zipPath), ExitNotebook, "sections/team/link.md")
}

func TestExportRoleOnly(t *testing.T) {
	src := newFixture(t)
	zipPath := filepath.Join(t.TempDir(), "guide.zip")
	r := run(t, src, "export", zipPath, "--role-only", "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if !reflect.DeepEqual(keys(j), []string{"files", "journal_entries", "name", "path", "role_only", "sections", "topics"}) {
		t.Fatalf("keys %v", keys(j))
	}
	if j["role_only"] != true || j["journal_entries"] != 0.0 || j["topics"] != 4.0 || j["sections"] != 2.0 {
		t.Fatalf("%v", j)
	}
	data, _ := os.ReadFile(zipPath)
	if bytes.Contains(data, []byte("journal/")) || bytes.Contains(data, []byte("scratch")) || bytes.Contains(data, []byte("Secret handshake")) {
		t.Fatal("personal data in a role-only archive")
	}
	dst := filepath.Join(t.TempDir(), "next")
	wantOK(t, runIn(t, dst, "import", zipPath, "--yes"))
	if r := run(t, dst, "journal"); r.stdout != "No journal entries.\n" {
		t.Errorf("%q", r.stdout)
	}
	if r := run(t, dst, "ls", "technical"); strings.Contains(r.stdout, "scratch") {
		t.Errorf("%q", r.stdout)
	}
}

func TestImportRefusesExistingNotebook(t *testing.T) {
	src := newFixture(t)
	zipPath := filepath.Join(t.TempDir(), "nb.zip")
	wantOK(t, run(t, src, "export", zipPath))
	target := newFixture(t)
	before := snapshotOf(t, target)
	wantFail(t, runIn(t, target, "import", zipPath, "--yes"), ExitUsage, "not empty")
	wantFail(t, runIn(t, t.TempDir(), "import", zipPath, "--yes", "--into", target), ExitUsage, "not empty")
	if !reflect.DeepEqual(before, snapshotOf(t, target)) {
		t.Fatal("existing notebook changed")
	}
	// A directory with any file at all is not empty.
	other := t.TempDir()
	must(t, os.WriteFile(filepath.Join(other, ".hidden"), nil, 0o600))
	wantFail(t, runIn(t, other, "import", zipPath, "--yes"), ExitUsage, "not empty")
	// An empty directory is fine.
	empty := t.TempDir()
	wantOK(t, runIn(t, empty, "import", zipPath, "--yes"))
}

func TestImportConfirmation(t *testing.T) {
	src := newFixture(t)
	zipPath := filepath.Join(t.TempDir(), "nb.zip")
	wantOK(t, run(t, src, "export", zipPath))
	dst := filepath.Join(t.TempDir(), "nb")
	env := map[string]string{"BEARINGS_NOTEBOOK": dst}

	// No terminal, no --yes.
	wantFail(t, runOpts(t, opts{env: env}, "import", zipPath), ExitUsage, "needs confirmation")
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("created without confirmation")
	}
	// Declined on a terminal.
	r := runOpts(t, opts{env: env, interactive: true, stdin: "n\n"}, "import", zipPath)
	wantFail(t, r, ExitFailure, "cancelled")
	if !strings.Contains(r.stderr, "Create a new notebook from it in "+dst+"? [y/N]") {
		t.Errorf("%q", r.stderr)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("created after declining")
	}
	// Accepted.
	r = runOpts(t, opts{env: env, interactive: true, stdin: "y\n"}, "import", zipPath, "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if !reflect.DeepEqual(keys(j), []string{"journal_entries", "name", "path", "sections", "topics"}) {
		t.Fatalf("keys %v", keys(j))
	}
	if j["name"] != "Example role" || j["journal_entries"] != 3.0 {
		t.Fatalf("%v", j)
	}
}

func writeZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		must(t, err)
		_, err = w.Write([]byte(content))
		must(t, err)
	}
	must(t, zw.Close())
	p := filepath.Join(t.TempDir(), "in.zip")
	must(t, os.WriteFile(p, buf.Bytes(), 0o600))
	return p
}

func TestImportRefusesBadArchives(t *testing.T) {
	cases := []struct {
		name    string
		entries map[string]string
		want    string
	}{
		{"zip slip", map[string]string{"bearings.yaml": "format: 1\n", "../evil.md": "x"}, `unsafe or invalid name (entry "../evil.md")`},
		{"case duplicate", map[string]string{"bearings.yaml": "format: 1\n", "Bearings.yaml": "format: 1\n"}, "same name"},
		{"invalid topic", map[string]string{"bearings.yaml": "format: 1\n", "sections/a/_section.md": "x"}, "file sections/a/_section.md"},
		{"no config", map[string]string{"README.md": "x"}, "not a Bearings notebook"},
		{"control characters", map[string]string{"bearings.yaml": "format: 1\n", "a\x1b[2Jb": "x"}, `entry "a\x1b[2Jb"`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			zipPath := writeZip(t, c.entries)
			parent := t.TempDir()
			dst := filepath.Join(parent, "nb")
			r := runIn(t, dst, "import", zipPath, "--yes")
			wantFail(t, r, ExitNotebook, "the archive was refused and nothing was written")
			if !strings.Contains(r.stderr, c.want) {
				t.Errorf("stderr %q does not contain %q", r.stderr, c.want)
			}
			if strings.ContainsRune(r.stderr, 0x1b) {
				t.Error("raw escape character on the terminal")
			}
			names, _ := os.ReadDir(parent)
			if len(names) != 0 {
				t.Errorf("something was written: %v", names)
			}
		})
	}
	wantFail(t, runIn(t, filepath.Join(t.TempDir(), "nb"), "import", filepath.Join(t.TempDir(), "missing.zip"), "--yes"), ExitFailure, "cannot read or write the archive")
	notZip := filepath.Join(t.TempDir(), "x.zip")
	must(t, os.WriteFile(notZip, []byte("hello"), 0o600))
	wantFail(t, runIn(t, filepath.Join(t.TempDir(), "nb"), "import", notZip, "--yes"), ExitNotebook, "not a valid ZIP archive")
}

// TestEveryArchiveErrorCodeHasAMessage reads the codes declared in the
// archive package and checks each has a message.
func TestEveryArchiveErrorCodeHasAMessage(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "archive", "errors.go"))
	must(t, err)
	f, err := parser.ParseFile(token.NewFileSet(), "errors.go", src, 0)
	must(t, err)
	n := 0
	for _, d := range f.Decls {
		gd, ok := d.(*ast.GenDecl)
		if !ok || gd.Tok != token.CONST {
			continue
		}
		for _, spec := range gd.Specs {
			vs := spec.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "Code" || len(vs.Values) != 1 {
				continue
			}
			lit, ok := vs.Values[0].(*ast.BasicLit)
			if !ok {
				continue
			}
			code, _ := strconv.Unquote(lit.Value)
			n++
			if !i18n.Has("en", "err.code."+code) {
				t.Errorf("no message for archive error code %q", code)
			}
		}
	}
	if n < 15 {
		t.Fatalf("found only %d codes", n)
	}
}
