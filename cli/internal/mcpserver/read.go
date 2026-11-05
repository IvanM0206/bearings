package mcpserver

import (
	"context"
	"encoding/json"
	"unicode/utf8"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// Read tool names.
const (
	ToolListSections = "list_sections"
	ToolListTopics   = "list_topics"
	ToolGetTopic     = "get_topic"
	ToolSearch       = "search"
	ToolListJournal  = "list_journal"
	ToolGetRolePath  = "get_role_path"
	ToolGetProgress  = "get_progress"
)

// pageIn is the pagination part of list inputs.
type pageIn struct {
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

// ---- list_sections ---------------------------------------------------------

type listSectionsIn struct {
	pageIn
}

type listSectionsOut struct {
	Sections   []sectionData `json:"sections"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// ---- list_topics -----------------------------------------------------------

type listTopicsIn struct {
	Section string `json:"section,omitempty"`
	Status  string `json:"status,omitempty"`
	Type    string `json:"type,omitempty"`
	pageIn
}

type listTopicsOut struct {
	Topics     []topicSummary `json:"topics"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

// ---- get_topic -------------------------------------------------------------

type getTopicIn struct {
	Topic       string `json:"topic"`
	NotesOffset int    `json:"notes_offset,omitempty"`
}

type getTopicOut struct {
	topicSummary
	SectionTitle       string         `json:"section_title" jsonschema:"data: section title"`
	Created            string         `json:"created"`
	ResourceList       []resourceData `json:"resource_list"`
	ResourcesTruncated bool           `json:"resources_truncated"`
	Notes              string         `json:"notes" jsonschema:"data: topic notes (Markdown), or one chunk of them"`
	NotesOffset        int            `json:"notes_offset"`
	NotesTotalBytes    int            `json:"notes_total_bytes"`
	NotesTruncated     bool           `json:"notes_truncated"`
	NotesNextOffset    int            `json:"notes_next_offset,omitempty"`
}

// ---- search ----------------------------------------------------------------

type searchIn struct {
	Query string `json:"query"`
	pageIn
}

type hitData struct {
	Kind    string `json:"kind"`
	Section string `json:"section,omitempty"`
	Topic   string `json:"topic,omitempty"`
	EntryID string `json:"entry_id,omitempty"`
	Date    string `json:"date,omitempty"`
	Title   string `json:"title,omitempty" jsonschema:"data: title of the matching item"`
	Field   string `json:"field"`
	Snippet string `json:"snippet" jsonschema:"data: matching text"`
}

type searchOut struct {
	Hits       []hitData `json:"hits"`
	NextCursor string    `json:"next_cursor,omitempty"`
}

// ---- list_journal ----------------------------------------------------------

type listJournalIn struct {
	From    string `json:"from,omitempty"`
	To      string `json:"to,omitempty"`
	Section string `json:"section,omitempty"`
	pageIn
}

type listJournalOut struct {
	Entries    []entryData `json:"entries"`
	NextCursor string      `json:"next_cursor,omitempty"`
}

// ---- get_role_path ---------------------------------------------------------

type rolePathIn struct {
	pageIn
}

type stepData struct {
	Step         int    `json:"step"`
	SectionTitle string `json:"section_title" jsonschema:"data: section title"`
	topicSummary
}

type rolePathOut struct {
	Steps      []stepData `json:"steps"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

// ---- get_progress ----------------------------------------------------------

type progressIn struct {
	Cursor string `json:"cursor,omitempty"`
}

type sectionProgressData struct {
	Slug  string     `json:"slug"`
	Title string     `json:"title" jsonschema:"data: section title"`
	All   countsData `json:"all"`
	Role  countsData `json:"role"`
}

type progressOut struct {
	All            countsData            `json:"all"`
	Role           countsData            `json:"role"`
	Personal       countsData            `json:"personal"`
	Suggested      int                   `json:"suggested"`
	JournalEntries int                   `json:"journal_entries"`
	Sections       []sectionProgressData `json:"sections"`
	NextCursor     string                `json:"next_cursor,omitempty"`
}

func pageProps(extra map[string]*jsonschema.Schema) map[string]*jsonschema.Schema {
	extra["cursor"] = cursorSchema()
	extra["limit"] = limitSchema()
	return extra
}

func (h *server) addReadTools(s *mcp.Server) {
	addTool(s, h, &mcp.Tool{
		Name:        ToolListSections,
		Title:       "List sections",
		Description: "List the sections of the notebook (the large areas of the role), in order, with their descriptions. Paginated: pass next_cursor back as cursor.",
		InputSchema: inputSchema[listSectionsIn](pageProps(map[string]*jsonschema.Schema{})),
		Annotations: readOnly(),
	}, h.listSections)

	addTool(s, h, &mcp.Tool{
		Name:        ToolListTopics,
		Title:       "List topics",
		Description: "List topics (things to learn) without their notes, in section then topic order, optionally filtered by section slug, status and type. Use get_topic for the notes. Paginated.",
		InputSchema: inputSchema[listTopicsIn](pageProps(map[string]*jsonschema.Schema{
			"section": slugSchema("Only topics of this section (slug)."),
			"status":  statusSchema("Only topics with this status."),
			"type":    typeSchema("Only topics of this type: role (part of the role path) or personal."),
		})),
		Annotations: readOnly(),
	}, h.listTopics)

	addTool(s, h, &mcp.Tool{
		Name:  ToolGetTopic,
		Title: "Get topic",
		Description: "Get one topic with its resources and notes. Long notes come in chunks: when notes_truncated is true, " +
			"call again with notes_offset set to notes_next_offset.",
		InputSchema: inputSchema[getTopicIn](map[string]*jsonschema.Schema{
			"topic":        refSchema("Topic reference: <section-slug>/<topic-slug>."),
			"notes_offset": integer("Byte offset in the notes to start from (from notes_next_offset); default 0.", 0, 4*notebook.MaxTopicNotesSize),
		}),
		Annotations: readOnly(),
	}, h.getTopic)

	addTool(s, h, &mcp.Tool{
		Name:        ToolSearch,
		Title:       "Search",
		Description: "Search section, topic and journal titles and text for a phrase, ignoring case and accents. Returns hits with a short snippet. Paginated.",
		InputSchema: inputSchema[searchIn](pageProps(map[string]*jsonschema.Schema{
			"query": str("Text to look for.", 1, notebook.MaxQueryLen, ""),
		})),
		Annotations: readOnly(),
	}, h.search)

	addTool(s, h, &mcp.Tool{
		Name:  ToolListJournal,
		Title: "List journal entries",
		Description: "List journal entries (dated notes about something learned), newest first, optionally between two dates (inclusive) " +
			"and linked to a section. Long entries are truncated (body_truncated). Paginated.",
		InputSchema: inputSchema[listJournalIn](pageProps(map[string]*jsonschema.Schema{
			"from":    dateSchema("Only entries on or after this date, YYYY-MM-DD."),
			"to":      dateSchema("Only entries on or before this date, YYYY-MM-DD."),
			"section": slugSchema("Only entries linked to this section (slug)."),
		})),
		Annotations: readOnly(),
	}, h.listJournal)

	addTool(s, h, &mcp.Tool{
		Name:        ToolGetRolePath,
		Title:       "Get role path",
		Description: "Get the role path: every topic of type role, numbered in order across sections, with its status. It is the guide for whoever takes the role next. Paginated.",
		InputSchema: inputSchema[rolePathIn](pageProps(map[string]*jsonschema.Schema{})),
		Annotations: readOnly(),
	}, h.rolePath)

	addTool(s, h, &mcp.Tool{
		Name:        ToolGetProgress,
		Title:       "Get progress",
		Description: "Count topics by status for the whole notebook, for role and personal topics, and per section, plus the number of journal entries. The per-section list is paginated.",
		InputSchema: inputSchema[progressIn](map[string]*jsonschema.Schema{"cursor": cursorSchema()}),
		Annotations: readOnly(),
	}, h.progress)
}

func (h *server) listSections(_ context.Context, in listSectionsIn) (listSectionsOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return listSectionsOut{}, "", err
	}
	secs, err := h.nb.Sections()
	if err != nil {
		return listSectionsOut{}, "", err
	}
	all := make([]sectionData, len(secs))
	for i, sec := range secs {
		all[i] = toSection(sec)
	}
	page, next := paginate(all, off, pageLimit(in.Limit), false)
	return listSectionsOut{Sections: page, NextCursor: next}, "", nil
}

func (h *server) listTopics(_ context.Context, in listTopicsIn) (listTopicsOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return listTopicsOut{}, "", err
	}
	f := notebook.TopicFilter{Section: in.Section, Status: notebook.Status(in.Status), Type: notebook.TopicType(in.Type)}
	if f.Section != "" {
		if err := notebook.CheckSlug(f.Section); err != nil {
			return listTopicsOut{}, "section", err
		}
	}
	if (f.Status != "" && !f.Status.Valid()) || (f.Type != "" && !f.Type.Valid()) {
		return listTopicsOut{}, "", errArg("status or type")
	}
	ts, err := h.nb.FindTopics(f)
	if err != nil {
		return listTopicsOut{}, "section", err
	}
	all := make([]topicSummary, len(ts))
	for i, t := range ts {
		all[i] = toTopicSummary(t)
	}
	page, next := paginate(all, off, pageLimit(in.Limit), false)
	return listTopicsOut{Topics: page, NextCursor: next}, "", nil
}

func (h *server) getTopic(_ context.Context, in getTopicIn) (getTopicOut, string, error) {
	ref, err := notebook.ParseTopicRef(in.Topic)
	if err != nil {
		return getTopicOut{}, "topic", err
	}
	t, err := h.nb.Topic(ref)
	if err != nil {
		return getTopicOut{}, "topic", err
	}
	sec, err := h.nb.Section(ref.Section)
	if err != nil {
		return getTopicOut{}, "topic", err
	}
	out := getTopicOut{
		topicSummary: toTopicSummary(t),
		SectionTitle: line(sec.Title),
		Created:      notebook.FormatTimestamp(t.Created),
		ResourceList: make([]resourceData, 0, len(t.Resources)),
	}
	// Resources take at most half of the budget; notes get the rest.
	used := 0
	for _, r := range t.Resources {
		d := resourceData{URL: line(r.URL), Title: line(r.Title)}
		b, _ := json.Marshal(d)
		if used+len(b) > MaxResultBytes/2 {
			out.ResourcesTruncated = true
			break
		}
		used += len(b)
		out.ResourceList = append(out.ResourceList, d)
	}
	notes := output.Text(t.Notes)
	off := in.NotesOffset
	if off < 0 || off > len(notes) || !utf8.RuneStart(notesByte(notes, off)) {
		return getTopicOut{}, "", errArg("notes_offset")
	}
	head, _ := json.Marshal(out)
	part, next := chunk(notes, off, max(MaxResultBytes-len(head), MaxResultBytes/4))
	out.Notes, out.NotesOffset, out.NotesTotalBytes = part, off, len(notes)
	out.NotesTruncated, out.NotesNextOffset = next != 0, next
	return out, "", nil
}

// notesByte returns s[i], or a valid rune start at the end of s.
func notesByte(s string, i int) byte {
	if i >= len(s) {
		return 0
	}
	return s[i]
}

func (h *server) search(_ context.Context, in searchIn) (searchOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return searchOut{}, "", err
	}
	limit := pageLimit(in.Limit)
	hits, err := h.nb.Search(in.Query, notebook.SearchOptions{Limit: off + limit + 1})
	if err != nil {
		return searchOut{}, "query", err
	}
	more := len(hits) > off+limit
	if more {
		hits = hits[:off+limit]
	}
	all := make([]hitData, len(hits))
	for i, x := range hits {
		snip, _ := text(x.Snippet, MaxSnippetBytes)
		d := hitData{
			Kind: string(x.Kind), Section: x.Section, Topic: x.Topic.String(), EntryID: x.EntryID,
			Date: x.Date.String(), Title: line(x.Title), Field: x.Field, Snippet: snip,
		}
		all[i] = d
	}
	page, next := paginate(all, off, limit, more)
	return searchOut{Hits: page, NextCursor: next}, "", nil
}

func (h *server) listJournal(_ context.Context, in listJournalIn) (listJournalOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return listJournalOut{}, "", err
	}
	limit := pageLimit(in.Limit)
	f := notebook.JournalFilter{Section: in.Section, Limit: off + limit + 1}
	if in.From != "" {
		if f.From, err = notebook.ParseDate(in.From); err != nil {
			return listJournalOut{}, "from", err
		}
	}
	if in.To != "" {
		if f.To, err = notebook.ParseDate(in.To); err != nil {
			return listJournalOut{}, "to", err
		}
	}
	if f.Section != "" {
		if err := notebook.CheckSlug(f.Section); err != nil {
			return listJournalOut{}, "section", err
		}
	}
	es, err := h.nb.Journal(f)
	if err != nil {
		return listJournalOut{}, "", err
	}
	more := len(es) > off+limit
	if more {
		es = es[:off+limit]
	}
	all := make([]entryData, len(es))
	for i, e := range es {
		all[i] = toEntry(e)
	}
	page, next := paginate(all, off, limit, more)
	return listJournalOut{Entries: page, NextCursor: next}, "", nil
}

func (h *server) rolePath(_ context.Context, in rolePathIn) (rolePathOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return rolePathOut{}, "", err
	}
	steps, err := h.nb.RolePath()
	if err != nil {
		return rolePathOut{}, "", err
	}
	all := make([]stepData, len(steps))
	for i, st := range steps {
		all[i] = stepData{Step: st.Step, SectionTitle: line(st.SectionTitle), topicSummary: toTopicSummary(st.Topic)}
	}
	page, next := paginate(all, off, pageLimit(in.Limit), false)
	return rolePathOut{Steps: page, NextCursor: next}, "", nil
}

func (h *server) progress(_ context.Context, in progressIn) (progressOut, string, error) {
	off, err := parseCursor(in.Cursor)
	if err != nil {
		return progressOut{}, "", err
	}
	pr, err := h.nb.Progress()
	if err != nil {
		return progressOut{}, "", err
	}
	all := make([]sectionProgressData, len(pr.Sections))
	for i, sp := range pr.Sections {
		all[i] = sectionProgressData{Slug: sp.Slug, Title: line(sp.Title), All: toCounts(sp.All), Role: toCounts(sp.Role)}
	}
	page, next := paginate(all, off, MaxPageItems, false)
	return progressOut{
		All: toCounts(pr.All), Role: toCounts(pr.Role), Personal: toCounts(pr.Personal),
		Suggested: pr.Suggested, JournalEntries: pr.Entries,
		Sections: page, NextCursor: next,
	}, "", nil
}
