package cmd

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/mcpserver"
	"github.com/IvanM0206/bearings/cli/internal/notebook"
	"github.com/IvanM0206/bearings/cli/internal/output"
)

var logLevels = map[string]slog.Level{
	"error": slog.LevelError,
	"warn":  slog.LevelWarn,
	"info":  slog.LevelInfo,
	"debug": slog.LevelDebug,
}

func (a *app) newMCP() *cobra.Command {
	var allowWrite bool
	var level string
	c := &cobra.Command{
		Use:   "mcp",
		Short: a.t("cmd.mcp.short"),
		Long:  a.t("cmd.mcp.long"),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return a.wrap(a.runMCP(c.Context(), allowWrite, level))
		},
	}
	c.Flags().BoolVar(&allowWrite, "allow-write", false, a.t("flag.allow_write"))
	c.Flags().StringVar(&level, "log-level", "warn", a.t("flag.log_level"))
	_ = c.RegisterFlagCompletionFunc("log-level", fixed("error", "warn", "info", "debug"))
	return c
}

// runMCP serves the notebook over stdin and stdout. stdout carries only the
// protocol: everything else (logs, warnings) goes to stderr.
func (a *app) runMCP(ctx context.Context, allowWrite bool, level string) error {
	lvl, ok := logLevels[strings.ToLower(level)]
	if !ok {
		return a.usagef("err.bad_log_level", output.Line(level))
	}
	stderr := a.env.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: lvl}))

	dir, err := a.notebookDir()
	if err != nil {
		return err
	}
	nb, err := notebook.Open(dir, &notebook.Options{Now: a.env.Now, OnProblem: func(p notebook.Problem) {
		log.Warn("skipped file", "path", output.Line(p.Path), "code", string(notebook.CodeOf(p.Err)))
	}})
	if err != nil {
		if notebook.CodeOf(err) == notebook.CodeNotNotebook {
			return &exitError{code: ExitNotebook, msg: a.t("err.no_notebook", output.Line(dir))}
		}
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	in := a.env.Stdin
	if in == nil {
		in = strings.NewReader("")
	}
	err = mcpserver.Run(ctx, mcpserver.Config{Notebook: nb, AllowWrite: allowWrite, Logger: log}, in, a.env.Stdout)
	if err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}
