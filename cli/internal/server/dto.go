package server

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// The types below are the JSON contract of the API. They are separate from
// the notebook types so that the wire format does not change by accident.
// Every string is notebook content (untrusted): clients must render it as
// text, never as HTML.

// encodeVersion turns a file version into an opaque string clients send
// back with updates.
func encodeVersion(v notebook.Version) string {
	return fmt.Sprintf("%d-%x", v.Size, v.Hash)
}

func decodeVersion(s string) (notebook.Version, error) {
	size, hash, ok := strings.Cut(s, "-")
	if !ok || len(hash) != 64 {
		return notebook.Version{}, badRequest("invalid_version", "version")
	}
	n, err := strconv.ParseInt(size, 10, 64)
	if err != nil || n < 0 {
		return notebook.Version{}, badRequest("invalid_version", "version")
	}
	raw, err := hex.DecodeString(hash)
	if err != nil {
		return notebook.Version{}, badRequest("invalid_version", "version")
	}
	v := notebook.Version{Size: n}
	copy(v.Hash[:], raw)
	return v, nil
}

type infoDTO struct {
	Name     string `json:"name"`
	Language string `json:"language"`
	Format   int    `json:"format"`
	ReadOnly bool   `json:"read_only"`
	Template *struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"template,omitempty"`
}

type countsDTO struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Mastered   int `json:"mastered"`
	Total      int `json:"total"`
}

func countsOf(c notebook.Counts) countsDTO {
	return countsDTO{Pending: c.Pending, InProgress: c.InProgress, Mastered: c.Mastered, Total: c.Total}
}

func countTopics(ts []notebook.Topic) countsDTO {
	var c countsDTO
	for _, t := range ts {
		c.Total++
		switch t.Status {
		case notebook.StatusPending:
			c.Pending++
		case notebook.StatusInProgress:
			c.InProgress++
		case notebook.StatusMastered:
			c.Mastered++
		}
	}
	return c
}

type sectionDTO struct {
	Slug        string     `json:"slug"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	Order       int        `json:"order"`
	Created     string     `json:"created"`
	Updated     string     `json:"updated"`
	Version     string     `json:"version"`
	Counts      *countsDTO `json:"counts,omitempty"`
}

func sectionOf(s notebook.Section, topics []notebook.Topic, withCounts bool) sectionDTO {
	d := sectionDTO{
		Slug: s.Slug, Title: s.Title, Description: s.Description, Order: s.Order,
		Created: notebook.FormatTimestamp(s.Created), Updated: notebook.FormatTimestamp(s.Updated),
		Version: encodeVersion(s.Version),
	}
	if withCounts {
		c := countTopics(topics)
		d.Counts = &c
	}
	return d
}

type resourceDTO struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

type topicSummaryDTO struct {
	Ref       string `json:"ref"`
	Section   string `json:"section"`
	Slug      string `json:"slug"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	Suggested bool   `json:"suggested"`
	Order     int    `json:"order"`
	Resources int    `json:"resources"`
	Updated   string `json:"updated"`
}

func topicSummaryOf(t notebook.Topic) topicSummaryDTO {
	return topicSummaryDTO{
		Ref: t.Ref().String(), Section: t.Section, Slug: t.Slug, Title: t.Title,
		Status: string(t.Status), Type: string(t.Type), Suggested: t.Suggested, Order: t.Order,
		Resources: len(t.Resources), Updated: notebook.FormatTimestamp(t.Updated),
	}
}

type topicDTO struct {
	Ref          string        `json:"ref"`
	Section      string        `json:"section"`
	SectionTitle string        `json:"section_title"`
	Slug         string        `json:"slug"`
	Title        string        `json:"title"`
	Status       string        `json:"status"`
	Type         string        `json:"type"`
	Suggested    bool          `json:"suggested"`
	Order        int           `json:"order"`
	Resources    []resourceDTO `json:"resources"`
	Created      string        `json:"created"`
	Updated      string        `json:"updated"`
	Notes        string        `json:"notes"`
	Version      string        `json:"version"`
}

func topicOf(t notebook.Topic, sectionTitle string) topicDTO {
	d := topicDTO{
		Ref: t.Ref().String(), Section: t.Section, SectionTitle: sectionTitle, Slug: t.Slug, Title: t.Title,
		Status: string(t.Status), Type: string(t.Type), Suggested: t.Suggested, Order: t.Order,
		Resources: make([]resourceDTO, 0, len(t.Resources)),
		Created:   notebook.FormatTimestamp(t.Created), Updated: notebook.FormatTimestamp(t.Updated),
		Notes: t.Notes, Version: encodeVersion(t.Version),
	}
	for _, r := range t.Resources {
		d.Resources = append(d.Resources, resourceDTO{URL: r.URL, Title: r.Title})
	}
	return d
}

type outlineDTO struct {
	Section sectionDTO        `json:"section"`
	Topics  []topicSummaryDTO `json:"topics"`
}

type entryDTO struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Section string `json:"section"`
	Topic   string `json:"topic"`
	Created string `json:"created"`
	Updated string `json:"updated"`
	Body    string `json:"body"`
	Version string `json:"version"`
}

func entryOf(e notebook.Entry) entryDTO {
	return entryDTO{
		ID: e.ID, Date: e.Date.String(), Section: e.Section, Topic: e.Topic.String(),
		Created: notebook.FormatTimestamp(e.Created), Updated: notebook.FormatTimestamp(e.Updated),
		Body: e.Body, Version: encodeVersion(e.Version),
	}
}

type roleStepDTO struct {
	Step         int      `json:"step"`
	SectionTitle string   `json:"section_title"`
	Topic        topicDTO `json:"topic"`
}

type sectionProgressDTO struct {
	Slug  string    `json:"slug"`
	Title string    `json:"title"`
	All   countsDTO `json:"all"`
	Role  countsDTO `json:"role"`
}

type progressDTO struct {
	All       countsDTO            `json:"all"`
	Role      countsDTO            `json:"role"`
	Personal  countsDTO            `json:"personal"`
	Suggested int                  `json:"suggested"`
	Entries   int                  `json:"journal_entries"`
	Sections  []sectionProgressDTO `json:"sections"`
}

type hitDTO struct {
	Kind    string `json:"kind"`
	Section string `json:"section"`
	Topic   string `json:"topic"`
	EntryID string `json:"entry_id"`
	Date    string `json:"date"`
	Title   string `json:"title"`
	Field   string `json:"field"`
	Snippet string `json:"snippet"`
}
