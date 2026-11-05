package archive

import (
	"archive/zip"
	"bufio"
	"bytes"
	"compress/flate"
	"errors"
	"hash/crc32"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// Options configures an export.
type Options struct {
	// RoleOnly leaves out the journal and every topic of type personal, so
	// the archive can be shared as a guide or a template.
	RoleOnly bool
}

// Summary counts what an archive holds.
type Summary struct {
	Name     string // notebook name ("" when not set)
	Sections int
	Topics   int
	Entries  int // journal entries
	Files    int // entries in the archive
}

// SummaryOf counts the contents of a notebook tree.
func SummaryOf(t *notebook.Tree) Summary {
	return Summary{Name: t.Config.Name, Sections: len(t.Sections), Topics: len(t.Topics), Entries: len(t.Entries),
		Files: 1 + len(t.Sections) + len(t.Topics) + len(t.Entries)}
}

// Plan is a validated export, ready to be written.
type Plan struct {
	paths   []string
	files   map[string][]byte
	Summary Summary
}

// Prepare validates the raw files of a notebook, as returned by
// [notebook.Notebook.Snapshot], with [notebook.ParseTree] (failing on the
// first invalid file, which the error names), keeps only layout paths
// (without the journal and personal topics when opts.RoleOnly), and checks
// that the result stays within the limits the reader enforces.
func Prepare(files map[string][]byte, opts Options) (*Plan, error) {
	tree, err := notebook.ParseTree(files)
	if err != nil {
		return nil, err
	}
	personal := map[notebook.TopicRef]bool{}
	for _, t := range tree.Topics {
		if t.Type == notebook.TypePersonal {
			personal[t.Ref()] = true
		}
	}
	p := &Plan{files: map[string][]byte{}}
	var total int64
	for path, data := range files {
		info, err := notebook.ClassifyPath(path)
		if err != nil {
			return nil, err
		}
		switch info.Kind {
		case notebook.PathUnknown:
			continue
		case notebook.PathEntry:
			if opts.RoleOnly {
				continue
			}
		case notebook.PathTopic:
			if opts.RoleOnly && personal[notebook.TopicRef{Section: info.Section, Topic: info.Topic}] {
				continue
			}
		}
		if len(data) > MaxEntrySize {
			return nil, limitErr(CodeEntrySize, path, MaxEntrySize)
		}
		total += int64(len(data))
		p.files[path] = data
		p.paths = append(p.paths, path)
	}
	if len(p.paths) > maxEntries {
		return nil, limitErr(CodeTooMany, "", int64(maxEntries))
	}
	if total > maxTotalSize {
		return nil, limitErr(CodeTotalSize, "", maxTotalSize)
	}
	sort.Strings(p.paths)
	// Validate what will actually be written, and count it.
	kept, err := notebook.ParseTree(p.files)
	if err != nil {
		return nil, err
	}
	p.Summary = SummaryOf(kept)
	return p, nil
}

// dosEpoch is 1980-01-01 00:00:00, the earliest MS-DOS date, used as the
// modification time of every entry.
const (
	dosEpochDate = 1<<5 | 1 // day 1, month 1, year 1980
	dosEpochTime = 0
)

// WriteTo writes the archive to w. The output depends only on the files:
// entries are sorted by path, every entry has the same fixed modification
// time, mode 0600, no extra field, no data descriptor, and no comment.
// Entries are deflated, or stored when deflating would not make them
// smaller or would exceed the reader's ratio limit (a long run of one
// character), so every archive written here can be read back.
func (p *Plan) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	zw := zip.NewWriter(cw)
	var deflated bytes.Buffer
	fw, err := flate.NewWriter(&deflated, flate.BestCompression)
	if err != nil {
		return cw.n, err
	}
	for _, path := range p.paths {
		data := p.files[path]
		deflated.Reset()
		fw.Reset(&deflated)
		if _, err := fw.Write(data); err != nil {
			return cw.n, err
		}
		if err := fw.Close(); err != nil {
			return cw.n, err
		}
		method, body := zip.Deflate, deflated.Bytes()
		if len(body) >= len(data) || int64(len(data)) > MaxRatio*int64(len(body)) {
			method, body = zip.Store, data
		}
		fh := &zip.FileHeader{
			Name:               path,
			Method:             method,
			CRC32:              crc32.ChecksumIEEE(data),
			CompressedSize64:   uint64(len(body)),
			UncompressedSize64: uint64(len(data)),
		}
		// The legacy MS-DOS fields are set directly: setting Modified
		// would add an extended timestamp extra field.
		fh.ModifiedDate = dosEpochDate //nolint:staticcheck // SA1019: deliberate, see above
		fh.ModifiedTime = dosEpochTime //nolint:staticcheck // SA1019: deliberate, see above
		fh.SetMode(0o600)
		ew, err := zw.CreateRaw(fh)
		if err != nil {
			return cw.n, err
		}
		if _, err := ew.Write(body); err != nil {
			return cw.n, err
		}
	}
	err = zw.Close()
	return cw.n, err
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(b []byte) (int, error) {
	n, err := c.w.Write(b)
	c.n += int64(n)
	return n, err
}

// testHookBeforeRename, if set, runs after the temporary archive is
// written and synced and before it is moved into place.
var testHookBeforeRename func(tmp string) error

// CreateFile writes the archive produced by write to path, atomically: it
// is written to a temporary 0600 file in the destination directory,
// fsynced, then moved into place, and the directory is fsynced. The
// destination directory must exist and must not be inside notebookRoot
// (checked by file identity, so symlinks, bind mounts and
// case-insensitive names do not get around it). An existing file is
// refused unless force is set; a directory is always refused. On failure
// nothing is left behind and an existing file is untouched.
func CreateFile(path string, force bool, notebookRoot string, write func(io.Writer) error) (err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return &Error{Code: CodeIO, Err: err}
	}
	base := filepath.Base(abs)
	if base == "." || base == ".." || base == string(filepath.Separator) {
		return newErr(CodeNotRegular, "")
	}
	dir, err := filepath.EvalSymlinks(filepath.Dir(abs))
	if err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	if inside, err := isInside(dir, notebookRoot); err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	} else if inside {
		return newErr(CodeInsideRoot, "")
	}
	target := filepath.Join(dir, base)
	if err := checkDestination(target, force); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(dir, ".bearings-export-*.tmp")
	if err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	tmpPath := tmp.Name()
	defer func() {
		if tmp != nil {
			_ = tmp.Close()
		}
		_ = os.Remove(tmpPath) // after a link, or on failure
	}()
	if err := tmp.Chmod(0o600); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	bw := bufio.NewWriterSize(tmp, 1<<16)
	if err := write(bw); err != nil {
		return err
	}
	if err := bw.Flush(); err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	if err := tmp.Sync(); err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	cerr := tmp.Close()
	tmp = nil
	if cerr != nil {
		return &Error{Code: CodeIO, Err: stripPath(cerr)}
	}
	if testHookBeforeRename != nil {
		if err := testHookBeforeRename(tmpPath); err != nil {
			return &Error{Code: CodeIO, Err: err}
		}
	}
	if err := place(tmpPath, target, force); err != nil {
		return err
	}
	syncDir(dir)
	return nil
}

// checkDestination refuses a directory, and an existing file unless force.
func checkDestination(target string, force bool) error {
	fi, err := os.Lstat(target)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil
	case err != nil:
		return &Error{Code: CodeIO, Err: stripPath(err)}
	case fi.IsDir():
		return newErr(CodeNotRegular, "")
	case !force:
		return newErr(CodeExists, "")
	}
	return nil
}

// place moves the finished temporary file to target. Without force it
// uses a hard link, which fails if target appeared in the meantime, and
// falls back to a last check and a rename on file systems without links.
func place(tmpPath, target string, force bool) error {
	if !force {
		err := os.Link(tmpPath, target)
		if err == nil {
			return nil
		}
		if errors.Is(err, fs.ErrExist) {
			return newErr(CodeExists, "")
		}
		if err := checkDestination(target, false); err != nil {
			return err
		}
	}
	if err := os.Rename(tmpPath, target); err != nil {
		return &Error{Code: CodeIO, Err: stripPath(err)}
	}
	return nil
}

// isInside reports whether dir is root or below it, comparing file
// identities on the way up.
func isInside(dir, root string) (bool, error) {
	if root == "" {
		return false, nil
	}
	rfi, err := os.Stat(root)
	if err != nil {
		return false, err
	}
	for d := dir; ; d = filepath.Dir(d) {
		fi, err := os.Stat(d)
		if err != nil {
			return false, err
		}
		if os.SameFile(fi, rfi) {
			return true, nil
		}
		if filepath.Dir(d) == d {
			return false, nil
		}
	}
}

// syncDir makes a rename in dir durable. Errors are ignored: not every
// platform can fsync a directory, and the data itself is already synced.
func syncDir(dir string) {
	d, err := os.Open(dir) //nolint:gosec // G304: the destination directory, opened only to fsync it
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}
