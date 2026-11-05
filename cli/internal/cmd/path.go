package cmd

import (
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newPath() *cobra.Command {
	var md bool
	c := &cobra.Command{
		Use:   "path",
		Short: a.t("cmd.path.short"),
		Long:  a.t("cmd.path.long"),
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			return a.wrap(a.runPath(md))
		},
	}
	c.Flags().BoolVar(&md, "md", false, a.t("flag.md"))
	return c
}

type stepJSON struct {
	Step         int    `json:"step"`
	Ref          string `json:"ref"`
	Section      string `json:"section"`
	SectionTitle string `json:"section_title"`
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Status       string `json:"status"`
	Suggested    bool   `json:"suggested"`
}

type pathJSON struct {
	Mastered int        `json:"mastered"`
	Total    int        `json:"total"`
	Steps    []stepJSON `json:"steps"`
}

func (a *app) runPath(md bool) error {
	if md && a.jsonOut {
		return a.usagef("err.md_json")
	}
	nb, cfg, err := a.open()
	if err != nil {
		return err
	}
	steps, err := nb.RolePath()
	if err != nil {
		return err
	}
	if md {
		return a.pathMarkdown(nb, cfg, steps)
	}
	res := pathJSON{Total: len(steps), Steps: make([]stepJSON, 0, len(steps))}
	for _, s := range steps {
		if s.Topic.Status == notebook.StatusMastered {
			res.Mastered++
		}
		res.Steps = append(res.Steps, stepJSON{
			Step: s.Step, Ref: s.Topic.Ref().String(), Section: s.Topic.Section, SectionTitle: s.SectionTitle,
			Slug: s.Topic.Slug, Title: s.Topic.Title, Status: string(s.Topic.Status), Suggested: s.Topic.Suggested,
		})
	}
	if a.jsonOut {
		return a.writeJSON(res)
	}

	p := a.out()
	p.println(a.style.Bold(a.t("path.title", res.Mastered, res.Total)))
	if len(steps) == 0 {
		p.println("")
		p.println(a.t("path.empty"))
		return p.err
	}
	numW := len(strconv.Itoa(len(steps)))
	last := ""
	for _, s := range res.Steps {
		if s.Section != last {
			last = s.Section
			p.println("")
			p.println(a.style.Bold(output.Line(s.SectionTitle)))
		}
		mark := " "
		if s.Status == string(notebook.StatusMastered) {
			mark = a.style.Green("✓")
		}
		p.printf("  %s %s. %s  %s\n", mark, output.PadLeft(strconv.Itoa(s.Step), numW), output.Line(s.Title),
			a.style.Dim("("+a.t("status."+s.Status)+")"))
	}
	return p.err
}

// pathMarkdown prints the role path as a guide that can be shared: only
// topics of type role, with their notes and resources, and none of the
// personal progress (status, journal).
func (a *app) pathMarkdown(nb *notebook.Notebook, cfg notebook.Config, steps []notebook.RoleStep) error {
	p := a.out()
	name := output.Line(cfg.Name)
	if name == "" {
		name = a.t("path.md.default_name")
	}
	p.println("# " + a.t("path.md.title", name))
	if len(steps) == 0 {
		p.println("")
		p.println(a.t("path.empty"))
		return p.err
	}
	last := ""
	for _, s := range steps {
		t := s.Topic
		if t.Section != last {
			last = t.Section
			sec, err := nb.Section(t.Section)
			if err != nil {
				return err
			}
			p.println("")
			p.println("## " + output.Line(sec.Title))
			if d := strings.TrimSpace(output.Text(sec.Description)); d != "" {
				p.println("")
				p.println(a.wrapText(d, 0))
			}
		}
		p.println("")
		p.println("### " + strconv.Itoa(s.Step) + ". " + output.Line(t.Title))
		if n := strings.TrimSpace(output.Text(t.Notes)); n != "" {
			p.println("")
			p.println(n)
		}
		if len(t.Resources) > 0 {
			p.println("")
			p.println("**" + a.t("path.md.resources") + "**")
			p.println("")
			for _, r := range t.Resources {
				p.println("- " + mdLink(r))
			}
		}
	}
	return p.err
}

// mdLink formats a resource as a Markdown list item body. URLs were
// validated as absolute http(s) URLs; angle brackets keep spaces and
// parentheses from breaking the link.
func mdLink(r notebook.Resource) string {
	u := strings.NewReplacer(">", "%3E", "<", "%3C", "\n", "", "\r", "").Replace(output.Line(r.URL))
	title := output.Line(r.Title)
	if title == "" {
		return "<" + u + ">"
	}
	title = strings.NewReplacer("[", `\[`, "]", `\]`).Replace(title)
	return "[" + title + "](<" + u + ">)"
}
