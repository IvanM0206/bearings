package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newTopic() *cobra.Command {
	c := a.group("topic", a.t("cmd.topic.short"), a.t("cmd.topic.long"))
	c.AddCommand(a.newTopicAdd(), a.newTopicEdit(), a.newTopicRm(), a.newTopicMv(), a.newTopicLs())
	return c
}

type topicResultJSON struct {
	Topic topicJSON `json:"topic"`
}

// topicResult builds the JSON view of a topic that a command just wrote.
func (a *app) topicResult(nb *notebook.Notebook, t notebook.Topic) (topicJSON, error) {
	sec, err := nb.Section(t.Section)
	if err != nil {
		return topicJSON{}, a.wrap(err)
	}
	return topicOf(t, sec.Title), nil
}

func parseStatus(a *app, s string) (notebook.Status, error) {
	st := notebook.Status(s)
	if !st.Valid() {
		return "", a.usagef("err.bad_status", output.Line(s))
	}
	return st, nil
}

func parseType(a *app, s string) (notebook.TopicType, error) {
	ty := notebook.TopicType(s)
	if !ty.Valid() {
		return "", a.usagef("err.bad_type", output.Line(s))
	}
	return ty, nil
}

// ---- topic add -------------------------------------------------------------

func (a *app) newTopicAdd() *cobra.Command {
	var typ, status string
	c := &cobra.Command{
		Use:   "add <section> <title>",
		Short: a.t("cmd.topic_add.short"),
		Long:  a.t("cmd.topic_add.long"),
		Args:  cobra.MinimumNArgs(2),
		RunE: func(c *cobra.Command, args []string) error {
			return a.runTopicAdd(args[0], strings.Join(args[1:], " "), typ, status, c.Flags().Changed("type"), c.Flags().Changed("status"))
		},
	}
	c.Flags().StringVar(&typ, "type", "", a.t("flag.type_new"))
	c.Flags().StringVar(&status, "status", "", a.t("flag.status_new"))
	_ = c.RegisterFlagCompletionFunc("status", fixed(statusNames()...))
	_ = c.RegisterFlagCompletionFunc("type", fixed(string(notebook.TypeRole), string(notebook.TypePersonal)))
	return c
}

func (a *app) runTopicAdd(section, title, typ, status string, typeSet, statusSet bool) error {
	in := notebook.NewTopic{Title: title}
	var err error
	if typeSet {
		if in.Type, err = parseType(a, typ); err != nil {
			return err
		}
	}
	if statusSet {
		if in.Status, err = parseStatus(a, status); err != nil {
			return err
		}
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	sec, err := a.sectionArg(nb, section)
	if err != nil {
		return err
	}
	in.Section = sec.Slug
	t, err := nb.AddTopic(in)
	if err != nil {
		return a.wrapInput(err)
	}
	if a.jsonOut {
		return a.writeJSON(topicResultJSON{Topic: topicOf(t, sec.Title)})
	}
	p := a.out()
	p.println(a.t("topic.added", t.Ref().String(), output.Line(t.Title), a.t("type."+string(t.Type)), a.t("status."+string(t.Status))))
	return p.err
}

// ---- topic edit ------------------------------------------------------------

type topicEditFlags struct {
	title, typ                     string
	clearSuggested                 bool
	addResource, resourceTitle     string
	removeResource                 string
	yes                            bool
	titleSet, typeSet, resTitleSet bool
}

func (a *app) newTopicEdit() *cobra.Command {
	var f topicEditFlags
	c := &cobra.Command{
		Use:   "edit <section>/<topic>",
		Short: a.t("cmd.topic_edit.short"),
		Long:  a.t("cmd.topic_edit.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			fl := c.Flags()
			f.titleSet, f.typeSet, f.resTitleSet = fl.Changed("title"), fl.Changed("type"), fl.Changed("resource-title")
			return a.runTopicEdit(args[0], f)
		},
	}
	fl := c.Flags()
	fl.StringVar(&f.title, "title", "", a.t("flag.new_title"))
	fl.StringVar(&f.typ, "type", "", a.t("flag.type_new"))
	fl.BoolVar(&f.clearSuggested, "clear-suggested", false, a.t("flag.clear_suggested"))
	fl.StringVar(&f.addResource, "add-resource", "", a.t("flag.add_resource"))
	fl.StringVar(&f.resourceTitle, "resource-title", "", a.t("flag.resource_title"))
	fl.StringVar(&f.removeResource, "remove-resource", "", a.t("flag.remove_resource"))
	fl.BoolVarP(&f.yes, "yes", "y", false, a.t("flag.yes"))
	_ = c.RegisterFlagCompletionFunc("type", fixed(string(notebook.TypeRole), string(notebook.TypePersonal)))
	return c
}

func (a *app) runTopicEdit(arg string, f topicEditFlags) error {
	// Check the flags before touching the notebook.
	if f.resTitleSet && f.addResource == "" {
		return a.usagef("err.resource_title_needs_url")
	}
	if !f.titleSet && !f.typeSet && !f.clearSuggested && f.addResource == "" && f.removeResource == "" {
		return a.usagef("err.nothing_to_edit")
	}
	var typ notebook.TopicType
	var err error
	if f.typeSet {
		if typ, err = parseType(a, f.typ); err != nil {
			return err
		}
	}
	if f.addResource != "" {
		if notebook.CheckURL(f.addResource) != nil {
			return a.usagef("err.bad_url")
		}
	}
	if f.removeResource != "" && notebook.CheckURL(f.removeResource) != nil {
		return a.usagef("err.bad_url")
	}

	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	if f.titleSet {
		t.Title = f.title
	}
	if f.typeSet {
		t.Type = typ
	}
	if f.clearSuggested {
		t.Suggested = false
	}
	if f.removeResource != "" {
		kept := make([]notebook.Resource, 0, len(t.Resources))
		for _, r := range t.Resources {
			if r.URL != f.removeResource {
				kept = append(kept, r)
			}
		}
		if len(kept) == len(t.Resources) {
			return a.usagef("err.resource_missing", output.Line(f.removeResource))
		}
		t.Resources = kept
	}
	if f.addResource != "" {
		for _, r := range t.Resources {
			if r.URL == f.addResource {
				return a.usagef("err.resource_exists", output.Line(f.addResource))
			}
		}
		t.Resources = append(t.Resources, notebook.Resource{URL: f.addResource, Title: strings.TrimSpace(f.resourceTitle)})
	}
	if f.removeResource != "" {
		prompt := a.t("topic.remove_resource_prompt", t.Ref().String(), output.Line(f.removeResource), 1)
		if err := a.confirm(f.yes, prompt); err != nil {
			return err
		}
	}
	t, err = nb.UpdateTopic(t)
	if err != nil {
		return a.wrapInput(err)
	}
	if a.jsonOut {
		tj, err := a.topicResult(nb, t)
		if err != nil {
			return err
		}
		return a.writeJSON(topicResultJSON{Topic: tj})
	}
	p := a.out()
	p.println(a.t("topic.updated", t.Ref().String()))
	return p.err
}

// ---- topic rm --------------------------------------------------------------

type topicRemovedJSON struct {
	Deleted struct {
		Topic     string `json:"topic"`
		Resources int    `json:"resources"`
		Entries   int    `json:"journal_entries_unlinked"`
	} `json:"deleted"`
}

func (a *app) newTopicRm() *cobra.Command {
	var yes bool
	c := &cobra.Command{
		Use:   "rm <section>/<topic>",
		Short: a.t("cmd.topic_rm.short"),
		Long:  a.t("cmd.topic_rm.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runTopicRm(args[0], yes)
		},
	}
	c.Flags().BoolVarP(&yes, "yes", "y", false, a.t("flag.yes"))
	return c
}

func (a *app) runTopicRm(arg string, yes bool) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	entries, err := nb.Journal(notebook.JournalFilter{Topic: t.Ref()})
	if err != nil {
		return a.wrap(err)
	}
	if err := a.confirm(yes, a.t("topic.rm_prompt", t.Ref().String(), output.Line(t.Title), len(t.Resources), len(entries))); err != nil {
		return err
	}
	if err := nb.DeleteTopic(t.Ref()); err != nil {
		return a.partial(a.wrap(err))
	}
	if a.jsonOut {
		var res topicRemovedJSON
		res.Deleted.Topic, res.Deleted.Resources, res.Deleted.Entries = t.Ref().String(), len(t.Resources), len(entries)
		return a.writeJSON(res)
	}
	p := a.out()
	p.println(a.t("topic.removed", t.Ref().String(), len(t.Resources), len(entries)))
	return p.err
}

// ---- topic mv --------------------------------------------------------------

type topicMovedJSON struct {
	Topic topicJSON `json:"topic"`
	Moved bool      `json:"moved"`
	Order []string  `json:"order"`
}

func (a *app) newTopicMv() *cobra.Command {
	var up, down bool
	var toSection string
	c := &cobra.Command{
		Use:   "mv <section>/<topic> (--up | --down | --to-section <section>)",
		Short: a.t("cmd.topic_mv.short"),
		Long:  a.t("cmd.topic_mv.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			n := 0
			for _, set := range []bool{up, down, c.Flags().Changed("to-section")} {
				if set {
					n++
				}
			}
			if n != 1 {
				return a.usagef("err.need_mv_target")
			}
			switch {
			case up:
				return a.runTopicReorder(args[0], -1)
			case down:
				return a.runTopicReorder(args[0], 1)
			}
			return a.runTopicToSection(args[0], toSection)
		},
	}
	c.Flags().BoolVar(&up, "up", false, a.t("flag.up"))
	c.Flags().BoolVar(&down, "down", false, a.t("flag.down"))
	c.Flags().StringVar(&toSection, "to-section", "", a.t("flag.to_section"))
	return c
}

func (a *app) runTopicReorder(arg string, delta int) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	moved, err := nb.ReorderTopic(t.Ref(), delta)
	if err != nil {
		return a.partial(a.wrap(err))
	}
	all, err := nb.Topics(t.Section)
	if err != nil {
		return a.wrap(err)
	}
	order := make([]string, 0, len(all))
	for _, x := range all {
		order = append(order, x.Ref().String())
		if x.Slug == t.Slug {
			t = x
		}
	}
	if a.jsonOut {
		tj, err := a.topicResult(nb, t)
		if err != nil {
			return err
		}
		return a.writeJSON(topicMovedJSON{Topic: tj, Moved: moved, Order: order})
	}
	p := a.out()
	ref := t.Ref().String()
	switch {
	case !moved && delta < 0:
		p.println(a.t("topic.already_first", ref))
	case !moved:
		p.println(a.t("topic.already_last", ref))
	case delta < 0:
		p.println(a.t("topic.moved_up", ref))
	default:
		p.println(a.t("topic.moved_down", ref))
	}
	return p.err
}

func (a *app) runTopicToSection(arg, section string) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	target, err := a.sectionArg(nb, section)
	if err != nil {
		return err
	}
	if target.Slug == t.Section {
		return a.usagef("err.same_section", t.Ref().String(), target.Slug)
	}
	to := notebook.TopicRef{Section: target.Slug, Topic: t.Slug}
	if _, err := nb.Topic(to); err == nil {
		return a.usagef("err.topic_exists_in_target", to.String())
	} else if notebook.CodeOf(err) != notebook.CodeNotFound {
		return a.wrap(err)
	}
	moved, err := nb.MoveTopic(t.Ref(), to)
	if err != nil {
		return a.partial(a.wrapInput(err))
	}
	if a.jsonOut {
		tj, err := a.topicResult(nb, moved)
		if err != nil {
			return err
		}
		return a.writeJSON(topicMovedJSON{Topic: tj, Moved: true, Order: topicRefs(nb, target.Slug)})
	}
	p := a.out()
	p.println(a.t("topic.moved_to", t.Ref().String(), to.String()))
	return p.err
}

func topicRefs(nb *notebook.Notebook, section string) []string {
	all, _ := nb.Topics(section)
	out := make([]string, 0, len(all))
	for _, x := range all {
		out = append(out, x.Ref().String())
	}
	return out
}

// ---- topic ls --------------------------------------------------------------

func (a *app) newTopicLs() *cobra.Command {
	var status, typ string
	c := &cobra.Command{
		Use:   "ls <section>",
		Short: a.t("cmd.topic_ls.short"),
		Long:  a.t("cmd.topic_ls.long"),
		Args:  cobra.ExactArgs(1),
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
