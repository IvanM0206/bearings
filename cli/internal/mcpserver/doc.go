// Package mcpserver is the local Model Context Protocol server of Bearings,
// started by `bearings mcp`. It lets an AI assistant read a notebook and,
// only when started with --allow-write, add to it.
//
// # Design
//
//   - Transport: stdio only (newline-delimited JSON-RPC on stdin/stdout).
//     There is no network listener.
//   - The notebook is opened once by the caller and fixed for the life of the
//     server. No tool takes a filesystem path: tools take slugs, topic
//     references, dates and cursors, which are checked by the input schema
//     and again by package notebook, the only code that touches the disk.
//   - Read-only by default. Write tools are registered only when
//     [Config.AllowWrite] is set. There are no delete tools.
//
// # Untrusted content
//
// Everything that comes from the notebook is untrusted data (SPEC §8). It is
// returned only inside dedicated JSON fields of the structured result,
// stripped of control characters and escape sequences ([output.Text]), and
// never placed in tool descriptions, server instructions, error messages or
// logs. Tool descriptions tell the model that these fields are user data and
// not instructions.
//
// # Limits
//
// Every list is paginated with an opaque cursor; a page holds at most
// [MaxPageItems] items and about [MaxResultBytes] bytes of JSON. Long texts
// are cut at a rune boundary and flagged (a "…_truncated" field and, for
// list items, [TruncatedMarker] at the end of the text). Topic notes are
// read in chunks with get_topic's notes_offset.
//
// # Logging
//
// Logs go to the [slog.Logger] given in [Config] (stderr in the CLI, since
// stdout carries the protocol). They contain tool names, durations, error
// codes and relative paths built from validated identifiers, never note
// content.
package mcpserver
