package notebook

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// This file is the only place that touches the filesystem. It implements
// SPEC §8 "Filesystem":
//
//   - the root is resolved once to an absolute, symlink-free path;
//   - every path is built from a slash-separated relative path whose parts
//     were validated by the caller, and is checked to stay inside the root;
//   - symlinks and special files are never followed: every directory on the
//     way and the final file are checked with lstat, and files are opened
//     with O_NOFOLLOW where the platform has it, then compared with fstat;
//   - writes go to a temporary file in the same directory, are fsynced and
//     renamed over the target; the directory is fsynced afterwards;
//   - new files are created 0600 and new directories 0700;
//   - an overwrite first checks that the file still has the content it had
//     when it was read (conflict detection).

const (
	fileMode = 0o600
	dirMode  = 0o700
	tmpGlob  = ".bearings-*.tmp"
)

// testHookBeforeRename, if set, is called after the temporary file is
// written and synced and before it is renamed over the target. Tests use it
// to simulate a failure at the last moment.
var testHookBeforeRename func(tmpPath string) error

type fsys struct {
	root string // absolute, symlink-free, cleaned
}

// resolveRoot returns the absolute, symlink-free form of dir, which must be
// an existing directory.
func resolveRoot(dir string) (string, error) {
	if dir == "" {
		return "", newErr(CodeNotNotebook, "", "")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", ioErr("", err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", newErr(CodeNotNotebook, "", "")
		}
		return "", ioErr("", err)
	}
	fi, err := os.Stat(resolved)
	if err != nil {
		return "", ioErr("", err)
	}
	if !fi.IsDir() {
		return "", newErr(CodeNotNotebook, "", "")
	}
	return filepath.Clean(resolved), nil
}

// abs converts a validated relative path to an absolute one inside root.
// It refuses anything that is not a plain relative path; this is a second
// line of defense, callers only pass paths built from validated parts.
func (f *fsys) abs(rel string) (string, error) {
	if rel == "" || strings.ContainsAny(rel, "\\:\x00") || strings.HasPrefix(rel, "/") {
		return "", newErr(CodeOutsideRoot, "", "")
	}
	for _, p := range strings.Split(rel, "/") {
		if p == "" || p == "." || p == ".." {
			return "", newErr(CodeOutsideRoot, "", "")
		}
	}
	p := filepath.Join(f.root, filepath.FromSlash(rel))
	r, err := filepath.Rel(f.root, p)
	if err != nil || r == ".." || strings.HasPrefix(r, ".."+string(filepath.Separator)) || filepath.IsAbs(r) {
		return "", newErr(CodeOutsideRoot, "", "")
	}
	return p, nil
}

// lstat returns the lstat result of rel, after checking that every parent
// directory below the root is a real directory (not a symlink).
func (f *fsys) lstat(rel string) (fs.FileInfo, error) {
	if err := f.checkParents(rel); err != nil {
		return nil, err
	}
	p, err := f.abs(rel)
	if err != nil {
		return nil, err
	}
	fi, err := os.Lstat(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, newErr(CodeNotFound, rel, "")
		}
		return nil, ioErr(rel, err)
	}
	return fi, nil
}

// checkParents checks each directory between the root and rel.
func (f *fsys) checkParents(rel string) error {
	parts := strings.Split(rel, "/")
	for i := 1; i < len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		p, err := f.abs(dir)
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return newErr(CodeNotFound, rel, "")
			}
			return ioErr(dir, err)
		}
		if err := checkDirInfo(dir, fi); err != nil {
			return err
		}
	}
	return nil
}

func checkDirInfo(rel string, fi fs.FileInfo) error {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return newErr(CodeSymlink, rel, "")
	case !fi.IsDir():
		return newErr(CodeNotRegular, rel, "")
	}
	return nil
}

func checkFileInfo(rel string, fi fs.FileInfo) error {
	switch {
	case fi.Mode()&fs.ModeSymlink != 0:
		return newErr(CodeSymlink, rel, "")
	case !fi.Mode().IsRegular():
		return newErr(CodeNotRegular, rel, "")
	}
	return nil
}

// readFile reads a regular file of at most MaxFileSize bytes.
func (f *fsys) readFile(rel string) ([]byte, error) {
	fi, err := f.lstat(rel)
	if err != nil {
		return nil, err
	}
	if err := checkFileInfo(rel, fi); err != nil {
		return nil, err
	}
	if fi.Size() > MaxFileSize {
		return nil, limitErr(LimitFileSize, rel, MaxFileSize)
	}
	p, err := f.abs(rel)
	if err != nil {
		return nil, err
	}
	fh, err := openNoFollow(p)
	if err != nil {
		if isSymlinkErr(err) {
			return nil, newErr(CodeSymlink, rel, "")
		}
		if errors.Is(err, fs.ErrNotExist) {
			return nil, newErr(CodeNotFound, rel, "")
		}
		return nil, ioErr(rel, err)
	}
	defer fh.Close()
	ofi, err := fh.Stat()
	if err != nil {
		return nil, ioErr(rel, err)
	}
	// The file must still be the one that was checked with lstat.
	if !ofi.Mode().IsRegular() || !os.SameFile(fi, ofi) {
		return nil, newErr(CodeNotRegular, rel, "")
	}
	data, err := io.ReadAll(io.LimitReader(fh, MaxFileSize+1))
	if err != nil {
		return nil, ioErr(rel, err)
	}
	if len(data) > MaxFileSize {
		return nil, limitErr(LimitFileSize, rel, MaxFileSize)
	}
	return data, nil
}

// exists reports whether rel exists (as anything, including a symlink).
func (f *fsys) exists(rel string) (bool, error) {
	_, err := f.lstat(rel)
	switch {
	case err == nil:
		return true, nil
	case CodeOf(err) == CodeNotFound:
		return false, nil
	}
	return false, err
}

// readDir lists a real directory. A missing directory yields no entries.
func (f *fsys) readDir(rel string) ([]fs.DirEntry, error) {
	fi, err := f.lstat(rel)
	if err != nil {
		if CodeOf(err) == CodeNotFound {
			return nil, nil
		}
		return nil, err
	}
	if err := checkDirInfo(rel, fi); err != nil {
		return nil, err
	}
	p, err := f.abs(rel)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(p)
	if err != nil {
		return nil, ioErr(rel, err)
	}
	return entries, nil
}

// ensureDir creates rel and its missing parents with mode 0700, refusing
// to go through symlinks or non-directories.
func (f *fsys) ensureDir(rel string) error {
	parts := strings.Split(rel, "/")
	for i := 1; i <= len(parts); i++ {
		dir := strings.Join(parts[:i], "/")
		p, err := f.abs(dir)
		if err != nil {
			return err
		}
		fi, err := os.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			if err := os.Mkdir(p, dirMode); err != nil && !errors.Is(err, fs.ErrExist) {
				return ioErr(dir, err)
			}
			fi, err = os.Lstat(p)
		}
		if err != nil {
			return ioErr(dir, err)
		}
		if err := checkDirInfo(dir, fi); err != nil {
			return err
		}
	}
	return nil
}

// checkCurrent verifies the state of the target before a write or a
// removal: with expect == nil the file must not exist; otherwise it must
// be a regular file whose content still matches expect.
func (f *fsys) checkCurrent(rel string, expect *Version) error {
	if expect == nil {
		ok, err := f.exists(rel)
		if err != nil {
			return err
		}
		if ok {
			return newErr(CodeExists, rel, "")
		}
		return nil
	}
	cur, err := f.readFile(rel)
	if err != nil {
		if CodeOf(err) == CodeNotFound {
			return newErr(CodeConflict, rel, "")
		}
		return err
	}
	if versionOf(cur) != *expect {
		return newErr(CodeConflict, rel, "")
	}
	return nil
}

// writeFile atomically writes data to rel. With expect == nil the file is
// created and must not exist; otherwise it is replaced only if it still
// has the content identified by expect. On any error the previous file is
// left untouched and the temporary file is removed.
func (f *fsys) writeFile(rel string, data []byte, expect *Version) (err error) {
	if len(data) > MaxFileSize {
		return limitErr(LimitFileSize, rel, MaxFileSize)
	}
	dirRel := rel[:max(strings.LastIndexByte(rel, '/'), 0)]
	if dirRel != "" {
		if err := f.ensureDir(dirRel); err != nil {
			return err
		}
	}
	if err := f.checkCurrent(rel, expect); err != nil {
		return err
	}
	target, err := f.abs(rel)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), tmpGlob)
	if err != nil {
		return ioErr(rel, err)
	}
	tmpPath := tmp.Name()
	defer func() {
		if err != nil {
			_ = tmp.Close()
			_ = os.Remove(tmpPath)
		}
	}()
	if err = tmp.Chmod(fileMode); err != nil && !isChmodUnsupported(err) {
		return ioErr(rel, err)
	}
	if _, err = tmp.Write(data); err != nil {
		return ioErr(rel, err)
	}
	if err = tmp.Sync(); err != nil {
		return ioErr(rel, err)
	}
	if err = tmp.Close(); err != nil {
		return ioErr(rel, err)
	}
	if testHookBeforeRename != nil {
		if err = testHookBeforeRename(tmpPath); err != nil {
			return ioErr(rel, err)
		}
	}
	// Check again right before replacing, to keep the window small.
	if err = f.checkCurrent(rel, expect); err != nil {
		return err
	}
	if err = os.Rename(tmpPath, target); err != nil {
		return ioErr(rel, err)
	}
	syncDir(filepath.Dir(target))
	return nil
}

// removeFile removes rel if its content still matches expect.
func (f *fsys) removeFile(rel string, expect Version) error {
	if err := f.checkCurrent(rel, &expect); err != nil {
		return err
	}
	p, err := f.abs(rel)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		return ioErr(rel, err)
	}
	syncDir(filepath.Dir(p))
	return nil
}

// removeEmptyDir removes rel if it is a real, empty directory. Anything
// else is left alone.
func (f *fsys) removeEmptyDir(rel string) {
	fi, err := f.lstat(rel)
	if err != nil || checkDirInfo(rel, fi) != nil {
		return
	}
	p, err := f.abs(rel)
	if err != nil {
		return
	}
	_ = os.Remove(p) // fails, as wanted, if the directory is not empty
}

// renameDir renames the real directory from to the new path to, which
// must not exist.
func (f *fsys) renameDir(from, to string) error {
	fi, err := f.lstat(from)
	if err != nil {
		return err
	}
	if err := checkDirInfo(from, fi); err != nil {
		return err
	}
	if err := f.checkCurrent(to, nil); err != nil {
		return err
	}
	pf, err := f.abs(from)
	if err != nil {
		return err
	}
	pt, err := f.abs(to)
	if err != nil {
		return err
	}
	if err := os.Rename(pf, pt); err != nil {
		return ioErr(from, err)
	}
	syncDir(filepath.Dir(pt))
	return nil
}

func isChmodUnsupported(err error) bool {
	return errors.Is(err, errors.ErrUnsupported)
}
