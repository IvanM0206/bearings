package archive

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"io/fs"
	"os"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// General purpose flags that mean the entry is encrypted: traditional
// encryption, strong encryption, and the masked central directory of the
// strong encryption specification.
const encryptedFlags = 0x1 | 0x40 | 0x2000

// unicodePathExtraID is the Info-ZIP Unicode Path extra field: tools that
// honor it would extract the entry under a name the checks never saw.
const unicodePathExtraID = 0x7075

// ReadFile reads and validates the archive at path; see [Read]. The path
// is the one the user named: it may be a symlink, but it must end at a
// regular file of at most MaxArchiveSize bytes, checked before the file is
// opened (so a FIFO or device never blocks or feeds the reader).
func ReadFile(path string) (*notebook.Tree, error) {
	fi, err := os.Stat(path)
	if err != nil {
		return nil, &Error{Code: CodeIO, Err: stripPath(err)}
	}
	if !fi.Mode().IsRegular() {
		return nil, newErr(CodeNotRegular, "")
	}
	if fi.Size() > MaxArchiveSize {
		return nil, limitErr(CodeTooLarge, "", MaxArchiveSize)
	}
	f, err := os.Open(path) //nolint:gosec // G304: the archive the user asked to import
	if err != nil {
		return nil, &Error{Code: CodeIO, Err: stripPath(err)}
	}
	defer f.Close()
	ofi, err := f.Stat()
	if err != nil {
		return nil, &Error{Code: CodeIO, Err: stripPath(err)}
	}
	if !ofi.Mode().IsRegular() || !os.SameFile(fi, ofi) {
		return nil, newErr(CodeNotRegular, "")
	}
	return Read(f, ofi.Size())
}

// stripPath keeps the kind of a filesystem error without the path.
func stripPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// entry is a regular-file entry that passed the header checks.
type entry struct {
	f    *zip.File
	keep bool // part of the layout: its content is kept
}

// Read checks the archive held by r (size bytes) against every rule of
// SPEC §8 "Archives" and parses it into a validated notebook. Nothing is
// written anywhere. The whole archive is refused on the first problem:
// the returned error is an [*Error], or a [*notebook.Error] (with the entry
// name as path) for a layout file that is not a valid notebook file.
func Read(r io.ReaderAt, size int64) (*notebook.Tree, error) {
	if size > MaxArchiveSize {
		return nil, limitErr(CodeTooLarge, "", MaxArchiveSize)
	}
	if size < lenEnd {
		return nil, newErr(CodeFormat, "")
	}
	sc, err := scan(r, size)
	if err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(r, size)
	if err != nil && (!errors.Is(err, zip.ErrInsecurePath) || zr == nil) {
		// ErrInsecurePath (only with GODEBUG=zipinsecurepath=0) is left
		// to the name checks below, which are stricter.
		return nil, formatErr(err)
	}
	// archive/zip must see exactly the records the pre-scan checked.
	if len(zr.File) != len(sc.hashes) {
		return nil, newErr(CodeFormat, "")
	}
	for i, f := range zr.File {
		h := fnv.New64a()
		_, _ = h.Write([]byte(f.Name))
		if h.Sum64() != sc.hashes[i] {
			return nil, newErr(CodeFormat, "")
		}
	}

	entries, err := checkHeaders(zr.File, size)
	if err != nil {
		return nil, err
	}
	files, err := inflate(entries)
	if err != nil {
		return nil, err
	}
	return notebook.ParseTree(files)
}

// checkHeaders applies every check that needs no decompression, to every
// entry, before anything is inflated.
func checkHeaders(zfs []*zip.File, size int64) ([]entry, error) {
	names := newNameSet()
	var declared uint64
	out := make([]entry, 0, len(zfs))
	for _, f := range zfs {
		segments, isDir, err := checkName(f.Name)
		if err != nil {
			return nil, err
		}
		if err := names.add(f.Name, segments, isDir); err != nil {
			return nil, err
		}
		if f.Flags&encryptedFlags != 0 {
			return nil, newErr(CodeEncrypted, f.Name)
		}
		if f.Method != zip.Store && f.Method != zip.Deflate {
			return nil, newErr(CodeMethod, f.Name)
		}
		if err := checkType(f, isDir); err != nil {
			return nil, err
		}
		if err := checkExtra(f); err != nil {
			return nil, err
		}
		if f.CompressedSize64 > toU64(size) {
			return nil, newErr(CodeFormat, f.Name)
		}
		if isDir {
			if f.UncompressedSize64 != 0 {
				return nil, newErr(CodeEntryType, f.Name)
			}
			continue // plain directory entries are ignored
		}
		if f.UncompressedSize64 > MaxEntrySize {
			return nil, limitErr(CodeEntrySize, f.Name, MaxEntrySize)
		}
		if declared += f.UncompressedSize64; declared > toU64(maxTotalSize) {
			return nil, limitErr(CodeTotalSize, "", maxTotalSize)
		}
		if f.UncompressedSize64 > MaxRatio*f.CompressedSize64 {
			return nil, limitErr(CodeRatio, f.Name, MaxRatio)
		}
		info, err := notebook.ClassifyPath(f.Name)
		if err != nil {
			return nil, err
		}
		out = append(out, entry{f: f, keep: info.Kind != notebook.PathUnknown})
	}
	return out, nil
}

// checkType refuses symlinks, devices, FIFOs, sockets and anything else
// that is not a regular file or a plain directory. Both the mode that
// archive/zip derives from the creator system and the raw Unix mode bits
// are checked (whatever system the archive claims to come from), as well
// as the MS-DOS directory attribute.
func checkType(f *zip.File, isDir bool) error {
	m := f.Mode()
	unixType := (f.ExternalAttrs >> 16) & 0xf000
	dosDir := f.ExternalAttrs&0x10 != 0
	const sIFDIR, sIFREG = 0x4000, 0x8000
	var ok bool
	if isDir {
		ok = m.Type()&^fs.ModeDir == 0 && (unixType == 0 || unixType == sIFDIR)
	} else {
		ok = m.Type() == 0 && (unixType == 0 || unixType == sIFREG) && !dosDir
	}
	if !ok {
		return newErr(CodeEntryType, f.Name)
	}
	return nil
}

// checkExtra refuses malformed extra fields and Info-ZIP Unicode Path
// fields.
func checkExtra(f *zip.File) error {
	extra := f.Extra
	le := binary.LittleEndian
	for len(extra) > 0 {
		if len(extra) < 4 {
			return newErr(CodeFormat, f.Name)
		}
		id, sz := le.Uint16(extra), int(le.Uint16(extra[2:]))
		if sz > len(extra)-4 {
			return newErr(CodeFormat, f.Name)
		}
		if id == unicodePathExtraID {
			return newErr(CodeBadName, f.Name)
		}
		extra = extra[4+sz:]
	}
	return nil
}

// inflate decompresses every regular entry through a hard limit, counting
// the real bytes; declared sizes are never trusted. Content of layout
// paths is kept, the rest is discarded.
func inflate(entries []entry) (map[string][]byte, error) {
	files := make(map[string][]byte)
	var total int64
	for _, e := range entries {
		f := e.f
		rc, err := f.Open()
		if err != nil {
			return nil, &Error{Code: CodeFormat, Name: truncName(f.Name), Err: err}
		}
		limit := min(int64(MaxEntrySize), maxTotalSize-total)
		lr := io.LimitReader(rc, limit+1)
		var buf bytes.Buffer
		var n int64
		if e.keep {
			n, err = buf.ReadFrom(lr)
		} else {
			n, err = io.Copy(io.Discard, lr)
		}
		_ = rc.Close()
		switch {
		case n > MaxEntrySize:
			return nil, limitErr(CodeEntrySize, f.Name, MaxEntrySize)
		case total+n > maxTotalSize:
			return nil, limitErr(CodeTotalSize, "", maxTotalSize)
		case toU64(n) > MaxRatio*f.CompressedSize64:
			return nil, limitErr(CodeRatio, f.Name, MaxRatio)
		case err != nil:
			// Corrupt data or a checksum mismatch.
			return nil, &Error{Code: CodeFormat, Name: truncName(f.Name), Err: err}
		case toU64(n) != f.UncompressedSize64:
			return nil, newErr(CodeFormat, f.Name)
		}
		total += n
		if e.keep {
			files[f.Name] = buf.Bytes()
		}
	}
	return files, nil
}

func truncName(name string) string { return newErr("", name).Name }
