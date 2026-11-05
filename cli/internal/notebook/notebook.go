package notebook

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Options configures a Notebook. The zero value is valid.
type Options struct {
	// Now returns the current time; it defaults to time.Now. Timestamps are
	// stored in UTC with second precision.
	Now func() time.Time
	// Rand is the source for entry ids; it defaults to crypto/rand.Reader
	// and must be cryptographically secure.
	Rand io.Reader
	// OnProblem is called for every file skipped while reading (invalid
	// content, symlink, special file, dangling reference). It may be nil.
	OnProblem func(Problem)
}

// Notebook is an open notebook directory. Every method reads the files it
// needs from disk, so changes made by other programs are seen. Methods are
// safe for concurrent use; writes are serialized within the process and
// protected against concurrent writers by conflict detection.
type Notebook struct {
	fs   fsys
	opts Options
	mu   sync.Mutex
}

// Open opens the notebook in dir. The directory is resolved once to an
// absolute, symlink-free path; it must contain a valid bearings.yaml.
func Open(dir string, opts *Options) (*Notebook, error) {
	root, err := resolveRoot(dir)
	if err != nil {
		return nil, err
	}
	nb := newNotebook(root, opts)
	if _, err := nb.Config(); err != nil {
		if CodeOf(err) == CodeNotFound {
			return nil, newErr(CodeNotNotebook, "", "")
		}
		return nil, err
	}
	return nb, nil
}

func newNotebook(root string, opts *Options) *Notebook {
	nb := &Notebook{fs: fsys{root: root}}
	if opts != nil {
		nb.opts = *opts
	}
	if nb.opts.Now == nil {
		nb.opts.Now = time.Now
	}
	if nb.opts.Rand == nil {
		nb.opts.Rand = rand.Reader
	}
	return nb
}

// Root returns the absolute, symlink-free path of the notebook.
func (nb *Notebook) Root() string { return nb.fs.root }

func (nb *Notebook) now() time.Time { return nb.opts.Now().UTC().Truncate(time.Second) }

func (nb *Notebook) problem(path string, err error) {
	if nb.opts.OnProblem != nil {
		nb.opts.OnProblem(Problem{Path: path, Err: withPath(err, path)})
	}
}

// Config reads bearings.yaml.
func (nb *Notebook) Config() (Config, error) {
	data, err := nb.fs.readFile(ConfigFile)
	if err != nil {
		return Config{}, err
	}
	c, err := ParseConfig(data)
	if err != nil {
		return Config{}, withPath(err, ConfigFile)
	}
	return c, nil
}

// UpdateConfig writes c to bearings.yaml if the file still has the
// content identified by c.Version.
func (nb *Notebook) UpdateConfig(c Config) (Config, error) {
	nb.mu.Lock()
	defer nb.mu.Unlock()
	c.Format = FormatVersion
	data, err := MarshalConfig(c)
	if err != nil {
		return Config{}, withPath(err, ConfigFile)
	}
	if err := nb.fs.writeFile(ConfigFile, data, &c.Version); err != nil {
		return Config{}, err
	}
	c.Version = versionOf(data)
	return c, nil
}

// ---- Sections --------------------------------------------------------------

func (nb *Notebook) loadSection(slug string) (Section, error) {
	if !ValidSlug(slug) {
		return Section{}, newErr(CodeInvalidSlug, "", "")
	}
	rel := sectionRel(slug)
	data, err := nb.fs.readFile(rel)
	if err != nil {
		if CodeOf(err) == CodeNotFound {
			return Section{}, newErr(CodeNotFound, sectionDirRel(slug), "")
		}
		return Section{}, err
	}
	s, err := ParseSection(slug, data)
	if err != nil {
		return Section{}, withPath(err, rel)
	}
	return s, nil
}

// Section returns one section.
func (nb *Notebook) Section(slug string) (Section, error) { return nb.loadSection(slug) }

// sectionSlugs lists the directory names under sections/ that are valid
// slugs, enforcing the section limit. Symlinks are reported and skipped.
func (nb *Notebook) sectionSlugs() ([]string, error) {
	entries, err := nb.fs.readDir(SectionsDir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if !ValidSlug(name) {
			continue
		}
		rel := sectionDirRel(name)
		if e.Type()&fs.ModeSymlink != 0 {
			nb.problem(rel, newErr(CodeSymlink, rel, ""))
			continue
		}
		if !e.IsDir() {
			continue
		}
		out = append(out, name)
		if len(out) > MaxSections {
			return nil, limitErr(LimitSections, SectionsDir, MaxSections)
		}
	}
	sort.Strings(out)
	return out, nil
}

// Sections returns every valid section, in section order (SPEC §6).
// Invalid sections are skipped and reported through Options.OnProblem.
func (nb *Notebook) Sections() ([]Section, error) {
	slugs, err := nb.sectionSlugs()
	if err != nil {
		return nil, err
	}
	out := make([]Section, 0, len(slugs))
	for _, slug := range slugs {
		s, err := nb.loadSection(slug)
		if err != nil {
			if CodeOf(err) == CodeNotFound {
				// A directory without _section.md is only an error if it
				// holds topic files.
				if names, lerr := nb.topicFileNames(slug); lerr == nil && len(names) > 0 {
					nb.problem(sectionDirRel(slug), newErr(CodeSectionMissing, sectionDirRel(slug), ""))
				}
				continue
			}
			if isLimit(err) {
				return nil, err
			}
			nb.problem(sectionRel(slug), err)
			continue
		}
		out = append(out, s)
	}
	sortSections(out)
	return out, nil
}

func sortSections(s []Section) {
	sort.Slice(s, func(i, j int) bool {
		if s[i].Order != s[j].Order {
			return s[i].Order < s[j].Order
		}
		return s[i].Slug < s[j].Slug
	})
}

func isLimit(err error) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == CodeLimit && e.Limit != LimitFileSize && e.Limit != LimitFrontmatterSize
}

// ---- Topics ----------------------------------------------------------------

// topicFileNames lists the topic slugs of a section directory, enforcing
// the topic limit. Symlinks and special files are reported and skipped.
func (nb *Notebook) topicFileNames(section string) ([]string, error) {
	entries, err := nb.fs.readDir(sectionDirRel(section))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		slug, ok := strings.CutSuffix(e.Name(), ".md")
		if !ok || !ValidSlug(slug) {
			continue
		}
		rel := topicRel(TopicRef{section, slug})
		if e.Type()&fs.ModeSymlink != 0 {
			nb.problem(rel, newErr(CodeSymlink, rel, ""))
			continue
		}
		if !e.Type().IsRegular() {
			nb.problem(rel, newErr(CodeNotRegular, rel, ""))
			continue
		}
		out = append(out, slug)
		if len(out) > MaxTopicsPerSection {
			return nil, limitErr(LimitTopicsPerSection, sectionDirRel(section), MaxTopicsPerSection)
		}
	}
	sort.Strings(out)
	return out, nil
}

func (nb *Notebook) loadTopic(ref TopicRef) (Topic, error) {
	rel := topicRel(ref)
	data, err := nb.fs.readFile(rel)
	if err != nil {
		return Topic{}, err
	}
	t, err := ParseTopic(ref.Section, ref.Topic, data)
	if err != nil {
		return Topic{}, withPath(err, rel)
	}
	return t, nil
}

// Topic returns one topic. Its section must be valid.
func (nb *Notebook) Topic(ref TopicRef) (Topic, error) {
	if !ref.valid() {
		return Topic{}, newErr(CodeInvalidRef, "", "")
	}
	if _, err := nb.loadSection(ref.Section); err != nil {
		return Topic{}, err
	}
	return nb.loadTopic(ref)
}

// Topics returns the valid topics of a section, in topic order (SPEC §6).
func (nb *Notebook) Topics(section string) ([]Topic, error) {
	if _, err := nb.loadSection(section); err != nil {
		return nil, err
	}
	return nb.topicsOf(section)
}

func (nb *Notebook) topicsOf(section string) ([]Topic, error) {
	names, err := nb.topicFileNames(section)
	if err != nil {
		return nil, err
	}
	out := make([]Topic, 0, len(names))
	for _, slug := range names {
		t, err := nb.loadTopic(TopicRef{section, slug})
		if err != nil {
			nb.problem(topicRel(TopicRef{section, slug}), err)
			continue
		}
		out = append(out, t)
	}
	sortTopics(out)
	return out, nil
}

func sortTopics(t []Topic) {
	sort.Slice(t, func(i, j int) bool {
		if t[i].Order != t[j].Order {
			return t[i].Order < t[j].Order
		}
		return t[i].Slug < t[j].Slug
	})
}

// SectionTopics is a section with its topics.
type SectionTopics struct {
	Section Section `json:"section"`
	Topics  []Topic `json:"topics"`
}

// Outline returns every section with its topics, in order.
func (nb *Notebook) Outline() ([]SectionTopics, error) {
	secs, err := nb.Sections()
	if err != nil {
		return nil, err
	}
	out := make([]SectionTopics, 0, len(secs))
	for _, s := range secs {
		ts, err := nb.topicsOf(s.Slug)
		if err != nil {
			return nil, err
		}
		out = append(out, SectionTopics{Section: s, Topics: ts})
	}
	return out, nil
}

// TopicFilter selects topics in [Notebook.FindTopics]. Zero fields match
// everything.
type TopicFilter struct {
	Section string
	Status  Status
	Type    TopicType
}

// FindTopics returns the topics matching f, in section then topic order.
func (nb *Notebook) FindTopics(f TopicFilter) ([]Topic, error) {
	if f.Section != "" {
		ts, err := nb.Topics(f.Section)
		if err != nil {
			return nil, err
		}
		return filterTopics(ts, f), nil
	}
	outline, err := nb.Outline()
	if err != nil {
		return nil, err
	}
	var out []Topic
	for _, st := range outline {
		out = append(out, filterTopics(st.Topics, f)...)
	}
	return out, nil
}

func filterTopics(ts []Topic, f TopicFilter) []Topic {
	out := ts[:0:0]
	for _, t := range ts {
		if (f.Status == "" || t.Status == f.Status) && (f.Type == "" || t.Type == f.Type) {
			out = append(out, t)
		}
	}
	return out
}

// ---- Journal ---------------------------------------------------------------

// entryLoc is the location of a journal file, from its path only.
type entryLoc struct {
	date Date
	id   string
}

func (l entryLoc) rel() string { return entryRel(l.date, l.id) }

// entryLocs lists every journal file name, enforcing the entry limit.
// Symlinks, special files and path mismatches are reported and skipped.
func (nb *Notebook) entryLocs() ([]entryLoc, error) {
	years, err := nb.fs.readDir(JournalDir)
	if err != nil {
		return nil, err
	}
	var out []entryLoc
	for _, y := range years {
		if !yearDirRe.MatchString(y.Name()) {
			continue
		}
		yRel := JournalDir + "/" + y.Name()
		if y.Type()&fs.ModeSymlink != 0 {
			nb.problem(yRel, newErr(CodeSymlink, yRel, ""))
			continue
		}
		if !y.IsDir() {
			continue
		}
		months, err := nb.fs.readDir(yRel)
		if err != nil {
			return nil, err
		}
		for _, m := range months {
			if !monthDirRe.MatchString(m.Name()) {
				continue
			}
			mRel := yRel + "/" + m.Name()
			if m.Type()&fs.ModeSymlink != 0 {
				nb.problem(mRel, newErr(CodeSymlink, mRel, ""))
				continue
			}
			if !m.IsDir() {
				continue
			}
			files, err := nb.fs.readDir(mRel)
			if err != nil {
				return nil, err
			}
			for _, f := range files {
				rel := mRel + "/" + f.Name()
				info, err := ClassifyPath(rel)
				if err != nil {
					nb.problem(rel, err)
					continue
				}
				if info.Kind != PathEntry {
					continue
				}
				if f.Type()&fs.ModeSymlink != 0 {
					nb.problem(rel, newErr(CodeSymlink, rel, ""))
					continue
				}
				if !f.Type().IsRegular() {
					nb.problem(rel, newErr(CodeNotRegular, rel, ""))
					continue
				}
				out = append(out, entryLoc{info.Date, info.ID})
				if len(out) > MaxJournalEntries {
					return nil, limitErr(LimitJournalEntries, JournalDir, MaxJournalEntries)
				}
			}
		}
	}
	return out, nil
}

func (nb *Notebook) loadEntry(l entryLoc) (Entry, error) {
	rel := l.rel()
	data, err := nb.fs.readFile(rel)
	if err != nil {
		return Entry{}, err
	}
	e, err := ParseEntry(l.date, l.id, data)
	if err != nil {
		return Entry{}, withPath(err, rel)
	}
	return e, nil
}

// JournalFilter selects journal entries. Zero fields match everything.
type JournalFilter struct {
	From    Date     // inclusive
	To      Date     // inclusive
	Section string   // entries linked to this section
	Topic   TopicRef // entries linked to this topic
	Limit   int      // maximum number of entries returned (0 = no limit)
}

func (f JournalFilter) matchDate(d Date) bool {
	return (f.From.IsZero() || d.Compare(f.From) >= 0) && (f.To.IsZero() || d.Compare(f.To) <= 0)
}

// Journal returns the entries matching f, newest first (SPEC §6).
//
// References to sections or topics that no longer exist are treated as
// unlinked (SPEC §5.4): they are cleared in the returned entries and
// reported through Options.OnProblem.
func (nb *Notebook) Journal(f JournalFilter) ([]Entry, error) {
	if f.Section != "" && !ValidSlug(f.Section) {
		return nil, newErr(CodeInvalidSlug, "", "section")
	}
	if !f.Topic.IsZero() && !f.Topic.valid() {
		return nil, newErr(CodeInvalidRef, "", "topic")
	}
	locs, err := nb.entryLocs()
	if err != nil {
		return nil, err
	}
	res := newRefResolver(nb)
	var out []Entry
	for _, l := range locs {
		if !f.matchDate(l.date) {
			continue
		}
		e, err := nb.loadEntry(l)
		if err != nil {
			nb.problem(l.rel(), err)
			continue
		}
		res.unlinkDangling(&e, l.rel())
		if f.Section != "" && e.Section != f.Section {
			continue
		}
		if !f.Topic.IsZero() && e.Topic != f.Topic {
			continue
		}
		out = append(out, e)
	}
	sortEntries(out)
	if f.Limit > 0 && len(out) > f.Limit {
		out = out[:f.Limit]
	}
	return out, nil
}

func sortEntries(e []Entry) {
	sort.Slice(e, func(i, j int) bool {
		if c := e[i].Date.Compare(e[j].Date); c != 0 {
			return c > 0
		}
		if !e[i].Created.Equal(e[j].Created) {
			return e[i].Created.After(e[j].Created)
		}
		return e[i].ID < e[j].ID
	})
}

// Entry returns the journal entry with the given id.
func (nb *Notebook) Entry(id string) (Entry, error) {
	l, err := nb.findEntry(id)
	if err != nil {
		return Entry{}, err
	}
	e, err := nb.loadEntry(l)
	if err != nil {
		return Entry{}, err
	}
	newRefResolver(nb).unlinkDangling(&e, l.rel())
	return e, nil
}

func (nb *Notebook) findEntry(id string) (entryLoc, error) {
	if !ValidEntryID(id) {
		return entryLoc{}, newErr(CodeInvalidID, "", "")
	}
	locs, err := nb.entryLocs()
	if err != nil {
		return entryLoc{}, err
	}
	var found []entryLoc
	for _, l := range locs {
		if l.id == id {
			found = append(found, l)
		}
	}
	switch len(found) {
	case 0:
		return entryLoc{}, newErr(CodeNotFound, "", "")
	case 1:
		return found[0], nil
	}
	return entryLoc{}, newErr(CodeAmbiguous, "", "")
}

// refResolver checks journal references against the notebook, caching the
// answers for one operation.
type refResolver struct {
	nb       *Notebook
	sections map[string]bool
	topics   map[TopicRef]bool
}

func newRefResolver(nb *Notebook) *refResolver {
	return &refResolver{nb: nb, sections: map[string]bool{}, topics: map[TopicRef]bool{}}
}

func (r *refResolver) section(slug string) bool {
	ok, seen := r.sections[slug]
	if !seen {
		_, err := r.nb.loadSection(slug)
		ok = err == nil
		r.sections[slug] = ok
	}
	return ok
}

func (r *refResolver) topic(ref TopicRef) bool {
	ok, seen := r.topics[ref]
	if !seen {
		ok = r.section(ref.Section)
		if ok {
			_, err := r.nb.loadTopic(ref)
			ok = err == nil
		}
		r.topics[ref] = ok
	}
	return ok
}

func (r *refResolver) unlinkDangling(e *Entry, rel string) {
	if e.Section != "" && !r.section(e.Section) {
		r.nb.problem(rel, newErr(CodeNotFound, rel, "section"))
		e.Section, e.Topic = "", TopicRef{}
		return
	}
	if !e.Topic.IsZero() && !r.topic(e.Topic) {
		r.nb.problem(rel, newErr(CodeNotFound, rel, "topic"))
		e.Topic = TopicRef{}
	}
}

// ---- Creating notebooks ----------------------------------------------------

// Init creates a new notebook with configuration cfg in dir, which must
// not exist or be an empty directory.
func Init(dir string, cfg Config, opts *Options) (*Notebook, error) {
	return Create(dir, &Tree{Config: cfg}, opts)
}

// Create writes the notebook t into dir, which must not exist or be an
// empty directory. Files are first written to a temporary sibling
// directory, which is then renamed to dir, so a failure leaves nothing
// behind.
func Create(dir string, t *Tree, opts *Options) (*Notebook, error) {
	if t == nil {
		return nil, newErr(CodeNotNotebook, "", "")
	}
	files, err := t.Files()
	if err != nil {
		return nil, err
	}
	target, err := prepareTarget(dir)
	if err != nil {
		return nil, err
	}
	staging, err := os.MkdirTemp(filepath.Dir(target), ".bearings-new-*")
	if err != nil {
		return nil, ioErr("", err)
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, dirMode); err != nil {
		return nil, ioErr("", err)
	}
	sfs := fsys{root: staging}
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		if err := sfs.writeFile(p, files[p], nil); err != nil {
			return nil, err
		}
	}
	// The target is either missing or an empty directory (checked above).
	if fi, err := os.Lstat(target); err == nil && fi.IsDir() {
		if err := os.Remove(target); err != nil {
			return nil, newErr(CodeNotEmpty, "", "")
		}
	}
	if err := os.Rename(staging, target); err != nil {
		_ = os.Mkdir(target, dirMode) // put back the empty directory removed above
		return nil, ioErr("", err)
	}
	ok = true
	syncDir(filepath.Dir(target))
	return Open(target, opts)
}

// prepareTarget resolves dir for Create: its parent is created (0700) if
// needed and resolved to a symlink-free path; dir itself must be missing
// or a real, empty directory.
func prepareTarget(dir string) (string, error) {
	if dir == "" {
		return "", newErr(CodeNotNotebook, "", "")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", ioErr("", err)
	}
	parent, base := filepath.Dir(abs), filepath.Base(abs)
	if base == "." || base == string(filepath.Separator) || base == ".." {
		return "", newErr(CodeNotEmpty, "", "")
	}
	if err := os.MkdirAll(parent, dirMode); err != nil {
		return "", ioErr("", err)
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", ioErr("", err)
	}
	target := filepath.Join(parent, base)
	fi, err := os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return target, nil
	case err != nil:
		return "", ioErr("", err)
	case fi.Mode()&fs.ModeSymlink != 0:
		return "", newErr(CodeSymlink, "", "")
	case !fi.IsDir():
		return "", newErr(CodeNotEmpty, "", "")
	}
	d, err := os.Open(target) //nolint:gosec // G304: the directory the user asked to create the notebook in
	if err != nil {
		return "", ioErr("", err)
	}
	defer d.Close()
	names, err := d.Readdirnames(1)
	if len(names) > 0 {
		return "", newErr(CodeNotEmpty, "", "")
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return "", ioErr("", err)
	}
	return target, nil
}
