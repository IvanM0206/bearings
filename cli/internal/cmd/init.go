package cmd

import (
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/i18n"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
	"github.com/IvanM0206/bearings/cli/internal/templates"
)

func (a *app) newInit() *cobra.Command {
	var tmpl, name, language string
	var list bool
	c := &cobra.Command{
		Use:   "init",
		Short: a.t("cmd.init.short"),
		Long:  a.t("cmd.init.long"),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			if list {
				return a.wrap(a.runListTemplates())
			}
			return a.runInit(tmpl, name, language, c.Flags().Changed("name"))
		},
	}
	c.Flags().StringVar(&tmpl, "template", "", a.t("flag.template"))
	c.Flags().StringVar(&name, "name", "", a.t("flag.name"))
	c.Flags().StringVar(&language, "language", "", a.t("flag.language"))
	c.Flags().BoolVar(&list, "list-templates", false, a.t("flag.list_templates"))
	_ = c.RegisterFlagCompletionFunc("language", fixed(i18n.Known...))
	_ = c.RegisterFlagCompletionFunc("template", func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
		infos, _ := templates.List()
		ids := make([]string, 0, len(infos))
		for _, i := range infos {
			ids = append(ids, i.ID)
		}
		return ids, cobra.ShellCompDirectiveNoFileComp
	})
	return c
}

type templatesJSON struct {
	Templates []templates.Info `json:"templates"`
}

func (a *app) runListTemplates() error {
	infos, err := templates.List()
	if err != nil {
		return err
	}
	if infos == nil {
		infos = []templates.Info{}
	}
	if a.jsonOut {
		return a.writeJSON(templatesJSON{Templates: infos})
	}
	rows := make([][]string, 0, len(infos))
	for _, i := range infos {
		rows = append(rows, []string{i.ID, i.Title, i.Language, fmt.Sprint(i.Sections), fmt.Sprint(i.Topics)})
	}
	hdr := []string{a.t("col.id"), a.t("col.title"), a.t("col.language"), a.t("col.sections"), a.t("col.topics")}
	return a.table(hdr, rows)
}

type initJSON struct {
	Path     string `json:"path"`
	Name     string `json:"name"`
	Language string `json:"language"`
	Template string `json:"template"`
	Sections int    `json:"sections"`
	Topics   int    `json:"topics"`
}

func (a *app) runInit(tmpl, name, language string, nameSet bool) error {
	dir, err := a.notebookDir()
	if err != nil {
		return err
	}
	if nameSet {
		name = strings.TrimSpace(name)
		if name == "" {
			return a.usagef("err.empty_name")
		}
		if hasControl(name) {
			return a.usagef("err.control_in_name")
		}
	}
	lang := ""
	if language != "" {
		lang = i18n.Normalize(language)
		if !i18n.IsKnown(lang) {
			return a.usagef("err.unknown_language", output.Line(language), strings.Join(i18n.Known, ", "))
		}
	}
	now := a.now()

	var tree *notebook.Tree
	if tmpl != "" {
		tree, err = templates.Instantiate(tmpl, name, now)
		if errors.Is(err, templates.ErrNotFound) {
			ids := []string{}
			infos, _ := templates.List()
			for _, i := range infos {
				ids = append(ids, i.ID)
			}
			return a.usagef("err.unknown_template", output.Line(tmpl), strings.Join(ids, ", "))
		}
		if err != nil {
			return a.wrap(err)
		}
		if lang != "" {
			tree.Config.Language = lang
		}
	} else {
		if lang == "" {
			lang = a.tr.Lang()
		}
		tree = &notebook.Tree{Config: notebook.Config{Format: notebook.FormatVersion, Name: name, Language: lang}}
	}

	nb, err := notebook.Create(dir, tree, a.nbOptions())
	if err != nil {
		if notebook.CodeOf(err) == notebook.CodeNotEmpty {
			return a.usagef("err.init_not_empty", output.Line(dir))
		}
		return a.wrapInput(err)
	}
	res := initJSON{
		Path: nb.Root(), Name: tree.Config.Name, Language: tree.Config.Language, Template: tmpl,
		Sections: len(tree.Sections), Topics: len(tree.Topics),
	}
	if a.jsonOut {
		return a.writeJSON(res)
	}
	p := a.out()
	p.println(a.t("init.created", output.Line(nb.Root())))
	if tmpl != "" {
		p.println(a.t("init.from_template", output.Line(tmpl), res.Sections, res.Topics))
		p.println(a.t("init.review_hint"))
	}
	p.println(a.t("init.next"))
	return p.err
}

// hasControl reports whether s contains a control character.
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}
