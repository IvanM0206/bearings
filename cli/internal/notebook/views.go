package notebook

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// ---- Role path -------------------------------------------------------------

// RoleStep is one step of the role path (SPEC §1, §6).
type RoleStep struct {
	Step         int    `json:"step"` // 1…n across the whole path
	SectionTitle string `json:"section_title"`
	Topic        Topic  `json:"topic"`
}

// RolePath returns every topic of type role: sections in section order,
// topics in topic order, numbered from 1.
func (nb *Notebook) RolePath() ([]RoleStep, error) {
	outline, err := nb.Outline()
	if err != nil {
		return nil, err
	}
	var out []RoleStep
	for _, st := range outline {
		for _, t := range st.Topics {
			if t.Type == TypeRole {
				out = append(out, RoleStep{Step: len(out) + 1, SectionTitle: st.Section.Title, Topic: t})
			}
		}
	}
	return out, nil
}

// ---- Progress --------------------------------------------------------------

// Counts counts topics by status.
type Counts struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Mastered   int `json:"mastered"`
	Total      int `json:"total"`
}

func (c *Counts) add(s Status) {
	c.Total++
	switch s {
	case StatusPending:
		c.Pending++
	case StatusInProgress:
		c.InProgress++
	case StatusMastered:
		c.Mastered++
	}
}

// SectionProgress is the progress of one section.
type SectionProgress struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	All   Counts `json:"all"`
	Role  Counts `json:"role"`
}

// Progress summarizes the notebook.
type Progress struct {
	All       Counts            `json:"all"`
	Role      Counts            `json:"role"`
	Personal  Counts            `json:"personal"`
	Suggested int               `json:"suggested"` // topics not reviewed yet
	Entries   int               `json:"journal_entries"`
	Sections  []SectionProgress `json:"sections"`
}

// Progress counts topics by status, type and section, and journal entries.
func (nb *Notebook) Progress() (Progress, error) {
	outline, err := nb.Outline()
	if err != nil {
		return Progress{}, err
	}
	p := Progress{Sections: []SectionProgress{}}
	for _, st := range outline {
		sp := SectionProgress{Slug: st.Section.Slug, Title: st.Section.Title}
		for _, t := range st.Topics {
			sp.All.add(t.Status)
			p.All.add(t.Status)
			if t.Type == TypeRole {
				sp.Role.add(t.Status)
				p.Role.add(t.Status)
			} else {
				p.Personal.add(t.Status)
			}
			if t.Suggested {
				p.Suggested++
			}
		}
		p.Sections = append(p.Sections, sp)
	}
	locs, err := nb.entryLocs()
	if err != nil {
		return Progress{}, err
	}
	p.Entries = len(locs)
	return p, nil
}

// ---- Search ----------------------------------------------------------------

// MaxQueryLen is the maximum length of a search query, in characters.
const MaxQueryLen = 200

// HitKind is what a search hit points to.
type HitKind string

// Hit kinds.
const (
	HitSection HitKind = "section"
	HitTopic   HitKind = "topic"
	HitEntry   HitKind = "entry"
)

// SearchHit is one search result. Snippet is a single line of the matching
// text around the match (untrusted content, like everything else).
type SearchHit struct {
	Kind    HitKind  `json:"kind"`
	Section string   `json:"section,omitempty"`
	Topic   TopicRef `json:"topic"`
	EntryID string   `json:"entry_id,omitempty"`
	Date    Date     `json:"date"`
	Title   string   `json:"title,omitempty"`
	Field   string   `json:"field"` // "title" or "body"
	Snippet string   `json:"snippet"`
}

// SearchOptions configures [Notebook.Search].
type SearchOptions struct {
	Limit int // maximum number of hits (0 = no limit)
}

// Search finds sections, topics and journal entries whose title or body
// contains query, ignoring case and accents. Results come in notebook
// order: sections and their topics, then the journal.
func (nb *Notebook) Search(query string, opts SearchOptions) ([]SearchHit, error) {
	q := strings.TrimSpace(query)
	if q == "" || !utf8.ValidString(q) || utf8.RuneCountInString(q) > MaxQueryLen || hasControl(q) {
		return nil, newErr(CodeInvalidQuery, "", "query")
	}
	needle, _ := fold(q)
	var hits []SearchHit
	full := func() bool { return opts.Limit > 0 && len(hits) >= opts.Limit }
	add := func(h SearchHit, title, body string) {
		if s, ok := match(needle, title); ok {
			h.Field, h.Snippet = "title", s
		} else if s, ok := match(needle, body); ok {
			h.Field, h.Snippet = "body", s
		} else {
			return
		}
		hits = append(hits, h)
	}
	outline, err := nb.Outline()
	if err != nil {
		return nil, err
	}
	for _, st := range outline {
		s := st.Section
		add(SearchHit{Kind: HitSection, Section: s.Slug, Title: s.Title}, s.Title, s.Description)
		for _, t := range st.Topics {
			if full() {
				return hits, nil
			}
			add(SearchHit{Kind: HitTopic, Section: t.Section, Topic: t.Ref(), Title: t.Title}, t.Title, t.Notes)
		}
	}
	entries, err := nb.Journal(JournalFilter{})
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if full() {
			break
		}
		add(SearchHit{Kind: HitEntry, Section: e.Section, Topic: e.Topic, EntryID: e.ID, Date: e.Date}, "", e.Body)
	}
	if opts.Limit > 0 && len(hits) > opts.Limit {
		hits = hits[:opts.Limit]
	}
	return hits, nil
}

// fold lowercases and strips accents from s. It returns the folded runes
// and, for each of them, the index of the rune of s it came from.
func fold(s string) ([]rune, []int) {
	out := make([]rune, 0, len(s))
	idx := make([]int, 0, len(s))
	i := 0
	for _, r := range s {
		if t, ok := translit[r]; ok {
			for _, c := range t {
				out = append(out, c)
				idx = append(idx, i)
			}
		} else {
			out = append(out, unicode.ToLower(r))
			idx = append(idx, i)
		}
		i++
	}
	return out, idx
}

const snippetContext = 40

// match looks for needle in text and returns a one-line snippet around the
// first match.
func match(needle []rune, text string) (string, bool) {
	if text == "" {
		return "", false
	}
	hay, idx := fold(text)
	hayStr := string(hay)
	off := strings.Index(hayStr, string(needle))
	if off < 0 {
		return "", false
	}
	pos := utf8.RuneCountInString(hayStr[:off])
	orig := []rune(text)
	start := idx[pos]
	end := idx[pos+len(needle)-1] + 1
	from, to := max(start-snippetContext, 0), min(end+snippetContext, len(orig))
	snip := strings.Join(strings.Fields(string(orig[from:to])), " ")
	if from > 0 {
		snip = "…" + snip
	}
	if to < len(orig) {
		snip += "…"
	}
	return snip, true
}
