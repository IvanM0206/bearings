package cmd

import (
	"strings"
	"testing"
)

// Arguments echoed back in error messages must not reach the terminal
// with escape sequences or control characters.
func TestErrorMessagesAreSanitized(t *testing.T) {
	dir := newFixture(t)
	for _, args := range [][]string{
		{"show", "bad\x1b]0;pwned\x07\x1b[31mref"},
		{"topic", "rm", "x/\x1b[2Jy\r", "--yes"},
	} {
		r := run(t, dir, args...)
		if r.code == 0 {
			t.Fatalf("%q: expected an error", args)
		}
		for _, c := range []string{"\x1b", "\x07", "\r"} {
			if strings.Contains(r.stderr, c) {
				t.Errorf("%q: stderr contains %q: %q", args, c, r.stderr)
			}
		}
	}
}
