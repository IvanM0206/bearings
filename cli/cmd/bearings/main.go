// Command bearings is the Bearings command line interface.
package main

import (
	"os"
	"os/exec"

	"github.com/IvanM0206/bearings/cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:], cmd.Env{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: isTerminal(os.Stdin) && isTerminal(os.Stderr),
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,
		Environ:     os.Environ,
		LookPath:    exec.LookPath,
	}))
}

// isTerminal reports whether f is a character device (a terminal), using
// only the standard library.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
