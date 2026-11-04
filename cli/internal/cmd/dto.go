package cmd

import (
	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// The types below define the JSON output of the commands. They are separate
// from the notebook package's types on purpose: the shape of --json output
// is part of the CLI's contract (see docs/json-output.md) and must not
// change by accident when the storage types do.

type countsJSON struct {
	Total      int `json:"total"`
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Mastered   int `json:"mastered"`
}

func countsOf(topics []notebook.Topic) countsJSON {
	var c countsJSON
	for _, t := range topics {
		c.add(t.Status)
	}
	return c
}

func (c *countsJSON) add(s notebook.Status) {
	c.Total++
	switch s {
	case notebook.StatusPending:
		c.Pending++
	case notebook.StatusInProgress:
		c.InProgress++
	case notebook.StatusMastered:
		c.Mastered++
	}
}

func fromCounts(c notebook.Counts) countsJSON {
	return countsJSON{Total: c.Total, Pending: c.Pending, InProgress: c.InProgress, Mastered: c.Mastered}
}

type sectionJSON struct {
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Order       int        `json:"order"`
	Counts      countsJSON `json:"counts"`
}

func sectionOf(s notebook.Section, topics []notebook.Topic) sectionJSON {
	return sectionJSON{Slug: s.Slug, Title: s.Title, Description: s.Description, Order: s.Order, Counts: countsOf(topics)}
}

type topicSummaryJSON struct {
	Ref       string `json:"ref"`
	Section   string `json:"section"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	Suggested bool   `json:"suggested"`
	Order     int    `json:"order"`
}

func topicSummary(t notebook.Topic) topicSummaryJSON {
	return topicSummaryJSON{
		Ref: t.Ref().String(), Section: t.Section, Slug: t.Slug, Title: t.Title,
		Status: string(t.Status), Type: string(t.Type), Suggested: t.Suggested, Order: t.Order,
	}
}

type resourceJSON struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type topicJSON struct {
	topicSummaryJSON
	SectionTitle string         `json:"section_title"`
	Resources    []resourceJSON `json:"resources"`
	Created      string         `json:"created"`
	Updated      string         `json:"updated"`
	Notes        string         `json:"notes"`
}

type entryJSON struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Section string `json:"section"`
	Topic   string `json:"topic"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Body    string `json:"body"`
}

func entryOf(e notebook.Entry) entryJSON {
	return entryJSON{
		ID: e.ID, Date: e.Date.String(), Section: e.Section, Topic: e.Topic.String(),
		Created: notebook.FormatTimestamp(e.Created), Updated: notebook.FormatTimestamp(e.Updated), Body: e.Body,
	}
}

func entriesOf(es []notebook.Entry) []entryJSON {
	out := make([]entryJSON, 0, len(es))
	for _, e := range es {
		out = append(out, entryOf(e))
	}
	return out
}
