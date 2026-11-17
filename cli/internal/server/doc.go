// Package server implements `bearings serve`: a local web server that
// exposes one notebook through a JSON API and serves the embedded web
// interface.
//
// # Threat model
//
// The server holds private work notes and runs next to a browser that may
// have other, hostile pages open. It defends against:
//
//   - other machines: it listens on 127.0.0.1 only (the address is a
//     constant; there is no option to change it);
//   - other local users and processes: every /api/ request needs a session
//     cookie that is only handed out in exchange for a random per-run token
//     (256 bits, crypto/rand) printed to the person who started the server;
//   - DNS rebinding: requests whose Host header is not exactly
//     127.0.0.1:PORT or localhost:PORT are refused with 421;
//   - cross-site request forgery: the cookie is SameSite=Strict, mutating
//     requests must carry an Origin header equal to the server's own
//     origin and a JSON content type, and Sec-Fetch-Site values other than
//     same-origin and none are refused. No CORS header is ever sent;
//   - script injection: a strict Content-Security-Policy (no inline
//     scripts or styles), nosniff and the other headers of [setSecurityHeaders]
//     are set on every response; JSON responses never carry HTML.
//
// # Static assets
//
// The embedded interface (index.html and its assets) is served WITHOUT the
// session cookie: it contains no notebook data and must load before the
// person is signed in or while the cookie is being set. Everything that
// holds data lives under /api/ and always needs the cookie.
//
// # Notebook content
//
// The package never touches the filesystem: it only calls internal/notebook.
// Notebook content is untrusted. The API returns it as raw JSON data
// (strings are validated UTF-8 and the notebook package already rejects
// control characters in titles); the browser interface is responsible for
// rendering it safely (no raw HTML). Error responses are generic: they
// carry a stable code and a fixed message, never file contents, absolute
// paths or file names.
package server
