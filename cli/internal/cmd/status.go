package cmd

import (
	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func (a *app) newAdvance() *cobra.Command {
	return &cobra.Command{
		Use:   "advance <section>/<topic>",
		Short: a.t("cmd.advance.short"),
		Long:  a.t("cmd.advance.long"),
		Args:  cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSetStatus(args[0], "", true)
		},
	}
}

func (a *app) newStatus() *cobra.Command {
	c := &cobra.Command{
		Use:   "status <section>/<topic> <pending|in-progress|mastered>",
		Short: a.t("cmd.status.short"),
		Long:  a.t("cmd.status.long"),
		Args:  cobra.ExactArgs(2),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runSetStatus(args[0], args[1], false)
		},
		ValidArgsFunction: func(_ *cobra.Command, args []string, _ string) ([]string, cobra.ShellCompDirective) {
			if len(args) == 1 {
				return statusNames(), cobra.ShellCompDirectiveNoFileComp
			}
			return nil, cobra.ShellCompDirectiveNoFileComp
		},
	}
	return c
}

type statusJSON struct {
	Topic          topicJSON `json:"topic"`
	PreviousStatus string    `json:"previous_status"`
	Changed        bool      `json:"changed"`
}

// runSetStatus implements both advance (next == true) and status.
func (a *app) runSetStatus(arg, status string, next bool) error {
	var want notebook.Status
	var err error
	if !next {
		if want, err = parseStatus(a, status); err != nil {
			return err
		}
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	prev := t.Status
	if next {
		want, _ = prev.Next() // mastered stays mastered
	}
	changed := want != prev
	if changed {
		t.Status = want
		if t, err = nb.UpdateTopic(t); err != nil {
			return a.wrapInput(err)
		}
	}
	if a.jsonOut {
		tj, err := a.topicResult(nb, t)
		if err != nil {
			return err
		}
		return a.writeJSON(statusJSON{Topic: tj, PreviousStatus: string(prev), Changed: changed})
	}
	p := a.out()
	ref := t.Ref().String()
	switch {
	case changed:
		p.println(a.t("status.changed", ref, a.t("status."+string(prev)), a.t("status."+string(want))))
	case next:
		p.println(a.t("status.already_mastered", ref))
	default:
		p.println(a.t("status.unchanged", ref, a.t("status."+string(want))))
	}
	return p.err
}
