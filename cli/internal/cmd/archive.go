package cmd

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/archive"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// ---- export ----------------------------------------------------------------

func (a *app) newExport() *cobra.Command {
	var force, roleOnly bool
	c := &cobra.Command{
		Use:   "export <file.zip>",
		Short: a.t("cmd.export.short"),
		Long:  a.t("cmd.export.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runExport(args[0], force, roleOnly)
		},
	}
	c.Flags().BoolVar(&force, "force", false, a.t("flag.force"))
	c.Flags().BoolVar(&roleOnly, "role-only", false, a.t("flag.role_only"))
	c.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"zip"}, cobra.ShellCompDirectiveFilterFileExt
	}
	return c
}

type exportJSON struct {
	Path     string `json:"path"`
	RoleOnly bool   `json:"role_only"`
	Name     string `json:"name"`
	Sections int    `json:"sections"`
	Topics   int    `json:"topics"`
	Entries  int    `json:"journal_entries"`
	Files    int    `json:"files"`
}

func (a *app) runExport(file string, force, roleOnly bool) error {
	if file == "" {
		return a.usagef("err.export_no_file")
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	files, err := nb.Snapshot(archive.MaxTotalSize)
	if err != nil {
		return a.exportErr(err)
	}
	plan, err := archive.Prepare(files, archive.Options{RoleOnly: roleOnly})
	if err != nil {
		return a.exportErr(err)
	}
	err = archive.CreateFile(file, force, nb.Root(), func(w io.Writer) error {
		_, err := plan.WriteTo(w)
		return err
	})
	if err != nil {
		return a.exportErr(err)
	}
	s := plan.Summary
	abs, _ := filepath.Abs(file)
	if a.jsonOut {
		return a.writeJSON(exportJSON{Path: abs, RoleOnly: roleOnly, Name: s.Name, Sections: s.Sections,
			Topics: s.Topics, Entries: s.Entries, Files: s.Files})
	}
	p := a.out()
	p.println(a.t("export.done", output.Line(file), s.Sections, s.Topics, s.Entries))
	if roleOnly {
		p.println(a.t("export.role_only"))
	}
	return p.err
}

// exportErr explains why nothing was exported.
func (a *app) exportErr(err error) error {
	var ae *archive.Error
	if errors.As(err, &ae) {
		switch ae.Code {
		case archive.CodeExists:
			return a.usagef("err.export_exists")
		case archive.CodeInsideRoot:
			return a.usagef("err.export_inside")
		case archive.CodeNotRegular:
			return a.usagef("err.export_target_dir")
		}
		return a.wrapArchive(err)
	}
	var ne *notebook.Error
	if errors.As(err, &ne) && ne.Code != notebook.CodeIO {
		return &exitError{code: ExitNotebook, msg: a.t("err.export_invalid", a.describe(ne))}
	}
	return a.wrap(err)
}

// ---- import ----------------------------------------------------------------

func (a *app) newImport() *cobra.Command {
	var into string
	var yes bool
	c := &cobra.Command{
		Use:   "import <file.zip>",
		Short: a.t("cmd.import.short"),
		Long:  a.t("cmd.import.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runImport(args[0], into, yes)
		},
	}
	c.Flags().StringVar(&into, "into", "", a.t("flag.into"))
	c.Flags().BoolVarP(&yes, "yes", "y", false, a.t("flag.yes_import"))
	_ = c.MarkFlagDirname("into")
	c.ValidArgsFunction = func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		return []string{"zip"}, cobra.ShellCompDirectiveFilterFileExt
	}
	return c
}

type importJSON struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Sections int    `json:"sections"`
	Topics   int    `json:"topics"`
	Entries  int    `json:"journal_entries"`
}

func (a *app) runImport(file, into string, yes bool) error {
	dir := into
	if dir == "" {
		var err error
		if dir, err = a.notebookDir(); err != nil {
			return err
		}
	}
	// Refuse an existing notebook before reading the archive or asking.
	if err := a.importTarget(dir, notebook.CheckNewTarget(dir)); err != nil {
		return err
	}
	tree, err := archive.ReadFile(file)
	if err != nil {
		return a.importErr(err)
	}
	s := archive.SummaryOf(tree)
	name := s.Name
	if name == "" {
		name = a.t("common.none")
	}
	_, _ = fmt.Fprintln(a.env.Stderr, output.Line(a.t("import.summary", name, s.Sections, s.Topics, s.Entries)))
	if err := a.confirmWith(yes, a.t("import.prompt", output.Line(dir)), "err.confirm_import"); err != nil {
		return err
	}
	nb, err := notebook.Create(dir, tree, a.nbOptions())
	if err != nil {
		if terr := a.importTarget(dir, err); terr != nil {
			return terr
		}
		return a.wrap(err)
	}
	if a.jsonOut {
		return a.writeJSON(importJSON{Path: nb.Root(), Name: s.Name, Sections: s.Sections, Topics: s.Topics, Entries: s.Entries})
	}
	p := a.out()
	p.println(a.t("import.done", output.Line(nb.Root()), s.Sections, s.Topics, s.Entries))
	return p.err
}

// importTarget turns a refused import destination into a usage error.
func (a *app) importTarget(dir string, err error) error {
	switch notebook.CodeOf(err) {
	case "":
		return nil
	case notebook.CodeNotEmpty:
		return a.usagef("err.import_not_empty", output.Line(dir))
	case notebook.CodeSymlink:
		return a.usagef("err.import_symlink", output.Line(dir))
	}
	return a.wrap(err)
}

// importErr explains why an archive was refused. Nothing has been
// written at that point.
func (a *app) importErr(err error) error {
	var ne *notebook.Error
	if errors.As(err, &ne) {
		return &exitError{code: ExitNotebook, msg: a.t("err.archive_refused", a.describe(ne))}
	}
	return a.wrapArchive(err)
}

// wrapArchive renders an *archive.Error: the reason, the entry name
// (quoted, printable ASCII only) and the limit.
func (a *app) wrapArchive(err error) error {
	var ae *archive.Error
	if !errors.As(err, &ae) {
		return a.wrap(err)
	}
	msg := a.t("err.code." + string(ae.Code))
	var details []string
	if ae.Name != "" {
		details = append(details, a.t("err.detail.entry", ae.QuotedName()))
	}
	if ae.Max > 0 {
		details = append(details, a.t("err.detail.max", ae.Max))
	}
	if ae.Err != nil {
		details = append(details, output.Line(ae.Err.Error()))
	}
	if len(details) > 0 {
		msg += " (" + strings.Join(details, ", ") + ")"
	}
	if ae.Code == archive.CodeIO {
		return &exitError{code: ExitFailure, msg: a.t("err.archive_io", msg)}
	}
	return &exitError{code: ExitNotebook, msg: a.t("err.archive_refused", msg)}
}
