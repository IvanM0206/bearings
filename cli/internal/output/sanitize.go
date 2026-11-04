// Package output renders command results for the terminal and as JSON.
//
// Everything that comes from a notebook is untrusted (SPEC §8): before it
// reaches a terminal it goes through [Text] or [Line], which remove ANSI
// escape sequences and control characters so that content cannot move the
// cursor, change colors, set the window title or hide text.
package output

import (
	"strings"
	"unicode/utf8"
)

// Text makes s safe to print to a terminal, keeping line breaks.
//
// It removes ANSI escape sequences (CSI, OSC, DCS, SOS, PM, APC and the
// two-character forms, in both their 7-bit and 8-bit spellings), C0 and C1
// control characters except the newline, DEL, and the Unicode characters
// that reorder or hide text (bidirectional controls, zero-width
// characters, BOM). CR is dropped (so CRLF becomes LF); tabs
// become four spaces; U+2028 and U+2029 become newlines; invalid UTF-8
// becomes U+FFFD.
//
// The result never contains a control character other than '\n'.
func Text(s string) string {
	if isPlain(s) {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			b.WriteRune(utf8.RuneError)
			i++
			continue
		}
		switch {
		case r == 0x1b:
			i += escapeLen(s[i:])
			continue
		case r == 0x9b: // 8-bit CSI
			i += size + csiLen(s[i+size:])
			continue
		case r == 0x9d || r == 0x90 || r == 0x98 || r == 0x9e || r == 0x9f: // OSC, DCS, SOS, PM, APC
			i += size + stringLen(s[i+size:])
			continue
		case r == '\n' || r == 0x2028 || r == 0x2029:
			b.WriteByte('\n')
		case r == '\t':
			b.WriteString("    ")
		case isDropped(r):
			// skip
		default:
			b.WriteRune(r)
		}
		i += size
	}
	return b.String()
}

// Line is [Text] for a value that must stay on one line (titles, table
// cells): line breaks become spaces.
func Line(s string) string {
	return strings.ReplaceAll(Text(s), "\n", " ")
}

// isPlain reports whether s needs no cleaning (the common case).
func isPlain(s string) bool {
	if !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if r == '\n' {
			continue
		}
		if isDropped(r) || r == '\t' || r == 0x1b || r == 0x2028 || r == 0x2029 {
			return false
		}
	}
	return true
}

// isDropped reports runes that are removed without replacement.
func isDropped(r rune) bool {
	switch {
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true // C0 (including CR), DEL, C1
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true // bidi embeddings, overrides, isolates
	case r == 0x200b, r == 0x200c, r == 0x200d, r == 0x200e, r == 0x200f, r == 0x061c, r == 0x2060, r == 0xfeff:
		return true // zero-width and directional marks, word joiner, BOM
	}
	return false
}

// escapeLen returns the length in bytes of the escape sequence that starts
// at s[0] == ESC. It always consumes at least the ESC itself.
func escapeLen(s string) int {
	if len(s) < 2 {
		return len(s)
	}
	switch s[1] {
	case '[':
		return 2 + csiLen(s[2:])
	case ']', 'P', 'X', '^', '_':
		return 2 + stringLen(s[2:])
	case 0x1b:
		return 1 // a new ESC starts its own sequence
	}
	// nF sequences: ESC, intermediates 0x20-0x2f, final 0x30-0x7e.
	n := 1
	for n < len(s) && s[n] >= 0x20 && s[n] <= 0x2f {
		n++
	}
	if n < len(s) && s[n] >= 0x30 && s[n] <= 0x7e {
		n++
	}
	return n
}

// csiLen returns the length of the rest of a CSI sequence: parameter bytes
// 0x30-0x3f, intermediate bytes 0x20-0x2f and one final byte 0x40-0x7e. A
// sequence interrupted by any other byte ends there (the byte is kept for
// the caller to examine).
func csiLen(s string) int {
	n := 0
	for n < len(s) && s[n] >= 0x30 && s[n] <= 0x3f {
		n++
	}
	for n < len(s) && s[n] >= 0x20 && s[n] <= 0x2f {
		n++
	}
	if n < len(s) && s[n] >= 0x40 && s[n] <= 0x7e {
		n++
	}
	return n
}

// stringLen returns the length of the rest of a string-type sequence (OSC,
// DCS, ...): everything up to and including BEL or ST (ESC \ or U+009C), or
// to the end of s. A bare ESC that does not form ST ends the string
// without being consumed.
func stringLen(s string) int {
	for n := 0; n < len(s); n++ {
		switch s[n] {
		case 0x07:
			return n + 1
		case 0x1b:
			if n+1 < len(s) && s[n+1] == '\\' {
				return n + 2
			}
			return n
		case 0xc2:
			if n+1 < len(s) && s[n+1] == 0x9c { // U+009C in UTF-8
				return n + 2
			}
		}
	}
	return len(s)
}
