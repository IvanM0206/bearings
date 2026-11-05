package i18n

import (
	"regexp"
	"strings"
	"testing"
)

// identicalOK lists the Spanish messages that are deliberately equal to the
// English ones. Each one is a word that is the same in both languages, or a
// program name. Every entry is checked: an entry that no longer matches the
// English text fails the test, so this list cannot go stale silently.
var identicalOK = map[string]string{
	"app.name":          "the program name, not translated",
	"common.no":         "\"no\" is the same word in Spanish",
	"type.personal":     "\"personal\" is the same word in Spanish",
	"progress.personal": "\"Personal\" is the same word in Spanish",
	"col.id":            "\"ID\" is the same abbreviation in Spanish",
	"status.changed":    "only placeholders and an arrow, which are the same in Spanish",
}

// verbRe matches fmt verbs, with an optional explicit argument index such as
// %[1]s. "%%" is not a verb here: the catalogs do not use it.
var verbRe = regexp.MustCompile(`%(\[\d+\])?[-+# 0]*\d*(\.\d+)?[a-zA-Z]`)

func verbs(s string) []string {
	return verbRe.FindAllString(s, -1)
}

func TestSpanishCatalogComplete(t *testing.T) {
	en, es := Keys("en"), Keys("es")
	if len(es) == 0 {
		t.Fatal("es catalog is empty")
	}
	for _, k := range en {
		if !Has("es", k) {
			t.Errorf("key %q is missing from the es catalog", k)
		}
	}
	for _, k := range es {
		if !Has("en", k) {
			t.Errorf("key %q is in the es catalog but not in en", k)
		}
	}
	if len(en) != len(es) {
		t.Errorf("en has %d keys, es has %d", len(en), len(es))
	}
}

func TestSpanishPlaceholdersMatch(t *testing.T) {
	for _, k := range Keys("en") {
		if !Has("es", k) {
			continue // reported by TestSpanishCatalogComplete
		}
		want := verbs(catalogs["en"][k])
		got := verbs(catalogs["es"][k])
		if strings.Join(want, " ") != strings.Join(got, " ") {
			t.Errorf("key %q: placeholders %v in en, %v in es", k, want, got)
		}
	}
}

func TestSpanishValuesNotEmptyOrUntranslated(t *testing.T) {
	for _, k := range Keys("es") {
		v := catalogs["es"][k]
		if strings.TrimSpace(v) == "" {
			t.Errorf("empty message for %q", k)
			continue
		}
		en := catalogs["en"][k]
		if v != en {
			continue
		}
		if _, ok := identicalOK[k]; !ok {
			t.Errorf("key %q is identical to English (%q): translate it, or add it to identicalOK with a reason", k, v)
		}
	}
	for k, reason := range identicalOK {
		if reason == "" {
			t.Errorf("identicalOK[%q] has no reason", k)
		}
		if !Has("es", k) {
			t.Errorf("identicalOK lists %q, which the es catalog lacks", k)
			continue
		}
		if catalogs["es"][k] != catalogs["en"][k] {
			t.Errorf("identicalOK lists %q, but its Spanish text differs from English; remove it", k)
		}
	}
}

func TestSpanishConfirmAnswers(t *testing.T) {
	// The answer words are read from the catalog; they must include the
	// Spanish affirmative and must not be empty.
	tr := New("es")
	answers := strings.Split(tr.T("confirm.yes_answers"), ",")
	found := false
	for _, a := range answers {
		if a == "" {
			t.Errorf("empty answer in %q", tr.T("confirm.yes_answers"))
		}
		if a == "s" || a == "sí" || a == "si" {
			found = true
		}
	}
	if !found {
		t.Errorf("no Spanish affirmative answer in %v", answers)
	}
}

func TestVerbsHelper(t *testing.T) {
	got := verbs("a %s b %[2]d c %-5.2f d %q")
	want := []string{"%s", "%[2]d", "%-5.2f", "%q"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("verbs = %v, want %v", got, want)
	}
}
