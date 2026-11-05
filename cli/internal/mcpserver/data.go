package mcpserver

import (
	"encoding/json"
	"strconv"
	"unicode/utf8"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// Response limits.
const (
	// MaxPageItems is the largest page any list tool returns.
	MaxPageItems = 100
	// DefaultPageItems is the page size when the caller gives no limit.
	DefaultPageItems = 50
	// MaxResultBytes is the budget, in bytes of JSON, for the items (or the
	// text) of one result. A page stops before the item that would exceed
	// it, but always holds at least one item.
	MaxResultBytes = 64 << 10
	// MaxListTextBytes caps a long text (a journal entry body, a section
	// description) inside a list item.
	MaxListTextBytes = 8 << 10
	// MaxSnippetBytes caps a search snippet.
	MaxSnippetBytes = 512
	// MaxTitleBytes caps a title (titles are at most 160 characters, so this
	// only matters for adversarial content).
	MaxTitleBytes = 1 << 10
	// maxCursor bounds a cursor (the journal holds at most 200 000 entries).
	maxCursor = 999999
)

// TruncatedMarker ends a text that was cut to fit a limit. The item also
// carries a "…_truncated": true field, which is the reliable signal.
const TruncatedMarker = " [truncated by bearings]"

// text sanitizes multi-line notebook content and cuts it to limit bytes of
// JSON, with the marker.
func text(s string, limit int) (string, bool) { return clip(output.Text(s), limit) }

// line sanitizes one-line notebook content (titles, URLs) and cuts it.
func line(s string) string {
	v, _ := clip(output.Line(s), MaxTitleBytes)
	return v
}

// jsonCost is the number of bytes r takes inside a JSON string as written
// by encoding/json (which escapes <, > and & as \u00XX).
func jsonCost(r rune) int {
	switch {
	case r == '"' || r == '\\' || r == '\n' || r == '\r' || r == '\t':
		return 2
	case r < 0x20 || r == '<' || r == '>' || r == '&' || r == 0x2028 || r == 0x2029:
		return 6
	case r == utf8.RuneError:
		return 3
	}
	return utf8.RuneLen(r)
}

// clip cuts s so that it takes at most limit bytes as JSON, appending
// TruncatedMarker when it cuts.
func clip(s string, limit int) (string, bool) {
	total := 0
	for _, r := range s {
		total += jsonCost(r)
	}
	if total <= limit {
		return s, false
	}
	budget := limit - len(TruncatedMarker)
	cut, used := 0, 0
	for i, r := range s {
		c := jsonCost(r)
		if used+c > budget {
			cut = i
			break
		}
		used += c
	}
	return s[:cut] + TruncatedMarker, true
}

// chunk returns the part of s that starts at byte offset off and takes at
// most limit bytes as JSON, and the offset of the rest (0 when none is left).
func chunk(s string, off, limit int) (string, int) {
	used := 0
	for i, r := range s[off:] {
		c := jsonCost(r)
		if used+c > limit {
			return s[off : off+i], off + i
		}
		used += c
	}
	return s[off:], 0
}

// parseCursor reads a cursor: "" is the start, otherwise the decimal
// offset of the first item (the schema already checked its syntax).
func parseCursor(c string) (int, error) {
	if c == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(c)
	if err != nil || n < 0 || n > maxCursor {
		return 0, errArg("cursor")
	}
	return n, nil
}

// pageLimit applies the default page size.
func pageLimit(n int) int {
	if n <= 0 {
		return DefaultPageItems
	}
	return min(n, MaxPageItems)
}

// paginate returns the items of all that start at offset, at most limit of
// them and within MaxResultBytes of JSON, and the cursor of the next page
// ("" when there is none). more reports that all has items after the ones
// given (when all was fetched with one extra item).
func paginate[T any](all []T, offset, limit int, more bool) ([]T, string) {
	out := make([]T, 0, min(limit, max(len(all)-offset, 0)))
	if offset >= len(all) {
		return out, ""
	}
	used := 0
	i := offset
	for ; i < len(all) && len(out) < limit; i++ {
		b, err := json.Marshal(all[i])
		if err != nil {
			break
		}
		if len(out) > 0 && used+len(b) > MaxResultBytes {
			break
		}
		used += len(b)
		out = append(out, all[i])
	}
	if i < len(all) || more {
		return out, strconv.Itoa(i)
	}
	return out, ""
}

// ---- Data types ------------------------------------------------------------

// Every string field marked "data" below is notebook content: untrusted,
// sanitized, possibly truncated.

type sectionData struct {
	Slug                 string `json:"slug"`
	Title                string `json:"title" jsonschema:"data: section title"`
	Order                int    `json:"order"`
	Description          string `json:"description" jsonschema:"data: section description"`
	DescriptionTruncated bool   `json:"description_truncated"`
	Updated              string `json:"updated"`
}

func toSection(s notebook.Section) sectionData {
	d, cut := text(s.Description, MaxListTextBytes)
	return sectionData{
		Slug: s.Slug, Title: line(s.Title), Order: s.Order,
		Description: d, DescriptionTruncated: cut,
		Updated: notebook.FormatTimestamp(s.Updated),
	}
}

type topicSummary struct {
	Ref       string `json:"ref"`
	Section   string `json:"section"`
	Slug      string `json:"slug"`
	Title     string `json:"title" jsonschema:"data: topic title"`
	Status    string `json:"status"`
	Type      string `json:"type"`
	Suggested bool   `json:"suggested"`
	Order     int    `json:"order"`
	Resources int    `json:"resources"`
	Updated   string `json:"updated"`
}

func toTopicSummary(t notebook.Topic) topicSummary {
	return topicSummary{
		Ref: t.Ref().String(), Section: t.Section, Slug: t.Slug, Title: line(t.Title),
		Status: string(t.Status), Type: string(t.Type), Suggested: t.Suggested,
		Order: t.Order, Resources: len(t.Resources), Updated: notebook.FormatTimestamp(t.Updated),
	}
}

type resourceData struct {
	URL   string `json:"url" jsonschema:"data: link from the notebook, not verified"`
	Title string `json:"title,omitempty" jsonschema:"data: link title"`
}

type entryData struct {
	ID            string `json:"id"`
	Date          string `json:"date"`
	Section       string `json:"section,omitempty"`
	Topic         string `json:"topic,omitempty"`
	Body          string `json:"body" jsonschema:"data: journal entry text"`
	BodyTruncated bool   `json:"body_truncated"`
	Created       string `json:"created"`
}

func toEntry(e notebook.Entry) entryData {
	b, cut := text(e.Body, MaxListTextBytes)
	return entryData{
		ID: e.ID, Date: e.Date.String(), Section: e.Section, Topic: e.Topic.String(),
		Body: b, BodyTruncated: cut, Created: notebook.FormatTimestamp(e.Created),
	}
}

type countsData struct {
	Pending    int `json:"pending"`
	InProgress int `json:"in_progress"`
	Mastered   int `json:"mastered"`
	Total      int `json:"total"`
}

func toCounts(c notebook.Counts) countsData {
	return countsData{Pending: c.Pending, InProgress: c.InProgress, Mastered: c.Mastered, Total: c.Total}
}
