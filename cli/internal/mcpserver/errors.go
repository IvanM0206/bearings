package mcpserver

import (
	"errors"
	"log/slog"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// toolError is an error whose message is safe to return to the client: a
// fixed sentence, optionally naming an argument of the tool (never a value
// and never notebook content).
type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

// errArg reports an invalid argument by name.
func errArg(name string) error {
	return &toolError{msg: "invalid argument: " + name}
}

// errText reports text input with characters writes do not accept.
func errText(name string) error {
	return &toolError{msg: "invalid argument: " + name + " contains control characters"}
}

// publicError maps any error to a toolError. arg names the argument the
// error is about, when the caller knows it. Notebook errors are mapped by
// code only; their path and field, which may come from file names or YAML
// keys, are logged (sanitized) and never returned.
func publicError(log *slog.Logger, tool, arg string, err error) error {
	var te *toolError
	if errors.As(err, &te) {
		return te
	}
	var ne *notebook.Error
	if !errors.As(err, &ne) {
		log.Error("tool failed", "tool", tool, "error", output.Line(err.Error()))
		return &toolError{msg: "internal error"}
	}
	about := func(s string) string {
		if arg != "" {
			return s + ": " + arg
		}
		return s
	}
	switch ne.Code {
	case notebook.CodeInvalidSlug, notebook.CodeInvalidRef, notebook.CodeInvalidDate,
		notebook.CodeInvalidQuery, notebook.CodeInvalidID:
		return &toolError{msg: about("invalid argument")}
	case notebook.CodeNotFound:
		return &toolError{msg: about("not found: no such section or topic")}
	case notebook.CodeExists:
		return &toolError{msg: about("already exists")}
	case notebook.CodeConflict:
		return &toolError{msg: "conflict: the file changed since it was read; read it again and retry"}
	case notebook.CodeTopicSectionMismatch:
		return &toolError{msg: "invalid argument: topic must belong to section"}
	case notebook.CodeTooLong:
		return &toolError{msg: about("too long")}
	case notebook.CodeEmpty, notebook.CodeControlChar, notebook.CodeInvalidValue,
		notebook.CodeInvalidUTF8, notebook.CodeNULByte, notebook.CodeOutOfRange:
		return &toolError{msg: about("invalid value")}
	case notebook.CodeLimit:
		return &toolError{msg: "a notebook limit would be exceeded"}
	}
	log.Warn("tool failed", "tool", tool, "code", string(ne.Code), "path", output.Line(ne.Path))
	return &toolError{msg: "the notebook could not be read or written"}
}
