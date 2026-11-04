package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newLs() *cobra.Command {
	var status, typ string
	c := &cobra.Command{
		Use:   "ls [section]",
		Short: a.t("cmd.ls.short"),
		Long:  a.t("cmd.ls.long"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.wrap(a.runLs(args, status, typ))
		},
	}
	c.Flags().StringVar(&status, "status", "", a.t("flag.status"))
	c.Flags().StringVar(&typ, "type", "", a.t("flag.type"))
	_ = c.RegisterFlagCompletionFunc("status", fixed(statusNames()...))
	_ = c.RegisterFlagCompletionFunc("type", fixed(string(notebook.TypeRole), string(notebook.TypePersonal)))
	return c
}

type lsSectionsJSON struct {
	Sections []sectionJSON `json:"sections"`
}

type lsTopicsJSON struct {
	Section sectionJSON        `json:"section"`
	Filters filtersJSON        `json:"filters"`
	Topics  []topicSummaryJSON `json:"topics"`
}

type filtersJSON struct {
	Status string `json:"status"`
	Type   string `json:"type"`
}

func (a *app) runLs(args []string, status, typ string) error {
	f := notebook.TopicFilter{Status: notebook.Status(status), Type: notebook.TopicType(typ)}
	if status != "" && !f.Status.Valid() {
		return a.usagef("err.bad_status", status)
	}
	if typ != "" && !f.Type.Valid() {
		return a.usagef("err.bad_type", typ)
	}
	if len(args) == 0 && (status != "" || typ != "") {
		return a.usagef("err.filter_needs_section")
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	if len(args) == 0 {
		return a.lsSections(nb)
	}
	return a.lsTopics(nb, args[0], f)
}

func (a *app) lsSections(nb *notebook.Notebook) error {
	outline, err := nb.Outline()
	if err != nil {
		return err
	}
	res := lsSectionsJSON{Sections: make([]sectionJSON, 0, len(outline))}
	for _, st := range outline {
		res.Sections = append(res.Sections, sectionOf(st.Section, st.Topics))
	}
	if a.jsonOut {
		return a.writeJSON(res)
	}
	p := a.out()
	if len(res.Sections) == 0 {
		p.println(a.t("ls.no_sections"))
		return p.err
	}
	rows := make([][]string, 0, len(res.Sections))
	for _, s := range res.Sections {
		rows = append(rows, []string{s.Slug, s.Title, fmt.Sprintf("%d/%d", s.Counts.Mastered, s.Counts.Total)})
	}
	if err := output.Table(a.env.Stdout, []string{a.t("col.section"), a.t("col.title"), a.t("col.mastered")}, rows, a.style.Bold); err != nil {
		return err
	}
	return p.err
}

func (a *app) lsTopics(nb *notebook.Notebook, slug string, f notebook.TopicFilter) error {
	sec, err := a.sectionArg(nb, slug)
	if err != nil {
		return err
	}
	all, err := nb.Topics(sec.Slug)
	if err != nil {
		return err
	}
	topics := make([]notebook.Topic, 0, len(all))
	for _, t := range all {
		if (f.Status == "" || t.Status == f.Status) && (f.Type == "" || t.Type == f.Type) {
			topics = append(topics, t)
		}
	}
	if a.jsonOut {
		res := lsTopicsJSON{
			Section: sectionOf(sec, all),
			Filters: filtersJSON{Status: string(f.Status), Type: string(f.Type)},
			Topics:  make([]topicSummaryJSON, 0, len(topics)),
		}
		for _, t := range topics {
			res.Topics = append(res.Topics, topicSummary(t))
		}
		return a.writeJSON(res)
	}
	p := a.out()
	p.println(a.style.Bold(output.Line(sec.Title)) + " (" + sec.Slug + ")")
	if d := output.Text(sec.Description); d != "" {
		p.println(d)
	}
	p.println("")
	if len(topics) == 0 {
		p.println(a.t("ls.no_topics"))
		return p.err
	}
	rows := make([][]string, 0, len(topics))
	for _, t := range topics {
		sugg := ""
		if t.Suggested {
			sugg = a.t("common.yes")
		}
		rows = append(rows, []string{t.Slug, t.Title, a.t("status." + string(t.Status)), a.t("type." + string(t.Type)), sugg})
	}
	hdr := []string{a.t("col.topic"), a.t("col.title"), a.t("col.status"), a.t("col.type"), a.t("col.suggested")}
	if err := output.Table(a.env.Stdout, hdr, rows, a.style.Bold); err != nil {
		return err
	}
	return p.err
}
