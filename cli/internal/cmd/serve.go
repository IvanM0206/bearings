package cmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/IvanM0206/bearings/cli/internal/output"
	"github.com/IvanM0206/bearings/cli/internal/server"
)

func (a *app) newServe() *cobra.Command {
	var port string
	var readOnly, noOpen bool
	c := &cobra.Command{
		Use:   "serve",
		Short: a.t("cmd.serve.short"),
		Long:  a.t("cmd.serve.long"),
		Args:  cobra.NoArgs,
		RunE: func(c *cobra.Command, _ []string) error {
			return a.wrap(a.runServe(c.Context(), port, readOnly, noOpen))
		},
	}
	c.Flags().StringVar(&port, "port", "0", a.t("flag.port"))
	c.Flags().BoolVar(&readOnly, "read-only", false, a.t("flag.read_only"))
	c.Flags().BoolVar(&noOpen, "no-open", false, a.t("flag.no_open"))
	return c
}

// runServe starts the local web server and blocks until it is interrupted.
// The address (with the one-time token) goes to stdout; every other message
// goes to stderr.
func (a *app) runServe(ctx context.Context, portArg string, readOnly, noOpen bool) error {
	port, err := strconv.Atoi(portArg)
	if err != nil || port < 0 || port > 65535 {
		return a.usagef("err.bad_port", output.Line(portArg))
	}
	nb, _, err := a.open()
	if err != nil {
		return err
	}
	stderr := a.env.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	log := slog.New(slog.NewTextHandler(stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))

	ln, err := server.Listen(port)
	if err != nil {
		return &exitError{code: ExitFailure, msg: a.t("err.serve_listen", output.Line(errReason(err)))}
	}
	srv, err := server.New(ln, server.Config{Notebook: nb, ReadOnly: readOnly, Logger: log, Rand: a.env.Rand})
	if err != nil {
		_ = ln.Close()
		return err
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	_, _ = fmt.Fprintln(a.env.Stdout, srv.URL())
	_, _ = fmt.Fprintln(stderr, a.t("serve.listening"))
	if readOnly {
		_, _ = fmt.Fprintln(stderr, a.t("serve.read_only"))
	}
	if !noOpen {
		open := a.env.OpenURL
		if open == nil {
			open = server.OpenBrowser
		}
		if err := open(srv.URL()); err != nil {
			_, _ = fmt.Fprintln(stderr, a.t("serve.open_failed"))
		}
	}
	if err := srv.Serve(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	_, _ = fmt.Fprintln(stderr, a.t("serve.stopped"))
	return nil
}

// errReason is the short reason of a listen error, without addresses.
func errReason(err error) string {
	var se interface{ Unwrap() error }
	if errors.As(err, &se) {
		if inner := se.Unwrap(); inner != nil {
			return errReason(inner)
		}
	}
	return err.Error()
}
