package notebook

import (
	"sort"
	"strings"
)

// Tree is a complete notebook held in memory: the result of validating a
// template or an archive before anything is written to disk.
type Tree struct {
	Config   Config
	Sections []Section // in section order
	Topics   []Topic   // in section order, then topic order
	Entries  []Entry   // in journal order
}

// ParseTree validates a whole notebook given as a map from slash-separated
// relative paths to file contents. Paths that are not part of the format
// are ignored (SPEC §2). Unlike reading a directory, which skips invalid
// files, ParseTree fails on the first invalid file: it is meant for content
// from elsewhere (templates, archives), which is accepted whole or not at
// all. Every limit of SPEC §7 is enforced.
func ParseTree(files map[string][]byte) (*Tree, error) {
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	t := &Tree{}
	haveConfig := false
	sections := map[string]bool{}
	topicsPer := map[string]int{}
	var topicSections []string
	for _, p := range paths {
		if strings.Count(p, "/")+1 > MaxDepth {
			return nil, limitErr(LimitDepth, "", MaxDepth)
		}
		info, err := ClassifyPath(p)
		if err != nil {
			return nil, err
		}
		data := files[p]
		if len(data) > MaxFileSize && info.Kind != PathUnknown {
			return nil, limitErr(LimitFileSize, p, MaxFileSize)
		}
		switch info.Kind {
		case PathConfig:
			c, err := ParseConfig(data)
			if err != nil {
				return nil, withPath(err, p)
			}
			t.Config, haveConfig = c, true
		case PathSection:
			s, err := ParseSection(info.Section, data)
			if err != nil {
				return nil, withPath(err, p)
			}
			t.Sections = append(t.Sections, s)
			sections[s.Slug] = true
			if len(t.Sections) > MaxSections {
				return nil, limitErr(LimitSections, SectionsDir, MaxSections)
			}
		case PathTopic:
			tp, err := ParseTopic(info.Section, info.Topic, data)
			if err != nil {
				return nil, withPath(err, p)
			}
			t.Topics = append(t.Topics, tp)
			topicsPer[info.Section]++
			if topicsPer[info.Section] == 1 {
				topicSections = append(topicSections, info.Section)
			}
			if topicsPer[info.Section] > MaxTopicsPerSection {
				return nil, limitErr(LimitTopicsPerSection, sectionDirRel(info.Section), MaxTopicsPerSection)
			}
		case PathEntry:
			e, err := ParseEntry(info.Date, info.ID, data)
			if err != nil {
				return nil, withPath(err, p)
			}
			t.Entries = append(t.Entries, e)
			if len(t.Entries) > MaxJournalEntries {
				return nil, limitErr(LimitJournalEntries, JournalDir, MaxJournalEntries)
			}
		}
	}
	if !haveConfig {
		return nil, newErr(CodeNotNotebook, ConfigFile, "")
	}
	for _, s := range topicSections {
		if !sections[s] {
			return nil, newErr(CodeSectionMissing, sectionDirRel(s), "")
		}
	}
	sortSections(t.Sections)
	order := map[string]int{}
	for i, s := range t.Sections {
		order[s.Slug] = i
	}
	sortTopicsInTree(t.Topics, order)
	sortEntries(t.Entries)
	return t, nil
}

func sortTopicsInTree(ts []Topic, sectionOrder map[string]int) {
	sort.SliceStable(ts, func(i, j int) bool {
		a, b := sectionOrder[ts[i].Section], sectionOrder[ts[j].Section]
		if a != b {
			return a < b
		}
		if ts[i].Order != ts[j].Order {
			return ts[i].Order < ts[j].Order
		}
		return ts[i].Slug < ts[j].Slug
	})
}

// Files encodes the tree as a map from relative paths to file contents.
// Every file is validated by the encoder.
func (t *Tree) Files() (map[string][]byte, error) {
	out := make(map[string][]byte, 1+len(t.Sections)+len(t.Topics)+len(t.Entries))
	c := t.Config
	c.Format = FormatVersion
	data, err := MarshalConfig(c)
	if err != nil {
		return nil, withPath(err, ConfigFile)
	}
	out[ConfigFile] = data
	sections := map[string]bool{}
	topicsPer := map[string]int{}
	for _, s := range t.Sections {
		p := sectionRel(s.Slug)
		if _, dup := out[p]; dup {
			return nil, newErr(CodeExists, p, "")
		}
		if out[p], err = MarshalSection(s); err != nil {
			return nil, withPath(err, p)
		}
		sections[s.Slug] = true
	}
	for _, tp := range t.Topics {
		p := topicRel(tp.Ref())
		if !sections[tp.Section] {
			return nil, newErr(CodeSectionMissing, sectionDirRel(tp.Section), "")
		}
		if _, dup := out[p]; dup {
			return nil, newErr(CodeExists, p, "")
		}
		if topicsPer[tp.Section]++; topicsPer[tp.Section] > MaxTopicsPerSection {
			return nil, limitErr(LimitTopicsPerSection, sectionDirRel(tp.Section), MaxTopicsPerSection)
		}
		if out[p], err = MarshalTopic(tp); err != nil {
			return nil, withPath(err, p)
		}
	}
	for _, e := range t.Entries {
		p := entryRel(e.Date, e.ID)
		if _, dup := out[p]; dup {
			return nil, newErr(CodeExists, p, "")
		}
		if out[p], err = MarshalEntry(e); err != nil {
			return nil, withPath(err, p)
		}
	}
	if len(t.Sections) > MaxSections {
		return nil, limitErr(LimitSections, SectionsDir, MaxSections)
	}
	if len(t.Entries) > MaxJournalEntries {
		return nil, limitErr(LimitJournalEntries, JournalDir, MaxJournalEntries)
	}
	return out, nil
}
