package output

import (
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// Width returns the display width of s, counted in characters. Content is
// sanitized before measuring, so it matches what [Table] prints.
func Width(s string) int { return utf8.RuneCountInString(s) }

// Pad right-pads s with spaces to width w.
func Pad(s string, w int) string {
	if n := Width(s); n < w {
		return s + strings.Repeat(" ", w-n)
	}
	return s
}

// PadLeft left-pads s with spaces to width w.
func PadLeft(s string, w int) string {
	if n := Width(s); n < w {
		return strings.Repeat(" ", w-n) + s
	}
	return s
}

// Table prints rows as aligned columns separated by two spaces, under a
// header line. Every cell is cleaned with [Line]; trailing spaces are
// trimmed. Header cells are trusted (they come from the catalog) and may be
// styled by the caller through hdr, which receives the padded header line.
func Table(w io.Writer, header []string, rows [][]string, hdr func(string) string) error {
	cols := len(header)
	widths := make([]int, cols)
	clean := make([][]string, len(rows))
	for i, h := range header {
		widths[i] = Width(h)
	}
	for r, row := range rows {
		clean[r] = make([]string, cols)
		for c := 0; c < cols && c < len(row); c++ {
			clean[r][c] = Line(row[c])
			widths[c] = max(widths[c], Width(clean[r][c]))
		}
	}
	line := func(cells []string) string {
		parts := make([]string, cols)
		for c := range parts {
			parts[c] = Pad(cells[c], widths[c])
		}
		return strings.TrimRight(strings.Join(parts, "  "), " ")
	}
	h := line(header)
	if hdr != nil {
		h = hdr(h)
	}
	if _, err := fmt.Fprintln(w, h); err != nil {
		return err
	}
	for _, row := range clean {
		if _, err := fmt.Fprintln(w, line(row)); err != nil {
			return err
		}
	}
	return nil
}

// Bar draws a fixed-width ASCII progress bar: done of total in width cells,
// e.g. "[#####-----]". With total 0 the bar is empty. The result always has
// width+2 characters.
func Bar(done, total, width int) string {
	filled := 0
	if total > 0 {
		done = min(max(done, 0), total)
		filled = done * width / total
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}

// Percent returns done/total as a whole percentage (0 when total is 0).
func Percent(done, total int) int {
	if total <= 0 {
		return 0
	}
	return done * 100 / total
}
