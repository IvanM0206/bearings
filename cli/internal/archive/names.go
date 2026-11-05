package archive

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// checkName validates an entry name and reports whether it names a
// directory (trailing slash). It returns the name's segments.
//
// Refused: empty names, names longer than MaxNameLen, invalid UTF-8, NUL
// and every other C0/C1 control character and DEL, backslashes, colons
// (drive letters such as "C:", NTFS alternate streams), absolute paths,
// and segments that are empty, "." or "..", also once trailing dots and
// spaces are removed (Windows turns ".. " into "..").
func checkName(name string) (segments []string, isDir bool, err error) {
	if name == "" || len(name) > MaxNameLen || !utf8.ValidString(name) {
		return nil, false, newErr(CodeBadName, name)
	}
	for _, r := range name {
		if r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f) || r == '\\' || r == ':' {
			return nil, false, newErr(CodeBadName, name)
		}
	}
	if name[0] == '/' {
		return nil, false, newErr(CodeBadName, name)
	}
	trimmed, isDir := strings.CutSuffix(name, "/")
	segments = strings.Split(trimmed, "/")
	for _, s := range segments {
		w := strings.TrimRight(s, ". ")
		if s == "" || s == "." || s == ".." || w == "" {
			return nil, false, newErr(CodeBadName, name)
		}
	}
	if len(segments) > notebook.MaxDepth {
		return nil, false, limitErr(CodeDepth, name, notebook.MaxDepth)
	}
	return segments, isDir, nil
}

// foldKey returns the form of a path used to find duplicates: each
// segment without trailing dots and spaces (ignored by Windows), then
// case-folded with Unicode simple folding (so "K", "k" and the Kelvin sign
// U+212A, which case-insensitive file systems treat as one name, give the
// same key).
func foldKey(segments []string) string {
	var b strings.Builder
	for i, s := range segments {
		if i > 0 {
			b.WriteByte('/')
		}
		for _, r := range strings.TrimRight(s, ". ") {
			b.WriteRune(foldRune(r))
		}
	}
	return b.String()
}

// foldRune maps r to the smallest rune of its case-folding orbit.
func foldRune(r rune) rune {
	m := r
	for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
		if f < m {
			m = f
		}
	}
	return m
}

// nameSet finds duplicate names after normalization, including a name used
// both as a file and as a directory (explicitly, or as the parent of
// another entry).
type nameSet struct {
	files map[string]bool
	dirs  map[string]bool // directory entries and parents of entries
	exact map[string]bool // explicit directory entries, to refuse their duplicates
}

func newNameSet() *nameSet {
	return &nameSet{files: map[string]bool{}, dirs: map[string]bool{}, exact: map[string]bool{}}
}

func (s *nameSet) add(name string, segments []string, isDir bool) error {
	key := foldKey(segments)
	if s.files[key] || (!isDir && s.dirs[key]) || (isDir && s.exact[key]) {
		return newErr(CodeDuplicate, name)
	}
	for i := 1; i < len(segments); i++ {
		p := foldKey(segments[:i])
		if s.files[p] {
			return newErr(CodeDuplicate, name)
		}
		s.dirs[p] = true
	}
	if isDir {
		s.dirs[key] = true
		s.exact[key] = true
	} else {
		s.files[key] = true
	}
	return nil
}
