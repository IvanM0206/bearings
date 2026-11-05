// Package cmd implements the commands of the bearings CLI.
//
// Exit codes:
//
//	0  success
//	1  unexpected failure (I/O error, internal error)
//	2  usage error (bad flag, argument, reference, date or query)
//	3  the notebook is missing or invalid
//	4  the requested section or topic does not exist
//
// Commands that delete data ask for confirmation on a terminal and refuse
// to run without --yes when there is none (exit code 2).
//
// Messages for people go to stderr and never contain file contents;
// results go to stdout.
package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/i18n"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

// Exit codes returned by [Execute].
const (
	ExitOK       = 0
	ExitFailure  = 1
	ExitUsage    = 2
	ExitNotebook = 3
	ExitNotFound = 4
)

// Env is everything the commands take from the process, so that tests can
// run them in memory.
type Env struct {
	Stdin  io.Reader // may be nil: reads as empty
	Stdout io.Writer
	Stderr io.Writer
	// Interactive says that a person is at the terminal: only then do
	// commands ask for confirmation and open an editor on request.
	Interactive bool
	Getenv      func(string) string
	UserHomeDir func() (string, error)
	// Now returns the current time; nil means time.Now. Tests set it.
	Now func() time.Time
}

// exitError is an error with a user-facing message and an exit code.
type exitError struct {
	code int
	msg  string
}

func (e *exitError) Error() string { return e.msg }

type app struct {
	env Env

	// Global flags.
	notebookFlag string
	jsonOut      bool
	langFlag     string
	noColor      bool

	tr       *i18n.Translator
	langDone bool // language fixed by --lang or BEARINGS_LANG
	style    output.Style
	warned   map[string]bool
}

// Execute runs the CLI with args (without the program name) and returns the
// process exit code.
func Execute(args []string, env Env) int {
	a := &app{env: env, warned: map[string]bool{}}
	a.initLanguage(args)
	root := a.newRoot()
	root.SetArgs(args)
	root.SetOut(env.Stdout)
	root.SetErr(env.Stderr)
	err := root.Execute()
	if err == nil {
		return ExitOK
	}
	var ee *exitError
	if !errors.As(err, &ee) {
		// Errors that cobra itself produces (unknown flag, wrong number of
		// arguments) are usage errors.
		ee = &exitError{code: ExitUsage, msg: output.Line(err.Error())}
	}
	_, _ = fmt.Fprintf(env.Stderr, "%s: %s\n", a.t("app.name"), ee.msg)
	if ee.code == ExitUsage {
		_, _ = fmt.Fprintln(env.Stderr, a.t("err.usage_hint"))
	}
	return ee.code
}

func (a *app) t(key string, args ...any) string { return a.tr.T(key, args...) }

func (a *app) getenv(k string) string {
	if a.env.Getenv == nil {
		return ""
	}
	return a.env.Getenv(k)
}

// initLanguage picks the language before the command tree is built, so that
// help texts are translated. The notebook's own language is applied later,
// once it is open.
func (a *app) initLanguage(args []string) {
	flag, provided := scanLangFlag(args)
	lang, err := i18n.Resolve(a.inputs(flag, provided, ""))
	if err != nil {
		lang = i18n.Default
	}
	a.tr = i18n.New(lang)
}

func (a *app) inputs(flag string, provided bool, nbLang string) i18n.Inputs {
	return i18n.Inputs{
		Flag:         flag,
		FlagProvided: provided,
		Env:          a.getenv("BEARINGS_LANG"),
		Notebook:     nbLang,
		LCAll:        a.getenv("LC_ALL"),
		LCMessages:   a.getenv("LC_MESSAGES"),
		Lang:         a.getenv("LANG"),
	}
}

// scanLangFlag finds --lang in args without parsing the whole command line.
func scanLangFlag(args []string) (value string, provided bool) {
	for i, arg := range args {
		if arg == "--" {
			break
		}
		if v, ok := strings.CutPrefix(arg, "--lang="); ok {
			return v, true
		}
		if arg == "--lang" && i+1 < len(args) {
			return args[i+1], true
		}
	}
	return "", false
}

// prepare runs before every command: it validates the global flags and sets
// the language and the color mode.
func (a *app) prepare(langProvided bool) error {
	lang, err := i18n.Resolve(a.inputs(a.langFlag, langProvided, ""))
	if err != nil {
		return a.usagef("err.unknown_language", output.Line(a.langFlag), strings.Join(i18n.Known, ", "))
	}
	a.tr = i18n.New(lang)
	a.langDone = langProvided || i18n.IsKnown(i18n.Normalize(a.getenv("BEARINGS_LANG")))
	var out *os.File
	if f, ok := a.env.Stdout.(*os.File); ok {
		out = f
	}
	a.style = output.Style{On: output.ColorEnabled(a.noColor, a.getenv, out)}
	return nil
}

// ---- Errors ----------------------------------------------------------------

func (a *app) usagef(key string, args ...any) error {
	return &exitError{code: ExitUsage, msg: a.t(key, args...)}
}

func (a *app) notFoundf(key string, args ...any) error {
	return &exitError{code: ExitNotFound, msg: a.t(key, args...)}
}

// wrap converts any error into an *exitError with a translated message.
func (a *app) wrap(err error) error {
	if err == nil {
		return nil
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return err
	}
	var ne *notebook.Error
	if errors.As(err, &ne) {
		return &exitError{code: exitFor(ne.Code), msg: a.describe(ne)}
	}
	return &exitError{code: ExitFailure, msg: output.Line(err.Error())}
}

// wrapInput is like wrap for errors that come from what the person typed
// (a title, a URL, a description): they are usage errors, not a sign that
// the notebook is broken.
func (a *app) wrapInput(err error) error {
	var ne *notebook.Error
	if errors.As(err, &ne) {
		switch ne.Code {
		case notebook.CodeEmpty, notebook.CodeTooLong, notebook.CodeControlChar,
			notebook.CodeInvalidValue, notebook.CodeInvalidUTF8, notebook.CodeOutOfRange,
			notebook.CodeExists, notebook.CodeLimit, notebook.CodeTopicSectionMismatch:
			return &exitError{code: ExitUsage, msg: a.describe(ne)}
		}
	}
	return a.wrap(err)
}

func exitFor(c notebook.Code) int {
	switch c {
	case notebook.CodeInvalidSlug, notebook.CodeInvalidRef, notebook.CodeInvalidID,
		notebook.CodeInvalidDate, notebook.CodeInvalidQuery:
		return ExitUsage
	case notebook.CodeNotFound:
		return ExitNotFound
	case notebook.CodeIO, notebook.CodeConflict:
		return ExitFailure
	}
	return ExitNotebook
}

// describe renders a notebook error. It uses only the code, the relative
// path, the field and the limit; sanitized, since a field can carry a key
// name from the file.
func (a *app) describe(e *notebook.Error) string {
	msg := a.t("err.code." + string(e.Code))
	var details []string
	if e.Path != "" {
		details = append(details, a.t("err.detail.file", output.Line(e.Path)))
	}
	if e.Field != "" {
		details = append(details, a.t("err.detail.field", output.Line(e.Field)))
	}
	if e.Line > 0 {
		details = append(details, a.t("err.detail.line", e.Line))
	}
	if e.Limit != "" {
		details = append(details, a.t("err.detail.limit", output.Line(e.Limit), e.Max))
	}
	if e.Code == notebook.CodeIO && e.Err != nil {
		details = append(details, output.Line(e.Err.Error()))
	}
	if len(details) == 0 {
		return output.Line(msg)
	}
	return output.Line(msg) + " (" + strings.Join(details, ", ") + ")"
}

// warn reports a file that was skipped while reading, once per file.
func (a *app) warn(p notebook.Problem) {
	var code notebook.Code
	field := ""
	var ne *notebook.Error
	if errors.As(p.Err, &ne) {
		code, field = ne.Code, ne.Field
	}
	key := p.Path + "|" + string(code) + "|" + field
	if a.warned[key] {
		return
	}
	a.warned[key] = true
	_, _ = fmt.Fprintln(a.env.Stderr, output.Line(a.t("warn.skipped", p.Path, a.t("err.code."+string(code)))))
}

// ---- Notebook --------------------------------------------------------------

// notebookDir returns the notebook directory: --notebook, then
// BEARINGS_NOTEBOOK, then ~/bearings.
func (a *app) notebookDir() (string, error) {
	if a.notebookFlag != "" {
		return a.notebookFlag, nil
	}
	if v := a.getenv("BEARINGS_NOTEBOOK"); v != "" {
		return v, nil
	}
	if a.env.UserHomeDir == nil {
		return "", a.usagef("err.no_home")
	}
	home, err := a.env.UserHomeDir()
	if err != nil || home == "" {
		return "", a.usagef("err.no_home")
	}
	return filepath.Join(home, "bearings"), nil
}

// now returns the current time, as set by the environment.
func (a *app) now() time.Time {
	if a.env.Now != nil {
		return a.env.Now()
	}
	return time.Now()
}

func (a *app) nbOptions() *notebook.Options {
	return &notebook.Options{OnProblem: a.warn, Now: a.env.Now}
}

// open opens the notebook and applies its language when neither --lang nor
// BEARINGS_LANG fixed one.
func (a *app) open() (*notebook.Notebook, notebook.Config, error) {
	dir, err := a.notebookDir()
	if err != nil {
		return nil, notebook.Config{}, err
	}
	nb, err := notebook.Open(dir, a.nbOptions())
	if err != nil {
		if notebook.CodeOf(err) == notebook.CodeNotNotebook {
			return nil, notebook.Config{}, &exitError{code: ExitNotebook, msg: a.t("err.no_notebook", output.Line(dir))}
		}
		return nil, notebook.Config{}, a.wrap(err)
	}
	cfg, err := nb.Config()
	if err != nil {
		return nil, notebook.Config{}, a.wrap(err)
	}
	if !a.langDone {
		if lang, err := i18n.Resolve(a.inputs("", false, cfg.Language)); err == nil {
			a.tr = i18n.New(lang)
		}
	}
	return nb, cfg, nil
}

// sectionArg validates a section slug and loads the section.
func (a *app) sectionArg(nb *notebook.Notebook, slug string) (notebook.Section, error) {
	if !notebook.ValidSlug(slug) {
		return notebook.Section{}, a.usagef("err.bad_section", slug)
	}
	sec, err := nb.Section(slug)
	if notebook.CodeOf(err) == notebook.CodeNotFound {
		return notebook.Section{}, a.notFoundf("err.no_section", slug)
	}
	return sec, a.wrap(err)
}

// topicArg validates a topic reference and loads the topic.
func (a *app) topicArg(nb *notebook.Notebook, s string) (notebook.Topic, error) {
	ref, err := notebook.ParseTopicRef(s)
	if err != nil {
		return notebook.Topic{}, a.usagef("err.bad_ref", s)
	}
	t, err := nb.Topic(ref)
	if notebook.CodeOf(err) == notebook.CodeNotFound {
		return notebook.Topic{}, a.notFoundf("err.no_topic", ref.String())
	}
	return t, a.wrap(err)
}

// ---- Printing --------------------------------------------------------------

// printer writes to a stream and remembers the first error.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

func (p *printer) println(s string) {
	if p.err == nil {
		_, p.err = fmt.Fprintln(p.w, s)
	}
}

func (a *app) out() *printer { return &printer{w: a.env.Stdout} }

func (a *app) writeJSON(v any) error { return output.JSON(a.env.Stdout, v) }
