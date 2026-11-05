package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWidth(t *testing.T) {
	tests := []struct {
		in   string
		want int
	}{
		{"", 0},
		{"abc", 3},
		{"café", 4},
		{"e\u0301", 1},  // combining acute accent
		{"日本語", 6},      // East Asian Wide
		{"ＡＢ", 4},       // fullwidth Latin
		{"한글", 4},       // Hangul syllables
		{"🚀", 2},        // emoji
		{"a\u200bb", 2}, // zero-width space
		{"👩\u200d💻", 4}, // ZWJ is zero width
		{"ok ✅", 5},     // wide dingbat emoji
		{"✓", 1},        // narrow check mark
		{"a\x07b", 2},   // a control character takes no cell
		{"\ufe0f", 0},   // variation selector
	}
	for _, tt := range tests {
		if got := Width(tt.in); got != tt.want {
			t.Errorf("Width(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestTableMixedWidths(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]string{{"日本語", "1"}, {"cafe\u0301", "2"}, {"🚀 launch", "3"}, {"plain", "4"}}
	if err := Table(&buf, []string{"TITLE", "N"}, rows, nil); err != nil {
		t.Fatal(err)
	}
	for i, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		// The second column starts at cell 11 in every line.
		idx := strings.LastIndex(l, " ")
		if got := Width(l[:idx+1]); got != 11 {
			t.Errorf("line %d %q: second column at cell %d, want 11", i, l, got)
		}
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		in   string
		w    int
		want string
	}{
		{"short", 10, "short"},
		{"exactly", 7, "exactly"},
		{"abcdefghij", 6, "abcde…"},
		{"日本語日本語", 7, "日本語…"},
		{"日本語日本語", 6, "日本…"},
		{"abc", 1, "…"},
		{"abc", 0, ""},
	}
	for _, tt := range tests {
		got := Truncate(tt.in, tt.w)
		if got != tt.want || Width(got) > max(tt.w, 0) {
			t.Errorf("Truncate(%q, %d) = %q, want %q", tt.in, tt.w, got, tt.want)
		}
	}
}

func TestTableMaxTruncates(t *testing.T) {
	var buf bytes.Buffer
	rows := [][]string{{"a very long title that does not fit in the terminal at all", "x"}, {"短い", "y"}}
	if err := TableMax(&buf, []string{"TITLE", "N"}, rows, nil, 30); err != nil {
		t.Fatal(err)
	}
	for _, l := range strings.Split(strings.TrimRight(buf.String(), "\n"), "\n") {
		if Width(l) > 30 {
			t.Errorf("line too wide (%d): %q", Width(l), l)
		}
	}
	if !strings.Contains(buf.String(), "…") {
		t.Errorf("no truncation mark: %q", buf.String())
	}
}

func TestWrap(t *testing.T) {
	tests := []struct {
		name, in string
		w        int
		want     string
	}{
		{"fits", "short line", 20, "short line"},
		{"disabled", "a long line that would wrap", 0, "a long line that would wrap"},
		{"on spaces", "one two three four five", 10, "one two\nthree four\nfive"},
		{"keeps indentation", "  - one two three four", 12, "  - one two\n  three four"},
		{"long word split", "abcdefghijkl x", 5, "abcde\nfghij\nkl x"},
		{"long word alone after text", "ab abcdefgh", 4, "ab\nabcd\nefgh"},
		{"blank lines kept", "one two three\n\nfour five six", 8, "one two\nthree\n\nfour\nfive six"},
		{"indented code untouched", "    a very long code line here\nand then some prose words", 12, "    a very long code line here\nand then\nsome prose\nwords"},
		{"fenced code untouched", "```\nlong line inside a fence block\n```\nlong line outside the fence", 12, "```\nlong line inside a fence block\n```\nlong line\noutside the\nfence"},
		{"wide characters", "日本語 日本語 日本語", 13, "日本語 日本語\n日本語"},
	}
	for _, tt := range tests {
		if got := Wrap(tt.in, tt.w); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}
