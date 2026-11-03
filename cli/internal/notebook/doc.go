// Package notebook reads and writes Bearings notebooks as specified in
// format/SPEC.md (format version 1). It is the only package of the CLI that
// touches the filesystem, and it is shared by the commands and the MCP
// server. It has no terminal or CLI concerns.
//
// # Overview
//
// [Open] opens an existing notebook directory; [Init] and [Create] make a
// new one. A [Notebook] gives typed access to sections ([Notebook.Sections],
// [Notebook.Section]), topics ([Notebook.Topics], [Notebook.Topic],
// [Notebook.FindTopics], [Notebook.Outline]), the journal
// ([Notebook.Journal], [Notebook.Entry]), the role path
// ([Notebook.RolePath]), progress ([Notebook.Progress]) and full-text
// search ([Notebook.Search]), and to the matching create, update, move and
// delete operations.
//
// Every value is read from disk on each call. Values returned by the read
// methods carry a [Version]; update methods take the value back and refuse
// with [CodeConflict] if the file changed in the meantime.
//
// Parse* and Marshal* functions convert single files from and to bytes
// with full validation, and [ParseTree] validates a whole notebook held in
// memory (used for templates and archives) before [Create] writes it.
//
// # Errors
//
// Every error is an [*Error] with a stable [Code], the relative path of the
// file and the field concerned. Errors never contain file contents, apart
// from a truncated YAML key name for unknown or duplicate keys. Files that
// are invalid while reading a directory are skipped and reported through
// [Options.OnProblem] instead of failing the whole operation; exceeding a
// limit of SPEC §7 is always an error.
//
// # Security
//
// All content is untrusted. The package enforces every rule of SPEC §4, §5,
// §7 and §8: strict UTF-8, a restricted YAML subset (no anchors, aliases,
// tags, merge keys, multiple documents, duplicate or unknown keys, exact
// types), size and count limits, paths built only from validated slugs,
// dates and ids and checked to stay inside the root, no symlink or special
// file ever followed, atomic writes (temporary file, fsync, rename) with
// modes 0600/0700, and conflict detection before overwriting or removing a
// file. Returned strings are raw content: callers must escape them for
// their output (terminal, HTML, AI model).
package notebook
