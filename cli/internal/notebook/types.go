package notebook

import (
	"crypto/sha256"
	"time"
)

// FormatVersion is the notebook format implemented by this package.
const FormatVersion = 1

// Status of a topic (SPEC §5.3).
type Status string

// Topic statuses, in learning order.
const (
	StatusPending    Status = "pending"
	StatusInProgress Status = "in-progress"
	StatusMastered   Status = "mastered"
)

// Statuses lists every status in learning order.
var Statuses = []Status{StatusPending, StatusInProgress, StatusMastered}

// Valid reports whether s is a known status.
func (s Status) Valid() bool {
	return s == StatusPending || s == StatusInProgress || s == StatusMastered
}

// Next returns the status after s ("pending" → "in-progress" → "mastered")
// and false if s is already "mastered".
func (s Status) Next() (Status, bool) {
	switch s {
	case StatusPending:
		return StatusInProgress, true
	case StatusInProgress:
		return StatusMastered, true
	}
	return s, false
}

// TopicType says whether a topic belongs to the role path (SPEC §5.3).
type TopicType string

// Topic types.
const (
	TypeRole     TopicType = "role"
	TypePersonal TopicType = "personal"
)

// Valid reports whether t is a known topic type.
func (t TopicType) Valid() bool { return t == TypeRole || t == TypePersonal }

// Version identifies the exact content of a file as it was read. Writers
// pass it back so that a file changed by someone else in the meantime is
// not overwritten (SPEC §8: conflict detection). The zero Version means
// "not read from disk".
type Version struct {
	Size int64
	Hash [sha256.Size]byte
}

// IsZero reports whether v is the zero Version.
func (v Version) IsZero() bool { return v == Version{} }

func versionOf(data []byte) Version {
	return Version{Size: int64(len(data)), Hash: sha256.Sum256(data)}
}

// Config is the content of bearings.yaml (SPEC §5.1).
type Config struct {
	Format   int           `json:"format"`
	Name     string        `json:"name,omitempty"`
	Language string        `json:"language,omitempty"`
	Template *TemplateInfo `json:"template,omitempty"`
	Version  Version       `json:"-"`
}

// TemplateInfo is the `template` mapping of a template's bearings.yaml.
type TemplateInfo struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
}

// Section is a section of the notebook (SPEC §5.2).
type Section struct {
	Slug        string    `json:"slug"`
	Title       string    `json:"title"`
	Order       int       `json:"order"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Description string    `json:"description"`
	Version     Version   `json:"-"`
}

// Resource is a link attached to a topic.
type Resource struct {
	URL   string `json:"url"`
	Title string `json:"title,omitempty"`
}

// Topic is a topic of a section (SPEC §5.3).
type Topic struct {
	Section   string     `json:"section"`
	Slug      string     `json:"slug"`
	Title     string     `json:"title"`
	Status    Status     `json:"status"`
	Type      TopicType  `json:"type"`
	Suggested bool       `json:"suggested"`
	Order     int        `json:"order"`
	Resources []Resource `json:"resources"`
	Created   time.Time  `json:"created"`
	Updated   time.Time  `json:"updated"`
	Notes     string     `json:"notes"`
	Version   Version    `json:"-"`
}

// Ref returns the topic's reference.
func (t Topic) Ref() TopicRef { return TopicRef{Section: t.Section, Topic: t.Slug} }

// Entry is a journal entry (SPEC §5.4).
type Entry struct {
	ID      string    `json:"id"`
	Date    Date      `json:"date"`
	Section string    `json:"section,omitempty"`
	Topic   TopicRef  `json:"topic"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`
	Body    string    `json:"body"`
	Version Version   `json:"-"`
}

// Limits and field sizes from SPEC §5 and §7.
const (
	MaxFileSize         = 1 << 20  // any .md or bearings.yaml
	MaxFrontmatterSize  = 16 << 10 // bytes between the delimiters
	MaxSections         = 500
	MaxTopicsPerSection = 2000
	MaxJournalEntries   = 200000
	MaxDepth            = 4

	MaxNameLen                = 80
	MaxTemplateTitleLen       = 80
	MaxTemplateDescriptionLen = 500
	MaxSectionTitleLen        = 80
	MaxSectionDescriptionLen  = 2000 // characters
	MaxTopicTitleLen          = 160
	MaxTopicNotesSize         = 256 << 10 // bytes
	MaxResources              = 50
	MaxURLLen                 = 2048
	MaxResourceTitleLen       = 160
	MaxEntryBodySize          = 64 << 10 // bytes
	MaxOrder                  = 1000000
)
