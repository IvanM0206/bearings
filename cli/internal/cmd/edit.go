package cmd

import (
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newEdit() *cobra.Command {
	var journalID string
	c := &cobra.Command{
		Use:   "edit <section>/<topic> | --journal <id>",
		Short: a.t("cmd.edit.short"),
		Long:  a.t("cmd.edit.long"),
		Args:  cobra.MaximumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			journal := c.Flags().Changed("journal")
			if journal == (len(args) == 1) {
				return a.usagef("err.edit_target")
			}
			if journal {
				return a.runEditEntry(journalID)
			}
			return a.runEditTopic(args[0])
		},
	}
	c.Flags().StringVar(&journalID, "journal", "", a.t("flag.journal_id"))
	return c
}

type editTopicJSON struct {
	Topic   topicJSON `json:"topic"`
	Changed bool      `json:"changed"`
}

type editEntryJSON struct {
	Entry   entryJSON `json:"entry"`
	Changed bool      `json:"changed"`
}

// sameText compares two texts the way the notebook stores them: LF line
// ends, no leading blank lines, no trailing white space.
func sameText(x, y string) bool { return storedText(x) == storedText(y) }

func storedText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	return strings.TrimLeft(strings.TrimRightFunc(s, unicode.IsSpace), "\n")
}

func (a *app) runEditTopic(arg string) error {
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	t, err := a.topicArg(nb, arg)
	if err != nil {
		return err
	}
	edited, err := a.editText("notes.md", t.Notes)
	if err != nil {
		return err
	}
	changed := !sameText(edited, t.Notes)
	if changed {
		t.Notes = edited
		// t still carries the version read before editing: if the file
		// changed in the meantime the write is refused.
		if t, err = nb.UpdateTopic(t); err != nil {
			return a.withRecovery(a.wrapInput(err), "notes.md", edited)
		}
	}
	if a.jsonOut {
		tj, err := a.topicResult(nb, t)
		if err != nil {
			return err
		}
		return a.writeJSON(editTopicJSON{Topic: tj, Changed: changed})
	}
	p := a.out()
	if changed {
		p.println(a.t("edit.saved_topic", t.Ref().String()))
	} else {
		p.println(a.t("edit.unchanged"))
	}
	return p.err
}

func (a *app) runEditEntry(id string) error {
	if !notebook.ValidEntryID(id) {
		return a.usagef("err.bad_entry_id", output.Line(id))
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	e, err := nb.Entry(id)
	if notebook.CodeOf(err) == notebook.CodeNotFound {
		return a.notFoundf("err.no_entry", id)
	}
	if err != nil {
		return a.wrap(err)
	}
	edited, err := a.editText("entry.md", e.Body)
	if err != nil {
		return err
	}
	changed := !sameText(edited, e.Body)
	if changed {
		if strings.TrimSpace(edited) == "" {
			return &exitError{code: ExitFailure, msg: a.t("err.entry_aborted")}
		}
		e.Body = edited
		if e, err = nb.UpdateEntry(e); err != nil {
			return a.withRecovery(a.wrapInput(err), "entry.md", edited)
		}
	}
	if a.jsonOut {
		return a.writeJSON(editEntryJSON{Entry: entryOf(e), Changed: changed})
	}
	p := a.out()
	if changed {
		p.println(a.t("edit.saved_entry", e.ID))
	} else {
		p.println(a.t("edit.unchanged"))
	}
	return p.err
}
