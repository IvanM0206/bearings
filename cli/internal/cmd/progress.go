package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/output"
)

// barWidth is the number of cells inside every progress bar, so that bars
// line up whatever the numbers are.
const barWidth = 20

func (a *app) newProgress() *cobra.Command {
	return &cobra.Command{
		Use:   "progress",
		Short: a.t("cmd.progress.short"),
		Long:  a.t("cmd.progress.long"),
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.wrap(a.runProgress())
		},
	}
}

type progressSectionJSON struct {
	Slug  string     `json:"slug"`
	Title string     `json:"title"`
	All   countsJSON `json:"all"`
	Role  countsJSON `json:"role"`
}

type progressJSON struct {
	All            countsJSON            `json:"all"`
	Role           countsJSON            `json:"role"`
	Personal       countsJSON            `json:"personal"`
	Suggested      int                   `json:"suggested"`
	JournalEntries int                   `json:"journal_entries"`
	Sections       []progressSectionJSON `json:"sections"`
}

func (a *app) runProgress() error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	pr, err := nb.Progress()
	if err != nil {
		return err
	}
	res := progressJSON{
		All: fromCounts(pr.All), Role: fromCounts(pr.Role), Personal: fromCounts(pr.Personal),
		Suggested: pr.Suggested, JournalEntries: pr.Entries,
		Sections: make([]progressSectionJSON, 0, len(pr.Sections)),
	}
	for _, s := range pr.Sections {
		res.Sections = append(res.Sections, progressSectionJSON{Slug: s.Slug, Title: s.Title, All: fromCounts(s.All), Role: fromCounts(s.Role)})
	}
	if a.jsonOut {
		return a.writeJSON(res)
	}

	type row struct {
		label string
		c     countsJSON
	}
	var secRows, totRows []row
	for _, s := range res.Sections {
		secRows = append(secRows, row{output.Line(s.Title), s.All})
	}
	totRows = []row{
		{a.t("progress.total"), res.All},
		{a.t("progress.role"), res.Role},
		{a.t("progress.personal"), res.Personal},
	}
	lw, nw := 0, 0
	for _, r := range append(append([]row{}, secRows...), totRows...) {
		lw = max(lw, output.Width(r.label))
		nw = max(nw, len(fmt.Sprintf("%d/%d", r.c.Mastered, r.c.Total)))
	}
	line := func(r row) string {
		return fmt.Sprintf("%s  %s  %s  %3d%%", output.Pad(r.label, lw), output.Bar(r.c.Mastered, r.c.Total, barWidth),
			output.PadLeft(fmt.Sprintf("%d/%d", r.c.Mastered, r.c.Total), nw), output.Percent(r.c.Mastered, r.c.Total))
	}

	p := a.out()
	p.println(a.style.Bold(a.t("progress.title")))
	p.println("")
	if len(secRows) == 0 {
		p.println(a.t("ls.no_sections"))
	}
	for _, r := range secRows {
		p.println(line(r))
	}
	if len(secRows) > 0 {
		p.println("")
	}
	for _, r := range totRows {
		p.println(line(r))
	}
	p.println("")
	p.println(a.t("progress.suggested", res.Suggested))
	p.println(a.t("progress.journal", res.JournalEntries))
	return p.err
}
