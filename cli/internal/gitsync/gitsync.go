// Package gitsync commits a notebook to git and syncs it with its
// upstream, for `bearings sync`. It has no terminal concerns: it returns
// results and typed errors, and the caller renders them.
//
// # How git is run
//
// git is run with exec.Command and an explicit argument list, never
// through a shell, always with `-C <root>` (and the working directory set
// to the root), and never with notebook content as an argument: the
// commit message is fixed text and a timestamp, and the paths to stage
// (validated layout paths) are passed on standard input with
// --pathspec-from-file and --pathspec-file-nul.
//
// # Hardening against a hostile repository
//
// A notebook directory could come with a .git directory someone else
// prepared (copied, unpacked from a tarball). Its configuration and hooks
// are then attacker-controlled, and several git settings run programs.
// Every git invocation therefore gets, on the command line (which takes
// precedence over every configuration file):
//
//   - core.fsmonitor=false: an fsmonitor setting is a command that git
//     runs on any command that reads the index (status, add, commit).
//   - core.hooksPath=/dev/null: hooks are looked up in a directory that
//     cannot contain any, so no hook runs (pre-commit, commit-msg,
//     post-commit, pre-push, post-checkout, post-rewrite, reference-
//     transaction...), whether they sit in .git/hooks or in a directory
//     the repository's config names. An empty value also disables hooks
//     on recent git, but its meaning has varied between versions;
//     /dev/null works on every version and platform (on Windows it is a
//     path that does not exist, with the same effect). Trade-off: hooks
//     the user set up on purpose (a secret scanner, a formatter) do not
//     run during `bearings sync`; they still run with plain git.
//   - protocol.ext.allow=never: the ext:: transport runs an arbitrary
//     command named in the remote URL.
//   - protocol.file.allow=user: local paths stay usable as remotes when
//     the user asks for them (the default for direct use), but not for
//     fetches git starts on its own, such as submodules.
//   - submodule.recurse=false (plus --no-recurse-submodules on pull and
//     --recurse-submodules=no on push): submodules are never touched.
//   - push.default=upstream: push the current branch to its upstream and
//     nothing else.
//
// and the environment has GIT_EDITOR and GIT_SEQUENCE_EDITOR set to ":"
// (git's no-op editor), GIT_PAGER=cat, and, without a terminal,
// GIT_TERMINAL_PROMPT=0 so git never waits for input. Variables that
// would point git at another repository or change how paths are matched
// (GIT_DIR, GIT_WORK_TREE, GIT_INDEX_FILE, GIT_*_PATHSPECS...) are removed.
//
// Some dangerous settings cannot be neutralized without breaking what
// the user configured globally: core.sshCommand, credential helpers,
// core.askPass, URL rewriting, proxies, TLS verification, gpg.program and
// filter, diff and merge drivers. Overriding them on the command line
// would also override the user's own global values (an SSH key, a
// password manager). Instead, [Runner.CheckRepo] reads the names (never
// the values) of the repository-level configuration (scopes "local" and
// "worktree", with includes followed) and refuses to sync when any of
// them is set there, naming the keys; the same keys in the user's global
// or system configuration are trusted and keep working. Credentials are
// never passed by this package; git's own credential handling is used.
package gitsync

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// MinVersion is the oldest git that has every option used here
// (--show-scope, --pathspec-from-file).
var MinVersion = [2]int{2, 26}

// Kind classifies an [Error].
type Kind string

// Error kinds.
const (
	KindOldGit        Kind = "old_git"        // git is older than MinVersion
	KindNotRepo       Kind = "not_repo"       // the notebook is not in a git work tree
	KindNotTopLevel   Kind = "not_top_level"  // the notebook is inside a bigger work tree
	KindUnsafeConfig  Kind = "unsafe_config"  // the repository config sets refused keys
	KindFailed        Kind = "failed"         // a git command failed
	KindRebaseStopped Kind = "rebase_stopped" // pull --rebase stopped (conflict); git state left as is
	KindInProgress    Kind = "in_progress"    // a rebase, merge, cherry-pick or revert is already in progress
)

// Error is returned by the Runner methods. Stderr is git's own message
// (untrusted text: sanitize before printing), truncated.
type Error struct {
	Kind     Kind
	Op       string   // the git subcommand, for KindFailed and KindRebaseStopped
	Stderr   string   // git's message, if any
	TopLevel string   // for KindNotTopLevel
	Keys     []string // for KindUnsafeConfig: configuration key names
	Version  string   // for KindOldGit
	Err      error    // underlying error (starting git), if any
}

func (e *Error) Error() string {
	s := "git: " + string(e.Kind)
	if e.Op != "" {
		s += " (" + e.Op + ")"
	}
	if len(e.Keys) > 0 {
		s += ": " + strings.Join(e.Keys, ", ")
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// KindOf returns the Kind of err, or "" if it is not an *Error.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return ""
}

// hardening is passed to every git invocation; see the package comment.
var hardening = []string{
	"-c", "core.fsmonitor=false",
	"-c", "core.hooksPath=/dev/null",
	"-c", "protocol.ext.allow=never",
	"-c", "protocol.file.allow=user",
	"-c", "submodule.recurse=false",
	"-c", "push.default=upstream",
	"-c", "color.ui=false",
}

// removedEnv lists environment variables never passed to git.
var removedEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
	"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_COMMON_DIR", "GIT_NAMESPACE",
	"GIT_PREFIX", "GIT_EDITOR", "GIT_SEQUENCE_EDITOR", "GIT_PAGER",
	"GIT_EXTERNAL_DIFF", "GIT_LITERAL_PATHSPECS", "GIT_GLOB_PATHSPECS",
	"GIT_NOGLOB_PATHSPECS", "GIT_ICASE_PATHSPECS", "GIT_TERMINAL_PROMPT",
	"GIT_OPTIONAL_LOCKS",
}

// Output limits: git's standard output (a status of every journal entry
// fits easily) and the part of its error output kept for messages.
const (
	maxStdout = 64 << 20
	maxStderr = 8 << 10
)

// Runner runs git for one notebook.
type Runner struct {
	git  string
	root string
	env  []string
}

// New returns a Runner for the notebook at root (absolute, symlink-free,
// as [notebook.Notebook.Root] returns it), using the git executable at
// gitPath, the environment environ (os.Environ() form) as a base, and
// interactive to say whether a person can answer prompts.
func New(gitPath, root string, environ []string, interactive bool) *Runner {
	env := make([]string, 0, len(environ)+4)
	for _, kv := range environ {
		k, _, _ := strings.Cut(kv, "=")
		if !isRemoved(k) {
			env = append(env, kv)
		}
	}
	env = append(env, "GIT_EDITOR=:", "GIT_SEQUENCE_EDITOR=:", "GIT_PAGER=cat")
	if !interactive {
		env = append(env, "GIT_TERMINAL_PROMPT=0")
	}
	return &Runner{git: gitPath, root: root, env: env}
}

func isRemoved(k string) bool {
	for _, r := range removedEnv {
		if strings.EqualFold(k, r) {
			return true
		}
	}
	return false
}

// limitedBuffer keeps the first max bytes written to it.
type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.max - b.buf.Len(); room < len(p) {
		b.truncated = true
		if room > 0 {
			b.buf.Write(p[:room])
		}
		return len(p), nil
	}
	return b.buf.Write(p)
}

// run runs git with args (after the global options) and returns its
// standard output. A non-zero exit is a KindFailed *Error with git's
// error output.
func (r *Runner) run(stdin []byte, args ...string) ([]byte, error) {
	argv := make([]string, 0, 5+len(hardening)+len(args))
	argv = append(argv, "--no-pager", "--literal-pathspecs", "-C", r.root)
	argv = append(argv, hardening...)
	argv = append(argv, args...)
	// G204: the program is the git found on PATH and every argument is
	// either fixed here or a validated layout path; no shell is involved.
	c := exec.Command(r.git, argv...) //nolint:gosec // G204: see above
	c.Dir = r.root
	c.Env = r.env
	if stdin != nil {
		c.Stdin = bytes.NewReader(stdin)
	}
	out := &limitedBuffer{max: maxStdout}
	errb := &limitedBuffer{max: maxStderr}
	c.Stdout, c.Stderr = out, errb
	err := c.Run()
	op := ""
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			op = a
			break
		}
	}
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			return nil, &Error{Kind: KindFailed, Op: op, Err: err}
		}
		return nil, &Error{Kind: KindFailed, Op: op, Stderr: strings.TrimSpace(errb.buf.String())}
	}
	if out.truncated {
		return nil, &Error{Kind: KindFailed, Op: op, Err: errors.New("output too large")}
	}
	return out.buf.Bytes(), nil
}

// Version returns git's version string ("2.43.0") and refuses a git older
// than MinVersion.
func (r *Runner) Version() (string, error) {
	out, err := r.runPlain("--version")
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(string(out))
	v := strings.TrimPrefix(s, "git version ")
	major, minor, ok := parseVersion(v)
	if !ok || major < MinVersion[0] || (major == MinVersion[0] && minor < MinVersion[1]) {
		return v, &Error{Kind: KindOldGit, Version: v}
	}
	return v, nil
}

// runPlain runs git without -C or configuration, for --version.
func (r *Runner) runPlain(args ...string) ([]byte, error) {
	c := exec.Command(r.git, args...) //nolint:gosec // G204: fixed arguments
	c.Env = r.env
	out := &limitedBuffer{max: 4096}
	c.Stdout = out
	if err := c.Run(); err != nil {
		return nil, &Error{Kind: KindFailed, Op: args[0], Err: err}
	}
	return out.buf.Bytes(), nil
}

// parseVersion reads "2.43.0", "2.39.3 (Apple Git-145)", "2.42.0.windows.2".
func parseVersion(v string) (major, minor int, ok bool) {
	parts := strings.SplitN(v, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	minorStr := parts[1]
	if i := strings.IndexFunc(minorStr, func(r rune) bool { return r < '0' || r > '9' }); i >= 0 {
		minorStr = minorStr[:i]
	}
	major, err1 := strconv.Atoi(parts[0])
	minor, err2 := strconv.Atoi(minorStr)
	return major, minor, err1 == nil && err2 == nil
}

// CheckRepo checks that the notebook root is the top level of a git work
// tree and that the repository's own configuration sets nothing this
// package refuses (see the package comment). It returns the names of
// repository settings that are present but neutralized (fsmonitor,
// hooksPath), for the caller to mention.
func (r *Runner) CheckRepo() (neutralized []string, err error) {
	out, err := r.run(nil, "rev-parse", "--show-toplevel")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && ge.Err == nil {
			return nil, &Error{Kind: KindNotRepo, Stderr: ge.Stderr}
		}
		return nil, err
	}
	top := strings.TrimSpace(string(out))
	if !sameDir(top, r.root) {
		return nil, &Error{Kind: KindNotTopLevel, TopLevel: top}
	}
	out, err = r.run(nil, "config", "--list", "--includes", "--show-scope", "--name-only", "-z")
	if err != nil {
		return nil, err
	}
	var refused []string
	fields := strings.Split(string(out), "\x00")
	for i := 0; i+1 < len(fields); i += 2 {
		scope, key := fields[i], fields[i+1]
		if scope != "local" && scope != "worktree" {
			continue
		}
		switch classifyKey(key) {
		case keyRefused:
			refused = appendOnce(refused, key)
		case keyNeutralized:
			neutralized = appendOnce(neutralized, key)
		}
	}
	if len(refused) > 0 {
		return nil, &Error{Kind: KindUnsafeConfig, Keys: refused}
	}
	if r.busy() {
		return nil, &Error{Kind: KindInProgress}
	}
	return neutralized, nil
}

// busy reports whether git is in the middle of an operation that the
// person has to finish (a stopped rebase, a merge with conflicts...).
// Syncing then would commit half-resolved files.
func (r *Runner) busy() bool {
	for _, ref := range []string{"REBASE_HEAD", "MERGE_HEAD", "CHERRY_PICK_HEAD", "REVERT_HEAD"} {
		if r.inProgress(ref) {
			return true
		}
	}
	out, err := r.run(nil, "rev-parse", "--git-path", "rebase-merge", "--git-path", "rebase-apply")
	if err != nil {
		return true // fail closed
	}
	for _, p := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if p == "" {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(r.root, p)
		}
		if _, err := os.Lstat(p); err == nil {
			return true
		}
	}
	return false
}

func appendOnce(list []string, s string) []string {
	for _, x := range list {
		if x == s {
			return list
		}
	}
	return append(list, s)
}

func sameDir(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}

type keyClass int

const (
	keyOK keyClass = iota
	keyNeutralized
	keyRefused
)

// classifyKey says what to do about a key set in the repository's own
// configuration. Keys are "section.name" or "section.subsection.name"; git
// lowercases the section and the name, and the subsection may contain dots.
func classifyKey(key string) keyClass {
	key = strings.ToLower(key)
	i, j := strings.IndexByte(key, '.'), strings.LastIndexByte(key, '.')
	if i < 0 {
		return keyOK
	}
	section, name := key[:i], key[j+1:]
	hasSub := i != j
	switch section {
	case "core":
		switch name {
		case "fsmonitor", "hookspath":
			return keyNeutralized
		case "sshcommand", "askpass", "gitproxy", "alternaterefscommand", "worktree":
			return keyRefused
		}
	case "credential":
		if name == "helper" {
			return keyRefused
		}
	case "filter":
		if name == "clean" || name == "smudge" || name == "process" {
			return keyRefused
		}
	case "diff":
		if name == "textconv" || name == "command" || (!hasSub && name == "external") {
			return keyRefused
		}
	case "merge":
		if name == "driver" {
			return keyRefused
		}
	case "gpg":
		if name == "program" || name == "defaultkeycommand" {
			return keyRefused
		}
	case "remote":
		if name == "uploadpack" || name == "receivepack" || name == "proxy" {
			return keyRefused
		}
	case "protocol":
		if name == "allow" {
			return keyRefused
		}
	case "url":
		if name == "insteadof" || name == "pushinsteadof" {
			return keyRefused
		}
	case "http":
		switch name {
		case "proxy", "sslverify", "sslcainfo", "sslcapath", "sslcert", "sslkey",
			"cookiefile", "extraheader", "curloptresolve":
			return keyRefused
		}
	}
	return keyOK
}

// Change is a changed layout path.
type Change struct {
	Path string
	Kind ChangeKind
}

// ChangeKind says how a path changed.
type ChangeKind string

// Change kinds.
const (
	Added    ChangeKind = "added"
	Modified ChangeKind = "modified"
	Deleted  ChangeKind = "deleted"
)

// Changes lists the changed paths of the notebook layout, in git's order
// (sorted). keep says which paths are part of the layout; everything else
// (a stray file in a section directory, an invalid journal file name) is
// left alone, untracked or as it is. It does not write to the repository
// (no index refresh).
func (r *Runner) Changes(keep func(path string) bool) ([]Change, error) {
	out, err := r.run(nil, "--no-optional-locks", "status", "--porcelain=v1", "-z", "--untracked-files=all",
		"--no-renames", "--ignore-submodules=all", "--", "bearings.yaml", "sections", "journal")
	if err != nil {
		return nil, err
	}
	var changes []Change
	for _, rec := range strings.Split(string(out), "\x00") {
		if len(rec) < 4 || rec[2] != ' ' {
			continue
		}
		x, y, path := rec[0], rec[1], rec[3:]
		if x == 'U' || y == 'U' || (x == 'A' && y == 'A') || (x == 'D' && y == 'D') {
			return nil, &Error{Kind: KindInProgress} // unmerged: never commit conflict markers
		}
		if !keep(path) {
			continue
		}
		kind := Modified
		switch {
		case x == '?' || x == 'A':
			kind = Added
		case x == 'D' || y == 'D':
			kind = Deleted
		}
		changes = append(changes, Change{Path: path, Kind: kind})
	}
	return changes, nil
}

func pathspecInput(changes []Change) []byte {
	var b bytes.Buffer
	for _, c := range changes {
		b.WriteString(c.Path)
		b.WriteByte(0)
	}
	return b.Bytes()
}

// Commit stages exactly the given paths and commits only them, with the
// fixed message msg. Other staged changes stay staged and uncommitted. It
// returns the abbreviated commit hash.
func (r *Runner) Commit(changes []Change, msg string) (string, error) {
	if len(changes) == 0 {
		return "", nil
	}
	in := pathspecInput(changes)
	if _, err := r.run(in, "add", "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return "", err
	}
	if _, err := r.run(in, "commit", "--quiet", "--no-verify", "-m", msg, "--pathspec-from-file=-", "--pathspec-file-nul"); err != nil {
		return "", err
	}
	out, err := r.run(nil, "rev-parse", "--short", "HEAD")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Upstream returns the upstream of the current branch ("origin/main"), or
// false when there is none (no upstream, detached HEAD, no commit yet).
func (r *Runner) Upstream() (string, bool, error) {
	out, err := r.run(nil, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{upstream}")
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && ge.Err == nil {
			return "", false, nil
		}
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

// Pull rebases the local commits on the upstream. When git stops (a
// conflict), the repository is left exactly as git left it and the error
// is KindRebaseStopped: nothing is resolved, aborted or reset here.
func (r *Runner) Pull() error {
	_, err := r.run(nil, "pull", "--rebase", "--no-autostash", "--no-recurse-submodules", "--quiet")
	if err == nil {
		return nil
	}
	if r.inProgress("REBASE_HEAD") || r.inProgress("MERGE_HEAD") {
		var ge *Error
		stderr := ""
		if errors.As(err, &ge) {
			stderr = ge.Stderr
		}
		return &Error{Kind: KindRebaseStopped, Op: "pull", Stderr: stderr}
	}
	return err
}

func (r *Runner) inProgress(ref string) bool {
	_, err := r.run(nil, "rev-parse", "--quiet", "--verify", ref)
	return err == nil
}

// Push pushes the current branch to its upstream. It never forces: a
// rejected push is a KindFailed error.
func (r *Runner) Push() error {
	_, err := r.run(nil, "push", "--quiet", "--no-verify", "--recurse-submodules=no")
	return err
}

// CommitMessage is the fixed commit message, with a UTC timestamp.
func CommitMessage(ts string) string { return fmt.Sprintf("bearings sync %s", ts) }
