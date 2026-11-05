package mcpserver

import (
	"context"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// Write tool names. They exist only with --allow-write.
const (
	ToolAddJournalEntry  = "add_journal_entry"
	ToolSetTopicStatus   = "set_topic_status"
	ToolAddTopic         = "add_topic"
	ToolAppendTopicNotes = "append_topic_notes"
)

// WriteTools lists the write tools.
var WriteTools = []string{ToolAddJournalEntry, ToolSetTopicStatus, ToolAddTopic, ToolAppendTopicNotes}

// ---- add_journal_entry -----------------------------------------------------

type addEntryIn struct {
	Body    string `json:"body"`
	Date    string `json:"date,omitempty"`
	Section string `json:"section,omitempty"`
	Topic   string `json:"topic,omitempty"`
}

type addEntryOut struct {
	ID      string `json:"id"`
	Date    string `json:"date"`
	Section string `json:"section,omitempty"`
	Topic   string `json:"topic,omitempty"`
}

// ---- set_topic_status ------------------------------------------------------

type setStatusIn struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
}

type setStatusOut struct {
	Topic    string `json:"topic"`
	Previous string `json:"previous"`
	Status   string `json:"status"`
	Changed  bool   `json:"changed"`
}

// ---- add_topic -------------------------------------------------------------

type addTopicIn struct {
	Section string `json:"section"`
	Title   string `json:"title"`
	Slug    string `json:"slug,omitempty"`
	Type    string `json:"type,omitempty"`
	Status  string `json:"status,omitempty"`
	Notes   string `json:"notes,omitempty"`
}

type addTopicOut struct {
	Topic  string `json:"topic"`
	Status string `json:"status"`
	Type   string `json:"type"`
}

// ---- append_topic_notes ----------------------------------------------------

type appendNotesIn struct {
	Topic string `json:"topic"`
	Text  string `json:"text"`
}

type appendNotesOut struct {
	Topic      string `json:"topic"`
	NotesBytes int    `json:"notes_bytes"`
}

func (h *server) addWriteTools(s *mcp.Server) {
	addTool(s, h, &mcp.Tool{
		Name:  ToolAddJournalEntry,
		Title: "Add journal entry",
		Description: "Add a dated journal entry about something learned, optionally linked to an existing section and topic. " +
			"The date defaults to today. Writes to the notebook.",
		InputSchema: inputSchema[addEntryIn](map[string]*jsonschema.Schema{
			"body":    str("Entry text (Markdown), up to 64 KiB.", 1, notebook.MaxEntryBodySize, ""),
			"date":    dateSchema("Day the learning is about, YYYY-MM-DD; default today."),
			"section": slugSchema("Existing section slug to link the entry to."),
			"topic":   refSchema("Existing topic reference <section>/<topic> to link the entry to; its section must match section if both are given."),
		}),
		Annotations: writes(false),
	}, h.addEntry)

	addTool(s, h, &mcp.Tool{
		Name:        ToolSetTopicStatus,
		Title:       "Set topic status",
		Description: "Set the status of an existing topic: pending, in-progress or mastered. Writes to the notebook.",
		InputSchema: inputSchema[setStatusIn](map[string]*jsonschema.Schema{
			"topic":  refSchema("Topic reference: <section-slug>/<topic-slug>."),
			"status": statusSchema("New status."),
		}),
		Annotations: writes(true),
	}, h.setStatus)

	addTool(s, h, &mcp.Tool{
		Name:  ToolAddTopic,
		Title: "Add topic",
		Description: "Add a topic (something to learn) at the end of an existing section. The slug is derived from the title unless given; " +
			"type defaults to role and status to pending. Writes to the notebook.",
		InputSchema: inputSchema[addTopicIn](map[string]*jsonschema.Schema{
			"section": slugSchema("Existing section slug."),
			"title":   str("Topic title, 1 to 160 characters, one line.", 1, notebook.MaxTopicTitleLen, ""),
			"slug":    slugSchema("Slug for the new topic; default derived from the title."),
			"type":    typeSchema("role (part of the role path, default) or personal."),
			"status":  statusSchema("Initial status; default pending."),
			"notes":   str("Initial notes (Markdown), up to 256 KiB.", 0, notebook.MaxTopicNotesSize, ""),
		}),
		Annotations: writes(false),
	}, h.addTopic)

	addTool(s, h, &mcp.Tool{
		Name:  ToolAppendTopicNotes,
		Title: "Append to topic notes",
		Description: "Append text to the notes of an existing topic, after a blank line. Existing notes are kept. " +
			"Fails if the notes would exceed 256 KiB. Writes to the notebook.",
		InputSchema: inputSchema[appendNotesIn](map[string]*jsonschema.Schema{
			"topic": refSchema("Topic reference: <section-slug>/<topic-slug>."),
			"text":  str("Text to append (Markdown).", 1, notebook.MaxTopicNotesSize, ""),
		}),
		Annotations: writes(false),
	}, h.appendNotes)
}

// checkText rejects control characters other than tab and line breaks, and
// escape sequences, in text that will be written to the notebook.
func checkText(name, s string) error {
	for _, r := range s {
		if r == '\n' || r == '\t' || r == '\r' {
			continue
		}
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) {
			return errText(name)
		}
	}
	return nil
}

func (h *server) wrote(tool string, ref string) {
	h.log.Info("notebook updated", "tool", tool, "ref", ref)
}

func (h *server) addEntry(_ context.Context, in addEntryIn) (addEntryOut, string, error) {
	if err := checkText("body", in.Body); err != nil {
		return addEntryOut{}, "", err
	}
	if strings.TrimSpace(in.Body) == "" {
		return addEntryOut{}, "", errArg("body")
	}
	ne := notebook.NewEntry{Section: in.Section, Body: in.Body}
	var err error
	if in.Date != "" {
		if ne.Date, err = notebook.ParseDate(in.Date); err != nil {
			return addEntryOut{}, "date", err
		}
	}
	if in.Section != "" {
		if err := notebook.CheckSlug(in.Section); err != nil {
			return addEntryOut{}, "section", err
		}
	}
	if in.Topic != "" {
		if ne.Topic, err = notebook.ParseTopicRef(in.Topic); err != nil {
			return addEntryOut{}, "topic", err
		}
	}
	e, err := h.nb.AddEntry(ne)
	if err != nil {
		return addEntryOut{}, "body", err
	}
	h.wrote(ToolAddJournalEntry, e.ID)
	return addEntryOut{ID: e.ID, Date: e.Date.String(), Section: e.Section, Topic: e.Topic.String()}, "", nil
}

func (h *server) setStatus(_ context.Context, in setStatusIn) (setStatusOut, string, error) {
	ref, err := notebook.ParseTopicRef(in.Topic)
	if err != nil {
		return setStatusOut{}, "topic", err
	}
	st := notebook.Status(in.Status)
	if !st.Valid() {
		return setStatusOut{}, "", errArg("status")
	}
	t, err := h.nb.Topic(ref)
	if err != nil {
		return setStatusOut{}, "topic", err
	}
	out := setStatusOut{Topic: ref.String(), Previous: string(t.Status), Status: string(st)}
	if t.Status == st {
		return out, "", nil
	}
	t.Status = st
	if _, err := h.nb.UpdateTopic(t); err != nil {
		return setStatusOut{}, "topic", err
	}
	out.Changed = true
	h.wrote(ToolSetTopicStatus, ref.String())
	return out, "", nil
}

func (h *server) addTopic(_ context.Context, in addTopicIn) (addTopicOut, string, error) {
	if err := checkText("notes", in.Notes); err != nil {
		return addTopicOut{}, "", err
	}
	if err := notebook.CheckSlug(in.Section); err != nil {
		return addTopicOut{}, "section", err
	}
	if in.Slug != "" {
		if err := notebook.CheckSlug(in.Slug); err != nil {
			return addTopicOut{}, "slug", err
		}
	}
	nt := notebook.NewTopic{
		Section: in.Section, Slug: in.Slug, Title: in.Title, Notes: in.Notes,
		Status: notebook.Status(in.Status), Type: notebook.TopicType(in.Type),
	}
	if nt.Status == "" {
		nt.Status = notebook.StatusPending
	}
	if nt.Type == "" {
		nt.Type = notebook.TypeRole
	}
	if !nt.Status.Valid() || !nt.Type.Valid() {
		return addTopicOut{}, "", errArg("status or type")
	}
	// Titles are one line: a line break or any control character is
	// rejected by the notebook (CodeControlChar) and reported as invalid.
	t, err := h.nb.AddTopic(nt)
	if err != nil {
		arg := "title"
		switch notebook.CodeOf(err) {
		case notebook.CodeNotFound:
			arg = "section"
		case notebook.CodeExists:
			arg = "slug"
		}
		return addTopicOut{}, arg, err
	}
	h.wrote(ToolAddTopic, t.Ref().String())
	return addTopicOut{Topic: t.Ref().String(), Status: string(t.Status), Type: string(t.Type)}, "", nil
}

func (h *server) appendNotes(_ context.Context, in appendNotesIn) (appendNotesOut, string, error) {
	if err := checkText("text", in.Text); err != nil {
		return appendNotesOut{}, "", err
	}
	add := strings.TrimRight(strings.ReplaceAll(in.Text, "\r\n", "\n"), "\n")
	if strings.TrimSpace(add) == "" {
		return appendNotesOut{}, "", errArg("text")
	}
	ref, err := notebook.ParseTopicRef(in.Topic)
	if err != nil {
		return appendNotesOut{}, "topic", err
	}
	t, err := h.nb.Topic(ref)
	if err != nil {
		return appendNotesOut{}, "topic", err
	}
	if base := strings.TrimRight(t.Notes, "\n"); base != "" {
		t.Notes = base + "\n\n" + add
	} else {
		t.Notes = add
	}
	if len(t.Notes) > notebook.MaxTopicNotesSize {
		return appendNotesOut{}, "text", &notebook.Error{Code: notebook.CodeTooLong, Field: "body", Max: notebook.MaxTopicNotesSize}
	}
	u, err := h.nb.UpdateTopic(t)
	if err != nil {
		return appendNotesOut{}, "text", err
	}
	h.wrote(ToolAppendTopicNotes, ref.String())
	return appendNotesOut{Topic: ref.String(), NotesBytes: len(u.Notes)}, "", nil
}
