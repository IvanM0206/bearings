// Command bearings is the Bearings command line interface.
package main

import (
	"os"

	"github.com/IvanM0206/bearings/cli/internal/cmd"
)

func main() {
	os.Exit(cmd.Execute(os.Args[1:], cmd.Env{
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Getenv:      os.Getenv,
		UserHomeDir: os.UserHomeDir,
	}))
}
