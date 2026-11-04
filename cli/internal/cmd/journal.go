package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newJournal() *cobra.Command {
	var from, to, section string
	var limit int
	c := &cobra.Command{
		Use:   "journal",
		Short: a.t("cmd.journal.short"),
		Long:  a.t("cmd.journal.long"),
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.wrap(a.runJournal(from, to, section, limit))
		},
	}
	c.Flags().StringVar(&from, "from", "", a.t("flag.from"))
	c.Flags().StringVar(&to, "to", "", a.t("flag.to"))
	c.Flags().StringVarP(&section, "section", "s", "", a.t("flag.section"))
	c.Flags().IntVar(&limit, "limit", 0, a.t("flag.limit_entries"))
	return c
}

type journalJSON struct {
	Entries []entryJSON `json:"entries"`
}

func (a *app) runJournal(from, to, section string, limit int) error {
	var f notebook.JournalFilter
	var err error
	if from != "" {
		if f.From, err = notebook.ParseDate(from); err != nil {
			return a.usagef("err.bad_date", "--from", from)
		}
	}
	if to != "" {
		if f.To, err = notebook.ParseDate(to); err != nil {
			return a.usagef("err.bad_date", "--to", to)
		}
	}
	if !f.From.IsZero() && !f.To.IsZero() && f.From.Compare(f.To) > 0 {
		return a.usagef("err.date_range")
	}
	if limit < 0 {
		return a.usagef("err.bad_limit")
	}
	f.Limit = limit
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	if section != "" {
		sec, err := a.sectionArg(nb, section)
		if err != nil {
			return err
		}
		f.Section = sec.Slug
	}
	entries, err := nb.Journal(f)
	if err != nil {
		return err
	}
	if a.jsonOut {
		return a.writeJSON(journalJSON{Entries: entriesOf(entries)})
	}
	p := a.out()
	if len(entries) == 0 {
		p.println(a.t("journal.none"))
		return p.err
	}
	var day notebook.Date
	for i, e := range entries {
		if i == 0 || e.Date != day {
			if i > 0 {
				p.println("")
			}
			day = e.Date
			p.println(a.style.Bold(day.String()))
		}
		head := "  [" + e.ID + "]"
		switch {
		case !e.Topic.IsZero():
			head += " " + e.Topic.String()
		case e.Section != "":
			head += " " + e.Section
		}
		p.println(a.style.Dim(head))
		p.println(indent(output.Text(strings.TrimRight(e.Body, "\n")), "    "))
	}
	return p.err
}
