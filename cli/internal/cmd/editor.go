package cmd

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// ---- Confirmation ----------------------------------------------------------

// confirm asks before something is deleted. With --yes it returns at once.
// Without a terminal it refuses (usage error, exit code 2): a script must
// say --yes on purpose. On a terminal it prints prompt on stderr and reads
// the answer from stdin; anything but an affirmative answer cancels.
func (a *app) confirm(yes bool, prompt string) error {
	return a.confirmWith(yes, prompt, "err.confirm_needed")
}

// confirmWith is confirm with the message key used when there is no
// terminal and no --yes.
func (a *app) confirmWith(yes bool, prompt, neededKey string) error {
	if yes {
		return nil
	}
	if !a.env.Interactive {
		return a.usagef(neededKey)
	}
	_, _ = io.WriteString(a.env.Stderr, prompt+" "+a.t("confirm.suffix")+" ")
	var line string
	if a.env.Stdin != nil {
		line, _ = bufio.NewReader(io.LimitReader(a.env.Stdin, 1024)).ReadString('\n')
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	for _, ok := range strings.Split(a.t("confirm.yes_answers"), ",") {
		if answer == ok {
			return nil
		}
	}
	return &exitError{code: ExitFailure, msg: a.t("err.cancelled")}
}

// ---- Editor ----------------------------------------------------------------

// splitCommand splits an editor command such as `code --wait` or
// `"/opt/my editor/ed" -w` into an argument list, without a shell: no
// variable expansion, no command substitution, no globbing, no operators.
//
// Rules: arguments are separated by spaces or tabs; single quotes keep
// everything literally up to the next single quote; inside double quotes a
// backslash escapes only `"` and `\`; outside quotes a backslash escapes
// the next character when escapes is true (POSIX systems; on Windows it
// is an ordinary character, since it separates path components). An
// unterminated quote is an error.
func splitCommand(s string, escapes bool) ([]string, error) {
	var (
		args    []string
		cur     strings.Builder
		inArg   bool
		quote   rune // 0, '\'' or '"'
		escaped bool
	)
	flush := func() {
		if inArg {
			args = append(args, cur.String())
			cur.Reset()
			inArg = false
		}
	}
	for _, r := range s {
		switch {
		case escaped:
			if quote == '"' && r != '"' && r != '\\' {
				cur.WriteRune('\\') // not an escape: keep the backslash
			}
			cur.WriteRune(r)
			escaped = false
		case quote == '\'':
			if r == '\'' {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case quote == '"':
			switch r {
			case '"':
				quote = 0
			case '\\':
				escaped = true
			default:
				cur.WriteRune(r)
			}
		case r == ' ' || r == '\t':
			flush()
		case r == '\'' || r == '"':
			quote, inArg = r, true
		case r == '\\' && escapes:
			escaped, inArg = true, true
		default:
			cur.WriteRune(r)
			inArg = true
		}
	}
	if quote != 0 {
		return nil, errUnterminatedQuote
	}
	if escaped {
		cur.WriteRune('\\') // a trailing backslash is literal
	}
	flush()
	return args, nil
}

var errUnterminatedQuote = errors.New("unterminated quote")

// editorArgs returns the editor command from $VISUAL, else $EDITOR.
func (a *app) editorArgs() ([]string, error) {
	for _, name := range []string{"VISUAL", "EDITOR"} {
		v := strings.TrimSpace(a.getenv(name))
		if v == "" {
			continue
		}
		args, err := splitCommand(v, runtime.GOOS != "windows")
		if err != nil || len(args) == 0 || args[0] == "" {
			return nil, a.usagef("err.bad_editor", name)
		}
		for _, arg := range args {
			if strings.ContainsRune(arg, 0) {
				return nil, a.usagef("err.bad_editor", name)
			}
		}
		return args, nil
	}
	return nil, a.usagef("err.no_editor")
}

// editText lets the person edit text in their editor. The text goes to a
// 0600 file inside a fresh 0700 directory, which is removed when the editor
// ends, whatever the outcome. The edited text is checked to be UTF-8,
// NUL-free and not larger than a notebook file; nothing else is assumed
// about it.
func (a *app) editText(fileName string, initial string) (string, error) {
	argv, err := a.editorArgs()
	if err != nil {
		return "", err
	}
	// MkdirTemp creates the directory with mode 0700.
	dir, err := os.MkdirTemp("", "bearings-edit-*")
	if err != nil {
		return "", &exitError{code: ExitFailure, msg: a.t("err.temp_dir")}
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, fileName)
	if err := writeNew(path, []byte(initial)); err != nil {
		return "", &exitError{code: ExitFailure, msg: a.t("err.temp_dir")}
	}
	if err := a.runEditor(argv, path); err != nil {
		return "", err
	}
	return a.readEdited(path)
}

func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // G304: a fresh file inside a private temp directory we just created
	if err != nil {
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

// runEditor runs the editor with path as its last argument and waits for
// it. SIGTERM and SIGHUP stop the editor so that the temporary files are
// removed; SIGINT is left to the editor, which receives it from the
// terminal as well.
func (a *app) runEditor(argv []string, path string) error {
	args := append(append([]string{}, argv[1:]...), path)
	c := exec.Command(argv[0], args...) //nolint:gosec // G204: running the editor the person chose in $VISUAL/$EDITOR is the purpose; no shell is involved
	if f, ok := a.env.Stdin.(*os.File); ok {
		c.Stdin = f
	}
	c.Stdout, c.Stderr = a.env.Stdout, a.env.Stderr
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigs)
	if err := c.Start(); err != nil {
		return &exitError{code: ExitFailure, msg: a.t("err.editor_start", filepath.Base(argv[0]))}
	}
	done := make(chan error, 1)
	go func() { done <- c.Wait() }()
	for {
		select {
		case err := <-done:
			if err != nil {
				return &exitError{code: ExitFailure, msg: a.t("err.editor_failed", filepath.Base(argv[0]))}
			}
			return nil
		case s := <-sigs:
			if s != os.Interrupt {
				_ = c.Process.Kill()
			}
		}
	}
}

// readEdited reads the file the editor left and validates its encoding and
// size.
func (a *app) readEdited(path string) (string, error) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return "", &exitError{code: ExitFailure, msg: a.t("err.edited_missing")}
	}
	if fi.Size() > notebook.MaxFileSize {
		return "", a.usagef("err.edited_too_large", notebook.MaxFileSize)
	}
	f, err := os.Open(path) //nolint:gosec // G304: the file inside our private temp directory
	if err != nil {
		return "", &exitError{code: ExitFailure, msg: a.t("err.edited_missing")}
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, notebook.MaxFileSize+1))
	if err != nil {
		return "", &exitError{code: ExitFailure, msg: a.t("err.edited_missing")}
	}
	if len(data) > notebook.MaxFileSize {
		return "", a.usagef("err.edited_too_large", notebook.MaxFileSize)
	}
	if !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", a.usagef("err.edited_encoding")
	}
	return string(data), nil
}

// saveRecovery keeps text the person typed in a 0600 file inside a new 0700
// directory, so that a failed save does not lose their work. It returns
// the path.
func saveRecovery(fileName, text string) (string, error) {
	// MkdirTemp creates the directory with mode 0700.
	dir, err := os.MkdirTemp("", "bearings-recovery-*")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, fileName)
	if err := writeNew(path, []byte(text)); err != nil {
		return "", err
	}
	return path, nil
}

// withRecovery adds to err a note saying where the edited text was saved.
// If it cannot be saved the original error is returned.
func (a *app) withRecovery(err error, fileName, text string) error {
	if strings.TrimSpace(text) == "" {
		return err
	}
	path, serr := saveRecovery(fileName, text)
	if serr != nil {
		return err
	}
	var ee *exitError
	if errors.As(err, &ee) {
		return &exitError{code: ee.code, msg: ee.msg + "\n" + a.t("err.recovery_saved", path)}
	}
	return err
}
