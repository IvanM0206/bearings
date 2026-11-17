package server

import (
	"net/http"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// Handlers of the write routes. Each one runs under Server.writeMu (see
// serveAPI), and updates carry the version the client read, so that a file
// changed meanwhile, by this server or by another program, is a 409.

type resourceIn struct {
	URL   string `json:"url"`
	Title string `json:"title"`
}

func resourcesOf(in []resourceIn) []notebook.Resource {
	out := make([]notebook.Resource, 0, len(in))
	for _, r := range in {
		out = append(out, notebook.Resource{URL: r.URL, Title: r.Title})
	}
	return out
}

// requireVersion parses the mandatory version of an update.
func requireVersion(s string) (notebook.Version, error) {
	if s == "" {
		return notebook.Version{}, badRequest("missing_version", "version")
	}
	return decodeVersion(s)
}

// ---- Sections --------------------------------------------------------------

func (s *Server) addSection(w http.ResponseWriter, r *http.Request, _ params) {
	var in struct {
		Title       string `json:"title"`
		Slug        string `json:"slug"`
		Order       *int   `json:"order"`
		Description string `json:"description"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	sec, err := s.nb.AddSection(notebook.NewSection{Slug: in.Slug, Title: in.Title, Order: in.Order, Description: in.Description})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, sectionOf(sec, nil, false))
}

func (s *Server) updateSection(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Version     string  `json:"version"`
		Title       *string `json:"title"`
		Description *string `json:"description"`
		Order       *int    `json:"order"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := requireVersion(in.Version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	sec, err := s.nb.Section(slug)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Title != nil {
		sec.Title = *in.Title
	}
	if in.Description != nil {
		sec.Description = *in.Description
	}
	if in.Order != nil {
		sec.Order = *in.Order
	}
	sec.Version = v // the version the client read: a newer file is a conflict
	out, err := s.nb.UpdateSection(sec)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sectionOf(out, nil, false))
}

func (s *Server) deleteSection(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.nb.DeleteSection(slug); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) moveSection(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		To string `json:"to"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if !notebook.ValidSlug(in.To) {
		s.fail(w, r, badRequest("invalid_slug", "to"))
		return
	}
	out, err := s.nb.MoveSection(slug, in.To)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, sectionOf(out, nil, false))
}

type reorderIn struct {
	Delta int `json:"delta"`
}

// delta checks that the move is exactly one place up or down.
func (in reorderIn) check() error {
	if in.Delta != -1 && in.Delta != 1 {
		return badRequest("invalid_argument", "delta")
	}
	return nil
}

func (s *Server) reorderSection(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in reorderIn
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := in.check(); err != nil {
		s.fail(w, r, err)
		return
	}
	moved, err := s.nb.ReorderSection(slug, in.Delta)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"moved": moved})
}

// ---- Topics ----------------------------------------------------------------

func (s *Server) addTopic(w http.ResponseWriter, r *http.Request, p params) {
	slug, err := slugParam(p, "slug")
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Title     string       `json:"title"`
		Slug      string       `json:"slug"`
		Status    string       `json:"status"`
		Type      string       `json:"type"`
		Suggested bool         `json:"suggested"`
		Order     *int         `json:"order"`
		Resources []resourceIn `json:"resources"`
		Notes     string       `json:"notes"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.nb.AddTopic(notebook.NewTopic{
		Section: slug, Slug: in.Slug, Title: in.Title,
		Status: notebook.Status(in.Status), Type: notebook.TopicType(in.Type), Suggested: in.Suggested,
		Order: in.Order, Resources: resourcesOf(in.Resources), Notes: in.Notes,
	})
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, s.topicOut(t))
}

func (s *Server) updateTopic(w http.ResponseWriter, r *http.Request, p params) {
	ref, err := refParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Version   string        `json:"version"`
		Title     *string       `json:"title"`
		Status    *string       `json:"status"`
		Type      *string       `json:"type"`
		Suggested *bool         `json:"suggested"`
		Order     *int          `json:"order"`
		Resources *[]resourceIn `json:"resources"`
		Notes     *string       `json:"notes"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := requireVersion(in.Version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	t, err := s.nb.Topic(ref)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Title != nil {
		t.Title = *in.Title
	}
	if in.Status != nil {
		t.Status = notebook.Status(*in.Status)
	}
	if in.Type != nil {
		t.Type = notebook.TopicType(*in.Type)
	}
	if in.Suggested != nil {
		t.Suggested = *in.Suggested
	}
	if in.Order != nil {
		t.Order = *in.Order
	}
	if in.Resources != nil {
		t.Resources = resourcesOf(*in.Resources)
	}
	if in.Notes != nil {
		t.Notes = *in.Notes
	}
	t.Version = v
	out, err := s.nb.UpdateTopic(t)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.topicOut(out))
}

func (s *Server) deleteTopic(w http.ResponseWriter, r *http.Request, p params) {
	ref, err := refParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.nb.DeleteTopic(ref); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) moveTopic(w http.ResponseWriter, r *http.Request, p params) {
	ref, err := refParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		To string `json:"to"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	to, err := notebook.ParseTopicRef(in.To)
	if err != nil {
		s.fail(w, r, badRequest("invalid_reference", "to"))
		return
	}
	out, err := s.nb.MoveTopic(ref, to)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.topicOut(out))
}

func (s *Server) reorderTopic(w http.ResponseWriter, r *http.Request, p params) {
	ref, err := refParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in reorderIn
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	if err := in.check(); err != nil {
		s.fail(w, r, err)
		return
	}
	moved, err := s.nb.ReorderTopic(ref, in.Delta)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"moved": moved})
}

// changeStatus sets the status of a topic. The version is optional here:
// the change is a single field, applied to the file as it is now.
func (s *Server) changeStatus(w http.ResponseWriter, r *http.Request, p params, status string, next bool, version string) {
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
	if version != "" {
		v, err := decodeVersion(version)
		if err != nil {
			s.fail(w, r, err)
			return
		}
		t.Version = v
	}
	if next {
		n, ok := t.Status.Next()
		if !ok {
			writeJSON(w, http.StatusOK, s.topicOut(t)) // already mastered: nothing to do
			return
		}
		t.Status = n
	} else {
		t.Status = notebook.Status(status)
		if !t.Status.Valid() {
			s.fail(w, r, &requestError{status: http.StatusUnprocessableEntity, code: "invalid_value", message: "A value is not valid.", field: "status"})
			return
		}
	}
	out, err := s.nb.UpdateTopic(t)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, s.topicOut(out))
}

func (s *Server) setTopicStatus(w http.ResponseWriter, r *http.Request, p params) {
	var in struct {
		Status  string `json:"status"`
		Version string `json:"version"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	s.changeStatus(w, r, p, in.Status, false, in.Version)
}

func (s *Server) advanceTopic(w http.ResponseWriter, r *http.Request, p params) {
	var in struct {
		Version string `json:"version"`
	}
	if err := decode(r, &in, true); err != nil {
		s.fail(w, r, err)
		return
	}
	s.changeStatus(w, r, p, "", true, in.Version)
}

// ---- Journal ---------------------------------------------------------------

func (s *Server) addEntry(w http.ResponseWriter, r *http.Request, _ params) {
	var in struct {
		Date    string `json:"date"`
		Section string `json:"section"`
		Topic   string `json:"topic"`
		Body    string `json:"body"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	ne := notebook.NewEntry{Body: in.Body}
	var err error
	if in.Date != "" {
		if ne.Date, err = notebook.ParseDate(in.Date); err != nil {
			s.fail(w, r, badRequest("invalid_date", "date"))
			return
		}
	}
	if in.Section != "" {
		if !notebook.ValidSlug(in.Section) {
			s.fail(w, r, badRequest("invalid_slug", "section"))
			return
		}
		ne.Section = in.Section
	}
	if in.Topic != "" {
		if ne.Topic, err = notebook.ParseTopicRef(in.Topic); err != nil {
			s.fail(w, r, badRequest("invalid_reference", "topic"))
			return
		}
	}
	e, err := s.nb.AddEntry(ne)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, entryOf(e))
}

func (s *Server) updateEntry(w http.ResponseWriter, r *http.Request, p params) {
	id, err := idParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	var in struct {
		Version string  `json:"version"`
		Body    *string `json:"body"`
		Section *string `json:"section"`
		Topic   *string `json:"topic"`
	}
	if err := decode(r, &in, false); err != nil {
		s.fail(w, r, err)
		return
	}
	v, err := requireVersion(in.Version)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	e, err := s.nb.Entry(id)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if in.Body != nil {
		e.Body = *in.Body
	}
	if in.Section != nil {
		if *in.Section != "" && !notebook.ValidSlug(*in.Section) {
			s.fail(w, r, badRequest("invalid_slug", "section"))
			return
		}
		e.Section = *in.Section
	}
	if in.Topic != nil {
		e.Topic = notebook.TopicRef{}
		if *in.Topic != "" {
			if e.Topic, err = notebook.ParseTopicRef(*in.Topic); err != nil {
				s.fail(w, r, badRequest("invalid_reference", "topic"))
				return
			}
		}
	}
	e.Version = v
	out, err := s.nb.UpdateEntry(e)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, entryOf(out))
}

func (s *Server) deleteEntry(w http.ResponseWriter, r *http.Request, p params) {
	id, err := idParam(p)
	if err != nil {
		s.fail(w, r, err)
		return
	}
	if err := s.nb.DeleteEntry(id); err != nil {
		s.fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
