// Package archive exports a notebook to a ZIP archive and reads one back,
// enforcing every rule of SPEC §8 "Archives".
//
// # Reading
//
// [Read] and [ReadFile] accept an archive only as a whole: every check is
// done, and every file is parsed and validated into a [notebook.Tree],
// before the caller writes anything (with [notebook.Create], which builds
// the notebook in a temporary sibling directory and renames it into place).
// The reader:
//
//   - bounds the size of the archive file before opening it, and walks the
//     central directory itself first, with constant memory, to bound the
//     number of entries and the size of the directory before archive/zip
//     loads it;
//   - checks that each local header agrees with the central directory
//     (name, method, encryption) and that no two entries share data, so
//     that other unzip tools see the same archive and overlapping-entry
//     zip bombs are refused;
//   - refuses absolute paths, "..", ".", empty segments, backslashes,
//     colons (drive letters, NTFS streams), NUL and other control
//     characters, invalid UTF-8, Info-ZIP Unicode path fields that would
//     give an entry another name elsewhere, and names that are duplicates
//     after normalization (case folding, and the trailing dots and spaces
//     that Windows drops), including a file that is also a directory;
//   - refuses symlinks, devices and every other non-regular entry,
//     encrypted entries, and methods other than store and deflate;
//   - checks the declared sizes and compression ratio of every entry
//     first, then inflates every entry (known or not) through a hard
//     limit, counting the real bytes, so lying headers cannot get past the
//     per-entry (SPEC §7), total and ratio limits;
//   - keeps only the paths of the layout (SPEC §2); others are ignored.
//
// # Writing
//
// [Prepare] validates the raw files of a notebook (from
// [notebook.Notebook.Snapshot]) with [notebook.ParseTree] and keeps only
// layout paths; [Plan.WriteTo] produces a deterministic archive: entries
// sorted by path, a fixed modification time, no extra fields, no data
// descriptors, mode 0600, deflate (or store when that keeps an entry
// within the ratio limit the reader enforces). [CreateFile] writes it
// atomically next to its destination.
//
// The notebook itself is only ever read and written through package
// notebook; this package touches the filesystem only for the archive file.
package archive

import (
	"fmt"
	"strconv"
)

// Limits of SPEC §8 and of this implementation.
const (
	// MaxEntries is the maximum number of entries in an archive (SPEC §8).
	MaxEntries = 210000
	// MaxTotalSize is the maximum total uncompressed size (SPEC §8).
	MaxTotalSize = 200 << 20
	// MaxRatio is the maximum compression ratio of any entry (SPEC §8).
	MaxRatio = 100
	// MaxEntrySize is the per-entry limit: the SPEC §7 file size limit,
	// applied to every entry, known or not.
	MaxEntrySize = 1 << 20
	// MaxArchiveSize bounds the archive file itself: the total size limit
	// with every entry stored, plus room for the headers.
	MaxArchiveSize = 256 << 20
	// MaxDirectorySize bounds the central directory, which archive/zip
	// holds in memory.
	MaxDirectorySize = 64 << 20
	// MaxNameLen is the maximum length of an entry name in bytes; the
	// longest layout path is far shorter.
	MaxNameLen = 1024
)

// Limits used by the code; variables only so that tests can lower them
// to build small archives that exceed them.
var (
	maxEntries   = MaxEntries
	maxTotalSize = int64(MaxTotalSize)
)

// Code identifies why an archive was refused. Codes are stable; the CLI
// maps them to translated messages.
type Code string

// Error codes.
const (
	CodeNotRegular    Code = "archive_not_regular"      // the archive path is not a regular file
	CodeTooLarge      Code = "archive_too_large"        // the archive file is larger than MaxArchiveSize
	CodeFormat        Code = "archive_format"           // not a ZIP file, corrupt, or inconsistent headers
	CodeDirectorySize Code = "archive_directory_size"   // central directory larger than MaxDirectorySize
	CodeTooMany       Code = "archive_too_many_entries" // more than MaxEntries entries
	CodeTotalSize     Code = "archive_total_size"       // more than MaxTotalSize uncompressed
	CodeEntrySize     Code = "archive_entry_size"       // an entry larger than MaxEntrySize
	CodeRatio         Code = "archive_ratio"            // an entry compressed more than MaxRatio:1
	CodeDepth         Code = "archive_depth"            // a name deeper than the SPEC §7 limit
	CodeBadName       Code = "archive_bad_name"         // unsafe or invalid entry name
	CodeDuplicate     Code = "archive_duplicate"        // duplicate name after normalization
	CodeEntryType     Code = "archive_entry_type"       // symlink, device or other non-regular entry
	CodeEncrypted     Code = "archive_encrypted"        // encrypted entry
	CodeMethod        Code = "archive_method"           // compression method other than store and deflate
	CodeExists        Code = "archive_exists"           // the export destination exists
	CodeInsideRoot    Code = "archive_inside_notebook"  // the export destination is inside the notebook
	CodeIO            Code = "archive_io"               // reading or writing the archive file failed
)

// Error is the error returned for a refused archive or a failed export.
// It never contains file contents. Name is the entry name as found in the
// archive (untrusted, truncated): display it with [Error.QuotedName].
type Error struct {
	Code Code
	Name string // entry name, if any
	Max  int64  // the limit, for limit codes
	Err  error  // underlying error, if any (I/O, archive/zip)
}

func (e *Error) Error() string {
	s := "archive: " + string(e.Code)
	if e.Name != "" {
		s += " in " + e.QuotedName()
	}
	if e.Max > 0 {
		s += fmt.Sprintf(" (max %d)", e.Max)
	}
	if e.Err != nil {
		s += ": " + e.Err.Error()
	}
	return s
}

func (e *Error) Unwrap() error { return e.Err }

// QuotedName returns the entry name as a quoted string of printable ASCII,
// safe for a terminal whatever the name contains.
func (e *Error) QuotedName() string { return strconv.QuoteToASCII(e.Name) }

const maxErrName = 160

func newErr(code Code, name string) *Error {
	if len(name) > maxErrName {
		name = name[:maxErrName]
	}
	return &Error{Code: code, Name: name}
}

func limitErr(code Code, name string, maxValue int64) *Error {
	e := newErr(code, name)
	e.Max = maxValue
	return e
}

// toU64 converts a size or count, never negative, to uint64.
func toU64(n int64) uint64 {
	if n < 0 {
		return 0
	}
	return uint64(n)
}

// toI64 converts a size or offset read from an archive to int64, clamping
// values above math.MaxInt64 (callers then refuse them as out of range).
func toI64(n uint64) int64 {
	if n > 1<<63-1 {
		return 1<<63 - 1
	}
	return int64(n)
}
