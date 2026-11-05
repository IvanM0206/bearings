package notebook

import (
	"strings"
	"time"
)

// Every write in this file follows the same steps: validate the input,
// encode it, check the encoding parses back (done by the Marshal
// functions), then write it atomically with conflict detection.

// ---- Sections --------------------------------------------------------------

// NewSection describes a section to create.
type NewSection struct {
	Slug        string // optional; derived from Title when empty
	Title       string
	Order       *int // optional; after the last section when nil
	Description string
}

// AddSection creates a section.
func (nb *Notebook) AddSection(in NewSection) (Section, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	title, err := checkText("title", in.Title, MaxSectionTitleLen, true)
	if err != nil {
		return Section{}, err
	}
	slugs, err := nb.sectionSlugs()
	if err != nil {
		return Section{}, err
	}
	if len(slugs) >= MaxSections {
		return Section{}, limitErr(LimitSections, SectionsDir, MaxSections)
	}
	slug, err := nb.pickSlug(in.Slug, title, "section", func(s string) (bool, error) {
		return nb.fs.exists(sectionDirRel(s))
	})
	if err != nil {
		return Section{}, err
	}
	order := 0
	if in.Order != nil {
		order = *in.Order
	} else {
		secs, err := nb.Sections()
		if err != nil {
			return Section{}, err
		}
		for _, s := range secs {
			order = max(order, s.Order+1)
		}
		order = min(order, MaxOrder)
	}
	now := nb.now()
	s := Section{Slug: slug, Title: title, Order: order, Created: now, Updated: now, Description: in.Description}
	return nb.writeSection(s, nil)
}

// pickSlug validates an explicit slug (which must be free) or derives a
// free one from the title.
func (nb *Notebook) pickSlug(explicit, title, fallback string, taken func(string) (bool, error)) (string, error) {
	if explicit != "" {
		if !ValidSlug(explicit) {
			return "", newErr(CodeInvalidSlug, "", "slug")
		}
		used, err := taken(explicit)
		if err != nil {
			return "", err
		}
		if used {
			return "", newErr(CodeExists, "", "slug")
		}
		return explicit, nil
	}
	var terr error
	slug := UniqueSlug(Slugify(title), fallback, func(s string) bool {
		used, err := taken(s)
		if err != nil {
			terr = err
			return false
		}
		return used
	})
	return slug, terr
}

func (nb *Notebook) writeSection(s Section, expect *Version) (Section, error) {
	rel := sectionRel(s.Slug)
	data, err := MarshalSection(s)
	if err != nil {
		return Section{}, withPath(err, rel)
	}
	if err := nb.fs.writeFile(rel, data, expect); err != nil {
		return Section{}, err
	}
	out, err := ParseSection(s.Slug, data)
	if err != nil {
		return Section{}, withPath(err, rel)
	}
	return out, nil
}

// UpdateSection writes the title, order and description of s, which must
// have been read from this notebook (its Version is used for conflict
// detection). `updated` is set to now.
func (nb *Notebook) UpdateSection(s Section) (Section, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !ValidSlug(s.Slug) {
		return Section{}, newErr(CodeInvalidSlug, "", "slug")
	}
	if s.Version.IsZero() {
		return Section{}, newErr(CodeConflict, sectionRel(s.Slug), "")
	}
	s.Updated = nb.touch(s.Created)
	v := s.Version
	return nb.writeSection(s, &v)
}

// touch returns now, or created if the clock is behind it, so that
// updated ≥ created always holds.
func (nb *Notebook) touch(created time.Time) time.Time {
	now := nb.now()
	if now.Before(created) {
		return created
	}
	return now
}

// MoveSection changes the slug of a section (renaming its directory) and
// updates journal references to it.
func (nb *Notebook) MoveSection(from, to string) (Section, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !ValidSlug(from) || !ValidSlug(to) {
		return Section{}, newErr(CodeInvalidSlug, "", "slug")
	}
	if from == to {
		return nb.loadSection(from)
	}
	if _, err := nb.loadSection(from); err != nil {
		return Section{}, err
	}
	if err := nb.fs.renameDir(sectionDirRel(from), sectionDirRel(to)); err != nil {
		return Section{}, err
	}
	s, err := nb.loadSection(to)
	if err != nil {
		return Section{}, err
	}
	err = nb.rewriteEntries(func(e *Entry) bool {
		if e.Section != from {
			return false
		}
		e.Section = to
		if !e.Topic.IsZero() {
			e.Topic.Section = to
		}
		return true
	})
	return s, err
}

// DeleteSection deletes a section with all its topics and clears journal
// references to it.
func (nb *Notebook) DeleteSection(slug string) error {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	s, err := nb.loadSection(slug)
	if err != nil {
		return err
	}
	names, err := nb.topicFileNames(slug)
	if err != nil {
		return err
	}
	// Read everything first, so that a file that cannot be read stops the
	// operation before anything is deleted.
	type victim struct {
		rel string
		v   Version
	}
	victims := make([]victim, 0, len(names))
	for _, name := range names {
		rel := topicRel(TopicRef{slug, name})
		data, err := nb.fs.readFile(rel)
		if err != nil {
			return err
		}
		victims = append(victims, victim{rel, versionOf(data)})
	}
	for _, v := range victims {
		if err := nb.fs.removeFile(v.rel, v.v); err != nil {
			return err
		}
	}
	if err := nb.fs.removeFile(sectionRel(slug), s.Version); err != nil {
		return err
	}
	nb.fs.removeEmptyDir(sectionDirRel(slug))
	return nb.rewriteEntries(func(e *Entry) bool {
		if e.Section != slug {
			return false
		}
		e.Section, e.Topic = "", TopicRef{}
		return true
	})
}

// ---- Topics ----------------------------------------------------------------

// NewTopic describes a topic to create.
type NewTopic struct {
	Section   string
	Slug      string // optional; derived from Title when empty
	Title     string
	Status    Status    // default pending
	Type      TopicType // default personal
	Suggested bool
	Order     *int // optional; after the last topic of the section when nil
	Resources []Resource
	Notes     string
}

// AddTopic creates a topic in an existing section.
func (nb *Notebook) AddTopic(in NewTopic) (Topic, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if _, err := nb.loadSection(in.Section); err != nil {
		return Topic{}, err
	}
	title, err := checkText("title", in.Title, MaxTopicTitleLen, true)
	if err != nil {
		return Topic{}, err
	}
	existing, err := nb.topicsOf(in.Section)
	if err != nil {
		return Topic{}, err
	}
	names, err := nb.topicFileNames(in.Section)
	if err != nil {
		return Topic{}, err
	}
	if len(names) >= MaxTopicsPerSection {
		return Topic{}, limitErr(LimitTopicsPerSection, sectionDirRel(in.Section), MaxTopicsPerSection)
	}
	slug, err := nb.pickSlug(in.Slug, title, "topic", func(s string) (bool, error) {
		return nb.fs.exists(topicRel(TopicRef{in.Section, s}))
	})
	if err != nil {
		return Topic{}, err
	}
	t := Topic{
		Section: in.Section, Slug: slug, Title: title,
		Status: in.Status, Type: in.Type, Suggested: in.Suggested,
		Resources: in.Resources, Notes: in.Notes,
	}
	if t.Status == "" {
		t.Status = StatusPending
	}
	if t.Type == "" {
		t.Type = TypePersonal
	}
	if in.Order != nil {
		t.Order = *in.Order
	} else {
		for _, x := range existing {
			t.Order = max(t.Order, x.Order+1)
		}
		t.Order = min(t.Order, MaxOrder)
	}
	if err := checkTopicFields(t); err != nil {
		return Topic{}, err
	}
	now := nb.now()
	t.Created, t.Updated = now, now
	return nb.writeTopic(t, nil)
}

// checkTopicFields gives a precise error for invalid fields before
// encoding (the encoder would catch them too).
func checkTopicFields(t Topic) error {
	if !t.Status.Valid() {
		return newErr(CodeInvalidValue, "", "status")
	}
	if !t.Type.Valid() {
		return newErr(CodeInvalidValue, "", "type")
	}
	if t.Order < 0 || t.Order > MaxOrder {
		return &Error{Code: CodeOutOfRange, Field: "order", Max: MaxOrder}
	}
	if len(t.Resources) > MaxResources {
		return &Error{Code: CodeTooLong, Field: "resources", Max: MaxResources}
	}
	for _, r := range t.Resources {
		if err := CheckURL(r.URL); err != nil {
			return err
		}
		if r.Title != "" {
			if _, err := checkText("resources.title", r.Title, MaxResourceTitleLen, true); err != nil {
				return err
			}
		}
	}
	return checkTopicBody(normalizeBody(t.Notes))
}

func (nb *Notebook) writeTopic(t Topic, expect *Version) (Topic, error) {
	rel := topicRel(t.Ref())
	data, err := MarshalTopic(t)
	if err != nil {
		return Topic{}, withPath(err, rel)
	}
	if err := nb.fs.writeFile(rel, data, expect); err != nil {
		return Topic{}, err
	}
	out, err := ParseTopic(t.Section, t.Slug, data)
	if err != nil {
		return Topic{}, withPath(err, rel)
	}
	return out, nil
}

// UpdateTopic writes t, which must have been read from this notebook (its
// Version is used for conflict detection). `updated` is set to now.
func (nb *Notebook) UpdateTopic(t Topic) (Topic, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !t.Ref().valid() {
		return Topic{}, newErr(CodeInvalidRef, "", "")
	}
	if t.Version.IsZero() {
		return Topic{}, newErr(CodeConflict, topicRel(t.Ref()), "")
	}
	if _, err := nb.loadSection(t.Section); err != nil {
		return Topic{}, err
	}
	if err := checkTopicFields(t); err != nil {
		return Topic{}, err
	}
	t.Title = strings.TrimSpace(t.Title)
	t.Updated = nb.touch(t.Created)
	v := t.Version
	return nb.writeTopic(t, &v)
}

// MoveTopic moves a topic to another reference (another section and/or
// slug) and updates journal references to it.
func (nb *Notebook) MoveTopic(from, to TopicRef) (Topic, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !from.valid() || !to.valid() {
		return Topic{}, newErr(CodeInvalidRef, "", "")
	}
	if from == to {
		return nb.loadTopic(from)
	}
	if _, err := nb.loadSection(from.Section); err != nil {
		return Topic{}, err
	}
	if _, err := nb.loadSection(to.Section); err != nil {
		return Topic{}, err
	}
	t, err := nb.loadTopic(from)
	if err != nil {
		return Topic{}, err
	}
	if to.Section != from.Section {
		names, err := nb.topicFileNames(to.Section)
		if err != nil {
			return Topic{}, err
		}
		if len(names) >= MaxTopicsPerSection {
			return Topic{}, limitErr(LimitTopicsPerSection, sectionDirRel(to.Section), MaxTopicsPerSection)
		}
	}
	old := t
	if to.Section != from.Section {
		// A topic that changes section goes last in its new one.
		existing, err := nb.topicsOf(to.Section)
		if err != nil {
			return Topic{}, err
		}
		t.Order = 0
		for _, x := range existing {
			t.Order = max(t.Order, x.Order+1)
		}
		t.Order = min(t.Order, MaxOrder)
	}
	t.Section, t.Slug = to.Section, to.Topic
	t.Updated = nb.touch(t.Created)
	moved, err := nb.writeTopic(t, nil)
	if err != nil {
		return Topic{}, err
	}
	if err := nb.fs.removeFile(topicRel(from), old.Version); err != nil {
		// Undo the copy so the topic does not exist twice.
		_ = nb.fs.removeFile(topicRel(to), moved.Version)
		return Topic{}, err
	}
	err = nb.rewriteEntries(func(e *Entry) bool {
		if e.Topic != from {
			return false
		}
		e.Section, e.Topic = to.Section, to
		return true
	})
	return moved, err
}

// DeleteTopic deletes a topic and clears journal references to it (the
// entries stay linked to the section).
func (nb *Notebook) DeleteTopic(ref TopicRef) error {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	if !ref.valid() {
		return newErr(CodeInvalidRef, "", "")
	}
	rel := topicRel(ref)
	data, err := nb.fs.readFile(rel)
	if err != nil {
		return err
	}
	if err := nb.fs.removeFile(rel, versionOf(data)); err != nil {
		return err
	}
	return nb.rewriteEntries(func(e *Entry) bool {
		if e.Topic != ref {
			return false
		}
		e.Topic = TopicRef{}
		return true
	})
}

// ---- Journal ---------------------------------------------------------------

// NewEntry describes a journal entry to create.
type NewEntry struct {
	Date    Date     // default: today in the local time zone
	Section string   // optional
	Topic   TopicRef // optional; sets Section when Section is empty
	Body    string
}

// AddEntry creates a journal entry. Referenced sections and topics must
// exist.
func (nb *Notebook) AddEntry(in NewEntry) (Entry, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	e := Entry{Date: in.Date, Section: in.Section, Topic: in.Topic, Body: normalizeBody(in.Body)}
	if e.Date.IsZero() {
		e.Date = DateOf(nb.opts.Now())
	}
	if _, err := ParseDate(e.Date.String()); err != nil {
		return Entry{}, err
	}
	if e.Section == "" && !e.Topic.IsZero() {
		e.Section = e.Topic.Section
	}
	if err := nb.checkEntryRefs(e); err != nil {
		return Entry{}, err
	}
	if err := checkEntryBody(e.Body); err != nil {
		return Entry{}, err
	}
	locs, err := nb.entryLocs()
	if err != nil {
		return Entry{}, err
	}
	if len(locs) >= MaxJournalEntries {
		return Entry{}, limitErr(LimitJournalEntries, JournalDir, MaxJournalEntries)
	}
	now := nb.now()
	e.Created, e.Updated = now, now
	for attempt := 0; ; attempt++ {
		if e.ID, err = NewEntryID(nb.opts.Rand); err != nil {
			return Entry{}, err
		}
		out, err := nb.writeEntry(e, nil)
		if CodeOf(err) == CodeExists && attempt < 5 {
			continue
		}
		return out, err
	}
}

func (nb *Notebook) checkEntryRefs(e Entry) error {
	if e.Section != "" {
		if _, err := nb.loadSection(e.Section); err != nil {
			return err
		}
	}
	if !e.Topic.IsZero() {
		if !e.Topic.valid() {
			return newErr(CodeInvalidRef, "", "topic")
		}
		if e.Topic.Section != e.Section {
			return newErr(CodeTopicSectionMismatch, "", "topic")
		}
		if _, err := nb.loadTopic(e.Topic); err != nil {
			return err
		}
	}
	return nil
}

func (nb *Notebook) writeEntry(e Entry, expect *Version) (Entry, error) {
	rel := entryRel(e.Date, e.ID)
	data, err := MarshalEntry(e)
	if err != nil {
		return Entry{}, withPath(err, rel)
	}
	if err := nb.fs.writeFile(rel, data, expect); err != nil {
		return Entry{}, err
	}
	out, err := ParseEntry(e.Date, e.ID, data)
	if err != nil {
		return Entry{}, withPath(err, rel)
	}
	return out, nil
}

// UpdateEntry writes the section, topic and body of e, which must have been
// read from this notebook. The date and id cannot change (they are the
// file's path).
func (nb *Notebook) UpdateEntry(e Entry) (Entry, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	l, err := nb.findEntry(e.ID)
	if err != nil {
		return Entry{}, err
	}
	if l.date != e.Date {
		return Entry{}, newErr(CodePathMismatch, l.rel(), "date")
	}
	if e.Version.IsZero() {
		return Entry{}, newErr(CodeConflict, l.rel(), "")
	}
	if e.Section == "" && !e.Topic.IsZero() {
		e.Section = e.Topic.Section
	}
	if err := nb.checkEntryRefs(e); err != nil {
		return Entry{}, err
	}
	e.Body = normalizeBody(e.Body)
	if err := checkEntryBody(e.Body); err != nil {
		return Entry{}, err
	}
	e.Updated = nb.touch(e.Created)
	v := e.Version
	return nb.writeEntry(e, &v)
}

// DeleteEntry deletes a journal entry.
func (nb *Notebook) DeleteEntry(id string) error {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	l, err := nb.findEntry(id)
	if err != nil {
		return err
	}
	data, err := nb.fs.readFile(l.rel())
	if err != nil {
		return err
	}
	if err := nb.fs.removeFile(l.rel(), versionOf(data)); err != nil {
		return err
	}
	nb.fs.removeEmptyDir(entryDirRel(l.date))
	nb.fs.removeEmptyDir(JournalDir + "/" + l.date.String()[:4])
	return nil
}

// rewriteEntries applies fn to every valid journal entry (with references
// as stored, not unlinked) and writes back those for which fn returns true.
// It goes through every entry even if one fails and returns the first
// error.
func (nb *Notebook) rewriteEntries(fn func(*Entry) bool) error {
	locs, err := nb.entryLocs()
	if err != nil {
		return err
	}
	var first error
	for _, l := range locs {
		e, err := nb.loadEntry(l)
		if err != nil {
			continue // invalid entries are left alone
		}
		if !fn(&e) {
			continue
		}
		e.Updated = nb.touch(e.Created)
		v := e.Version
		if _, err := nb.writeEntry(e, &v); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// ---- Editing raw files -----------------------------------------------------

// Source is the raw content of a notebook file with its version, for
// editing in an external editor.
type Source struct {
	Path    string // relative path
	Data    []byte
	Version Version
}

func (nb *Notebook) source(rel string) (Source, error) {
	data, err := nb.fs.readFile(rel)
	if err != nil {
		return Source{}, err
	}
	return Source{Path: rel, Data: data, Version: versionOf(data)}, nil
}

// SectionSource returns the raw _section.md of a section.
func (nb *Notebook) SectionSource(slug string) (Source, error) {
	if !ValidSlug(slug) {
		return Source{}, newErr(CodeInvalidSlug, "", "")
	}
	return nb.source(sectionRel(slug))
}

// TopicSource returns the raw file of a topic.
func (nb *Notebook) TopicSource(ref TopicRef) (Source, error) {
	if !ref.valid() {
		return Source{}, newErr(CodeInvalidRef, "", "")
	}
	if _, err := nb.loadSection(ref.Section); err != nil {
		return Source{}, err
	}
	return nb.source(topicRel(ref))
}

// EntrySource returns the raw file of a journal entry.
func (nb *Notebook) EntrySource(id string) (Source, error) {
	l, err := nb.findEntry(id)
	if err != nil {
		return Source{}, err
	}
	return nb.source(l.rel())
}

// ReplaceSource validates edited content for the file src.Path and writes
// it, normalized, if the file still has the content identified by
// src.Version. The edited content cannot change the file's identity (slug,
// date, id). If the content changed, `updated` is set to now. It returns
// the parsed value: a Section, Topic or Entry.
func (nb *Notebook) ReplaceSource(src Source, edited []byte) (any, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	info, err := ClassifyPath(src.Path)
	if err != nil {
		return nil, err
	}
	changed := versionOf(edited) != src.Version
	switch info.Kind {
	case PathSection:
		s, err := ParseSection(info.Section, edited)
		if err != nil {
			return nil, withPath(err, src.Path)
		}
		if !changed {
			return s, nil
		}
		s.Updated = nb.touch(s.Created)
		return nb.writeSection(s, &src.Version)
	case PathTopic:
		t, err := ParseTopic(info.Section, info.Topic, edited)
		if err != nil {
			return nil, withPath(err, src.Path)
		}
		if !changed {
			return t, nil
		}
		t.Updated = nb.touch(t.Created)
		return nb.writeTopic(t, &src.Version)
	case PathEntry:
		e, err := ParseEntry(info.Date, info.ID, edited)
		if err != nil {
			return nil, withPath(err, src.Path)
		}
		if !changed {
			return e, nil
		}
		if err := nb.checkEntryRefs(e); err != nil {
			return nil, err
		}
		e.Updated = nb.touch(e.Created)
		return nb.writeEntry(e, &src.Version)
	}
	return nil, newErr(CodeOutsideRoot, "", "")
}
