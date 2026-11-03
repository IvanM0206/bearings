package notebook

import (
	"regexp"
	"strings"
)

// PathKind is the kind of a notebook file, from its relative path (SPEC §2).
type PathKind int

// Path kinds.
const (
	PathUnknown PathKind = iota // not part of the format: ignored
	PathConfig                  // bearings.yaml
	PathSection                 // sections/<slug>/_section.md
	PathTopic                   // sections/<slug>/<slug>.md
	PathEntry                   // journal/<YYYY>/<MM>/<YYYY>-<MM>-<DD>-<id>.md
)

// PathInfo describes a recognized notebook path.
type PathInfo struct {
	Kind    PathKind
	Section string // PathSection, PathTopic
	Topic   string // PathTopic
	Date    Date   // PathEntry
	ID      string // PathEntry
}

// File names and directories of the layout.
const (
	ConfigFile  = "bearings.yaml"
	SectionsDir = "sections"
	JournalDir  = "journal"
	SectionFile = "_section.md"
)

var (
	yearDirRe   = regexp.MustCompile(`^[0-9]{4}$`)
	monthDirRe  = regexp.MustCompile(`^[0-9]{2}$`)
	entryFileRe = regexp.MustCompile(`^([0-9]{4})-([0-9]{2})-([0-9]{2})-([0-9a-hjkmnp-tv-z]{10})\.md$`)
)

// ClassifyPath reports which file type a slash-separated relative path is.
// Only the exact patterns of SPEC §2 are recognized; everything else is
// PathUnknown and must be ignored.
//
// A journal path whose directories do not match the date in its file name
// is reported with ok=false and a CodePathMismatch error: it looks like an
// entry, so it is an error rather than an unknown file.
func ClassifyPath(rel string) (PathInfo, error) {
	parts := strings.Split(rel, "/")
	switch {
	case len(parts) == 1 && parts[0] == ConfigFile:
		return PathInfo{Kind: PathConfig}, nil
	case len(parts) == 3 && parts[0] == SectionsDir && ValidSlug(parts[1]):
		if parts[2] == SectionFile {
			return PathInfo{Kind: PathSection, Section: parts[1]}, nil
		}
		if name, ok := strings.CutSuffix(parts[2], ".md"); ok && ValidSlug(name) {
			return PathInfo{Kind: PathTopic, Section: parts[1], Topic: name}, nil
		}
	case len(parts) == 4 && parts[0] == JournalDir && yearDirRe.MatchString(parts[1]) && monthDirRe.MatchString(parts[2]):
		m := entryFileRe.FindStringSubmatch(parts[3])
		if m == nil {
			break
		}
		if m[1] != parts[1] || m[2] != parts[2] {
			return PathInfo{}, newErr(CodePathMismatch, rel, "")
		}
		d, err := ParseDate(m[1] + "-" + m[2] + "-" + m[3])
		if err != nil {
			return PathInfo{}, withPath(err, rel)
		}
		return PathInfo{Kind: PathEntry, Date: d, ID: m[4]}, nil
	}
	return PathInfo{Kind: PathUnknown}, nil
}

func sectionDirRel(slug string) string { return SectionsDir + "/" + slug }

func sectionRel(slug string) string { return SectionsDir + "/" + slug + "/" + SectionFile }

func topicRel(ref TopicRef) string { return SectionsDir + "/" + ref.Section + "/" + ref.Topic + ".md" }

func entryDirRel(d Date) string {
	s := d.String()
	return JournalDir + "/" + s[:4] + "/" + s[5:7]
}

func entryRel(d Date, id string) string { return entryDirRel(d) + "/" + d.String() + "-" + id + ".md" }

// PathOf returns the relative path of a recognized file. It is the inverse
// of ClassifyPath and is only built from validated parts.
func PathOf(p PathInfo) string {
	switch p.Kind {
	case PathConfig:
		return ConfigFile
	case PathSection:
		return sectionRel(p.Section)
	case PathTopic:
		return topicRel(TopicRef{Section: p.Section, Topic: p.Topic})
	case PathEntry:
		return entryRel(p.Date, p.ID)
	}
	return ""
}
