package output

import (
	"os"
	"strings"
)

// Style adds ANSI styling to text that the CLI itself produces (headings,
// marks). It never styles notebook content, and it is a no-op when color
// is disabled.
type Style struct{ On bool }

func (s Style) wrap(code, text string) string {
	if !s.On || text == "" {
		return text
	}
	return "\x1b[" + code + "m" + text + "\x1b[0m"
}

// Bold returns text in bold.
func (s Style) Bold(text string) string { return s.wrap("1", text) }

// Dim returns text dimmed.
func (s Style) Dim(text string) string { return s.wrap("2", text) }

// Green returns text in green.
func (s Style) Green(text string) string { return s.wrap("32", text) }

// Yellow returns text in yellow.
func (s Style) Yellow(text string) string { return s.wrap("33", text) }

// ColorEnabled decides whether to use color on out. Color is off when
// --no-color was given, when NO_COLOR is set to any non-empty value
// (https://no-color.org), when TERM is "dumb", or when out is not a
// terminal.
func ColorEnabled(noColorFlag bool, getenv func(string) string, out *os.File) bool {
	if noColorFlag || getenv("NO_COLOR") != "" || strings.EqualFold(getenv("TERM"), "dumb") {
		return false
	}
	if out == nil {
		return false
	}
	fi, err := out.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}
