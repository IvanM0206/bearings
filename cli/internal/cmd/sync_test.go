package cmd

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// gitTest is a sandbox for running real git: its own HOME and global
// configuration, no system configuration, nothing inherited from the
// environment of the test run but PATH.
type gitTest struct {
	t       *testing.T
	environ []string
}

func newGitTest(t *testing.T) *gitTest {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	home := t.TempDir()
	global := filepath.Join(home, ".gitconfig")
	must(t, os.WriteFile(global, []byte("[user]\n\tname = Test User\n\temail = test@example.com\n[init]\n\tdefaultBranch = main\n[advice]\n\tdetachedHead = false\n"), 0o600))
	return &gitTest{t: t, environ: []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GIT_CONFIG_GLOBAL=" + global,
		"GIT_CONFIG_NOSYSTEM=1",
		"LC_ALL=C",
	}}
}

// git runs git directly (for setting up and checking), failing the test on error.
func (g *gitTest) git(dir string, args ...string) string {
	g.t.Helper()
	out, err := g.try(dir, args...)
	if err != nil {
		g.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(out)
}

func (g *gitTest) try(dir string, args ...string) (string, error) {
	c := exec.Command("git", append([]string{"-C", dir}, args...)...) //nolint:gosec // G204: test setup with fixed arguments
	c.Env = g.environ
	var out bytes.Buffer
	c.Stdout, c.Stderr = &out, &out
	err := c.Run()
	return out.String(), err
}

// sync runs `bearings sync` against the notebook in dir.
func (g *gitTest) sync(dir string, args ...string) result {
	g.t.Helper()
	var out, errb bytes.Buffer
	env := map[string]string{"BEARINGS_NOTEBOOK": dir}
	code := Execute(append([]string{"sync"}, args...), Env{
		Stdout:      &out,
		Stderr:      &errb,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: func() (string, error) { return "", os.ErrNotExist },
		Now:         func() time.Time { return fixtureNow },
		Environ:     func() []string { return g.environ },
		LookPath:    exec.LookPath,
	})
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// repoFixture returns a fixture notebook that is a git repository.
func (g *gitTest) repoFixture() string {
	dir := newFixture(g.t)
	g.git(dir, "init", "-q")
	return dir
}

// withRemote makes dir push to a new bare repository, after a first sync,
// and returns the remote's path.
func (g *gitTest) withRemote(dir string) string {
	g.t.Helper()
	remote := filepath.Join(g.t.TempDir(), "remote.git")
	g.git(g.t.TempDir(), "init", "-q", "--bare", remote)
	wantOK(g.t, g.sync(dir))
	g.git(dir, "remote", "add", "origin", remote)
	g.git(dir, "push", "-q", "-u", "origin", "HEAD")
	return remote
}

func TestSyncNotARepo(t *testing.T) {
	g := newGitTest(t)
	dir := newFixture(t)
	r := g.sync(dir)
	wantFail(t, r, ExitUsage, "is not a git repository")
	for _, s := range []string{"git -C " + dir + " init", "remote add origin", "PRIVATE"} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr lacks %q:\n%s", s, r.stderr)
		}
	}
}

func TestSyncNotTopLevel(t *testing.T) {
	g := newGitTest(t)
	parent := t.TempDir()
	g.git(parent, "init", "-q")
	dir := filepath.Join(parent, "nb")
	_, err := notebook.Init(dir, notebook.Config{Format: 1}, nil)
	must(t, err)
	wantFail(t, g.sync(dir), ExitUsage, "is inside the git repository")
	if out := g.git(parent, "log", "--oneline", "--all"); out != "" {
		t.Fatalf("committed into the outer repository: %s", out)
	}
}

func TestSyncCommitOnlyStagesLayoutPaths(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	must(t, os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("stray"), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "sections", "technical", "stray.txt"), []byte("stray"), 0o600))
	must(t, os.WriteFile(filepath.Join(dir, "sections", "technical", ".backtesting.md.swp"), []byte("swap"), 0o600))
	must(t, os.MkdirAll(filepath.Join(dir, "journal", "2026", "12"), 0o700))
	must(t, os.WriteFile(filepath.Join(dir, "journal", "2026", "12", "2026-11-01-0123456789.md"), []byte("misplaced"), 0o600))

	r := g.sync(dir)
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Committed 11 changed files (") {
		t.Errorf("%q", r.stdout)
	}
	if !strings.Contains(r.stdout, "no upstream") || !strings.Contains(r.stdout, "push -u origin HEAD") {
		t.Errorf("%q", r.stdout)
	}
	tracked := strings.Split(g.git(dir, "ls-files"), "\n")
	if len(tracked) != 11 {
		t.Fatalf("tracked %v", tracked)
	}
	for _, p := range tracked {
		if !isLayoutPath(p) {
			t.Errorf("non-layout path committed: %s", p)
		}
	}
	untracked := g.git(dir, "status", "--porcelain", "--untracked-files=all")
	for _, s := range []string{"?? notes.txt", "?? sections/technical/stray.txt", "?? sections/technical/.backtesting.md.swp", "?? journal/2026/12/2026-11-01-0123456789.md"} {
		if !strings.Contains(untracked, s) {
			t.Errorf("%q should stay untracked:\n%s", s, untracked)
		}
	}
	if msg := g.git(dir, "log", "-1", "--format=%s"); msg != "bearings sync 2026-11-03T12:00:00Z" {
		t.Errorf("message %q", msg)
	}

	// Nothing more to do.
	r = g.sync(dir)
	wantOK(t, r)
	if !strings.HasPrefix(r.stdout, "No changes to commit.\n") {
		t.Errorf("%q", r.stdout)
	}
	if n := g.git(dir, "rev-list", "--count", "HEAD"); n != "1" {
		t.Errorf("%s commits", n)
	}

	// Something the user staged themselves is not committed by sync.
	g.git(dir, "add", "notes.txt")
	wantOK(t, runIn(t, dir, "status", "team/rituals", "mastered"))
	wantOK(t, g.sync(dir))
	if files := g.git(dir, "show", "--name-only", "--format=", "HEAD"); files != "sections/team/rituals.md" {
		t.Errorf("committed %q", files)
	}
	if st := g.git(dir, "status", "--porcelain", "notes.txt"); st != "A  notes.txt" {
		t.Errorf("user's staged file: %q", st)
	}
}

func TestSyncDeletionsAndRenames(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	wantOK(t, g.sync(dir))
	wantOK(t, runIn(t, dir, "topic", "rm", "technical/scratch", "--yes"))
	wantOK(t, runIn(t, dir, "topic", "mv", "team/rituals", "--to-section", "technical"))
	r := g.sync(dir, "--dry-run")
	wantOK(t, r)
	for _, s := range []string{"deleted   sections/technical/scratch.md", "added     sections/technical/rituals.md", "deleted   sections/team/rituals.md"} {
		if !strings.Contains(r.stdout, s) {
			t.Errorf("dry run lacks %q:\n%s", s, r.stdout)
		}
	}
	wantOK(t, g.sync(dir))
	if st := g.git(dir, "status", "--porcelain"); st != "" {
		t.Errorf("left uncommitted: %q", st)
	}
}

func TestSyncDryRun(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	r := g.sync(dir, "--dry-run")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Changed notebook files that would be committed (11):") ||
		!strings.Contains(r.stdout, "added     bearings.yaml") ||
		!strings.Contains(r.stdout, "it would only commit") {
		t.Errorf("%q", r.stdout)
	}
	if _, err := g.try(dir, "rev-parse", "--verify", "HEAD"); err == nil {
		t.Fatal("dry run committed")
	}
	if out := g.git(dir, "ls-files"); out != "" {
		t.Fatalf("dry run staged: %s", out)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git", "index")); err == nil {
		t.Fatal("dry run wrote the index")
	}

	r = g.sync(dir, "--dry-run", "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if got := strings.Join(keys(j), ","); got != "changes,commit,dry_run,pulled,pushed,upstream" {
		t.Fatalf("keys %s", got)
	}
	if ch := j["changes"].([]any); len(ch) != 11 || ch[0].(map[string]any)["change"] != "added" {
		t.Fatalf("%v", ch)
	}

	g.withRemote(dir)
	wantOK(t, runIn(t, dir, "status", "team/rituals", "mastered"))
	r = g.sync(dir, "-n")
	wantOK(t, r)
	if !strings.Contains(r.stdout, "modified  sections/team/rituals.md") || !strings.Contains(r.stdout, "from origin/main and push") {
		t.Errorf("%q", r.stdout)
	}
	if st := g.git(dir, "status", "--porcelain"); st != "M sections/team/rituals.md" {
		t.Errorf("dry run changed the index: %q", st)
	}
}

func TestSyncPushesToBareRemote(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	remote := g.withRemote(dir)
	wantOK(t, runIn(t, dir, "log", "Pushed from the CLI."))
	r := g.sync(dir, "--json")
	wantOK(t, r)
	j := decode(t, r.stdout)
	if j["pushed"] != true || j["pulled"] != true || j["upstream"] != "origin/main" || j["commit"] == "" {
		t.Fatalf("%v", j)
	}
	if local, rem := g.git(dir, "rev-parse", "HEAD"), g.git(remote, "rev-parse", "main"); local != rem {
		t.Fatalf("remote %s, local %s", rem, local)
	}
	if n := g.git(remote, "rev-list", "--count", "main"); n != "2" {
		t.Fatalf("%s commits on the remote", n)
	}
	r = g.sync(dir)
	wantOK(t, r)
	if r.stdout != "No changes to commit.\nSynced with origin/main.\n" {
		t.Errorf("%q", r.stdout)
	}
}

// otherClone clones remote, changes it with fn and pushes the change.
func (g *gitTest) otherClone(remote string, fn func(nb *notebook.Notebook)) {
	g.t.Helper()
	clone := filepath.Join(g.t.TempDir(), "clone")
	g.git(g.t.TempDir(), "clone", "-q", remote, clone)
	nb, err := notebook.Open(clone, &notebook.Options{Now: func() time.Time { return fixtureNow.Add(time.Hour) }})
	must(g.t, err)
	fn(nb)
	g.git(clone, "add", "-A")
	g.git(clone, "commit", "-q", "-m", "change elsewhere")
	g.git(clone, "push", "-q")
}

func TestSyncPullsNonConflictingChanges(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	remote := g.withRemote(dir)
	d, _ := notebook.ParseDate("2026-11-04")
	g.otherClone(remote, func(nb *notebook.Notebook) {
		_, err := nb.AddEntry(notebook.NewEntry{Date: d, Body: "Written on another machine."})
		must(t, err)
	})
	wantOK(t, runIn(t, dir, "status", "team/rituals", "in-progress"))
	r := g.sync(dir)
	wantOK(t, r)
	if !strings.Contains(r.stdout, "Synced with origin/main.") {
		t.Errorf("%q", r.stdout)
	}
	if out := runIn(t, dir, "journal").stdout; !strings.Contains(out, "Written on another machine.") {
		t.Errorf("remote change not pulled:\n%s", out)
	}
	if local, rem := g.git(dir, "rev-parse", "HEAD"), g.git(remote, "rev-parse", "main"); local != rem {
		t.Fatal("not pushed after the rebase")
	}
	if n := g.git(remote, "rev-list", "--count", "main"); n != "3" {
		t.Fatalf("%s commits", n)
	}
}

func TestSyncConflictStopsCleanly(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	remote := g.withRemote(dir)
	g.otherClone(remote, func(nb *notebook.Notebook) {
		tp, err := nb.Topic(notebook.TopicRef{Section: "team", Topic: "rituals"})
		must(t, err)
		tp.Status = notebook.StatusMastered
		_, err = nb.UpdateTopic(tp)
		must(t, err)
	})
	remoteHead := g.git(remote, "rev-parse", "main")
	wantOK(t, runIn(t, dir, "status", "team/rituals", "in-progress"))

	r := g.sync(dir)
	if r.code != ExitFailure || !strings.Contains(r.stderr, "stopped: your changes conflict") {
		t.Fatalf("exit %d: %s", r.code, r.stderr)
	}
	if !strings.HasPrefix(r.stdout, "Committed 1 changed files (") {
		t.Errorf("%q", r.stdout)
	}
	for _, s := range []string{
		"git -C " + dir + " status",
		"git -C " + dir + " rebase --continue",
		"git -C " + dir + " rebase --abort",
		"Nothing was pushed",
		"git said:",
	} {
		if !strings.Contains(r.stderr, s) {
			t.Errorf("stderr lacks %q:\n%s", s, r.stderr)
		}
	}
	// git's state is left as git left it: mid-rebase, nothing pushed.
	if _, err := g.try(dir, "rev-parse", "--verify", "REBASE_HEAD"); err != nil {
		t.Fatal("the rebase was aborted or never started")
	}
	if got := g.git(remote, "rev-parse", "main"); got != remoteHead {
		t.Fatal("something was pushed")
	}
	// A second sync refuses to touch the half-done rebase: it must not
	// commit the file with its conflict markers.
	before := g.git(dir, "rev-parse", "HEAD")
	r = g.sync(dir)
	wantFail(t, r, ExitFailure, "in the middle of a rebase or merge")
	if !strings.Contains(r.stderr, "git -C "+dir+" rebase --abort") {
		t.Errorf("%q", r.stderr)
	}
	if g.git(dir, "rev-parse", "HEAD") != before {
		t.Fatal("committed during a rebase")
	}
	if st := g.git(dir, "status", "--porcelain"); !strings.HasPrefix(st, "UU ") {
		t.Fatalf("the conflict state changed: %q", st)
	}
	if got := g.git(remote, "rev-parse", "main"); got != remoteHead {
		t.Fatal("something was pushed")
	}
	// The user can abort with the printed command.
	g.git(dir, "rebase", "--abort")
	if st := runIn(t, dir, "show", "team/rituals").stdout; !strings.Contains(st, "in progress") {
		t.Errorf("local change lost after abort:\n%s", st)
	}
}

func writeScript(t *testing.T, path, marker string) {
	t.Helper()
	must(t, os.MkdirAll(filepath.Dir(path), 0o700))
	must(t, os.WriteFile(path, []byte("#!/bin/sh\necho ran >> '"+marker+"'\n"), 0o700))
}

func TestSyncDoesNotRunHostileHooksOrFsmonitor(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell scripts")
	}
	g := newGitTest(t)
	dir := g.repoFixture()
	remote := g.withRemote(dir)
	marker := filepath.Join(t.TempDir(), "marker")
	evil := t.TempDir()
	writeScript(t, filepath.Join(evil, "fsmonitor"), marker)
	for _, hook := range []string{"pre-commit", "commit-msg", "prepare-commit-msg", "post-commit", "pre-push", "post-rewrite",
		"post-checkout", "post-merge", "pre-rebase", "reference-transaction", "pre-auto-gc"} {
		writeScript(t, filepath.Join(dir, ".git", "hooks", hook), marker)
		writeScript(t, filepath.Join(evil, "hooks", hook), marker)
	}
	// Check the scripts really run when git is not hardened.
	g.git(dir, "config", "core.fsmonitor", filepath.Join(evil, "fsmonitor"))
	if _, err := g.try(dir, "status"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatal("the fsmonitor script does not run under plain git; the test proves nothing")
	}
	must(t, os.Remove(marker))
	g.git(dir, "config", "core.hooksPath", filepath.Join(evil, "hooks"))

	wantOK(t, runIn(t, dir, "log", "Hooks must not run."))
	r := g.sync(dir)
	wantOK(t, r)
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		data, _ := os.ReadFile(marker)
		t.Fatalf("a hostile hook or fsmonitor ran (%d times)", bytes.Count(data, []byte("ran")))
	}
	for _, k := range []string{"core.fsmonitor", "core.hookspath"} {
		if !strings.Contains(r.stderr, "the repository sets "+k) {
			t.Errorf("no warning about %s: %q", k, r.stderr)
		}
	}
	if local, rem := g.git(dir, "rev-parse", "HEAD"), g.git(remote, "rev-parse", "main"); local != rem {
		t.Fatal("not pushed")
	}
}

func TestSyncRefusesUnsafeRepositoryConfig(t *testing.T) {
	g := newGitTest(t)
	marker := filepath.Join(t.TempDir(), "marker")
	cases := map[string][][2]string{
		"credential helper":  {{"credential.helper", "!touch " + marker}},
		"url credential":     {{"credential.https://example.com.helper", "!touch " + marker}},
		"ssh command":        {{"core.sshCommand", "touch " + marker}},
		"askpass":            {{"core.askPass", "/bin/false"}},
		"insteadOf":          {{"url.https://attacker.example/.insteadOf", "https://example.com/"}},
		"clean filter":       {{"filter.x.clean", "touch " + marker}},
		"textconv":           {{"diff.x.textconv", "touch " + marker}},
		"upload pack":        {{"remote.origin.uploadpack", "touch " + marker}},
		"ext protocol":       {{"protocol.ext.allow", "always"}},
		"tls verification":   {{"http.sslVerify", "false"}},
		"gpg program":        {{"gpg.program", "touch " + marker}},
		"ssh signing helper": {{"gpg.ssh.defaultKeyCommand", "touch " + marker}},
		"worktree":           {{"core.worktree", "/tmp"}},
	}
	for name, kvs := range cases {
		t.Run(name, func(t *testing.T) {
			dir := g.repoFixture()
			for _, kv := range kvs {
				g.git(dir, "config", kv[0], kv[1])
			}
			r := g.sync(dir)
			if name == "worktree" {
				// git itself then reports another top level, or sync refuses the key.
				if r.code != ExitUsage {
					t.Fatalf("exit %d: %s", r.code, r.stderr)
				}
				return
			}
			wantFail(t, r, ExitUsage, "refusing to sync")
			if !strings.Contains(strings.ToLower(r.stderr), strings.ToLower(kvs[0][0])) {
				t.Errorf("the key is not named: %s", r.stderr)
			}
			if strings.Contains(r.stderr, kvs[0][1]) {
				t.Error("the value must not be printed")
			}
			if _, err := g.try(dir, "rev-parse", "--verify", "HEAD"); err == nil {
				t.Error("committed anyway")
			}
		})
	}
	// Through an include of a file inside the work tree.
	dir := g.repoFixture()
	inc := filepath.Join(dir, "included.cfg")
	must(t, os.WriteFile(inc, []byte("[credential]\n\thelper = !touch "+marker+"\n"), 0o600))
	g.git(dir, "config", "include.path", inc)
	wantFail(t, g.sync(dir), ExitUsage, "credential.helper")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a refused program ran")
	}
	// The same keys in the user's global configuration are trusted.
	global := ""
	for _, kv := range g.environ {
		if v, ok := strings.CutPrefix(kv, "GIT_CONFIG_GLOBAL="); ok {
			global = v
		}
	}
	dir = g.repoFixture()
	g.git(dir, "config", "--file", global, "credential.helper", "cache")
	wantOK(t, g.sync(dir))
}

func TestSyncGitMissingAndVerbose(t *testing.T) {
	g := newGitTest(t)
	dir := g.repoFixture()
	var out, errb bytes.Buffer
	code := Execute([]string{"sync"}, Env{
		Stdout: &out, Stderr: &errb,
		Getenv:   func(k string) string { return map[string]string{"BEARINGS_NOTEBOOK": dir}[k] },
		LookPath: func(string) (string, error) { return "", exec.ErrNotFound },
	})
	if code != ExitFailure || !strings.Contains(errb.String(), "git is not installed") {
		t.Fatalf("exit %d: %s", code, errb.String())
	}
	r := g.sync(dir, "--verbose", "--dry-run")
	wantOK(t, r)
	if !strings.Contains(r.stderr, "Using git 2.") {
		t.Errorf("%q", r.stderr)
	}
	wantFail(t, g.sync(filepath.Join(t.TempDir(), "none")), ExitNotebook, "no notebook found")
}

func TestShellQuote(t *testing.T) {
	for in, want := range map[string]string{
		"/home/u/bearings":  "/home/u/bearings",
		"/home/u/my notes":  "'/home/u/my notes'",
		"/tmp/it's":         `'/tmp/it'\''s'`,
		"/tmp/$(rm -rf ~)":  "'/tmp/$(rm -rf ~)'",
		"/tmp/a\x1b[31mb":   "/tmp/ab",
		"~/bearings":        "'~/bearings'",
		`C:\Users\me\notes`: `'C:\Users\me\notes'`,
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}
