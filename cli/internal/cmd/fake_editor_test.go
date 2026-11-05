package cmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeEditor writes a small shell script that stands in for the user's
// editor. It first records facts about how it was started (argument count,
// file name, modes, initial content), then runs body, in which $last is the
// file to edit. The script lives in a directory with a space in its name,
// so that quoting is exercised. It returns the value to put in $EDITOR and
// the record.
type fakeEditorRun struct {
	t      *testing.T
	cmd    string
	logDir string
}

func newFakeEditor(t *testing.T, body string) *fakeEditorRun {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the fake editor is a POSIX shell script")
	}
	dir := filepath.Join(t.TempDir(), "my editors")
	must(t, os.Mkdir(dir, 0o700))
	script := `#!/bin/sh
for last; do :; done
{
  echo "args=$#"
  echo "first=$1"
  echo "file=$last"
  echo "filemode=$(stat -c %a "$last" 2>/dev/null || stat -f %Lp "$last")"
  echo "dirmode=$(stat -c %a "$(dirname "$last")" 2>/dev/null || stat -f %Lp "$(dirname "$last")")"
} > "` + dir + `/record"
cp "$last" "` + dir + `/initial"
` + body + "\n"
	path := filepath.Join(dir, "fake editor.sh")
	must(t, os.WriteFile(path, []byte(script), 0o700))
	return &fakeEditorRun{t: t, cmd: `"` + path + `" --wait`, logDir: dir}
}

// record returns the facts the editor wrote down.
func (f *fakeEditorRun) record() map[string]string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.logDir, "record"))
	if err != nil {
		f.t.Fatalf("the editor did not run: %v", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(b)), "\n") {
		k, v, _ := strings.Cut(line, "=")
		out[k] = v
	}
	return out
}

// initial returns the content the file had when the editor started.
func (f *fakeEditorRun) initial() string {
	f.t.Helper()
	b, err := os.ReadFile(filepath.Join(f.logDir, "initial"))
	must(f.t, err)
	return string(b)
}

// runEditing runs the CLI with the fake editor as $EDITOR and a private
// TMPDIR, which it returns so that tests can check for leftovers.
func runEditing(t *testing.T, dir string, f *fakeEditorRun, args ...string) (result, string) {
	t.Helper()
	tmp := t.TempDir()
	t.Setenv("TMPDIR", tmp)
	r := runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir, "EDITOR": f.cmd}, now: laterNow}, args...)
	return r, tmp
}

// leftovers lists what is left in the temp directory matching prefix.
func leftovers(t *testing.T, tmp, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(tmp)
	must(t, err)
	var out []string
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			out = append(out, filepath.Join(tmp, e.Name()))
		}
	}
	return out
}
