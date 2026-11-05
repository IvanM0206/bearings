package notebook

import (
	"bytes"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// checkEncoding applies the encoding rules of SPEC §4 and the file size
// limit of §7, and normalizes CRLF to LF.
func checkEncoding(data []byte) ([]byte, error) {
	if len(data) > MaxFileSize {
		return nil, limitErr(LimitFileSize, "", MaxFileSize)
	}
	if bytes.HasPrefix(data, []byte("\xef\xbb\xbf")) {
		return nil, newErr(CodeBOM, "", "")
	}
	if !utf8.Valid(data) {
		return nil, newErr(CodeInvalidUTF8, "", "")
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return nil, newErr(CodeNULByte, "", "")
	}
	return bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n")), nil
}

// splitFrontmatter splits a normalized .md file into frontmatter and body.
// The file must start with a "---" line and the frontmatter ends at the
// next "---" line (or "---" at the end of the file).
func splitFrontmatter(data []byte) (fm []byte, body string, err error) {
	const delim = "---\n"
	if !bytes.HasPrefix(data, []byte(delim)) {
		if bytes.Equal(data, []byte("---")) {
			return nil, "", newErr(CodeUnclosedFrontmatter, "", "")
		}
		return nil, "", newErr(CodeMissingFrontmatter, "", "")
	}
	rest := data[len(delim):]
	// Only the first MaxFrontmatterSize bytes (plus the closing line) are
	// searched, so a huge file without a closing delimiter is cheap.
	window := rest
	if len(window) > MaxFrontmatterSize+len(delim) {
		window = window[:MaxFrontmatterSize+len(delim)]
	}
	end := -1
	var after int
	switch {
	case bytes.HasPrefix(window, []byte(delim)):
		end, after = 0, len(delim)
	case bytes.Equal(rest, []byte("---")):
		end, after = 0, 3
	default:
		if i := bytes.Index(window, []byte("\n"+delim)); i >= 0 {
			end, after = i+1, i+1+len(delim)
		} else if bytes.HasSuffix(rest, []byte("\n---")) && len(rest)-3 <= MaxFrontmatterSize {
			end, after = len(rest)-3, len(rest)
		}
	}
	if end < 0 {
		if len(rest) > MaxFrontmatterSize {
			return nil, "", limitErr(LimitFrontmatterSize, "", MaxFrontmatterSize)
		}
		return nil, "", newErr(CodeUnclosedFrontmatter, "", "")
	}
	if end > MaxFrontmatterSize {
		return nil, "", limitErr(LimitFrontmatterSize, "", MaxFrontmatterSize)
	}
	return rest[:end], normalizeBody(string(rest[after:])), nil
}

// normalizeBody removes leading blank lines and trailing whitespace. Writers
// add exactly one trailing newline back.
func normalizeBody(s string) string {
	s = strings.TrimRightFunc(s, unicode.IsSpace)
	for strings.HasPrefix(s, "\n") {
		s = s[1:]
	}
	return s
}

// ParseConfig parses and validates bearings.yaml (SPEC §5.1).
func ParseConfig(data []byte) (Config, error) {
	norm, err := checkEncoding(data)
	if err != nil {
		return Config{}, err
	}
	root, err := parseYAMLMapping(norm)
	if err != nil {
		return Config{}, err
	}
	f, err := mappingFields(root, "", "format", "name", "language", "template")
	if err != nil {
		return Config{}, err
	}
	var c Config
	n, err := f.required("format", "")
	if err != nil {
		return Config{}, err
	}
	c.Format, err = yamlInt("format", n, 0, 1<<31-1)
	if err != nil {
		return Config{}, err
	}
	if c.Format != FormatVersion {
		return Config{}, &Error{Code: CodeUnsupportedFormat, Field: "format", Line: n.Line, Max: FormatVersion}
	}
	if n, ok := f["name"]; ok {
		if c.Name, err = yamlText("name", n, MaxNameLen, false); err != nil {
			return Config{}, err
		}
	}
	if n, ok := f["language"]; ok {
		if c.Language, err = yamlString("language", n); err != nil {
			return Config{}, err
		}
		if c.Language != "en" && c.Language != "es" {
			return Config{}, invalid("language", n)
		}
	}
	if n, ok := f["template"]; ok {
		t, err := parseTemplateInfo(n)
		if err != nil {
			return Config{}, err
		}
		c.Template = &t
	}
	c.Version = versionOf(data)
	return c, nil
}

func parseTemplateInfo(n *yamlNode) (TemplateInfo, error) {
	f, err := mappingFields(n, "template", "id", "title", "description")
	if err != nil {
		return TemplateInfo{}, err
	}
	var t TemplateInfo
	idn, err := f.required("id", "template")
	if err != nil {
		return t, err
	}
	if t.ID, err = yamlString("template.id", idn); err != nil {
		return t, err
	}
	if !ValidSlug(t.ID) {
		return t, &Error{Code: CodeInvalidSlug, Field: "template.id", Line: idn.Line}
	}
	tn, err := f.required("title", "template")
	if err != nil {
		return t, err
	}
	if t.Title, err = yamlText("template.title", tn, MaxTemplateTitleLen, false); err != nil {
		return t, err
	}
	if dn, ok := f["description"]; ok {
		if t.Description, err = yamlString("template.description", dn); err != nil {
			return t, err
		}
		t.Description = strings.TrimSpace(t.Description)
		if utf8.RuneCountInString(t.Description) > MaxTemplateDescriptionLen {
			return t, &Error{Code: CodeTooLong, Field: "template.description", Max: MaxTemplateDescriptionLen, Line: dn.Line}
		}
	}
	return t, nil
}

// MarshalConfig encodes c as bearings.yaml after validating it.
func MarshalConfig(c Config) ([]byte, error) {
	var b strings.Builder
	b.WriteString("format: " + strconv.Itoa(c.Format) + "\n")
	if c.Name != "" {
		b.WriteString("name: " + yamlQuote(c.Name) + "\n")
	}
	if c.Language != "" {
		b.WriteString("language: " + yamlQuote(c.Language) + "\n")
	}
	if t := c.Template; t != nil {
		b.WriteString("template:\n")
		b.WriteString("  id: " + yamlQuote(t.ID) + "\n")
		b.WriteString("  title: " + yamlQuote(t.Title) + "\n")
		if t.Description != "" {
			b.WriteString("  description: " + yamlQuote(t.Description) + "\n")
		}
	}
	out := []byte(b.String())
	if _, err := ParseConfig(out); err != nil {
		return nil, err
	}
	return out, nil
}

// parseTimes reads the created/updated pair and checks updated ≥ created.
func parseTimes(f fieldSet) (created, updated time.Time, err error) {
	cn, err := f.required("created", "")
	if err != nil {
		return
	}
	if created, err = yamlTimestamp("created", cn); err != nil {
		return
	}
	un, err := f.required("updated", "")
	if err != nil {
		return
	}
	if updated, err = yamlTimestamp("updated", un); err != nil {
		return
	}
	if updated.Before(created) {
		err = &Error{Code: CodeUpdatedBeforeCreated, Field: "updated", Line: un.Line}
	}
	return
}

// parseMD runs the common steps for a Markdown file: encoding, frontmatter
// split, YAML subset, allowed keys.
func parseMD(data []byte, keys ...string) (fieldSet, string, error) {
	norm, err := checkEncoding(data)
	if err != nil {
		return nil, "", err
	}
	fm, body, err := splitFrontmatter(norm)
	if err != nil {
		return nil, "", err
	}
	root, err := parseYAMLMapping(fm)
	if err != nil {
		return nil, "", err
	}
	f, err := mappingFields(root, "", keys...)
	if err != nil {
		return nil, "", err
	}
	return f, body, nil
}

// ParseSection parses and validates a _section.md file (SPEC §5.2). The
// slug comes from the file's directory and is not part of the content.
func ParseSection(slug string, data []byte) (Section, error) {
	if !ValidSlug(slug) {
		return Section{}, newErr(CodeInvalidSlug, "", "")
	}
	f, body, err := parseMD(data, "title", "order", "created", "updated")
	if err != nil {
		return Section{}, err
	}
	s := Section{Slug: slug, Description: body, Version: versionOf(data)}
	tn, err := f.required("title", "")
	if err != nil {
		return Section{}, err
	}
	if s.Title, err = yamlText("title", tn, MaxSectionTitleLen, true); err != nil {
		return Section{}, err
	}
	on, err := f.required("order", "")
	if err != nil {
		return Section{}, err
	}
	if s.Order, err = yamlInt("order", on, 0, MaxOrder); err != nil {
		return Section{}, err
	}
	if s.Created, s.Updated, err = parseTimes(f); err != nil {
		return Section{}, err
	}
	if err := checkSectionBody(s.Description); err != nil {
		return Section{}, err
	}
	return s, nil
}

func checkSectionBody(body string) error {
	if utf8.RuneCountInString(body) > MaxSectionDescriptionLen {
		return &Error{Code: CodeTooLong, Field: "body", Max: MaxSectionDescriptionLen}
	}
	return nil
}

// MarshalSection encodes s as a _section.md file after validating it.
func MarshalSection(s Section) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlQuote(s.Title) + "\n")
	b.WriteString("order: " + strconv.Itoa(s.Order) + "\n")
	b.WriteString("created: " + FormatTimestamp(s.Created) + "\n")
	b.WriteString("updated: " + FormatTimestamp(s.Updated) + "\n")
	b.WriteString("---\n")
	writeBody(&b, s.Description)
	out := []byte(b.String())
	if _, err := ParseSection(s.Slug, out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeBody(b *strings.Builder, body string) {
	body = normalizeBody(strings.ReplaceAll(body, "\r\n", "\n"))
	if body != "" {
		b.WriteString(body)
		b.WriteByte('\n')
	}
}

// ParseTopic parses and validates a topic file (SPEC §5.3). The section and
// slug come from the file's path.
func ParseTopic(section, slug string, data []byte) (Topic, error) {
	if !ValidSlug(section) || !ValidSlug(slug) {
		return Topic{}, newErr(CodeInvalidSlug, "", "")
	}
	f, body, err := parseMD(data, "title", "status", "type", "suggested", "order", "resources", "created", "updated")
	if err != nil {
		return Topic{}, err
	}
	t := Topic{Section: section, Slug: slug, Notes: body, Version: versionOf(data), Resources: []Resource{}}
	n, err := f.required("title", "")
	if err != nil {
		return Topic{}, err
	}
	if t.Title, err = yamlText("title", n, MaxTopicTitleLen, true); err != nil {
		return Topic{}, err
	}
	if n, err = f.required("status", ""); err != nil {
		return Topic{}, err
	}
	st, err := yamlString("status", n)
	if err != nil {
		return Topic{}, err
	}
	if t.Status = Status(st); !t.Status.Valid() {
		return Topic{}, invalid("status", n)
	}
	if n, err = f.required("type", ""); err != nil {
		return Topic{}, err
	}
	ty, err := yamlString("type", n)
	if err != nil {
		return Topic{}, err
	}
	if t.Type = TopicType(ty); !t.Type.Valid() {
		return Topic{}, invalid("type", n)
	}
	if n, ok := f["suggested"]; ok {
		if t.Suggested, err = yamlBool("suggested", n); err != nil {
			return Topic{}, err
		}
	}
	if n, err = f.required("order", ""); err != nil {
		return Topic{}, err
	}
	if t.Order, err = yamlInt("order", n, 0, MaxOrder); err != nil {
		return Topic{}, err
	}
	if n, ok := f["resources"]; ok {
		if t.Resources, err = parseResources(n); err != nil {
			return Topic{}, err
		}
	}
	if t.Created, t.Updated, err = parseTimes(f); err != nil {
		return Topic{}, err
	}
	if err := checkTopicBody(t.Notes); err != nil {
		return Topic{}, err
	}
	return t, nil
}

func checkTopicBody(body string) error {
	if len(body) > MaxTopicNotesSize {
		return &Error{Code: CodeTooLong, Field: "body", Max: MaxTopicNotesSize}
	}
	return nil
}

func parseResources(n *yamlNode) ([]Resource, error) {
	if n.Kind != yamlSequence {
		return nil, wrongType("resources", n)
	}
	if len(n.Content) > MaxResources {
		return nil, &Error{Code: CodeTooLong, Field: "resources", Max: MaxResources, Line: n.Line}
	}
	out := make([]Resource, 0, len(n.Content))
	for i, item := range n.Content {
		field := "resources[" + strconv.Itoa(i) + "]"
		f, err := mappingFields(item, field, "url", "title")
		if err != nil {
			return nil, err
		}
		un, err := f.required("url", field)
		if err != nil {
			return nil, err
		}
		var r Resource
		if r.URL, err = yamlString(field+".url", un); err != nil {
			return nil, err
		}
		if err := CheckURL(r.URL); err != nil {
			return nil, &Error{Code: CodeInvalidValue, Field: field + ".url", Line: un.Line}
		}
		if tn, ok := f["title"]; ok {
			if r.Title, err = yamlText(field+".title", tn, MaxResourceTitleLen, true); err != nil {
				return nil, err
			}
		}
		out = append(out, r)
	}
	return out, nil
}

// CheckURL validates a resource URL: absolute http or https, with a host,
// at most 2048 characters, no userinfo, no whitespace or control characters.
func CheckURL(s string) error {
	bad := newErr(CodeInvalidValue, "", "url")
	if s == "" || len(s) > MaxURLLen || !utf8.ValidString(s) {
		return bad
	}
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return bad
		}
	}
	u, err := url.Parse(s)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Opaque != "" ||
		u.Host == "" || u.User != nil || strings.Contains(u.Host, "@") {
		return bad
	}
	// Reject a scheme spelled with uppercase letters too: url.Parse
	// lowercases it, but the stored string is what other tools see.
	if !strings.HasPrefix(s, u.Scheme+"://") {
		return bad
	}
	return nil
}

// MarshalTopic encodes t as a topic file after validating it.
func MarshalTopic(t Topic) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: " + yamlQuote(t.Title) + "\n")
	b.WriteString("status: " + string(t.Status) + "\n")
	b.WriteString("type: " + string(t.Type) + "\n")
	if t.Suggested {
		b.WriteString("suggested: true\n")
	}
	b.WriteString("order: " + strconv.Itoa(t.Order) + "\n")
	if len(t.Resources) > 0 {
		b.WriteString("resources:\n")
		for _, r := range t.Resources {
			b.WriteString("  - url: " + yamlQuote(r.URL) + "\n")
			if r.Title != "" {
				b.WriteString("    title: " + yamlQuote(r.Title) + "\n")
			}
		}
	}
	b.WriteString("created: " + FormatTimestamp(t.Created) + "\n")
	b.WriteString("updated: " + FormatTimestamp(t.Updated) + "\n")
	b.WriteString("---\n")
	writeBody(&b, t.Notes)
	out := []byte(b.String())
	if _, err := ParseTopic(t.Section, t.Slug, out); err != nil {
		return nil, err
	}
	return out, nil
}

// ParseEntry parses and validates a journal entry (SPEC §5.4). date and id
// come from the file name; the `date` field must match it.
func ParseEntry(date Date, id string, data []byte) (Entry, error) {
	if !ValidEntryID(id) {
		return Entry{}, newErr(CodeInvalidID, "", "")
	}
	f, body, err := parseMD(data, "date", "section", "topic", "created", "updated")
	if err != nil {
		return Entry{}, err
	}
	e := Entry{ID: id, Body: body, Version: versionOf(data)}
	n, err := f.required("date", "")
	if err != nil {
		return Entry{}, err
	}
	if e.Date, err = yamlDate("date", n); err != nil {
		return Entry{}, err
	}
	if e.Date != date {
		return Entry{}, &Error{Code: CodePathMismatch, Field: "date", Line: n.Line}
	}
	if n, ok := f["section"]; ok {
		if e.Section, err = yamlString("section", n); err != nil {
			return Entry{}, err
		}
		if !ValidSlug(e.Section) {
			return Entry{}, &Error{Code: CodeInvalidSlug, Field: "section", Line: n.Line}
		}
	}
	if n, ok := f["topic"]; ok {
		s, err := yamlString("topic", n)
		if err != nil {
			return Entry{}, err
		}
		if e.Topic, err = ParseTopicRef(s); err != nil {
			return Entry{}, &Error{Code: CodeInvalidRef, Field: "topic", Line: n.Line}
		}
		if e.Section == "" || e.Topic.Section != e.Section {
			return Entry{}, &Error{Code: CodeTopicSectionMismatch, Field: "topic", Line: n.Line}
		}
	}
	if e.Created, e.Updated, err = parseTimes(f); err != nil {
		return Entry{}, err
	}
	if err := checkEntryBody(e.Body); err != nil {
		return Entry{}, err
	}
	return e, nil
}

func checkEntryBody(body string) error {
	t := strings.TrimSpace(body)
	if t == "" {
		return newErr(CodeEmpty, "", "body")
	}
	if len(t) > MaxEntryBodySize {
		return &Error{Code: CodeTooLong, Field: "body", Max: MaxEntryBodySize}
	}
	return nil
}

// MarshalEntry encodes e as a journal entry file after validating it.
func MarshalEntry(e Entry) ([]byte, error) {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("date: " + e.Date.String() + "\n")
	if e.Section != "" {
		b.WriteString("section: " + yamlQuote(e.Section) + "\n")
	}
	if !e.Topic.IsZero() {
		b.WriteString("topic: " + yamlQuote(e.Topic.String()) + "\n")
	}
	b.WriteString("created: " + FormatTimestamp(e.Created) + "\n")
	b.WriteString("updated: " + FormatTimestamp(e.Updated) + "\n")
	b.WriteString("---\n")
	writeBody(&b, e.Body)
	out := []byte(b.String())
	if _, err := ParseEntry(e.Date, e.ID, out); err != nil {
		return nil, err
	}
	return out, nil
}
