package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show <section>/<topic>",
		Short: a.t("cmd.show.short"),
		Long:  a.t("cmd.show.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.wrap(a.runShow(args[0]))
		},
	}
}

type showJSON struct {
	Topic   topicJSON   `json:"topic"`
	Journal []entryJSON `json:"journal"`
}

func (a *app) runShow(arg string) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	sec, err := nb.Section(t.Section)
	if err != nil {
		return err
	}
	entries, err := nb.Journal(notebook.JournalFilter{Topic: t.Ref()})
	if err != nil {
		return err
	}
	if a.jsonOut {
		tj := topicOf(t, sec.Title)
		return a.writeJSON(showJSON{Topic: tj, Journal: entriesOf(entries)})
	}

	p := a.out()
	p.println(a.style.Bold(output.Line(t.Title)))
	p.println(a.style.Dim(t.Ref().String() + "  -  " + output.Line(sec.Title)))
	p.println("")
	sugg := a.t("common.no")
	if t.Suggested {
		sugg = a.t("common.yes")
	}
	p.printf("%s %s\n", a.t("show.status"), a.t("status."+string(t.Status)))
	p.printf("%s %s\n", a.t("show.type"), a.t("type."+string(t.Type)))
	p.printf("%s %s\n", a.t("show.suggested"), sugg)
	p.printf("%s %s\n", a.t("show.created"), notebook.FormatTimestamp(t.Created))
	p.printf("%s %s\n", a.t("show.updated"), notebook.FormatTimestamp(t.Updated))

	p.println("")
	p.println(a.style.Bold(a.t("show.resources")))
	if len(t.Resources) == 0 {
		p.println("  " + a.style.Dim(a.t("common.none")))
	}
	for _, r := range t.Resources {
		if r.Title != "" {
			p.printf("  - %s  %s\n", output.Line(r.Title), output.Line(r.URL))
		} else {
			p.printf("  - %s\n", output.Line(r.URL))
		}
	}

	p.println("")
	p.println(a.style.Bold(a.t("show.notes")))
	if strings.TrimSpace(t.Notes) == "" {
		p.println("  " + a.style.Dim(a.t("common.none")))
	} else {
		p.println(indent(output.Text(strings.TrimRight(t.Notes, "\n")), "  "))
	}

	p.println("")
	p.println(a.style.Bold(a.t("show.journal", len(entries))))
	if len(entries) == 0 {
		p.println("  " + a.style.Dim(a.t("common.none")))
	}
	for _, e := range entries {
		p.printf("  %s  [%s]\n", e.Date, e.ID)
		p.println(indent(output.Text(strings.TrimRight(e.Body, "\n")), "    "))
	}
	return p.err
}

// indent prefixes every non-empty line of s.
func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = prefix + l
		}
	}
	return strings.Join(lines, "\n")
}
