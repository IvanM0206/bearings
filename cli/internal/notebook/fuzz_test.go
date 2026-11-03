package notebook

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

func addSeeds(f *testing.F, seeds ...string) {
	for _, s := range seeds {
		f.Add([]byte(s))
	}
}

// FuzzParseTopic checks that the frontmatter parser never panics or hangs,
// and that anything it accepts encodes and parses back to the same value.
func FuzzParseTopic(f *testing.F) {
	addSeeds(f, validTopic, topicWith("title", "title: &a x"), "---\n---\n", "---\n", "---",
		strings.ReplaceAll(validTopic, "\n", "\r\n"), "---\n<<: *a\n---\n", "---\n? [a]\n: b\n---\n",
		"---\ntitle: !x y\n---\n", "---\na: {b: [c, {d: e}]}\n---\n")
	f.Fuzz(func(t *testing.T, data []byte) {
		tp, err := ParseTopic("sec", "top", data)
		if err != nil {
			if CodeOf(err) == "" {
				t.Fatalf("untyped error: %v", err)
			}
			return
		}
		out, err := MarshalTopic(tp)
		if err != nil {
			t.Fatalf("accepted topic does not encode: %v", err)
		}
		back, err := ParseTopic("sec", "top", out)
		if err != nil {
			t.Fatalf("encoded topic does not parse: %v\n%q", err, out)
		}
		tp.Version, back.Version = Version{}, Version{}
		if !reflect.DeepEqual(tp, back) {
			t.Fatalf("round trip changed the topic:\n%+v\n%+v", tp, back)
		}
	})
}

func FuzzParseEntry(f *testing.F) {
	addSeeds(f, validEntry, "---\ndate: 2026-11-03\ncreated: 2026-11-03T00:00:00Z\nupdated: 2026-11-03T00:00:00Z\n---\nx")
	d := Date{2026, 11, 3}
	f.Fuzz(func(t *testing.T, data []byte) {
		e, err := ParseEntry(d, "7k2mq9xr4t", data)
		if err != nil {
			return
		}
		out, err := MarshalEntry(e)
		if err != nil {
			t.Fatalf("accepted entry does not encode: %v", err)
		}
		back, err := ParseEntry(d, "7k2mq9xr4t", out)
		if err != nil {
			t.Fatal(err)
		}
		e.Version, back.Version = Version{}, Version{}
		if !reflect.DeepEqual(e, back) {
			t.Fatalf("round trip:\n%+v\n%+v", e, back)
		}
	})
}

func FuzzParseSection(f *testing.F) {
	addSeeds(f, validSection)
	f.Fuzz(func(t *testing.T, data []byte) {
		s, err := ParseSection("sec", data)
		if err != nil {
			return
		}
		out, err := MarshalSection(s)
		if err != nil {
			t.Fatalf("accepted section does not encode: %v", err)
		}
		back, err := ParseSection("sec", out)
		if err != nil {
			t.Fatal(err)
		}
		s.Version, back.Version = Version{}, Version{}
		if !reflect.DeepEqual(s, back) {
			t.Fatalf("round trip:\n%+v\n%+v", s, back)
		}
	})
}

func FuzzParseConfig(f *testing.F) {
	addSeeds(f, "format: 1\n", "format: 1\nname: x\nlanguage: es\ntemplate:\n  id: a\n  title: b\n", "format: &a 1\nx: *a\n", "---\nformat: 1\n---\n")
	f.Fuzz(func(t *testing.T, data []byte) {
		c, err := ParseConfig(data)
		if err != nil {
			return
		}
		out, err := MarshalConfig(c)
		if err != nil {
			t.Fatalf("accepted config does not encode: %v", err)
		}
		back, err := ParseConfig(out)
		if err != nil {
			t.Fatal(err)
		}
		c.Version, back.Version = Version{}, Version{}
		if !reflect.DeepEqual(c, back) {
			t.Fatalf("round trip:\n%+v\n%+v", c, back)
		}
	})
}

// FuzzSlugify checks that derived slugs are always valid (or empty) and
// that UniqueSlug always returns a valid, free slug.
func FuzzSlugify(f *testing.F) {
	for _, s := range []string{"Técnico", "Año nuevo", "", "---", "CON", "日本語", strings.Repeat("a-", 40), "a\x00b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, title string) {
		s := Slugify(title)
		if s != "" && !slugRe.MatchString(s) {
			t.Fatalf("Slugify(%q) = %q is not a slug", title, s)
		}
		taken := map[string]bool{s: true, s + "-2": true}
		u := UniqueSlug(s, "topic", func(x string) bool { return taken[x] })
		if !ValidSlug(u) || taken[u] {
			t.Fatalf("UniqueSlug(%q) = %q", s, u)
		}
		if Slugify(s) != s {
			t.Fatalf("Slugify is not idempotent on %q", s)
		}
	})
}

func FuzzYAMLQuote(f *testing.F) {
	for _, s := range []string{"", "a", "\"", "\\", "\n", "\x1b", " ", "😀"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !utf8.ValidString(s) {
			return
		}
		src := []byte("k: " + yamlQuote(s) + "\n")
		n, err := parseYAMLMapping(src)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		fs, err := mappingFields(n, "", "k")
		if err != nil {
			t.Fatal(err)
		}
		got, err := yamlString("k", fs["k"])
		if err != nil || got != s {
			t.Fatalf("got %q want %q (%v)", got, s, err)
		}
		if bytes.ContainsAny(src[:len(src)-1], "\n\r") {
			t.Fatalf("quoted value spans lines: %q", src)
		}
	})
}

func FuzzClassifyPath(f *testing.F) {
	for _, s := range []string{"bearings.yaml", "sections/a/_section.md", "sections/a/b.md", "journal/2026/11/2026-11-03-7k2mq9xr4t.md", "../x", "a//b"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, p string) {
		info, err := ClassifyPath(p)
		if err != nil || info.Kind == PathUnknown {
			return
		}
		if PathOf(info) != p {
			t.Fatalf("PathOf(ClassifyPath(%q)) = %q", p, PathOf(info))
		}
		if strings.Contains(p, "..") || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") {
			t.Fatalf("unsafe path accepted: %q", p)
		}
	})
}
