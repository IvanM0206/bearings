package server

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

// Host is the only address the server listens on. It is a constant on
// purpose: there is no flag or option to change it.
const Host = "127.0.0.1"

// TokenBytes is the size of the per-run token before encoding.
const TokenBytes = 32

// HTTP server limits.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 60 * time.Second
	maxHeaderBytes    = 16 << 10
	shutdownTimeout   = 3 * time.Second
)

// MaxBodyBytes is the largest request body accepted: the largest file of
// SPEC §7 plus room for the JSON envelope and escaping overhead.
const MaxBodyBytes = notebook.MaxFileSize + 64<<10

// Config configures a [Server].
type Config struct {
	Notebook *notebook.Notebook
	ReadOnly bool
	Logger   *slog.Logger // nil discards logs
	// Rand is the source of the per-run token; nil means crypto/rand.
	// Tests use it to get a fixed token.
	Rand io.Reader
}

// Server serves one notebook on one loopback listener.
type Server struct {
	nb       *notebook.Notebook
	readOnly bool
	log      *slog.Logger
	ln       net.Listener
	port     int
	token    string
	hosts    map[string]bool
	origins  map[string]bool
	cookie   string
	routes   []route
	writeMu  sync.Mutex // serializes every write request
}

// Listen opens a TCP listener on 127.0.0.1 and the given port (0 picks a
// free one).
func Listen(port int) (net.Listener, error) {
	if port < 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %d", port)
	}
	return net.Listen("tcp4", net.JoinHostPort(Host, strconv.Itoa(port)))
}

// New creates a server on ln, which must be a TCP listener on 127.0.0.1
// (see [Listen]); anything else is refused. It generates the per-run token.
func New(ln net.Listener, cfg Config) (*Server, error) {
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok || !addr.IP.Equal(net.IPv4(127, 0, 0, 1)) || addr.Port == 0 {
		return nil, errors.New("server: listener must be on 127.0.0.1")
	}
	if cfg.Notebook == nil {
		return nil, errors.New("server: notebook is required")
	}
	r := cfg.Rand
	if r == nil {
		r = rand.Reader
	}
	raw := make([]byte, TokenBytes)
	if _, err := io.ReadFull(r, raw); err != nil {
		return nil, fmt.Errorf("server: generating token: %w", err)
	}
	log := cfg.Logger
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	port := strconv.Itoa(addr.Port)
	s := &Server{
		nb: cfg.Notebook, readOnly: cfg.ReadOnly, log: log, ln: ln, port: addr.Port,
		token:   base64.RawURLEncoding.EncodeToString(raw),
		hosts:   map[string]bool{"127.0.0.1:" + port: true, "localhost:" + port: true},
		origins: map[string]bool{"http://127.0.0.1:" + port: true, "http://localhost:" + port: true},
		// Cookies are not isolated by port, so the port is part of the name:
		// two instances on the same host do not overwrite each other.
		cookie: "bearings_session_" + port,
	}
	s.routes = s.buildRoutes()
	return s, nil
}

// Port returns the port the server listens on.
func (s *Server) Port() int { return s.port }

// BaseURL returns the origin of the server, without the token.
func (s *Server) BaseURL() string { return "http://" + Host + ":" + strconv.Itoa(s.port) }

// URL returns the address to give to the person: it carries the token, which
// is exchanged for a session cookie and removed from the address bar.
func (s *Server) URL() string { return s.BaseURL() + "/auth?token=" + s.token }

// ReadOnly reports whether writes are disabled.
func (s *Server) ReadOnly() bool { return s.readOnly }

// tokenEqual compares two tokens in constant time. Both are hashed first so
// that their lengths do not leak either.
func tokenEqual(a, b string) bool {
	ha, hb := sha256.Sum256([]byte(a)), sha256.Sum256([]byte(b))
	return subtle.ConstantTimeCompare(ha[:], hb[:]) == 1
}

// Serve serves requests until ctx is cancelled, then shuts down gracefully.
func (s *Server) Serve(ctx context.Context) error {
	srv := &http.Server{
		Handler:           s.Handler(),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
		ErrorLog:          slog.NewLogLogger(s.log.Handler(), slog.LevelWarn),
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.Serve(s.ln) }()
	select {
	case err := <-errc:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-ctx.Done():
		sctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		err := srv.Shutdown(sctx)
		if err != nil {
			// A connection that does not finish in time is cut: the
			// person asked to stop.
			_ = srv.Close()
		}
		<-errc
		if errors.Is(err, context.DeadlineExceeded) {
			return nil
		}
		return err
	}
}
