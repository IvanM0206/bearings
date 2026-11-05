package cmd

import (
	"bytes"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newLog() *cobra.Command {
	var section, topic, date string
	c := &cobra.Command{
		Use:   "log [text | -]",
		Short: a.t("cmd.log.short"),
		Long:  a.t("cmd.log.long"),
		Args:  cobra.ArbitraryArgs,
		RunE: func(_ *cobra.Command, args []string) error {
			return a.runLog(args, section, topic, date)
		},
	}
	c.Flags().StringVarP(&section, "section", "s", "", a.t("flag.log_section"))
	c.Flags().StringVarP(&topic, "topic", "t", "", a.t("flag.log_topic"))
	c.Flags().StringVar(&date, "date", "", a.t("flag.date"))
	return c
}

type logJSON struct {
	Entry entryJSON `json:"entry"`
}

func (a *app) runLog(args []string, section, topic, date string) error {
	var in notebook.NewEntry
	if date != "" {
		d, err := notebook.ParseDate(date)
		if err != nil {
			return a.usagef("err.bad_date", "--date", output.Line(date))
		}
		in.Date = d
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	// Check the references before asking for the text, so that a typo does
	// not cost an editing session.
	if section != "" {
		sec, err := a.sectionArg(nb, section)
		if err != nil {
			return err
		}
		in.Section = sec.Slug
	}
	if topic != "" {
		t, err := a.topicArg(nb, topic)
		if err != nil {
			return err
		}
		if in.Section != "" && in.Section != t.Section {
			return a.usagef("err.topic_not_in_section", t.Ref().String(), in.Section)
		}
		in.Section, in.Topic = t.Section, t.Ref()
	}

	fromEditor := false
	switch {
	case len(args) == 1 && args[0] == "-":
		if in.Body, err = a.readStdinText(notebook.MaxEntryBodySize); err != nil {
			return err
		}
	case len(args) > 0:
		in.Body = strings.Join(args, " ")
	default:
		fromEditor = true
		if in.Body, err = a.editText("entry.md", ""); err != nil {
			return err
		}
	}
	if strings.TrimSpace(in.Body) == "" {
		if fromEditor {
			return &exitError{code: ExitFailure, msg: a.t("err.entry_aborted")}
		}
		return a.usagef("err.empty_entry")
	}

	e, err := nb.AddEntry(in)
	if err != nil {
		err = a.wrapInput(err)
		if fromEditor {
			err = a.withRecovery(err, "entry.md", in.Body)
		}
		return err
	}
	if a.jsonOut {
		return a.writeJSON(logJSON{Entry: entryOf(e)})
	}
	p := a.out()
	where := ""
	switch {
	case !e.Topic.IsZero():
		where = " " + a.t("log.in", e.Topic.String())
	case e.Section != "":
		where = " " + a.t("log.in", e.Section)
	}
	p.println(a.t("log.added", e.ID, e.Date.String()) + where)
	return p.err
}

// readStdinText reads all of stdin, which must be UTF-8 text of at most
// limit bytes.
func (a *app) readStdinText(limit int) (string, error) {
	if a.env.Stdin == nil {
		return "", nil
	}
	data, err := io.ReadAll(io.LimitReader(a.env.Stdin, int64(limit)+1))
	if err != nil {
		return "", &exitError{code: ExitFailure, msg: a.t("err.stdin_read")}
	}
	if len(data) > limit {
		return "", a.usagef("err.entry_too_long", limit)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", a.usagef("err.edited_encoding")
	}
	return string(data), nil
}
