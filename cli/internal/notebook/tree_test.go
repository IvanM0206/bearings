package notebook

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func baseTree() map[string][]byte {
	return map[string][]byte{
		"bearings.yaml":                            []byte("format: 1\n"),
		"sections/technical/_section.md":           []byte(validSection),
		"sections/technical/backtesting.md":        []byte(validTopic),
		"journal/2026/11/2026-11-03-7k2mq9xr4t.md": []byte(strings.Replace(validEntry, "topic: technical/backtesting\n", "", 1)),
		"README.md":    []byte("ignored"),
		".git/HEAD":    []byte("ignored"),
		"sections/x/y": []byte("ignored"),
	}
}

func TestParseTree(t *testing.T) {
	tr, err := ParseTree(baseTree())
	if err != nil {
		t.Fatal(err)
	}
	if len(tr.Sections) != 1 || len(tr.Topics) != 1 || len(tr.Entries) != 1 {
		t.Fatalf("%+v", tr)
	}
	cases := map[string]func(map[string][]byte){
		"missing config":        func(m map[string][]byte) { delete(m, "bearings.yaml") },
		"bad config":            func(m map[string][]byte) { m["bearings.yaml"] = []byte("format: 3\n") },
		"bad topic":             func(m map[string][]byte) { m["sections/technical/x.md"] = []byte("---\ntitle: &a x\n---\n") },
		"topic without section": func(m map[string][]byte) { m["sections/orphan/x.md"] = []byte(validTopic) },
		"path mismatch":         func(m map[string][]byte) { m["journal/2026/12/2026-11-03-7k2mq9xr4t.md"] = []byte(validEntry) },
		"too deep":              func(m map[string][]byte) { m["a/b/c/d/e"] = []byte("x") },
		"file too large":        func(m map[string][]byte) { m["sections/technical/backtesting.md"] = make([]byte, MaxFileSize+1) },
	}
	want := map[string]Code{
		"missing config": CodeNotNotebook, "bad config": CodeUnsupportedFormat, "bad topic": CodeYAMLAnchor,
		"topic without section": CodeSectionMissing, "path mismatch": CodePathMismatch, "too deep": CodeLimit, "file too large": CodeLimit,
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			m := baseTree()
			mut(m)
			_, err := ParseTree(m)
			wantCode(t, err, want[name])
		})
	}
}

func TestParseTreeLimits(t *testing.T) {
	m := map[string][]byte{"bearings.yaml": []byte("format: 1\n")}
	for i := 0; i <= MaxSections; i++ {
		m[fmt.Sprintf("sections/s%d/_section.md", i)] = []byte(validSection)
	}
	_, err := ParseTree(m)
	if e := (*Error)(nil); !errors.As(err, &e) || e.Limit != LimitSections {
		t.Fatalf("sections: %v", err)
	}
	m = map[string][]byte{"bearings.yaml": []byte("format: 1\n"), "sections/s/_section.md": []byte(validSection)}
	for i := 0; i <= MaxTopicsPerSection; i++ {
		m[fmt.Sprintf("sections/s/t%d.md", i)] = []byte(validTopic)
	}
	_, err = ParseTree(m)
	if e := (*Error)(nil); !errors.As(err, &e) || e.Limit != LimitTopicsPerSection {
		t.Fatalf("topics: %v", err)
	}
}

func TestCreateFromTree(t *testing.T) {
	tr, err := ParseTree(baseTree())
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "a", "b", "nb")
	nb, err := Create(dir, tr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := nb.Topic(TopicRef{"technical", "backtesting"}); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"README.md", ".git"} {
		if _, err := os.Stat(filepath.Join(dir, p)); !os.IsNotExist(err) {
			t.Fatalf("unknown file %s was written", p)
		}
	}
	// No staging directory is left behind.
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Fatalf("leftovers: %v", entries)
	}
	if _, err := Create(dir, tr, nil); CodeOf(err) != CodeNotEmpty {
		t.Fatalf("create over existing: %v", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Create(file, tr, nil); CodeOf(err) != CodeNotEmpty {
		t.Fatalf("create over a file: %v", err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(t.TempDir(), link); err == nil {
		if _, err := Create(link, tr, nil); CodeOf(err) != CodeSymlink {
			t.Fatalf("create over a symlink: %v", err)
		}
	}
	// A failure while writing leaves nothing behind.
	bad := &Tree{Config: Config{Format: 1}, Topics: []Topic{{Section: "nope", Slug: "x"}}}
	dir2 := filepath.Join(t.TempDir(), "nb2")
	if _, err := Create(dir2, bad, nil); err == nil {
		t.Fatal("expected error")
	}
	if entries, _ := os.ReadDir(filepath.Dir(dir2)); len(entries) != 0 {
		t.Fatalf("leftovers: %v", entries)
	}
}
