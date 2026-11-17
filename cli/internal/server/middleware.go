package server

import (
	"mime"
	"net/http"
	"net/url"
	"strings"
)

// contentSecurityPolicy has no 'unsafe-inline' or 'unsafe-eval': the web
// interface must ship its scripts and styles as files.
const contentSecurityPolicy = "default-src 'none'; script-src 'self'; style-src 'self'; img-src 'self' data:; " +
	"font-src 'self'; connect-src 'self'; base-uri 'none'; form-action 'self'; frame-ancestors 'none'"

// setSecurityHeaders sets the headers every response carries, errors
// included. No Access-Control-* header is ever set: cross-origin pages must
// not be able to read any response.
func setSecurityHeaders(h http.Header) {
	h.Set("Content-Security-Policy", contentSecurityPolicy)
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Cross-Origin-Opener-Policy", "same-origin")
	h.Set("Cross-Origin-Resource-Policy", "same-origin")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=(), interest-cohort=()")
}

// Handler returns the HTTP handler of the server.
func (s *Server) Handler() http.Handler { return http.HandlerFunc(s.serveHTTP) }

func (s *Server) serveHTTP(w http.ResponseWriter, r *http.Request) {
	setSecurityHeaders(w.Header())
	w.Header().Set("Cache-Control", "no-store") // static files override this
	// DNS rebinding: a page on evil.example that resolves to 127.0.0.1 sends
	// "Host: evil.example:PORT". Only our own names are accepted.
	if !s.hosts[r.Host] {
		writeError(w, http.StatusMisdirectedRequest, "bad_host", "Unexpected host.")
		return
	}
	// Fetch metadata (sent by browsers only): refuse requests started by
	// another site, or by another port of the same host.
	if sf := r.Header.Get("Sec-Fetch-Site"); sf != "" && sf != "same-origin" && sf != "none" {
		writeError(w, http.StatusForbidden, "forbidden", "Cross-site requests are not allowed.")
		return
	}
	if r.Method == http.MethodOptions {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}
	switch {
	case r.URL.Path == "/auth":
		s.serveAuth(w, r)
	case r.URL.Path == "/api" || strings.HasPrefix(r.URL.Path, "/api/"):
		s.serveAPI(w, r)
	default:
		s.serveStatic(w, r)
	}
}

// serveAuth exchanges the per-run token for a session cookie.
func (s *Server) serveAuth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
		return
	}
	if !tokenEqual(r.URL.Query().Get("token"), s.token) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	// A session cookie: no Expires or Max-Age, so it goes away when the
	// browser closes. Secure is not set because the page is plain http on
	// loopback.
	http.SetCookie(w, &http.Cookie{
		Name:     s.cookie,
		Value:    s.token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) authenticated(r *http.Request) bool {
	c, err := r.Cookie(s.cookie)
	return err == nil && tokenEqual(c.Value, s.token)
}

// pathIsClean reports whether the request path of an API call is in its
// canonical form: no encoded slashes or other odd escapes, no backslashes,
// no empty or dot segments.
func pathIsClean(u *url.URL) bool {
	if u.RawPath != "" || strings.ContainsRune(u.Path, '\\') || strings.ContainsRune(u.Path, 0) {
		return false
	}
	for i, seg := range strings.Split(strings.TrimPrefix(u.Path, "/"), "/") {
		if seg == "." || seg == ".." || (seg == "" && !(i == 0 && u.Path == "/")) {
			return false
		}
	}
	return true
}

func isMutation(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// checkMutation verifies the CSRF defenses of a mutating request: the
// Origin header must be our own origin, and a body must be JSON.
func (s *Server) checkMutation(w http.ResponseWriter, r *http.Request) bool {
	if !s.origins[r.Header.Get("Origin")] {
		writeError(w, http.StatusForbidden, "forbidden_origin", "Request origin not allowed.")
		return false
	}
	return true
}

func (s *Server) checkBody(w http.ResponseWriter, r *http.Request) bool {
	if r.ContentLength > MaxBodyBytes {
		writeError(w, http.StatusRequestEntityTooLarge, "payload_too_large", "Request body too large.")
		return false
	}
	if r.ContentLength == 0 {
		return true
	}
	mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mt != "application/json" {
		writeError(w, http.StatusUnsupportedMediaType, "unsupported_media_type", "Content-Type must be application/json.")
		return false
	}
	return true
}

func (s *Server) serveAPI(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if !pathIsClean(r.URL) {
		writeError(w, http.StatusBadRequest, "bad_path", "Invalid path.")
		return
	}
	if !s.authenticated(r) {
		writeError(w, http.StatusUnauthorized, "unauthorized", "Authentication required.")
		return
	}
	rt, params, allowed := s.match(r.Method, r.URL.Path)
	if rt == nil {
		if len(allowed) > 0 {
			w.Header().Set("Allow", strings.Join(allowed, ", "))
			writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "Method not allowed.")
			return
		}
		writeError(w, http.StatusNotFound, "not_found", "Not found.")
		return
	}
	if isMutation(r.Method) {
		if !s.checkMutation(w, r) {
			return
		}
		if rt.write && s.readOnly {
			writeError(w, http.StatusForbidden, "read_only", "The notebook is open read-only.")
			return
		}
		if !s.checkBody(w, r) {
			return
		}
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	if rt.write {
		s.writeMu.Lock()
		defer s.writeMu.Unlock()
	}
	rt.handler(s, w, r, params)
}
