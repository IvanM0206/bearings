package notebook

import (
	"errors"
	"fmt"
	"strings"
)

// Code identifies the kind of an [Error]. Codes are stable: callers (the CLI,
// the MCP server) map them to translated, user-facing messages.
type Code string

// Error codes returned by this package.
const (
	// File syntax (SPEC §4).
	CodeInvalidUTF8          Code = "invalid_utf8"
	CodeNULByte              Code = "nul_byte"
	CodeBOM                  Code = "bom"
	CodeMissingFrontmatter   Code = "missing_frontmatter"
	CodeUnclosedFrontmatter  Code = "unclosed_frontmatter"
	CodeYAMLSyntax           Code = "yaml_syntax"
	CodeYAMLAnchor           Code = "yaml_anchor"
	CodeYAMLAlias            Code = "yaml_alias"
	CodeYAMLTag              Code = "yaml_tag"
	CodeYAMLMergeKey         Code = "yaml_merge_key"
	CodeYAMLMultipleDocs     Code = "yaml_multiple_documents"
	CodeYAMLDuplicateKey     Code = "yaml_duplicate_key"
	CodeYAMLUnknownKey       Code = "yaml_unknown_key"
	CodeYAMLNotMapping       Code = "yaml_not_mapping"
	CodeYAMLTooDeep          Code = "yaml_too_deep"
	CodeMissingField         Code = "missing_field"
	CodeWrongType            Code = "wrong_type"
	CodeInvalidValue         Code = "invalid_value"
	CodeEmpty                Code = "empty"
	CodeTooLong              Code = "too_long"
	CodeControlChar          Code = "control_character"
	CodeOutOfRange           Code = "out_of_range"
	CodeUnsupportedFormat    Code = "unsupported_format"
	CodeUpdatedBeforeCreated Code = "updated_before_created"
	CodePathMismatch         Code = "path_mismatch"
	CodeTopicSectionMismatch Code = "topic_section_mismatch"
	CodeSectionMissing       Code = "section_metadata_missing"

	// Limits (SPEC §7).
	CodeLimit Code = "limit_exceeded"

	// Identifiers and arguments (SPEC §3).
	CodeInvalidSlug  Code = "invalid_slug"
	CodeInvalidRef   Code = "invalid_reference"
	CodeInvalidID    Code = "invalid_entry_id"
	CodeInvalidDate  Code = "invalid_date"
	CodeInvalidQuery Code = "invalid_query"

	// Filesystem and notebook state (SPEC §8).
	CodeNotNotebook Code = "not_a_notebook"
	CodeNotEmpty    Code = "directory_not_empty"
	CodeNotFound    Code = "not_found"
	CodeExists      Code = "already_exists"
	CodeAmbiguous   Code = "ambiguous"
	CodeConflict    Code = "conflict"
	CodeSymlink     Code = "symlink"
	// CodeSectionNotDeletable: the section holds files that are not valid topics.
	CodeSectionNotDeletable Code = "section_not_deletable"
	CodeNotRegular          Code = "not_regular_file"
	CodeOutsideRoot         Code = "outside_root"
	CodeIO                  Code = "io"
)

// Limit names, used in [Error.Limit] for [CodeLimit] errors.
const (
	LimitFileSize         = "file_size"
	LimitFrontmatterSize  = "frontmatter_size"
	LimitSections         = "sections"
	LimitTopicsPerSection = "topics_per_section"
	LimitJournalEntries   = "journal_entries"
	LimitDepth            = "directory_depth"
)

// Error is the error type returned by every function of this package.
//
// It never contains file contents: only the relative path of the file, the
// name of the field, and the violated rule. Field may contain a YAML key
// taken from the file (truncated); callers that print it to a terminal must
// sanitize it like any other notebook content.
type Error struct {
	Code  Code     // what went wrong
	Path  string   // slash-separated path relative to the notebook root, if any
	Field string   // YAML key or argument name, if any
	Limit string   // limit name, for CodeLimit
	Max   int64    // maximum allowed value, for CodeLimit, CodeTooLong and CodeOutOfRange
	Line  int      // 1-based line inside the frontmatter, for YAML errors, if known
	Names []string // file names (never contents), for CodeSectionNotDeletable
	Err   error    // underlying error (I/O), if any; never wraps file content
}

func (e *Error) Error() string {
	var b strings.Builder
	b.WriteString("notebook: ")
	b.WriteString(string(e.Code))
	if e.Path != "" {
		fmt.Fprintf(&b, " in %s", e.Path)
	}
	if e.Field != "" {
		fmt.Fprintf(&b, " (field %q)", e.Field)
	}
	if e.Line > 0 {
		fmt.Fprintf(&b, " at line %d", e.Line)
	}
	if e.Limit != "" {
		fmt.Fprintf(&b, ": limit %s", e.Limit)
	}
	if e.Max > 0 {
		fmt.Fprintf(&b, " (max %d)", e.Max)
	}
	if e.Err != nil {
		fmt.Fprintf(&b, ": %v", e.Err)
	}
	return b.String()
}

func (e *Error) Unwrap() error { return e.Err }

// Is reports whether target is an *Error with the same Code, so that
// errors.Is(err, &notebook.Error{Code: notebook.CodeConflict}) works.
func (e *Error) Is(target error) bool {
	t, ok := target.(*Error)
	return ok && t.Code == e.Code
}

// CodeOf returns the Code of err if it is (or wraps) an *Error, or "" otherwise.
func CodeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

func newErr(code Code, path, field string) *Error {
	return &Error{Code: code, Path: path, Field: field}
}

func limitErr(limit, path string, maxValue int64) *Error {
	return &Error{Code: CodeLimit, Path: path, Limit: limit, Max: maxValue}
}

// withPath returns err with Path set to path if err is an *Error without a path.
func withPath(err error, path string) error {
	var e *Error
	if errors.As(err, &e) && e.Path == "" {
		c := *e
		c.Path = path
		return &c
	}
	return err
}

// ioErr wraps a filesystem error. Only the error's kind is kept (not
// existing, permission...), never the absolute path, so that messages stay
// relative to the notebook root.
func ioErr(path string, err error) *Error {
	return &Error{Code: CodeIO, Path: path, Err: stripPath(err)}
}

// stripPath removes the absolute path from *fs.PathError and friends.
func stripPath(err error) error {
	var pe interface{ Unwrap() error }
	if errors.As(err, &pe) {
		if inner := pe.Unwrap(); inner != nil {
			return inner
		}
	}
	return err
}

// Problem describes a file that was skipped while reading the notebook
// (invalid content, symlink, special file...). Problems are not fatal: the
// rest of the notebook is still returned. They are reported through
// [Options.OnProblem].
type Problem struct {
	Path string // relative path
	Err  error  // always an *Error
}
