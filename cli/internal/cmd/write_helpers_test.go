package cmd

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"
)

// opts configures runOpts.
type opts struct {
	env         map[string]string
	stdin       string
	interactive bool
	now         time.Time
}

// runOpts runs the CLI with stdin and terminal mode under the test's control.
func runOpts(t *testing.T, o opts, args ...string) result {
	t.Helper()
	var out, errb bytes.Buffer
	env := o.env
	if env == nil {
		env = map[string]string{}
	}
	e := Env{
		Stdin:       strings.NewReader(o.stdin),
		Stdout:      &out,
		Stderr:      &errb,
		Interactive: o.interactive,
		Getenv:      func(k string) string { return env[k] },
		UserHomeDir: func() (string, error) { return "", os.ErrNotExist },
	}
	if !o.now.IsZero() {
		e.Now = func() time.Time { return o.now }
	}
	code := Execute(args, e)
	return result{stdout: out.String(), stderr: errb.String(), code: code}
}

// runIn runs the CLI against the notebook in dir, not on a terminal.
func runIn(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return runOpts(t, opts{env: map[string]string{"BEARINGS_NOTEBOOK": dir}}, args...)
}
