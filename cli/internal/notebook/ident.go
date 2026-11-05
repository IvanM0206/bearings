package notebook

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxSlugLen is the maximum length of a slug (SPEC §3).
const MaxSlugLen = 64

var (
	slugRe    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$`)
	entryIDRe = regexp.MustCompile(`^[0-9a-hjkmnp-tv-z]{10}$`)
)

// reservedSlugs are the Windows reserved device names (SPEC §3).
var reservedSlugs = map[string]bool{
	"con": true, "prn": true, "aux": true, "nul": true,
	"com1": true, "com2": true, "com3": true, "com4": true, "com5": true,
	"com6": true, "com7": true, "com8": true, "com9": true,
	"lpt1": true, "lpt2": true, "lpt3": true, "lpt4": true, "lpt5": true,
	"lpt6": true, "lpt7": true, "lpt8": true, "lpt9": true,
}

// ValidSlug reports whether s is a valid section or topic slug (SPEC §3).
func ValidSlug(s string) bool {
	return len(s) <= MaxSlugLen && slugRe.MatchString(s) && !reservedSlugs[s]
}

// CheckSlug returns a CodeInvalidSlug error if s is not a valid slug.
func CheckSlug(s string) error {
	if !ValidSlug(s) {
		return newErr(CodeInvalidSlug, "", "")
	}
	return nil
}

// ValidEntryID reports whether s is a valid journal entry id (SPEC §3).
func ValidEntryID(s string) bool { return entryIDRe.MatchString(s) }

const crockford = "0123456789abcdefghjkmnpqrstvwxyz"

// NewEntryID returns a new random entry id: 50 bits from r encoded as 10
// lowercase Crockford base32 characters. r must be a cryptographically
// secure source such as crypto/rand.Reader; nil means crypto/rand.Reader.
func NewEntryID(r io.Reader) (string, error) {
	if r == nil {
		r = rand.Reader
	}
	var b [7]byte // 56 bits, 50 used
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", &Error{Code: CodeIO, Err: err}
	}
	var v uint64
	for _, c := range b {
		v = v<<8 | uint64(c)
	}
	var out [10]byte
	for i := 9; i >= 0; i-- {
		out[i] = crockford[v&31]
		v >>= 5
	}
	return string(out[:]), nil
}

// TopicRef is a topic reference: "<section-slug>/<topic-slug>" (SPEC §3).
// The zero value means "no topic".
type TopicRef struct {
	Section string
	Topic   string
}

// ParseTopicRef parses and validates a topic reference.
func ParseTopicRef(s string) (TopicRef, error) {
	sec, top, ok := strings.Cut(s, "/")
	if !ok || !ValidSlug(sec) || !ValidSlug(top) {
		return TopicRef{}, newErr(CodeInvalidRef, "", "")
	}
	return TopicRef{Section: sec, Topic: top}, nil
}

// IsZero reports whether r is the zero reference.
func (r TopicRef) IsZero() bool { return r.Section == "" && r.Topic == "" }

// String returns "<section>/<topic>", or "" for the zero reference.
func (r TopicRef) String() string {
	if r.IsZero() {
		return ""
	}
	return r.Section + "/" + r.Topic
}

func (r TopicRef) valid() bool { return ValidSlug(r.Section) && ValidSlug(r.Topic) }

// MarshalJSON encodes the reference as a string ("" for none).
func (r TopicRef) MarshalJSON() ([]byte, error) { return json.Marshal(r.String()) }

// translit maps non-ASCII letters to ASCII for slug derivation and for
// accent-insensitive search (SPEC §3: "á"→"a", "ñ"→"n").
var translit = map[rune]string{
	'à': "a", 'á': "a", 'â': "a", 'ã': "a", 'ä': "a", 'å': "a", 'ā': "a", 'ă': "a", 'ą': "a",
	'À': "a", 'Á': "a", 'Â': "a", 'Ã': "a", 'Ä': "a", 'Å': "a", 'Ā': "a", 'Ă': "a", 'Ą': "a",
	'æ': "ae", 'Æ': "ae",
	'ç': "c", 'ć': "c", 'ĉ': "c", 'ċ': "c", 'č': "c", 'Ç': "c", 'Ć': "c", 'Ĉ': "c", 'Ċ': "c", 'Č': "c",
	'ď': "d", 'đ': "d", 'ð': "d", 'Ď': "d", 'Đ': "d", 'Ð': "d",
	'è': "e", 'é': "e", 'ê': "e", 'ë': "e", 'ē': "e", 'ĕ': "e", 'ė': "e", 'ę': "e", 'ě': "e",
	'È': "e", 'É': "e", 'Ê': "e", 'Ë': "e", 'Ē': "e", 'Ĕ': "e", 'Ė': "e", 'Ę': "e", 'Ě': "e",
	'ĝ': "g", 'ğ': "g", 'ġ': "g", 'ģ': "g", 'Ĝ': "g", 'Ğ': "g", 'Ġ': "g", 'Ģ': "g",
	'ĥ': "h", 'ħ': "h", 'Ĥ': "h", 'Ħ': "h",
	'ì': "i", 'í': "i", 'î': "i", 'ï': "i", 'ĩ': "i", 'ī': "i", 'ĭ': "i", 'į': "i", 'ı': "i",
	'Ì': "i", 'Í': "i", 'Î': "i", 'Ï': "i", 'Ĩ': "i", 'Ī': "i", 'Ĭ': "i", 'Į': "i", 'İ': "i",
	'ĳ': "ij", 'Ĳ': "ij", 'ĵ': "j", 'Ĵ': "j", 'ķ': "k", 'Ķ': "k",
	'ĺ': "l", 'ļ': "l", 'ľ': "l", 'ŀ': "l", 'ł': "l", 'Ĺ': "l", 'Ļ': "l", 'Ľ': "l", 'Ŀ': "l", 'Ł': "l",
	'ñ': "n", 'ń': "n", 'ņ': "n", 'ň': "n", 'Ñ': "n", 'Ń': "n", 'Ņ': "n", 'Ň': "n",
	'ò': "o", 'ó': "o", 'ô': "o", 'õ': "o", 'ö': "o", 'ø': "o", 'ō': "o", 'ŏ': "o", 'ő': "o",
	'Ò': "o", 'Ó': "o", 'Ô': "o", 'Õ': "o", 'Ö': "o", 'Ø': "o", 'Ō': "o", 'Ŏ': "o", 'Ő': "o",
	'œ': "oe", 'Œ': "oe",
	'ŕ': "r", 'ŗ': "r", 'ř': "r", 'Ŕ': "r", 'Ŗ': "r", 'Ř': "r",
	'ś': "s", 'ŝ': "s", 'ş': "s", 'š': "s", 'Ś': "s", 'Ŝ': "s", 'Ş': "s", 'Š': "s", 'ß': "ss",
	'ţ': "t", 'ť': "t", 'ŧ': "t", 'Ţ': "t", 'Ť': "t", 'Ŧ': "t", 'þ': "th", 'Þ': "th",
	'ù': "u", 'ú': "u", 'û': "u", 'ü': "u", 'ũ': "u", 'ū': "u", 'ŭ': "u", 'ů': "u", 'ű': "u", 'ų': "u",
	'Ù': "u", 'Ú': "u", 'Û': "u", 'Ü': "u", 'Ũ': "u", 'Ū': "u", 'Ŭ': "u", 'Ů': "u", 'Ű': "u", 'Ų': "u",
	'ŵ': "w", 'Ŵ': "w",
	'ý': "y", 'ÿ': "y", 'ŷ': "y", 'Ý': "y", 'Ÿ': "y", 'Ŷ': "y",
	'ź': "z", 'ż': "z", 'ž': "z", 'Ź': "z", 'Ż': "z", 'Ž': "z",
}

// Slugify derives a slug from a title (SPEC §3): transliterate to ASCII,
// lowercase, replace runs of other characters with "-", trim to 64
// characters. It returns "" if the title has no usable characters. The
// result may be a reserved name (e.g. "con"); [UniqueSlug] resolves that and
// collisions.
func Slugify(title string) string {
	var b strings.Builder
	pendingDash := false
	emit := func(s string) {
		if pendingDash && b.Len() > 0 {
			b.WriteByte('-')
		}
		pendingDash = false
		b.WriteString(s)
	}
	for _, r := range title {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			emit(string(r))
		case r >= 'A' && r <= 'Z':
			emit(string(r + ('a' - 'A')))
		default:
			if t, ok := translit[r]; ok {
				emit(t)
			} else {
				pendingDash = true
			}
		}
		if b.Len() > 4*MaxSlugLen {
			break
		}
	}
	return trimSlug(b.String(), MaxSlugLen)
}

func trimSlug(s string, n int) string {
	if len(s) > n {
		s = s[:n]
	}
	return strings.Trim(s, "-")
}

// UniqueSlug returns base if it is a valid, unused slug, otherwise base with
// "-2", "-3"... appended (trimming base so the result fits in 64
// characters). taken reports whether a slug is already used. If base is
// empty, fallback is used instead (it must be a valid slug).
func UniqueSlug(base, fallback string, taken func(string) bool) string {
	if base == "" {
		base = fallback
	}
	if ValidSlug(base) && !taken(base) {
		return base
	}
	for i := 2; ; i++ {
		suffix := "-" + strconv.Itoa(i)
		cand := trimSlug(base, MaxSlugLen-len(suffix)) + suffix
		if cand[0] == '-' {
			cand = fallback + suffix
		}
		if ValidSlug(cand) && !taken(cand) {
			return cand
		}
	}
}

// checkText validates a single-line text field: trimmed length between 1
// and max runes, valid UTF-8 and (if noControl) without control characters.
// It returns the trimmed value.
func checkText(field, s string, maxLen int, noControl bool) (string, error) {
	if !utf8.ValidString(s) {
		return "", newErr(CodeInvalidUTF8, "", field)
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return "", newErr(CodeEmpty, "", field)
	}
	if n := utf8.RuneCountInString(s); n > maxLen {
		return "", &Error{Code: CodeTooLong, Field: field, Max: int64(maxLen)}
	}
	if noControl && hasControl(s) {
		return "", newErr(CodeControlChar, "", field)
	}
	return s, nil
}

// hasControl reports whether s contains a Unicode control character (Cc).
func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

// String implements fmt.Stringer for Code.
func (c Code) String() string { return string(c) }

var _ fmt.Stringer = Code("")
