package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// API list limits.
const (
	defaultJournalLimit = 100
	maxJournalLimit     = 500
	maxJournalOffset    = notebook.MaxJournalEntries
	defaultSearchLimit  = 50
	maxSearchLimit      = 200
)

const prefix = "/api/v1"

// buildRoutes is the route table: the single place that lists the API.
// Every route that changes the notebook has write set (see addWrite), and
// the tests iterate over this table.
func (s *Server) buildRoutes() []route {
	var rs []route
	read := func(pattern string, h handlerFunc) {
		rs = append(rs, newRoute(http.MethodGet, prefix+pattern, false, h))
	}
	write := func(method, pattern string, h handlerFunc) {
		rs = append(rs, newRoute(method, prefix+pattern, true, h))
	}

	read("/info", (*Server).getInfo)
	read("/sections", (*Server).listSections)
	read("/sections/{slug}", (*Server).getSection)
	read("/sections/{slug}/topics", (*Server).listTopics)
	read("/outline", (*Server).getOutline)
	read("/topics/{section}/{topic}", (*Server).getTopic)
	read("/journal", (*Server).listJournal)
	read("/journal/{id}", (*Server).getEntry)
	read("/role-path", (*Server).getRolePath)
	read("/progress", (*Server).getProgress)
	read("/search", (*Server).search)

	write(http.MethodPost, "/sections", (*Server).addSection)
	write(http.MethodPatch, "/sections/{slug}", (*Server).updateSection)
	write(http.MethodDelete, "/sections/{slug}", (*Server).deleteSection)
	write(http.MethodPost, "/sections/{slug}/move", (*Server).moveSection)
	write(http.MethodPost, "/sections/{slug}/reorder", (*Server).reorderSection)
	write(http.MethodPost, "/sections/{slug}/topics", (*Server).addTopic)
	write(http.MethodPatch, "/topics/{section}/{topic}", (*Server).updateTopic)
	write(http.MethodDelete, "/topics/{section}/{topic}", (*Server).deleteTopic)
	write(http.MethodPost, "/topics/{section}/{topic}/move", (*Server).moveTopic)
	write(http.MethodPost, "/topics/{section}/{topic}/reorder", (*Server).reorderTopic)
	write(http.MethodPost, "/topics/{section}/{topic}/status", (*Server).setTopicStatus)
	write(http.MethodPost, "/topics/{section}/{topic}/advance", (*Server).advanceTopic)
	write(http.MethodPost, "/journal", (*Server).addEntry)
	write(http.MethodPatch, "/journal/{id}", (*Server).updateEntry)
	write(http.MethodDelete, "/journal/{id}", (*Server).deleteEntry)
	return rs
}

// decode reads the JSON body into dst: unknown fields and trailing data
// are errors. An empty body is an error unless optional is set.
func decode(r *http.Request, dst any, optional bool) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		if optional && errors.Is(err, io.EOF) {
			return nil
		}
		return decodeErr(err)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return decodeErr(err)
	}
	return nil
}

// ---- Parameters ------------------------------------------------------------

func slugParam(p params, name string) (string, error) {
	if !notebook.ValidSlug(p[name]) {
		return "", badRequest("invalid_slug", name)
	}
	return p[name], nil
}

func refParam(p params) (notebook.TopicRef, error) {
	if !notebook.ValidSlug(p["section"]) || !notebook.ValidSlug(p["topic"]) {
		return notebook.TopicRef{}, badRequest("invalid_reference", "topic")
	}
	return notebook.TopicRef{Section: p["section"], Topic: p["topic"]}, nil
}

func idParam(p params) (string, error) {
	if !notebook.ValidEntryID(p["id"]) {
		return "", badRequest("invalid_entry_id", "id")
	}
	return p["id"], nil
}

func intQuery(r *http.Request, name string, def, lo, hi int) (int, error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		return 0, badRequest("invalid_argument", name)
	}
	return n, nil
}

// ---- Reads -----------------------------------------------------------------

func (s *Server) getInfo(w http.ResponseWriter, r *http.Request, _ params) {
	cfg, err := s.nb.Config()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := infoDTO{Name: cfg.Name, Language: cfg.Language, Format: cfg.Format, ReadOnly: s.readOnly}
	if cfg.Template != nil {
		d.Template = &struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		}{cfg.Template.ID, cfg.Template.Title}
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) listSections(w http.ResponseWriter, r *http.Request, _ params) {
	outline, err := s.nb.Outline()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]sectionDTO, 0, len(outline))
	for _, st := range outline {
		out = append(out, sectionOf(st.Section, st.Topics, true))
	}
	writeJSON(w, http.StatusOK, map[string]any{"sections": out})
}

func (s *Server) getSection(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sec, err := s.nb.Section(slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	ts, err := s.nb.Topics(slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sectionOf(sec, ts, true))
}

func (s *Server) listTopics(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if _, err := s.nb.Section(slug); err != nil {
		s.fail(w, r, err)
		return
	}
	ts, err := s.nb.Topics(slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]topicSummaryDTO, 0, len(ts))
	for _, t := range ts {
		out = append(out, topicSummaryOf(t))
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": out})
}

func (s *Server) getOutline(w http.ResponseWriter, r *http.Request, _ params) {
	outline, err := s.nb.Outline()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]outlineDTO, 0, len(outline))
	for _, st := range outline {
		o := outlineDTO{Section: sectionOf(st.Section, st.Topics, true), Topics: make([]topicSummaryDTO, 0, len(st.Topics))}
		for _, t := range st.Topics {
			o.Topics = append(o.Topics, topicSummaryOf(t))
		}
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"outline": out})
}

// topicOut builds the full DTO of a topic, with its section's title.
func (s *Server) topicOut(t notebook.Topic) topicDTO {
	title := ""
	if sec, err := s.nb.Section(t.Section); err == nil {
		title = sec.Title
	}
	return topicOf(t, title)
}

func (s *Server) getTopic(w http.ResponseWriter, r *http.Request, p params) {
	ref, err := refParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.nb.Topic(ref)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.topicOut(t))
}

func (s *Server) listJournal(w http.ResponseWriter, r *http.Request, _ params) {
	q := r.URL.Query()
	var f notebook.JournalFilter
	var err error
	if v := q.Get("from"); v != "" {
		if f.From, err = notebook.ParseDate(v); err != nil {
			s.fail(w, r, badRequest("invalid_date", "from"))
			return
		}
	}
	if v := q.Get("to"); v != "" {
		if f.To, err = notebook.ParseDate(v); err != nil {
			s.fail(w, r, badRequest("invalid_date", "to"))
			return
		}
	}
	if v := q.Get("section"); v != "" {
		if !notebook.ValidSlug(v) {
			s.fail(w, r, badRequest("invalid_slug", "section"))
			return
		}
		f.Section = v
	}
	if v := q.Get("topic"); v != "" {
		if f.Topic, err = notebook.ParseTopicRef(v); err != nil {
			s.fail(w, r, badRequest("invalid_reference", "topic"))
			return
		}
	}
	limit, err := intQuery(r, "limit", defaultJournalLimit, 1, maxJournalLimit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	offset, err := intQuery(r, "offset", 0, 0, maxJournalOffset)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	f.Limit = offset + limit + 1 // one extra entry tells whether there is more
	es, err := s.nb.Journal(f)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	more := len(es) > offset+limit
	es = es[min(offset, len(es)):min(offset+limit, len(es))]
	out := make([]entryDTO, 0, len(es))
	for _, e := range es {
		out = append(out, entryOf(e))
	}
	writeJSON(w, http.StatusOK, map[string]any{"entries": out, "has_more": more})
}

func (s *Server) getEntry(w http.ResponseWriter, r *http.Request, p params) {
	id, err := idParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	e, err := s.nb.Entry(id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entryOf(e))
}

func (s *Server) getRolePath(w http.ResponseWriter, r *http.Request, _ params) {
	steps, err := s.nb.RolePath()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]roleStepDTO, 0, len(steps))
	for _, st := range steps {
		out = append(out, roleStepDTO{Step: st.Step, SectionTitle: st.SectionTitle, Topic: topicOf(st.Topic, st.SectionTitle)})
	}
	writeJSON(w, http.StatusOK, map[string]any{"steps": out})
}

func (s *Server) getProgress(w http.ResponseWriter, r *http.Request, _ params) {
	pr, err := s.nb.Progress()
	if err != nil {
		s.fail(w, r, err)
		return
	}
	d := progressDTO{
		All: countsOf(pr.All), Role: countsOf(pr.Role), Personal: countsOf(pr.Personal),
		Suggested: pr.Suggested, Entries: pr.Entries, Sections: make([]sectionProgressDTO, 0, len(pr.Sections)),
	}
	for _, sp := range pr.Sections {
		d.Sections = append(d.Sections, sectionProgressDTO{Slug: sp.Slug, Title: sp.Title, All: countsOf(sp.All), Role: countsOf(sp.Role)})
	}
	writeJSON(w, http.StatusOK, d)
}

func (s *Server) search(w http.ResponseWriter, r *http.Request, _ params) {
	limit, err := intQuery(r, "limit", defaultSearchLimit, 1, maxSearchLimit)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	hits, err := s.nb.Search(r.URL.Query().Get("q"), notebook.SearchOptions{Limit: limit})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	out := make([]hitDTO, 0, len(hits))
	for _, h := range hits {
		d := ""
		if !h.Date.IsZero() {
			d = h.Date.String()
		}
		out = append(out, hitDTO{
			Kind: string(h.Kind), Section: h.Section, Topic: h.Topic.String(), EntryID: h.EntryID,
			Date: d, Title: h.Title, Field: h.Field, Snippet: h.Snippet,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"hits": out})
}
