package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTextTable(t *testing.T) {
	tests := []struct {
		name, in, want string
	}{
		{"plain", "hello world", "hello world"},
		{"unicode kept", "Ñandú — café ✓ 日本", "Ñandú — café ✓ 日本"},
		{"newline kept", "a\nb", "a\nb"},
		{"crlf", "a\r\nb", "a\nb"},
		{"lone cr dropped", "abc\rXYZ", "abcXYZ"},
		{"tab", "a\tb", "a    b"},
		{"sgr color", "\x1b[31mred\x1b[0m", "red"},
		{"csi cursor", "a\x1b[2J\x1b[Hb", "ab"},
		{"csi with params and intermediates", "x\x1b[1;2;3 qy", "xy"},
		{"truncated csi", "x\x1b[31", "x"},
		{"truncated esc", "x\x1b", "x"},
		{"osc title bel", "a\x1b]0;pwned\x07b", "ab"},
		{"osc title st", "a\x1b]0;pwned\x1b\\b", "ab"},
		{"osc hyperlink", "\x1b]8;;http://evil\x07click\x1b]8;;\x07", "click"},
		{"osc unterminated", "a\x1b]0;rest of the line", "a"},
		{"dcs", "a\x1bPq#0;2;0;0;0\x1b\\b", "ab"},
		{"apc", "a\x1b_payload\x1b\\b", "ab"},
		{"two char escape", "a\x1bcb", "ab"},
		{"charset escape", "a\x1b(0b", "ab"},
		{"nested escape", "\x1b[3\x1b[1m1m", "1m"},
		{"double esc", "\x1b\x1b[31mX", "X"},
		{"8bit csi", "a\u009b31mb", "ab"},
		{"8bit osc", "a\u009d0;title\u0007b", "ab"},
		{"c1 other", "a\u0085b\u008fc", "abc"},
		{"nul bell backspace", "a\x00b\x07c\x08d", "abcd"},
		{"del", "a\x7fb", "ab"},
		{"bidi override", "a\u202eb\u2066c", "abc"},
		{"zero width", "a\u200bb\ufeffc", "abc"},
		{"line separator", "a\u2028b\u2029c", "a\nb\nc"},
		{"invalid utf8", "a\xffb", "a�b"},
		{"form feed and vt", "a\fb\vc", "abc"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Text(tc.in)
			if got != tc.want {
				t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
			checkSafe(t, got)
		})
	}
}

func TestLine(t *testing.T) {
	if got := Line("a\nb\r\nc\x1b[1m"); got != "a b c" {
		t.Errorf("Line = %q", got)
	}
}

func TestInjectionAttempts(t *testing.T) {
	attacks := []string{
		"\x1b[2J\x1b[3J\x1b[H",                // clear screen
		"\x1b]0;rm -rf\x07",                   // window title
		"\x1b]52;c;ZWNobyBoaQ==\x07",          // clipboard write
		"\x1b[?1049h",                         // alternate screen
		"\x1b[8mhidden\x1b[28m",               // concealed text
		"safe\x1b[1A\x1b[2Koverwritten",       // overwrite previous line
		"\x1bP$qm\x1b\\",                      // DECRQSS
		"\u009b2J",                            // 8-bit clear
		"\x1b[31",                             // incomplete, would eat next output
		strings.Repeat("\x1b[", 1000) + "m",   // many openers
		"\x1b]" + strings.Repeat("A", 100000), // huge unterminated OSC
	}
	for _, a := range attacks {
		checkSafe(t, Text(a))
		checkSafe(t, Line(a))
	}
}

func checkSafe(t *testing.T, s string) {
	t.Helper()
	if !utf8.ValidString(s) {
		t.Errorf("invalid UTF-8 in %q", s)
	}
	for _, r := range s {
		if r != '\n' && (r < 0x20 || (r >= 0x7f && r <= 0x9f)) {
			t.Errorf("control character %U left in %q", r, s)
			return
		}
	}
}

func FuzzText(f *testing.F) {
	for _, s := range []string{"", "plain", "\x1b[31mx", "\x1b]0;t\x07", "a\u009b1m", "\xff\xfe", "\x1bP\x1b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		out := Text(s)
		checkSafe(t, out)
		if Text(out) != out {
			t.Errorf("Text is not idempotent for %q", s)
		}
		if strings.Contains(Line(s), "\n") {
			t.Errorf("Line left a newline for %q", s)
		}
	})
}

func TestTable(t *testing.T) {
	var buf bytes.Buffer
	err := Table(&buf, []string{"NAME", "N"}, [][]string{
		{"alpha", "1"},
		{"b\x1b[31mé\x1b[0m", "22"},
		{"évil\nline", ""},
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "NAME       N\nalpha      1\nbé         22\névil line\n"
	if buf.String() != want {
		t.Errorf("got\n%q\nwant\n%q", buf.String(), want)
	}
}

func TestBar(t *testing.T) {
	tests := []struct {
		done, total, width int
		want               string
	}{
		{0, 0, 10, "[----------]"},
		{0, 4, 10, "[----------]"},
		{2, 4, 10, "[#####-----]"},
		{4, 4, 10, "[##########]"},
		{1, 3, 10, "[###-------]"},
		{9, 4, 10, "[##########]"},  // clamped
		{-1, 4, 10, "[----------]"}, // clamped
	}
	for _, tc := range tests {
		got := Bar(tc.done, tc.total, tc.width)
		if got != tc.want || len(got) != tc.width+2 {
			t.Errorf("Bar(%d,%d,%d) = %q, want %q", tc.done, tc.total, tc.width, got, tc.want)
		}
	}
	if Percent(1, 3) != 33 || Percent(0, 0) != 0 || Percent(3, 3) != 100 {
		t.Error("Percent")
	}
}

func TestJSON(t *testing.T) {
	var buf bytes.Buffer
	if err := JSON(&buf, map[string]string{"a": "<b>\x1b[31m&"}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b") {
		t.Errorf("raw ESC in JSON output: %q", out)
	}
	if !strings.Contains(out, "<b>") || !strings.Contains(out, `\u001b`) {
		t.Errorf("unexpected encoding: %q", out)
	}
	var back map[string]string
	if err := json.Unmarshal(buf.Bytes(), &back); err != nil || back["a"] != "<b>\x1b[31m&" {
		t.Errorf("round trip failed: %v %q", err, back)
	}
}

func TestStyle(t *testing.T) {
	if (Style{}).Bold("x") != "x" {
		t.Error("style must be a no-op when off")
	}
	if got := (Style{On: true}).Bold("x"); got != "\x1b[1mx\x1b[0m" {
		t.Errorf("Bold = %q", got)
	}
	env := func(m map[string]string) func(string) string { return func(k string) string { return m[k] } }
	if ColorEnabled(false, env(nil), nil) {
		t.Error("nil file must disable color")
	}
	if ColorEnabled(true, env(nil), nil) || ColorEnabled(false, env(map[string]string{"NO_COLOR": "1"}), nil) {
		t.Error("flag and NO_COLOR must disable color")
	}
}
