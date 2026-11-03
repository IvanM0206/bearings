// Package templates gives access to the starter notebooks embedded in the
// binary. They are copied from the repository's templates/ directory by
// `go generate` (see gen/) into files/, which is committed; CI checks that
// the copy is up to date. Templates are never fetched from the network.
//
// Embedded templates are still treated as untrusted input: they go through
// the same validation as any other notebook content.
package templates

//go:generate go run ./gen ../../../templates files

import (
	"embed"
	"errors"
	"io/fs"
	"path"
	"sort"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// "all:" is required: files starting with "_" (such as _section.md) are
// otherwise left out. The generator never copies hidden files.
//
//go:embed all:files
var embedded embed.FS

const root = "files"

// ErrNotFound is returned for an unknown template id.
var ErrNotFound = errors.New("templates: unknown template")

// ErrHasJournal is returned for a template that contains journal entries
// (SPEC §1: a template is a notebook without journal entries).
var ErrHasJournal = errors.New("templates: template contains journal entries")

// Info describes a template.
type Info struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Sections    int    `json:"sections"`
	Topics      int    `json:"topics"`
}

// List returns every embedded template, sorted by id.
func List() ([]Info, error) {
	dirs, err := fs.ReadDir(embedded, root)
	if err != nil {
		return nil, err
	}
	var out []Info
	for _, d := range dirs {
		if !d.IsDir() || !notebook.ValidSlug(d.Name()) {
			continue
		}
		t, err := load(d.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, info(t))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func info(t *notebook.Tree) Info {
	return Info{
		ID: t.Config.Template.ID, Title: t.Config.Template.Title, Description: t.Config.Template.Description,
		Language: t.Config.Language, Sections: len(t.Sections), Topics: len(t.Topics),
	}
}

// load reads and validates one template.
func load(id string) (*notebook.Tree, error) {
	if !notebook.ValidSlug(id) {
		return nil, ErrNotFound
	}
	dir := path.Join(root, id)
	if _, err := fs.Stat(embedded, path.Join(dir, notebook.ConfigFile)); err != nil {
		return nil, ErrNotFound
	}
	files := map[string][]byte{}
	err := fs.WalkDir(embedded, dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		if fi.Size() > notebook.MaxFileSize {
			return &notebook.Error{Code: notebook.CodeLimit, Limit: notebook.LimitFileSize, Max: notebook.MaxFileSize}
		}
		data, err := embedded.ReadFile(p)
		if err != nil {
			return err
		}
		files[p[len(dir)+1:]] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	t, err := notebook.ParseTree(files)
	if err != nil {
		return nil, err
	}
	if t.Config.Template == nil || t.Config.Template.ID != id {
		return nil, &notebook.Error{Code: notebook.CodeInvalidValue, Path: notebook.ConfigFile, Field: "template.id"}
	}
	if len(t.Entries) > 0 {
		return nil, ErrHasJournal
	}
	return t, nil
}

// Get returns the description of one template.
func Get(id string) (Info, error) {
	t, err := load(id)
	if err != nil {
		return Info{}, err
	}
	return info(t), nil
}

// Instantiate returns the content of a new notebook started from the
// template id: the template mapping is dropped from bearings.yaml, name is
// set (if not empty), and every section and topic gets created = updated =
// now. Topics keep their `suggested` flag so the user can review them.
func Instantiate(id, name string, now time.Time) (*notebook.Tree, error) {
	t, err := load(id)
	if err != nil {
		return nil, err
	}
	now = now.UTC().Truncate(time.Second)
	t.Config.Template = nil
	t.Config.Name = name
	for i := range t.Sections {
		t.Sections[i].Created, t.Sections[i].Updated = now, now
	}
	for i := range t.Topics {
		t.Topics[i].Created, t.Topics[i].Updated = now, now
	}
	return t, nil
}
