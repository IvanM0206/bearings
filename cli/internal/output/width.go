package output

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// wideRanges lists code point ranges that take two terminal cells: East
// Asian Wide and Fullwidth characters and the common emoji blocks. It is
// sorted and non-overlapping.
var wideRanges = [][2]rune{
	{0x1100, 0x115F}, {0x231A, 0x231B}, {0x2329, 0x232A}, {0x23E9, 0x23EC},
	{0x23F0, 0x23F0}, {0x23F3, 0x23F3}, {0x25FD, 0x25FE}, {0x2614, 0x2615},
	{0x2648, 0x2653}, {0x267F, 0x267F}, {0x2693, 0x2693}, {0x26A1, 0x26A1},
	{0x26AA, 0x26AB}, {0x26BD, 0x26BE}, {0x26C4, 0x26C5}, {0x26CE, 0x26CE},
	{0x26D4, 0x26D4}, {0x26EA, 0x26EA}, {0x26F2, 0x26F3}, {0x26F5, 0x26F5},
	{0x26FA, 0x26FA}, {0x26FD, 0x26FD}, {0x2705, 0x2705}, {0x270A, 0x270B},
	{0x2728, 0x2728}, {0x274C, 0x274C}, {0x274E, 0x274E}, {0x2753, 0x2755},
	{0x2757, 0x2757}, {0x2795, 0x2797}, {0x27B0, 0x27B0}, {0x27BF, 0x27BF},
	{0x2B1B, 0x2B1C}, {0x2B50, 0x2B50}, {0x2B55, 0x2B55},
	{0x2E80, 0x303E}, {0x3041, 0x33FF}, {0x3400, 0x4DBF}, {0x4E00, 0x9FFF},
	{0xA000, 0xA4CF}, {0xA960, 0xA97F}, {0xAC00, 0xD7A3}, {0xF900, 0xFAFF},
	{0xFE10, 0xFE19}, {0xFE30, 0xFE6F}, {0xFF00, 0xFF60}, {0xFFE0, 0xFFE6},
	{0x1F004, 0x1F004}, {0x1F0CF, 0x1F0CF}, {0x1F18E, 0x1F18E},
	{0x1F191, 0x1F19A}, {0x1F200, 0x1F320}, {0x1F32D, 0x1F335},
	{0x1F337, 0x1F37C}, {0x1F37E, 0x1F393}, {0x1F3A0, 0x1F3CA},
	{0x1F3CF, 0x1F3D3}, {0x1F3E0, 0x1F3F0}, {0x1F3F4, 0x1F3F4},
	{0x1F3F8, 0x1F43E}, {0x1F440, 0x1F440}, {0x1F442, 0x1F4FC},
	{0x1F4FF, 0x1F53D}, {0x1F54B, 0x1F54E}, {0x1F550, 0x1F567},
	{0x1F57A, 0x1F57A}, {0x1F595, 0x1F596}, {0x1F5A4, 0x1F5A4},
	{0x1F5FB, 0x1F64F}, {0x1F680, 0x1F6C5}, {0x1F6CC, 0x1F6CC},
	{0x1F6D0, 0x1F6D2}, {0x1F6D5, 0x1F6D7}, {0x1F6DC, 0x1F6DF},
	{0x1F6EB, 0x1F6EC}, {0x1F6F4, 0x1F6FC}, {0x1F7E0, 0x1F7EB},
	{0x1F7F0, 0x1F7F0}, {0x1F90C, 0x1F93A}, {0x1F93C, 0x1F945},
	{0x1F947, 0x1F9FF}, {0x1FA70, 0x1FAFF}, {0x20000, 0x2FFFD},
	{0x30000, 0x3FFFD},
}

func isWide(r rune) bool {
	lo, hi := 0, len(wideRanges)
	for lo < hi {
		m := (lo + hi) / 2
		switch {
		case r < wideRanges[m][0]:
			hi = m
		case r > wideRanges[m][1]:
			lo = m + 1
		default:
			return true
		}
	}
	return false
}

// RuneWidth returns the number of terminal cells r takes: 0 for combining
// marks, format characters (zero-width joiner and space, variation
// selectors) and control characters, 2 for wide East Asian characters and
// emoji, 1 otherwise.
func RuneWidth(r rune) int {
	switch {
	case r < 0x20 || (r >= 0x7f && r < 0xa0):
		return 0
	case r < 0x300:
		return 1
	case unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf):
		return 0
	case isWide(r):
		return 2
	}
	return 1
}

// Width returns the display width of s in terminal cells. Content is
// sanitized before measuring, so it matches what [Table] prints.
func Width(s string) int {
	n := 0
	for _, r := range s {
		n += RuneWidth(r)
	}
	return n
}

// Truncate shortens s to at most w cells, ending it with "…" when it had to
// cut. It never splits a character.
func Truncate(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if Width(s) <= w {
		return s
	}
	n := 0
	for i, r := range s {
		rw := RuneWidth(r)
		if n+rw > w-1 {
			return s[:i] + "…"
		}
		n += rw
	}
	return s
}

// Wrap wraps the lines of s to at most width cells. It breaks lines on
// spaces, keeps each line's indentation on its continuation lines, splits a
// word only when it alone is longer than the room left, and leaves code
// untouched: lines inside ``` fences and lines that start with four spaces.
// With width <= 0 it returns s unchanged.
func Wrap(s string, width int) string {
	if width <= 0 || Width(s) <= width && !strings.Contains(s, "\n") {
		return s
	}
	lines := strings.Split(s, "\n")
	fenced := false
	for i, l := range lines {
		trim := strings.TrimLeft(l, " ")
		if strings.HasPrefix(trim, "```") {
			fenced = !fenced
			continue
		}
		if fenced || strings.HasPrefix(l, "    ") || Width(l) <= width {
			continue
		}
		lines[i] = wrapLine(l, trim, width)
	}
	return strings.Join(lines, "\n")
}

func wrapLine(l, trim string, width int) string {
	ind := l[:len(l)-len(trim)]
	room := width - len(ind)
	if room < 1 {
		return l
	}
	var out []string
	cur, curW := "", 0
	flush := func() {
		out = append(out, ind+cur)
		cur, curW = "", 0
	}
	for _, word := range strings.Fields(trim) {
		ww := Width(word)
		for ww > room { // a word longer than a whole line
			if curW > 0 {
				flush()
			}
			head, rest := splitCells(word, room)
			out = append(out, ind+head)
			word, ww = rest, Width(rest)
		}
		switch {
		case curW == 0:
			cur, curW = word, ww
		case curW+1+ww <= room:
			cur += " " + word
			curW += 1 + ww
		default:
			flush()
			cur, curW = word, ww
		}
	}
	if curW > 0 {
		flush()
	}
	return strings.Join(out, "\n")
}

// splitCells splits s after at most w cells (at least one character).
func splitCells(s string, w int) (string, string) {
	n := 0
	for i, r := range s {
		rw := RuneWidth(r)
		if n+rw > w && i > 0 {
			return s[:i], s[i:]
		}
		n += rw
	}
	_, size := utf8.DecodeRuneInString(s)
	return s[:size], s[size:]
}
