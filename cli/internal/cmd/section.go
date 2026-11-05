package cmd

import (
	"errors"
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// group builds a command that only holds subcommands.
func (a *app) group(use, short, long string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.ArbitraryArgs,
		RunE: func(c *cobra.Command, args []string) error {
			if len(args) > 0 {
				return a.usagef("err.unknown_command", output.Line(args[0]))
			}
			return c.Help()
		},
	}
}

func (a *app) newSection() *cobra.Command {
	c := a.group("section", a.t("cmd.section.short"), a.t("cmd.section.long"))
	c.AddCommand(a.newSectionAdd(), a.newSectionRename(), a.newSectionRm(), a.newSectionMv(), a.newSectionLs())
	return c
}

// partial adds the "may be partly applied" note to a failure of an
// operation that changes several files.
func (a *app) partial(err error) error {
	var ee *exitError
	if errors.As(err, &ee) && ee.code != ExitUsage && ee.code != ExitNotFound {
		return &exitError{code: ee.code, msg: ee.msg + "\n" + a.t("err.partial")}
	}
	return err
}

type sectionResultJSON struct {
	Section sectionJSON `json:"section"`
}

func (a *app) newSectionAdd() *cobra.Command {
	var description string
	c := &cobra.Command{
		Use:   "add <title>",
		Short: a.t("cmd.section_add.short"),
		Long:  a.t("cmd.section_add.long"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSectionAdd(strings.Join(args, " "), description)
		},
	}
	c.Flags().StringVar(&description, "description", "", a.t("flag.description"))
	return c
}

func (a *app) runSectionAdd(title, description string) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	s, err := nb.AddSection(notebook.NewSection{Title: title, Description: description})
	if err != nil {
		return a.wrapInput(err)
	}
	if a.jsonOut {
		return a.writeJSON(sectionResultJSON{Section: sectionOf(s, nil)})
	}
	p := a.out()
	p.println(a.t("section.added", s.Slug, output.Line(s.Title)))
	return p.err
}

func (a *app) newSectionRename() *cobra.Command {
	return &cobra.Command{
		Use:   "rename <section> <new title>",
		Short: a.t("cmd.section_rename.short"),
		Long:  a.t("cmd.section_rename.long"),
		Args:  cobra.MinimumNArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSectionRename(args[0], strings.Join(args[1:], " "))
		},
	}
}

func (a *app) runSectionRename(slug, title string) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	s, err := a.sectionArg(nb, slug)
	if err != nil {
		return err
	}
	s.Title = strings.TrimSpace(title)
	s, err = nb.UpdateSection(s)
	if err != nil {
		return a.wrapInput(err)
	}
	topics, err := nb.Topics(s.Slug)
	if err != nil {
		return a.wrap(err)
	}
	if a.jsonOut {
		return a.writeJSON(sectionResultJSON{Section: sectionOf(s, topics)})
	}
	p := a.out()
	p.println(a.t("section.renamed", s.Slug, output.Line(s.Title)))
	return p.err
}

type sectionRemovedJSON struct {
	Deleted struct {
		Section string `json:"section"`
		Topics  int    `json:"topics"`
		Entries int    `json:"journal_entries_unlinked"`
	} `json:"deleted"`
}

func (a *app) newSectionRm() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "rm <section>",
		Short: a.t("cmd.section_rm.short"),
		Long:  a.t("cmd.section_rm.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSectionRm(args[0], yes)
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, a.t("flag.yes"))
	return c
}

func (a *app) runSectionRm(slug string, yes bool) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	s, err := a.sectionArg(nb, slug)
	if err != nil {
		return err
	}
	// Refuse before asking: the count in the prompt must be exactly what
	// DeleteSection will remove.
	topicCount, err := nb.SectionDeletable(s.Slug)
	if err != nil {
		return a.wrap(err)
	}
	entries, err := nb.Journal(notebook.JournalFilter{Section: s.Slug})
	if err != nil {
		return a.wrap(err)
	}
	if err := a.confirm(yes, a.t("section.rm_prompt", s.Slug, output.Line(s.Title), topicCount, len(entries))); err != nil {
		return err
	}
	if err := nb.DeleteSection(s.Slug); err != nil {
		return a.partial(a.wrap(err))
	}
	if a.jsonOut {
		var res sectionRemovedJSON
		res.Deleted.Section, res.Deleted.Topics, res.Deleted.Entries = s.Slug, topicCount, len(entries)
		return a.writeJSON(res)
	}
	p := a.out()
	p.println(a.t("section.removed", s.Slug, topicCount, len(entries)))
	return p.err
}

type sectionMovedJSON struct {
	Section sectionJSON `json:"section"`
	Moved   bool        `json:"moved"`
	Order   []string    `json:"order"`
}

func (a *app) newSectionMv() *cobra.Command {
	var up, down bool
	c := &cobra.Command{
		Use:   "mv <section> (--up | --down)",
		Short: a.t("cmd.section_mv.short"),
		Long:  a.t("cmd.section_mv.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			if up == down {
				return a.usagef("err.need_up_or_down")
			}
			delta := 1
			if up {
				delta = -1
			}
			return a.runSectionMv(args[0], delta)
		},
	}
	c.Flags().BoolVar(&up, "up", false, a.t("flag.up"))
	c.Flags().BoolVar(&down, "down", false, a.t("flag.down"))
	return c
}

func (a *app) runSectionMv(slug string, delta int) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	s, err := a.sectionArg(nb, slug)
	if err != nil {
		return err
	}
	moved, err := nb.ReorderSection(s.Slug, delta)
	if err != nil {
		return a.partial(a.wrap(err))
	}
	secs, err := nb.Sections()
	if err != nil {
		return a.wrap(err)
	}
	order := make([]string, 0, len(secs))
	for _, x := range secs {
		order = append(order, x.Slug)
		if x.Slug == s.Slug {
			s = x
		}
	}
	if a.jsonOut {
		topics, _ := nb.Topics(s.Slug)
		return a.writeJSON(sectionMovedJSON{Section: sectionOf(s, topics), Moved: moved, Order: order})
	}
	p := a.out()
	switch {
	case !moved && delta < 0:
		p.println(a.t("section.already_first", s.Slug))
	case !moved:
		p.println(a.t("section.already_last", s.Slug))
	case delta < 0:
		p.println(a.t("section.moved_up", s.Slug))
	default:
		p.println(a.t("section.moved_down", s.Slug))
	}
	return p.err
}

func (a *app) newSectionLs() *cobra.Command {
	return &cobra.Command{
		Use:   "ls",
		Short: a.t("cmd.section_ls.short"),
		Long:  a.t("cmd.section_ls.long"),
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.wrap(a.runLs(nil, "", ""))
		},
	}
}
