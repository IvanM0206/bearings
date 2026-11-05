package cmd

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

func (a *app) newSearch() *cobra.Command {
	var limit int
	c := &cobra.Command{
		Use:   "search <query>",
		Short: a.t("cmd.search.short"),
		Long:  a.t("cmd.search.long"),
		Args:  cobra.MinimumNArgs(1),
		RunE: func(_ *cobra.Command, args []string) error {
			return a.wrap(a.runSearch(strings.Join(args, " "), limit))
		},
	}
	c.Flags().IntVar(&limit, "limit", 0, a.t("flag.limit_hits"))
	return c
}

type hitJSON struct {
	Kind    string `json:"kind"`
	Section string `json:"section"`
	Topic   string `json:"topic"`
	EntryID string `json:"entry_id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Field   string `json:"field"`
	Snippet string `json:"snippet"`
}

type searchJSON struct {
	Query string    `json:"query"`
	Hits  []hitJSON `json:"hits"`
}

func (a *app) runSearch(query string, limit int) error {
	if limit < 0 {
		return a.usagef("err.bad_limit")
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	hits, err := nb.Search(query, notebook.SearchOptions{Limit: limit})
	if notebook.CodeOf(err) == notebook.CodeInvalidQuery {
		return a.usagef("err.bad_query", notebook.MaxQueryLen)
	}
	if err != nil {
		return err
	}
	if a.jsonOut {
		res := searchJSON{Query: strings.TrimSpace(query), Hits: make([]hitJSON, 0, len(hits))}
		for _, h := range hits {
			res.Hits = append(res.Hits, hitJSON{
				Kind: string(h.Kind), Section: h.Section, Topic: h.Topic.String(), EntryID: h.EntryID,
				Date: h.Date.String(), Title: h.Title, Field: h.Field, Snippet: h.Snippet,
			})
		}
		return a.writeJSON(res)
	}
	p := a.out()
	if len(hits) == 0 {
		p.println(a.t("search.none", output.Line(strings.TrimSpace(query))))
		return p.err
	}
	rows := make([][]string, 0, len(hits))
	for _, h := range hits {
		where := h.Section
		switch h.Kind {
		case notebook.HitTopic:
			where = h.Topic.String()
		case notebook.HitEntry:
			where = h.Date.String() + " " + h.EntryID
		}
		rows = append(rows, []string{a.t("search.in." + string(h.Kind) + "." + h.Field), where, h.Snippet})
	}
	if err := a.table([]string{a.t("col.found_in"), a.t("col.where"), a.t("col.match")}, rows); err != nil {
		return err
	}
	return p.err
}
