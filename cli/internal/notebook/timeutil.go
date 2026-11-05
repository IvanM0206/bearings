package notebook

import (
	"encoding/json"
	"fmt"
	"regexp"
	"time"
)

var (
	timestampRe = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}Z$`)
	dateRe      = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}$`)
)

// TimestampLayout is the layout of `created` and `updated` (SPEC §4).
const TimestampLayout = "2006-01-02T15:04:05Z"

// ParseTimestamp parses an RFC 3339 UTC timestamp with second precision and
// a "Z" suffix, e.g. 2026-11-03T14:05:09Z.
func ParseTimestamp(s string) (time.Time, error) {
	if !timestampRe.MatchString(s) {
		return time.Time{}, newErr(CodeInvalidValue, "", "")
	}
	t, err := time.Parse(TimestampLayout, s)
	if err != nil {
		return time.Time{}, newErr(CodeInvalidValue, "", "")
	}
	return t.UTC(), nil
}

// FormatTimestamp formats t as a SPEC §4 timestamp (UTC, seconds).
func FormatTimestamp(t time.Time) string {
	return t.UTC().Truncate(time.Second).Format(TimestampLayout)
}

// Date is a calendar date between 1970-01-01 and 9999-12-31 (SPEC §4).
// The zero value means "no date".
type Date struct {
	Year  int
	Month time.Month
	Day   int
}

// ParseDate parses and validates a YYYY-MM-DD date.
func ParseDate(s string) (Date, error) {
	if !dateRe.MatchString(s) {
		return Date{}, newErr(CodeInvalidDate, "", "")
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil || t.Year() < 1970 {
		return Date{}, newErr(CodeInvalidDate, "", "")
	}
	return Date{t.Year(), t.Month(), t.Day()}, nil
}

// DateOf returns the date of t in t's location.
func DateOf(t time.Time) Date {
	y, m, d := t.Date()
	return Date{y, m, d}
}

// IsZero reports whether d is the zero date.
func (d Date) IsZero() bool { return d == Date{} }

// String formats d as YYYY-MM-DD ("" for the zero date).
func (d Date) String() string {
	if d.IsZero() {
		return ""
	}
	return fmt.Sprintf("%04d-%02d-%02d", d.Year, int(d.Month), d.Day)
}

// Compare returns -1, 0 or +1 when d is before, equal to or after o.
func (d Date) Compare(o Date) int {
	switch {
	case d.Year != o.Year:
		return cmpInt(d.Year, o.Year)
	case d.Month != o.Month:
		return cmpInt(int(d.Month), int(o.Month))
	default:
		return cmpInt(d.Day, o.Day)
	}
}

// MarshalJSON encodes the date as a "YYYY-MM-DD" string.
func (d Date) MarshalJSON() ([]byte, error) { return json.Marshal(d.String()) }

func cmpInt(a, b int) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}
