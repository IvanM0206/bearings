// Package i18n holds the message catalogs of the CLI and resolves which
// language to use.
//
// Messages are looked up by key. Each language lives in its own file
// (en.go, later es.go) that registers a catalog from an init function, so
// adding a language is adding a file. A key missing from a language falls
// back to English, and a key missing everywhere is returned as is, so a
// bug never hides an error message.
package i18n

import (
	"fmt"
	"sort"
	"strings"
)

// Default is the language used when nothing else is configured.
const Default = "en"

// Known lists the languages accepted by --lang, BEARINGS_LANG and the
// notebook's `language` key (SPEC §5.1). A known language without a
// catalog yet is served with the English one.
var Known = []string{"en", "es"}

var catalogs = map[string]map[string]string{}

// register adds the catalog of a language. It is called from init
// functions of the catalog files.
func register(lang string, msgs map[string]string) { catalogs[lang] = msgs }

// IsKnown reports whether lang is an accepted language code.
func IsKnown(lang string) bool {
	for _, k := range Known {
		if k == lang {
			return true
		}
	}
	return false
}

// Catalogs returns the languages that have a catalog, sorted.
func Catalogs() []string {
	out := make([]string, 0, len(catalogs))
	for l := range catalogs {
		out = append(out, l)
	}
	sort.Strings(out)
	return out
}

// Keys returns the keys of the catalog of lang, sorted (nil if none).
func Keys(lang string) []string {
	var out []string
	for k := range catalogs[lang] {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Has reports whether the catalog of lang defines key.
func Has(lang, key string) bool {
	_, ok := catalogs[lang][key]
	return ok
}

// Inputs are the places the language can come from, as plain strings so
// that resolution is easy to test.
type Inputs struct {
	Flag         string // --lang
	Env          string // BEARINGS_LANG
	Notebook     string // `language` of bearings.yaml
	LCAll        string
	LCMessages   string
	Lang         string // LANG
	FlagProvided bool   // distinguishes "--lang ''" from no flag
}

// Normalize turns a locale or language string ("es_AR.UTF-8", "ES-es",
// "en@euro") into a bare lowercase language code ("es"). It returns ""
// for empty, "C" and "POSIX".
func Normalize(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	if i := strings.IndexAny(s, ".@"); i >= 0 {
		s = s[:i]
	}
	if i := strings.IndexAny(s, "_-"); i >= 0 {
		s = s[:i]
	}
	if s == "c" || s == "posix" {
		return ""
	}
	return s
}

// ErrUnknownLanguage is returned by [Resolve] for an explicit --lang value
// that is not a known language.
type ErrUnknownLanguage struct{ Value string }

func (e *ErrUnknownLanguage) Error() string { return fmt.Sprintf("unknown language %q", e.Value) }

// Resolve picks the language in this order: --lang, BEARINGS_LANG, the
// notebook's language, LC_ALL, LC_MESSAGES, LANG, then "en".
//
// An unknown value in --lang is an error. Unknown values from the
// environment, the notebook or the locale variables are skipped (the
// system locale is often something like "fr_FR", which is not a reason to
// fail).
func Resolve(in Inputs) (string, error) {
	if in.Flag != "" || in.FlagProvided {
		l := Normalize(in.Flag)
		if !IsKnown(l) {
			return "", &ErrUnknownLanguage{Value: in.Flag}
		}
		return l, nil
	}
	if l := Normalize(in.Env); IsKnown(l) {
		return l, nil
	}
	if l := Normalize(in.Notebook); IsKnown(l) {
		return l, nil
	}
	// The first non-empty locale variable wins, as in POSIX.
	for _, v := range []string{in.LCAll, in.LCMessages, in.Lang} {
		if strings.TrimSpace(v) == "" {
			continue
		}
		if l := Normalize(v); IsKnown(l) {
			return l, nil
		}
		break
	}
	return Default, nil
}

// Translator looks up messages for one language.
type Translator struct {
	lang string
	msgs map[string]string
}

// New returns a Translator for lang. A language without a catalog gets the
// English messages but keeps its code in [Translator.Lang].
func New(lang string) *Translator {
	t := &Translator{lang: lang, msgs: catalogs[lang]}
	if t.lang == "" {
		t.lang = Default
	}
	return t
}

// Lang returns the language code the translator was created for.
func (t *Translator) Lang() string { return t.lang }

// T returns the message for key, formatted with args as by fmt.Sprintf.
func (t *Translator) T(key string, args ...any) string {
	msg, ok := t.msgs[key]
	if !ok {
		msg, ok = catalogs[Default][key]
		if !ok {
			return key
		}
	}
	if len(args) == 0 {
		return msg
	}
	return fmt.Sprintf(msg, args...)
}
