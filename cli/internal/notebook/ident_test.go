package notebook

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidSlug(t *testing.T) {
	cases := map[string]bool{
		"a": true, "technical": true, "a-b": true, "a1-2b": true, "0": true,
		strings.Repeat("a", 64): true, strings.Repeat("a", 65): false,
		"": false, "-a": false, "a-": false, "A": false, "a_b": false, "a b": false, "á": false,
		"a/b": false, "..": false, ".": false, "a.md": false, "_section": false,
		"con": false, "prn": false, "aux": false, "nul": false, "com1": false, "com9": false, "lpt1": false, "lpt9": false,
		"com0": true, "com10": true, "console": true, "con-2": true,
		"a--b": true,
	}
	for s, want := range cases {
		if got := ValidSlug(s); got != want {
			t.Errorf("ValidSlug(%q) = %v, want %v", s, got, want)
		}
	}
	wantCode(t, CheckSlug("Bad"), CodeInvalidSlug)
	wantCode(t, CheckSlug("ok"), "")
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Technical":                     "technical",
		"Gestión de riesgo":             "gestion-de-riesgo",
		"Año nuevo, ¡vida nueva!":       "ano-nuevo-vida-nueva",
		"  --Hello__World--  ":          "hello-world",
		"Straße & Œuvre":                "strasse-oeuvre",
		"Backtesting: how it's tested":  "backtesting-how-it-s-tested",
		"日本語":                           "",
		"":                              "",
		"CON":                           "con",
		strings.Repeat("ab ", 40):       strings.TrimSuffix(strings.Repeat("ab-", 21), "-") + "-a",
		"a" + strings.Repeat("-", 100):  "a",
		strings.Repeat("x", 63) + " yz": strings.Repeat("x", 63),
	}
	for in, want := range cases {
		if got := Slugify(in); got != want {
			t.Errorf("Slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUniqueSlug(t *testing.T) {
	taken := map[string]bool{"a": true, "a-2": true, "con-2": true}
	f := func(s string) bool { return taken[s] }
	if got := UniqueSlug("a", "x", f); got != "a-3" {
		t.Fatal(got)
	}
	if got := UniqueSlug("b", "x", f); got != "b" {
		t.Fatal(got)
	}
	if got := UniqueSlug("", "topic", f); got != "topic" {
		t.Fatal(got)
	}
	if got := UniqueSlug("con", "x", f); got != "con-3" {
		t.Fatal(got)
	}
	long := strings.Repeat("y", 64)
	taken[long] = true
	got := UniqueSlug(long, "x", f)
	if got != strings.Repeat("y", 62)+"-2" || !ValidSlug(got) {
		t.Fatal(got)
	}
	// Trimming must not leave a trailing hyphen before the suffix.
	base := strings.Repeat("z", 61) + "-ab"
	taken[base] = true
	if got := UniqueSlug(base, "x", f); !ValidSlug(got) || strings.Contains(got, "--") {
		t.Fatal(got)
	}
}

func TestEntryID(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id, err := NewEntryID(nil)
		if err != nil {
			t.Fatal(err)
		}
		if !ValidEntryID(id) {
			t.Fatalf("invalid id %q", id)
		}
		if seen[id] {
			t.Fatalf("duplicate id %q", id)
		}
		seen[id] = true
	}
	// Deterministic encoding: all-ones gives the last symbol everywhere.
	id, err := NewEntryID(bytes.NewReader(bytes.Repeat([]byte{0xff}, 7)))
	if err != nil || id != "zzzzzzzzzz" {
		t.Fatalf("got %q, %v", id, err)
	}
	if _, err := NewEntryID(bytes.NewReader(nil)); err == nil {
		t.Fatal("expected error on short random source")
	}
	for _, bad := range []string{"", "7k2mq9xr4", "7k2mq9xr4tt", "7K2MQ9XR4T", "7k2mq9xr4i", "7k2mq9xr4l", "7k2mq9xr4o", "7k2mq9xr4u", "../../etc/"} {
		if ValidEntryID(bad) {
			t.Errorf("ValidEntryID(%q) = true", bad)
		}
	}
}

func TestParseTopicRef(t *testing.T) {
	r, err := ParseTopicRef("technical/backtesting")
	if err != nil || r.Section != "technical" || r.Topic != "backtesting" || r.String() != "technical/backtesting" {
		t.Fatal(r, err)
	}
	for _, bad := range []string{"", "a", "a/", "/b", "a/b/c", "../b", "a/..", "A/b", "a\\b", "con/b", "a/nul"} {
		if _, err := ParseTopicRef(bad); CodeOf(err) != CodeInvalidRef {
			t.Errorf("ParseTopicRef(%q): %v", bad, err)
		}
	}
	if (TopicRef{}).String() != "" || !(TopicRef{}).IsZero() {
		t.Fatal("zero ref")
	}
}

func TestDates(t *testing.T) {
	for _, ok := range []string{"1970-01-01", "9999-12-31", "2024-02-29"} {
		d, err := ParseDate(ok)
		if err != nil || d.String() != ok {
			t.Errorf("%s: %v %v", ok, d, err)
		}
	}
	for _, bad := range []string{"1969-12-31", "2023-02-29", "2026-13-01", "2026-00-10", "2026-1-1", "26-01-01", "2026-01-01T00:00:00Z", "10000-01-01", " 2026-01-01"} {
		if _, err := ParseDate(bad); err == nil {
			t.Errorf("ParseDate(%q) accepted", bad)
		}
	}
	a, _ := ParseDate("2026-01-02")
	b, _ := ParseDate("2026-02-01")
	if a.Compare(b) != -1 || b.Compare(a) != 1 || a.Compare(a) != 0 {
		t.Fatal("compare")
	}
	for _, bad := range []string{"2026-11-03T14:05:09", "2026-11-03T14:05:09+00:00", "2026-11-03 14:05:09Z", "2026-11-03T24:00:00Z", "2026-11-03T14:05:60Z"} {
		if _, err := ParseTimestamp(bad); err == nil {
			t.Errorf("ParseTimestamp(%q) accepted", bad)
		}
	}
}

func TestClassifyPath(t *testing.T) {
	cases := []struct {
		path string
		kind PathKind
		code Code
	}{
		{"bearings.yaml", PathConfig, ""},
		{"sections/technical/_section.md", PathSection, ""},
		{"sections/technical/backtesting.md", PathTopic, ""},
		{"journal/2026/11/2026-11-03-7k2mq9xr4t.md", PathEntry, ""},
		{"journal/2026/12/2026-11-03-7k2mq9xr4t.md", PathUnknown, CodePathMismatch},
		{"journal/2025/11/2026-11-03-7k2mq9xr4t.md", PathUnknown, CodePathMismatch},
		{"journal/2026/02/2026-02-30-7k2mq9xr4t.md", PathUnknown, CodeInvalidDate},
		{"journal/2026/11/2026-11-03-7K2MQ9XR4T.md", PathUnknown, ""},
		{"README.md", PathUnknown, ""},
		{".git/config", PathUnknown, ""},
		{"sections/Technical/_section.md", PathUnknown, ""},
		{"sections/technical/sub/x.md", PathUnknown, ""},
		{"sections/technical/con.md", PathUnknown, ""},
		{"sections/technical/.backtesting.md.swp", PathUnknown, ""},
		{"sections/technical/backtesting.MD", PathUnknown, ""},
		{"sections/../bearings.yaml", PathUnknown, ""},
		{"/bearings.yaml", PathUnknown, ""},
		{"journal/2026/11/x.md", PathUnknown, ""},
	}
	for _, c := range cases {
		info, err := ClassifyPath(c.path)
		if CodeOf(err) != c.code || (err == nil && info.Kind != c.kind) {
			t.Errorf("ClassifyPath(%q) = %v, %v", c.path, info, err)
			continue
		}
		if err == nil && info.Kind != PathUnknown && PathOf(info) != c.path {
			t.Errorf("PathOf(%v) = %q", info, PathOf(info))
		}
	}
}
