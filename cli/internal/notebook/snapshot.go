package notebook

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// LimitTotalSize is the limit name used by [Notebook.Snapshot] when the
// files of the notebook add up to more than the requested maximum.
const LimitTotalSize = "total_size"

// Snapshot reads every file of the notebook layout (SPEC §2) and returns
// their raw contents keyed by slash-separated relative path. It is meant
// for exporting the whole notebook: unlike the read methods, which skip
// and report what they cannot use, it fails closed. A symlink or special
// file at a layout path, a journal file whose directories do not match
// its name, or any SPEC §7 limit (and maxTotal, the sum of all file sizes,
// when positive) is an error naming the path.
//
// Paths that are not part of the layout (.git, README.md, editor files)
// are never read. The contents are not validated here: pass the result to
// [ParseTree] for that.
func (nb *Notebook) Snapshot(maxTotal int64) (map[string][]byte, error) {
	s := snapshotWalk{nb: nb, files: map[string][]byte{}, maxTotal: maxTotal}
	if err := s.add(ConfigFile); err != nil {
		if CodeOf(err) == CodeNotFound {
			return nil, newErr(CodeNotNotebook, ConfigFile, "")
		}
		return nil, err
	}
	if err := s.sections(); err != nil {
		return nil, err
	}
	if err := s.journal(); err != nil {
		return nil, err
	}
	return s.files, nil
}

type snapshotWalk struct {
	nb       *Notebook
	files    map[string][]byte
	total    int64
	maxTotal int64
}

func (s *snapshotWalk) add(rel string) error {
	data, err := s.nb.fs.readFile(rel)
	if err != nil {
		return err
	}
	s.total += int64(len(data))
	if s.maxTotal > 0 && s.total > s.maxTotal {
		return limitErr(LimitTotalSize, "", s.maxTotal)
	}
	s.files[rel] = data
	return nil
}

// layoutEntry rejects a symlink or a non-regular file found at a layout path.
func layoutEntry(rel string, e fs.DirEntry) error {
	switch {
	case e.Type()&fs.ModeSymlink != 0:
		return newErr(CodeSymlink, rel, "")
	case !e.Type().IsRegular():
		return newErr(CodeNotRegular, rel, "")
	}
	return nil
}

// layoutDir reports whether a directory entry found where the layout
// expects a directory should be walked. A symlink is an error; a plain
// file with that name is not part of the layout and is ignored.
func layoutDir(rel string, e fs.DirEntry) (bool, error) {
	if e.Type()&fs.ModeSymlink != 0 {
		return false, newErr(CodeSymlink, rel, "")
	}
	return e.IsDir(), nil
}

func (s *snapshotWalk) sections() error {
	entries, err := s.nb.fs.readDir(SectionsDir)
	if err != nil {
		return err
	}
	n := 0
	for _, e := range entries {
		slug := e.Name()
		if !ValidSlug(slug) {
			continue
		}
		dirRel := sectionDirRel(slug)
		walk, err := layoutDir(dirRel, e)
		if err != nil {
			return err
		}
		if !walk {
			continue
		}
		if n++; n > MaxSections {
			return limitErr(LimitSections, SectionsDir, MaxSections)
		}
		files, err := s.nb.fs.readDir(dirRel)
		if err != nil {
			return err
		}
		topics := 0
		for _, f := range files {
			name := f.Name()
			topic, isMD := strings.CutSuffix(name, ".md")
			if name != SectionFile && (!isMD || !ValidSlug(topic)) {
				continue
			}
			rel := dirRel + "/" + name
			if err := layoutEntry(rel, f); err != nil {
				return err
			}
			if name != SectionFile {
				if topics++; topics > MaxTopicsPerSection {
					return limitErr(LimitTopicsPerSection, dirRel, MaxTopicsPerSection)
				}
			}
			if err := s.add(rel); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *snapshotWalk) journal() error {
	years, err := s.nb.fs.readDir(JournalDir)
	if err != nil {
		return err
	}
	n := 0
	for _, y := range years {
		if !yearDirRe.MatchString(y.Name()) {
			continue
		}
		yRel := JournalDir + "/" + y.Name()
		walk, err := layoutDir(yRel, y)
		if err != nil {
			return err
		}
		if !walk {
			continue
		}
		months, err := s.nb.fs.readDir(yRel)
		if err != nil {
			return err
		}
		for _, m := range months {
			if !monthDirRe.MatchString(m.Name()) {
				continue
			}
			mRel := yRel + "/" + m.Name()
			walk, err := layoutDir(mRel, m)
			if err != nil {
				return err
			}
			if !walk {
				continue
			}
			files, err := s.nb.fs.readDir(mRel)
			if err != nil {
				return err
			}
			for _, f := range files {
				rel := mRel + "/" + f.Name()
				info, err := ClassifyPath(rel)
				if err != nil {
					return err
				}
				if info.Kind != PathEntry {
					continue
				}
				if err := layoutEntry(rel, f); err != nil {
					return err
				}
				if n++; n > MaxJournalEntries {
					return limitErr(LimitJournalEntries, JournalDir, MaxJournalEntries)
				}
				if err := s.add(rel); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// CheckNewTarget reports whether [Create] could write a notebook into dir:
// dir must not exist, or be a real, empty directory. Unlike Create it
// changes nothing, so callers can check before asking for confirmation.
// Create checks again when it runs.
func CheckNewTarget(dir string) error {
	if dir == "" {
		return newErr(CodeNotNotebook, "", "")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return ioErr("", err)
	}
	fi, err := os.Lstat(abs)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return ioErr("", err)
	case fi.Mode()&fs.ModeSymlink != 0:
		return newErr(CodeSymlink, "", "")
	case !fi.IsDir():
		return newErr(CodeNotEmpty, "", "")
	}
	d, err := os.Open(abs) //nolint:gosec // G304: the directory the user chose for a new notebook, only listed
	if err != nil {
		return ioErr("", err)
	}
	defer d.Close()
	names, err := d.Readdirnames(1)
	if len(names) > 0 {
		return newErr(CodeNotEmpty, "", "")
	}
	if err != nil && !errors.Is(err, io.EOF) {
		return ioErr("", err)
	}
	return nil
}
