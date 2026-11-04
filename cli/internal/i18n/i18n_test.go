package i18n

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"":            "",
		"C":           "",
		"POSIX":       "",
		"en":          "en",
		"EN":          "en",
		"es_AR.UTF-8": "es",
		"es-ES":       "es",
		"en_US@euro":  "en",
		"  es  ":      "es",
		"fr_FR.UTF-8": "fr",
		"C.UTF-8":     "",
		"pt_BR":       "pt",
	}
	for in, want := range tests {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestResolveOrder(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		want string
	}{
		{"nothing", Inputs{}, "en"},
		{"flag wins", Inputs{Flag: "es", Env: "en", Notebook: "en", LCAll: "en"}, "es"},
		{"env over notebook", Inputs{Env: "es", Notebook: "en", LCAll: "en"}, "es"},
		{"notebook over locale", Inputs{Notebook: "es", LCAll: "en_US.UTF-8"}, "es"},
		{"LC_ALL over LC_MESSAGES", Inputs{LCAll: "es_ES.UTF-8", LCMessages: "en", Lang: "en"}, "es"},
		{"LC_MESSAGES over LANG", Inputs{LCMessages: "es_ES", Lang: "en_US"}, "es"},
		{"LANG", Inputs{Lang: "es_AR.UTF-8"}, "es"},
		{"unknown env skipped", Inputs{Env: "fr", Notebook: "es"}, "es"},
		{"unknown notebook skipped", Inputs{Notebook: "fr", Lang: "es"}, "es"},
		{"first locale var wins even if unsupported", Inputs{LCAll: "fr_FR", Lang: "es_ES"}, "en"},
		{"C locale gives default", Inputs{LCAll: "C", Lang: "es_ES"}, "en"},
		{"blank locale var skipped", Inputs{LCAll: " ", Lang: "es_ES"}, "es"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Resolve(tc.in)
			if err != nil || got != tc.want {
				t.Fatalf("Resolve = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestResolveUnknownFlag(t *testing.T) {
	for _, in := range []Inputs{{Flag: "fr"}, {Flag: "", FlagProvided: true}, {Flag: "klingon"}} {
		_, err := Resolve(in)
		var u *ErrUnknownLanguage
		if !errors.As(err, &u) {
			t.Errorf("Resolve(%+v) error = %v, want ErrUnknownLanguage", in, err)
		}
	}
}

func TestTranslator(t *testing.T) {
	tr := New("en")
	if got := tr.T("no.such.key"); got != "no.such.key" {
		t.Errorf("missing key = %q, want the key", got)
	}
	// A known language without a catalog uses English but keeps its code.
	es := New("es")
	if es.Lang() != "es" {
		t.Errorf("Lang = %q", es.Lang())
	}
	if es.T("common.yes") != tr.T("common.yes") {
		t.Errorf("es without catalog must fall back to en")
	}
	if got := tr.T("path.title", 1, 2); !strings.Contains(got, "1") || !strings.Contains(got, "2") {
		t.Errorf("args not formatted: %q", got)
	}
}

func TestEnglishCatalogComplete(t *testing.T) {
	if len(Keys("en")) == 0 {
		t.Fatal("en catalog is empty")
	}
	for _, k := range Keys("en") {
		if strings.TrimSpace(catalogs["en"][k]) == "" {
			t.Errorf("empty message for %q", k)
		}
	}
	// Every other catalog must not define keys that en lacks.
	for _, l := range Catalogs() {
		for _, k := range Keys(l) {
			if !Has("en", k) {
				t.Errorf("catalog %s has key %q missing from en", l, k)
			}
		}
	}
}
