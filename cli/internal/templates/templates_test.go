package templates

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListAndInstantiate(t *testing.T) {
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) < 2 {
		t.Fatalf("expected the en and es templates, got %+v", list)
	}
	langs := map[string]bool{}
	for _, info := range list {
		langs[info.Language] = true
		if info.Sections == 0 || info.Topics == 0 || info.Title == "" {
			t.Errorf("empty template %+v", info)
		}
		now := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
		tr, err := Instantiate(info.ID, "Mine", now)
		if err != nil {
			t.Fatal(err)
		}
		if tr.Config.Template != nil || tr.Config.Name != "Mine" {
			t.Errorf("config not instantiated: %+v", tr.Config)
		}
		for _, tp := range tr.Topics {
			if !tp.Created.Equal(now) || !tp.Updated.Equal(now) {
				t.Errorf("timestamps not reset: %+v", tp)
			}
		}
		if _, err := tr.Files(); err != nil {
			t.Errorf("%s does not encode: %v", info.ID, err)
		}
	}
	if !langs["en"] || !langs["es"] {
		t.Fatalf("languages: %v", langs)
	}
	if _, err := Get("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if _, err := Get("../files"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

// TestEmbeddedCopyIsUpToDate compares the generated copy with the source
// templates when the repository is available (it is in CI and locally).
func TestEmbeddedCopyIsUpToDate(t *testing.T) {
	src := filepath.Join("..", "..", "..", "templates")
	if _, err := os.Stat(src); err != nil {
		t.Skip("source templates not available")
	}
	want := map[string][]byte{}
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name()[0] == '.' {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		data, err := os.ReadFile(p) //nolint:gosec // G122: test reads the repository templates
		want[filepath.ToSlash(rel)] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string][]byte{}
	err = fs.WalkDir(embedded, root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := embedded.ReadFile(p)
		got[p[len(root)+1:]] = data
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for p, w := range want {
		if !bytes.Equal(got[p], w) {
			t.Errorf("%s differs from templates/: run go generate ./...", p)
		}
	}
	for p := range got {
		if _, ok := want[p]; !ok {
			t.Errorf("%s is not in templates/: run go generate ./...", p)
		}
	}
}
