//go:build unix

package server

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A symlink inside the notebook is never followed, even when its target is
// a perfectly valid notebook file (so a template or archive cannot make the
// server read or overwrite files elsewhere).
func TestSymlinksAreNotFollowed(t *testing.T) {
	const marker = "OUTSIDE_SECRET_7c1d"
	dir, nb := buildNotebook(t)
	outside := t.TempDir()
	topic := "---\ntitle: " + marker + "\nstatus: pending\ntype: role\norder: 9\ncreated: 2026-11-01T12:00:00Z\nupdated: 2026-11-01T12:00:00Z\n---\n" + marker + "\n"
	must(t, os.WriteFile(filepath.Join(outside, "evil.md"), []byte(topic), 0o600))
	must(t, os.Symlink(filepath.Join(outside, "evil.md"), filepath.Join(dir, "sections", "technical", "evil.md")))
	// A whole section that is a symlink to a directory outside.
	must(t, os.MkdirAll(filepath.Join(outside, "sec"), 0o700))
	must(t, os.Symlink(filepath.Join(outside, "sec"), filepath.Join(dir, "sections", "linked")))
	// A journal entry that is a symlink.
	must(t, os.MkdirAll(filepath.Join(dir, "journal", "2026", "11"), 0o700))
	entry := "---\ndate: 2026-11-03\ncreated: 2026-11-03T09:00:00Z\nupdated: 2026-11-03T09:00:00Z\n---\n" + marker + "\n"
	must(t, os.WriteFile(filepath.Join(outside, "entry.md"), []byte(entry), 0o600))
	must(t, os.Symlink(filepath.Join(outside, "entry.md"), filepath.Join(dir, "journal", "2026", "11", "2026-11-03-aaaaaaaaaa.md")))

	e := newEnvFor(t, dir, nb, false)
	for _, target := range []string{
		"/api/v1/outline", "/api/v1/sections", "/api/v1/sections/technical/topics", "/api/v1/topics/technical/evil",
		"/api/v1/sections/linked", "/api/v1/journal", "/api/v1/journal/aaaaaaaaaa", "/api/v1/search?q=" + marker,
		"/api/v1/role-path", "/api/v1/progress",
	} {
		rec := e.do("GET", target, nil)
		if strings.Contains(rec.Body.String(), marker) {
			t.Errorf("GET %s followed a symlink: %s", target, rec.Body)
		}
		if rec.Code >= 500 && target != "/api/v1/topics/technical/evil" {
			t.Errorf("GET %s: %d", target, rec.Code)
		}
	}
	// Writes through the link are refused and the target is untouched.
	for _, c := range []struct {
		method, target string
		body           any
	}{
		{"POST", "/api/v1/topics/technical/evil/advance", nil},
		{"PATCH", "/api/v1/topics/technical/evil", map[string]any{"version": "1-" + strings.Repeat("0", 64), "title": "x"}},
		{"DELETE", "/api/v1/topics/technical/evil", nil},
		{"DELETE", "/api/v1/journal/aaaaaaaaaa", nil},
		{"POST", "/api/v1/sections/linked/topics", map[string]any{"title": "Planted"}},
	} {
		rec := e.do(c.method, c.target, c.body)
		if rec.Code < 400 {
			t.Errorf("%s %s succeeded through a symlink: %d", c.method, c.target, rec.Code)
		}
	}
	if got := readFile(t, filepath.Join(outside, "evil.md")); got != topic {
		t.Error("the file behind the symlink was changed")
	}
	if got := readFile(t, filepath.Join(outside, "entry.md")); got != entry {
		t.Error("the entry behind the symlink was changed")
	}
	if ents, _ := os.ReadDir(filepath.Join(outside, "sec")); len(ents) != 0 {
		t.Errorf("files planted outside the notebook: %v", ents)
	}
	// Deleting the section whose directory holds a symlink is refused too.
	if rec := e.do("DELETE", "/api/v1/sections/technical", nil); rec.Code != 409 || errCode(t, rec) != "section_not_deletable" {
		t.Errorf("delete section with a symlink inside: %d %s", rec.Code, rec.Body)
	}
	if _, err := os.Stat(filepath.Join(outside, "evil.md")); err != nil {
		t.Error("target removed")
	}
}
