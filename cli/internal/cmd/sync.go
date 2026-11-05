package cmd

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/gitsync"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newSync() *cobra.Command {
	var dryRun, verbose bool
	c := &cobra.Command{
		Use:   "sync",
		Short: a.t("cmd.sync.short"),
		Long:  a.t("cmd.sync.long"),
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return a.runSync(dryRun, verbose)
		},
	}
	c.Flags().BoolVarP(&dryRun, "dry-run", "n", false, a.t("flag.dry_run"))
	c.Flags().BoolVarP(&verbose, "verbose", "v", false, a.t("flag.verbose"))
	return c
}

type syncChangeJSON struct {
	Path   string `json:"path"`
	Change string `json:"change"`
}

type syncJSON struct {
	DryRun   bool             `json:"dry_run"`
	Changes  []syncChangeJSON `json:"changes"`
	Commit   string           `json:"commit"`
	Upstream string           `json:"upstream"`
	Pulled   bool             `json:"pulled"`
	Pushed   bool             `json:"pushed"`
}

// isLayoutPath reports whether a path git reports is a notebook file
// (SPEC §2); only those are ever staged.
func isLayoutPath(p string) bool {
	info, err := notebook.ClassifyPath(p)
	return err == nil && info.Kind != notebook.PathUnknown
}

func (a *app) runSync(dryRun, verbose bool) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	root := nb.Root()
	lookPath := a.env.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	gitPath, err := lookPath("git")
	if err != nil {
		return &exitError{code: ExitFailure, msg: a.t("err.git_missing")}
	}
	var environ []string
	if a.env.Environ != nil {
		environ = a.env.Environ()
	}
	g := gitsync.New(gitPath, root, environ, a.env.Interactive)
	version, err := g.Version()
	if err != nil {
		return a.gitErr(err, root)
	}
	if verbose {
		a.note(a.t("sync.git_version", output.Line(version), output.Line(gitPath)))
	}
	neutralized, err := g.CheckRepo()
	if err != nil {
		return a.gitErr(err, root)
	}
	for _, k := range neutralized {
		a.note(a.t("warn.git_neutralized", output.Line(k)))
	}
	changes, err := g.Changes(isLayoutPath)
	if err != nil {
		return a.gitErr(err, root)
	}
	upstream, hasUpstream, err := g.Upstream()
	if err != nil {
		return a.gitErr(err, root)
	}
	res := syncJSON{DryRun: dryRun, Changes: []syncChangeJSON{}, Upstream: upstream}
	for _, c := range changes {
		res.Changes = append(res.Changes, syncChangeJSON{Path: c.Path, Change: string(c.Kind)})
	}
	p := a.out()

	if dryRun {
		if a.jsonOut {
			return a.writeJSON(res)
		}
		if len(changes) == 0 {
			p.println(a.t("sync.no_changes"))
		} else {
			p.println(a.t("sync.dry_header", len(changes)))
			for _, c := range changes {
				p.printf("  %-8s  %s\n", a.t("sync.change."+string(c.Kind)), output.Line(c.Path))
			}
		}
		if hasUpstream {
			p.println(a.t("sync.dry_upstream", output.Line(upstream)))
		} else {
			p.println(a.t("sync.dry_no_upstream"))
		}
		return p.err
	}

	if len(changes) > 0 {
		msg := gitsync.CommitMessage(notebook.FormatTimestamp(a.now()))
		hash, err := g.Commit(changes, msg)
		if err != nil {
			return a.gitErr(err, root)
		}
		res.Commit = hash
		if !a.jsonOut {
			p.println(a.t("sync.committed", len(changes), output.Line(hash)))
		}
	} else if !a.jsonOut {
		p.println(a.t("sync.no_changes"))
	}
	if !hasUpstream {
		if a.jsonOut {
			return a.writeJSON(res)
		}
		p.println(a.t("sync.no_upstream", shellQuote(root)))
		return p.err
	}
	if err := g.Pull(); err != nil {
		return a.gitErr(err, root, upstream)
	}
	res.Pulled = true
	if err := g.Push(); err != nil {
		return a.gitErr(err, root, upstream)
	}
	res.Pushed = true
	if a.jsonOut {
		return a.writeJSON(res)
	}
	p.println(a.t("sync.synced", output.Line(upstream)))
	return p.err
}

// note writes an informational line to stderr.
func (a *app) note(s string) { _, _ = fmt.Fprintln(a.env.Stderr, output.Line(s)) }

// gitErr renders a gitsync error. git's own messages are untrusted text
// (a remote can send any message) and are sanitized.
func (a *app) gitErr(err error, root string, upstream ...string) error {
	var ge *gitsync.Error
	if !errors.As(err, &ge) {
		return a.wrap(err)
	}
	q := shellQuote(root)
	up := ""
	if len(upstream) > 0 {
		up = output.Line(upstream[0])
	}
	gitSaid := ""
	if ge.Stderr != "" {
		gitSaid = "\n" + a.t("sync.git_said") + "\n" + indent(output.Text(lastLines(ge.Stderr, 20)), "  ")
	}
	switch ge.Kind {
	case gitsync.KindOldGit:
		v := fmt.Sprintf("%d.%d", gitsync.MinVersion[0], gitsync.MinVersion[1])
		return &exitError{code: ExitUsage, msg: a.t("err.git_old", output.Line(ge.Version), v)}
	case gitsync.KindNotRepo:
		return &exitError{code: ExitUsage, msg: a.t("err.not_git_repo", output.Line(root), q) + gitSaid}
	case gitsync.KindNotTopLevel:
		return &exitError{code: ExitUsage, msg: a.t("err.not_git_toplevel", output.Line(root), output.Line(ge.TopLevel))}
	case gitsync.KindUnsafeConfig:
		keys := make([]string, len(ge.Keys))
		for i, k := range ge.Keys {
			keys[i] = output.Line(k)
		}
		return &exitError{code: ExitUsage, msg: a.t("err.git_unsafe_config", strings.Join(keys, ", "), q)}
	case gitsync.KindInProgress:
		return &exitError{code: ExitFailure, msg: a.t("err.git_in_progress", q)}
	case gitsync.KindRebaseStopped:
		return &exitError{code: ExitFailure, msg: a.t("err.git_rebase_stopped", up, q) + gitSaid}
	}
	if ge.Op == "push" {
		return &exitError{code: ExitFailure, msg: a.t("err.git_push_failed", up, q) + gitSaid}
	}
	if ge.Op == "pull" {
		return &exitError{code: ExitFailure, msg: a.t("err.git_pull_failed", up, q) + gitSaid}
	}
	detail := gitSaid
	if ge.Err != nil {
		detail = " (" + output.Line(ge.Err.Error()) + ")" + gitSaid
	}
	return &exitError{code: ExitFailure, msg: a.t("err.git_failed", output.Line(ge.Op)) + detail}
}

func lastLines(s string, n int) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}

// shellQuote quotes a path for the git commands shown to the person, so
// they can be pasted into a POSIX shell. Control characters are removed
// first (the path is printed to a terminal).
func shellQuote(s string) string {
	s = output.Line(s)
	safe := s != ""
	for _, r := range s {
		if !isShellSafe(r) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-+:@%,", r)
}
