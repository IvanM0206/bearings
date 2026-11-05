package notebook

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

const validTopic = `---
title: "Backtesting: how a strategy is tested"
status: in-progress
type: role
order: 4
resources:
  - title: Backtesting guide
    url: https://example.com/wiki/backtesting
created: 2026-11-01T12:00:00Z
updated: 2026-11-03T09:41:00Z
---
Before running a backtest, download the market data with the ingest script.

- Use at least two years of data.
`

// topicWith returns a valid topic file with the frontmatter line starting
// with key replaced by repl (or removed when repl is "").
func topicWith(key, repl string) string {
	lines := strings.Split(validTopic, "\n")
	var out []string
	done := false
	for _, l := range lines {
		if !done && strings.HasPrefix(l, key+":") {
			done = true
			if repl != "" {
				out = append(out, repl)
			}
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}

const resourcesBlock = "resources:\n  - title: Backtesting guide\n    url: https://example.com/wiki/backtesting\n"

func withResources(block string) string {
	return strings.Replace(validTopic, resourcesBlock, block, 1)
}

func addLine(after, line string) string {
	return strings.Replace(validTopic, after+"\n", after+"\n"+line+"\n", 1)
}

func wantCode(t *testing.T, err error, code Code) {
	t.Helper()
	if code == "" {
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return
	}
	if err == nil {
		t.Fatalf("expected %s, got nil", code)
	}
	if got := CodeOf(err); got != code {
		t.Fatalf("expected %s, got %s (%v)", code, got, err)
	}
}

func TestParseTopicValid(t *testing.T) {
	tp, err := ParseTopic("technical", "backtesting", []byte(validTopic))
	if err != nil {
		t.Fatal(err)
	}
	if tp.Title != "Backtesting: how a strategy is tested" || tp.Status != StatusInProgress || tp.Type != TypeRole ||
		tp.Order != 4 || len(tp.Resources) != 1 || tp.Resources[0].Title != "Backtesting guide" ||
		tp.Created != time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC) || tp.Suggested {
		t.Fatalf("unexpected topic: %+v", tp)
	}
	if !strings.HasPrefix(tp.Notes, "Before running") || strings.HasSuffix(tp.Notes, "\n") {
		t.Fatalf("unexpected notes: %q", tp.Notes)
	}
}

func TestParseTopicRules(t *testing.T) {
	long := func(n int) string { return strings.Repeat("a", n) }
	cases := []struct {
		name string
		src  string
		code Code
	}{
		{"valid", validTopic, ""},
		{"crlf accepted", strings.ReplaceAll(validTopic, "\n", "\r\n"), ""},
		{"no trailing newline", strings.TrimSuffix(validTopic, "\n"), ""},
		{"empty body", "---\ntitle: x\nstatus: pending\ntype: role\norder: 0\ncreated: 2026-01-01T00:00:00Z\nupdated: 2026-01-01T00:00:00Z\n---\n", ""},
		{"closing at eof", "---\ntitle: x\nstatus: pending\ntype: role\norder: 0\ncreated: 2026-01-01T00:00:00Z\nupdated: 2026-01-01T00:00:00Z\n---", ""},
		{"quoted timestamps", topicWith("created", `created: "2026-11-01T12:00:00Z"`), ""},
		{"suggested true", addLine("order: 4", "suggested: true"), ""},
		{"title 160", topicWith("title", "title: "+long(160)), ""},
		{"order max", topicWith("order", "order: 1000000"), ""},

		// Encoding (§4).
		{"bom", "\xef\xbb\xbf" + validTopic, CodeBOM},
		{"invalid utf8", strings.Replace(validTopic, "Before", "Bef\xffore", 1), CodeInvalidUTF8},
		{"nul byte", strings.Replace(validTopic, "Before", "Bef\x00ore", 1), CodeNULByte},
		{"no frontmatter", "title: x\n", CodeMissingFrontmatter},
		{"leading blank line", "\n" + validTopic, CodeMissingFrontmatter},
		{"unclosed frontmatter", "---\ntitle: x\n", CodeUnclosedFrontmatter},
		{"only delimiter", "---", CodeUnclosedFrontmatter},
		{"frontmatter too large", "---\n# " + long(MaxFrontmatterSize) + "\n---\n", CodeLimit},
		{"frontmatter unclosed and large", "---\n" + long(MaxFrontmatterSize+10), CodeLimit},
		{"file too large", validTopic + long(MaxFileSize), CodeLimit},

		// YAML subset (§4).
		{"anchor", topicWith("title", "title: &a x"), CodeYAMLAnchor},
		{"alias to undefined anchor", addLine("order: 4", "suggested: *a"), CodeYAMLSyntax},
		{"anchor and alias", addLine("order: 4", "suggested: &a true\nx: *a"), CodeYAMLAnchor},
		{"tag", topicWith("title", "title: !!str x"), CodeYAMLTag},
		{"non-specific tag", topicWith("title", "title: ! x"), CodeYAMLTag},
		{"local tag", topicWith("title", "title: !foo x"), CodeYAMLTag},
		{"verbatim tag", topicWith("title", "title: !<tag:yaml.org,2002:str> x"), CodeYAMLTag},
		{"merge key", addLine("order: 4", "<<: {title: x}"), CodeYAMLMergeKey},
		{"duplicate key", addLine("order: 4", "order: 5"), CodeYAMLDuplicateKey},
		{"unknown key", addLine("order: 4", "author: someone"), CodeYAMLUnknownKey},
		{"non-string key", addLine("order: 4", "1: x"), CodeYAMLUnknownKey},
		// A "---" line always closes the frontmatter, so a second document can
		// only be attempted after a "..." marker; the parser rejects it.
		{"document end marker", strings.Replace(validTopic, "order: 4\n", "order: 4\n...\nx: 1\n", 1), CodeYAMLSyntax},
		{"not a mapping", "---\n- a\n---\n", CodeYAMLNotMapping},
		{"syntax error", topicWith("title", "title: [unclosed"), CodeYAMLSyntax},
		{"too deep", addLine("order: 4", "x: [[[[[[[[[[1]]]]]]]]]]"), CodeYAMLTooDeep},
		{"empty frontmatter", "---\n---\nbody\n", CodeMissingField},

		// Required fields and types (§5.3).
		{"missing title", topicWith("title", ""), CodeMissingField},
		{"missing status", topicWith("status", ""), CodeMissingField},
		{"missing type", topicWith("type", ""), CodeMissingField},
		{"missing order", topicWith("order", ""), CodeMissingField},
		{"missing created", topicWith("created", ""), CodeMissingField},
		{"missing updated", topicWith("updated", ""), CodeMissingField},
		{"title int", topicWith("title", "title: 1"), CodeWrongType},
		{"title bool", topicWith("title", "title: true"), CodeWrongType},
		{"title null", topicWith("title", "title:"), CodeWrongType},
		{"title list", topicWith("title", "title: [a]"), CodeWrongType},
		{"title empty", topicWith("title", `title: "  "`), CodeEmpty},
		{"title too long", topicWith("title", "title: "+long(161)), CodeTooLong},
		{"title control char", topicWith("title", `title: "a\tb"`), CodeControlChar},
		{"title newline", topicWith("title", `title: "a\nb"`), CodeControlChar},
		{"title escape", topicWith("title", `title: "a\u001b[31mb"`), CodeControlChar},
		{"title c1 control", topicWith("title", `title: "a\u0085b"`), CodeControlChar},
		{"status unknown", topicWith("status", "status: done"), CodeInvalidValue},
		{"status quoted ok but int", topicWith("status", "status: 1"), CodeWrongType},
		{"type unknown", topicWith("type", "type: team"), CodeInvalidValue},
		{"suggested yes", addLine("order: 4", "suggested: yes"), CodeWrongType},
		{"suggested quoted", addLine("order: 4", `suggested: "true"`), CodeWrongType},
		{"suggested int", addLine("order: 4", "suggested: 1"), CodeWrongType},
		{"order string", topicWith("order", `order: "4"`), CodeWrongType},
		{"order float", topicWith("order", "order: 4.0"), CodeWrongType},
		{"order negative", topicWith("order", "order: -1"), CodeInvalidValue},
		{"order hex", topicWith("order", "order: 0x10"), CodeInvalidValue},
		{"order octal", topicWith("order", "order: 0o10"), CodeInvalidValue},
		{"order leading zero", topicWith("order", "order: 010"), CodeInvalidValue},
		{"order underscore", topicWith("order", "order: 1_000"), CodeInvalidValue},
		{"order plus", topicWith("order", "order: +1"), CodeInvalidValue},
		{"order too big", topicWith("order", "order: 1000001"), CodeOutOfRange},
		{"order huge", topicWith("order", "order: 99999999999999999999"), CodeWrongType},
		{"created not utc", topicWith("created", "created: 2026-11-01T12:00:00+01:00"), CodeInvalidValue},
		{"created fraction", topicWith("created", "created: 2026-11-01T12:00:00.5Z"), CodeInvalidValue},
		{"created lowercase z", topicWith("created", `created: "2026-11-01t12:00:00z"`), CodeInvalidValue},
		{"created date only", topicWith("created", "created: 2026-11-01"), CodeInvalidValue},
		{"created invalid day", topicWith("created", "created: 2026-02-30T12:00:00Z"), CodeInvalidValue},
		{"created int", topicWith("created", "created: 20261101"), CodeWrongType},
		{"updated before created", topicWith("updated", "updated: 2026-10-01T00:00:00Z"), CodeUpdatedBeforeCreated},

		// Resources (§5.3).
		{"resources not a list", withResources("resources: x\n"), CodeWrongType},
		{"resource not a mapping", strings.Replace(validTopic, "  - title: Backtesting guide\n    url: https://example.com/wiki/backtesting\n", "  - https://example.com\n", 1), CodeWrongType},
		{"resource missing url", strings.Replace(validTopic, "    url: https://example.com/wiki/backtesting\n", "", 1), CodeMissingField},
		{"resource unknown key", strings.Replace(validTopic, "    url:", "    rel: x\n    url:", 1), CodeYAMLUnknownKey},
		{"resource javascript url", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "javascript:alert(1)", 1), CodeInvalidValue},
		{"resource file url", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "file:///etc/passwd", 1), CodeInvalidValue},
		{"resource relative url", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "/wiki", 1), CodeInvalidValue},
		{"resource userinfo", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "https://user:pw@example.com/", 1), CodeInvalidValue},
		{"resource no host", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "https:///path", 1), CodeInvalidValue},
		{"resource uppercase scheme", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "HTTPS://example.com/", 1), CodeInvalidValue},
		{"resource url too long", strings.Replace(validTopic, "https://example.com/wiki/backtesting", "https://example.com/"+long(MaxURLLen), 1), CodeInvalidValue},
		{"resource empty title", strings.Replace(validTopic, "title: Backtesting guide", `title: ""`, 1), CodeEmpty},
		{"resource title too long", strings.Replace(validTopic, "title: Backtesting guide", "title: "+long(161), 1), CodeTooLong},
		{"too many resources", withResources("resources:\n" + strings.Repeat("  - url: https://example.com/\n", MaxResources+1)), CodeTooLong},
		{"max resources", withResources("resources:\n" + strings.Repeat("  - url: https://example.com/\n", MaxResources)), ""},

		// Body (§5.3).
		{"notes too large", validTopic + long(MaxTopicNotesSize), CodeTooLong},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseTopic("technical", "backtesting", []byte(c.src))
			wantCode(t, err, c.code)
		})
	}
}

func TestAliasNodeRejected(t *testing.T) {
	n := &yamlNode{Kind: yamlAlias, Line: 1}
	wantCode(t, checkYAMLNode(n, nil, 0), CodeYAMLAlias)
	if _, err := parseYAMLMapping([]byte("a: &x [1]\nb: *x\n")); err == nil {
		t.Fatal("alias accepted")
	}
}

func TestErrorsDoNotLeakContent(t *testing.T) {
	secret := "SECRET-CONTENT-42"
	srcs := []string{
		topicWith("title", "title: ["+secret),
		topicWith("title", "title: "+secret+": x: y"),
		topicWith("status", "status: "+secret),
		topicWith("order", "order: "+secret),
		topicWith("title", "title: \""+secret+"\x01\""),
		"---\n" + secret + "\n",
		topicWith("created", "created: "+secret),
	}
	for _, s := range srcs {
		_, err := ParseTopic("a", "b", []byte(s))
		if err == nil {
			t.Fatalf("expected error for %q", s)
		}
		if strings.Contains(err.Error(), secret) {
			t.Fatalf("error leaks content: %v", err)
		}
	}
}

const validSection = `---
title: "Technical"
order: 1
created: 2026-11-03T00:00:00Z
updated: 2026-11-03T00:00:00Z
---
Stack, bot architecture, tools.
`

func TestParseSectionRules(t *testing.T) {
	rep := func(old, repl string) string { return strings.Replace(validSection, old, repl, 1) }
	cases := []struct {
		name, src string
		code      Code
	}{
		{"valid", validSection, ""},
		{"title 80", rep(`title: "Technical"`, "title: "+strings.Repeat("é", 80)), ""},
		{"title too long", rep(`title: "Technical"`, "title: "+strings.Repeat("é", 81)), CodeTooLong},
		{"title trimmed empty", rep(`title: "Technical"`, `title: "   "`), CodeEmpty},
		{"title control", rep(`title: "Technical"`, `title: "a\u0007"`), CodeControlChar},
		{"missing order", rep("order: 1\n", ""), CodeMissingField},
		{"order out of range", rep("order: 1\n", "order: 2000000\n"), CodeOutOfRange},
		{"topic keys not allowed", rep("order: 1\n", "order: 1\nstatus: pending\n"), CodeYAMLUnknownKey},
		{"description 2000 chars", rep("Stack, bot architecture, tools.", strings.Repeat("ñ", 2000)), ""},
		{"description too long", rep("Stack, bot architecture, tools.", strings.Repeat("ñ", 2001)), CodeTooLong},
		{"updated before created", rep("updated: 2026-11-03T00:00:00Z", "updated: 2026-11-02T00:00:00Z"), CodeUpdatedBeforeCreated},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseSection("technical", []byte(c.src))
			wantCode(t, err, c.code)
		})
	}
	if _, err := ParseSection("Bad", []byte(validSection)); CodeOf(err) != CodeInvalidSlug {
		t.Fatalf("expected invalid slug, got %v", err)
	}
}

const validEntry = `---
date: 2026-11-03
section: technical
topic: technical/backtesting
created: 2026-11-03T09:41:00Z
updated: 2026-11-03T09:41:00Z
---
The backtester silently skips days with missing candles.
`

func TestParseEntryRules(t *testing.T) {
	d := Date{2026, 11, 3}
	rep := func(old, repl string) string { return strings.Replace(validEntry, old, repl, 1) }
	cases := []struct {
		name, src string
		code      Code
	}{
		{"valid", validEntry, ""},
		{"quoted date", rep("date: 2026-11-03", `date: "2026-11-03"`), ""},
		{"no links", rep("section: technical\ntopic: technical/backtesting\n", ""), ""},
		{"section only", rep("topic: technical/backtesting\n", ""), ""},
		{"date mismatch", rep("date: 2026-11-03", "date: 2026-11-04"), CodePathMismatch},
		{"date invalid", rep("date: 2026-11-03", "date: 2026-11-31"), CodeInvalidDate},
		{"date before 1970", rep("date: 2026-11-03", "date: 1969-12-31"), CodeInvalidDate},
		{"date with time", rep("date: 2026-11-03", "date: 2026-11-03T00:00:00Z"), CodeInvalidDate},
		{"date int", rep("date: 2026-11-03", "date: 20261103"), CodeWrongType},
		{"missing date", rep("date: 2026-11-03\n", ""), CodeMissingField},
		{"section invalid slug", rep("section: technical", "section: Technical"), CodeInvalidSlug},
		{"section int", rep("section: technical", "section: 1"), CodeWrongType},
		{"topic without section", rep("section: technical\n", ""), CodeTopicSectionMismatch},
		{"topic other section", rep("section: technical", "section: team"), CodeTopicSectionMismatch},
		{"topic not a ref", rep("topic: technical/backtesting", "topic: backtesting"), CodeInvalidRef},
		{"topic traversal", rep("topic: technical/backtesting", "topic: ../../etc"), CodeInvalidRef},
		{"empty body", rep("The backtester silently skips days with missing candles.\n", "  \n\n"), CodeEmpty},
		{"body 64k", rep("The backtester silently skips days with missing candles.", strings.Repeat("x", MaxEntryBodySize)), ""},
		{"body too large", rep("The backtester silently skips days with missing candles.", strings.Repeat("x", MaxEntryBodySize+1)), CodeTooLong},
		{"unknown key", rep("section: technical", "section: technical\ntags: x"), CodeYAMLUnknownKey},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseEntry(d, "7k2mq9xr4t", []byte(c.src))
			wantCode(t, err, c.code)
		})
	}
	if _, err := ParseEntry(d, "7K2MQ9XR4T", []byte(validEntry)); CodeOf(err) != CodeInvalidID {
		t.Fatalf("expected invalid id, got %v", err)
	}
}

func TestParseConfigRules(t *testing.T) {
	cases := []struct {
		name, src string
		code      Code
	}{
		{"minimal", "format: 1\n", ""},
		{"full", "format: 1\nname: My notebook\nlanguage: es\n", ""},
		{"template", "format: 1\ntemplate:\n  id: dev\n  title: Dev\n  description: d\n", ""},
		{"empty", "", CodeMissingField},
		{"format 2", "format: 2\n", CodeUnsupportedFormat},
		{"format 0", "format: 0\n", CodeUnsupportedFormat},
		{"format string", "format: \"1\"\n", CodeWrongType},
		{"format float", "format: 1.0\n", CodeWrongType},
		{"name empty", "format: 1\nname: \"\"\n", CodeEmpty},
		{"name too long", "format: 1\nname: " + strings.Repeat("n", 81) + "\n", CodeTooLong},
		{"language unknown", "format: 1\nlanguage: fr\n", CodeInvalidValue},
		{"unknown key", "format: 1\ntheme: dark\n", CodeYAMLUnknownKey},
		{"template missing id", "format: 1\ntemplate:\n  title: Dev\n", CodeMissingField},
		{"template bad id", "format: 1\ntemplate:\n  id: Dev!\n  title: Dev\n", CodeInvalidSlug},
		{"template missing title", "format: 1\ntemplate:\n  id: dev\n", CodeMissingField},
		{"template unknown key", "format: 1\ntemplate:\n  id: dev\n  title: Dev\n  url: x\n", CodeYAMLUnknownKey},
		{"template description too long", "format: 1\ntemplate:\n  id: dev\n  title: Dev\n  description: " + strings.Repeat("d", 501) + "\n", CodeTooLong},
		{"template not mapping", "format: 1\ntemplate: dev\n", CodeWrongType},
		{"anchor", "format: &f 1\n", CodeYAMLAnchor},
		{"bom", "\xef\xbb\xbfformat: 1\n", CodeBOM},
		{"multi doc", "format: 1\n---\nformat: 1\n", CodeYAMLMultipleDocs},
		{"too large", "format: 1\n#" + strings.Repeat("x", MaxFileSize) + "\n", CodeLimit},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := ParseConfig([]byte(c.src))
			wantCode(t, err, c.code)
		})
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	ts := time.Date(2026, 11, 3, 14, 5, 9, 0, time.UTC)
	tp := Topic{
		Section: "technical", Slug: "backtesting", Title: `Quotes "and" \backslashes\ and ñ`,
		Status: StatusMastered, Type: TypePersonal, Suggested: true, Order: 7,
		Resources: []Resource{{URL: "https://example.com/a?b=c#d", Title: "A: b"}, {URL: "http://example.org"}},
		Created:   ts, Updated: ts.Add(time.Hour), Notes: "line 1\n\n---\nline 2  \n\n\n",
	}
	data, err := MarshalTopic(tp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("line 2\n")) || bytes.Contains(data, []byte("\r")) {
		t.Fatalf("bad encoding: %q", data)
	}
	got, err := ParseTopic("technical", "backtesting", data)
	if err != nil {
		t.Fatal(err)
	}
	if got.Title != tp.Title || got.Notes != "line 1\n\n---\nline 2" || len(got.Resources) != 2 || !got.Suggested ||
		got.Resources[0] != tp.Resources[0] || !got.Updated.Equal(tp.Updated) {
		t.Fatalf("round trip mismatch: %+v", got)
	}
	again, err := MarshalTopic(got)
	if err != nil || !bytes.Equal(again, data) {
		t.Fatalf("encoding is not stable:\n%s\n%s", data, again)
	}

	e := Entry{ID: "7k2mq9xr4t", Date: Date{2026, 11, 3}, Section: "technical",
		Topic: TopicRef{"technical", "backtesting"}, Created: ts, Updated: ts, Body: "hello"}
	data, err = MarshalEntry(e)
	if err != nil {
		t.Fatal(err)
	}
	want := "---\ndate: 2026-11-03\nsection: \"technical\"\ntopic: \"technical/backtesting\"\ncreated: 2026-11-03T14:05:09Z\nupdated: 2026-11-03T14:05:09Z\n---\nhello\n"
	if string(data) != want {
		t.Fatalf("got %q", data)
	}

	s := Section{Slug: "team", Title: "Team", Order: 3, Created: ts, Updated: ts}
	data, err = MarshalSection(s)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte("---\n")) {
		t.Fatalf("empty body must end right after the delimiter: %q", data)
	}

	c := Config{Format: 1, Name: "Notes \"x\"", Language: "es", Template: &TemplateInfo{ID: "dev", Title: "Dev", Description: "multi\nline"}}
	data, err = MarshalConfig(c)
	if err != nil {
		t.Fatal(err)
	}
	gotC, err := ParseConfig(data)
	if err != nil || gotC.Name != c.Name || gotC.Template.Description != "multi\nline" {
		t.Fatalf("config round trip: %+v %v", gotC, err)
	}
}

func TestMarshalRejectsInvalid(t *testing.T) {
	ts := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	bad := []Topic{
		{Section: "a", Slug: "b", Title: "", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts},
		{Section: "a", Slug: "b", Title: "x\x1b[2J", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts},
		{Section: "a", Slug: "b", Title: "x", Status: "done", Type: TypeRole, Created: ts, Updated: ts},
		{Section: "a", Slug: "b", Title: "x", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts.Add(-time.Second)},
		{Section: "a", Slug: "b", Title: "x", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts, Order: -1},
		{Section: "a", Slug: "b", Title: "x", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts, Resources: []Resource{{URL: "javascript:x"}}},
		{Section: "a", Slug: "../b", Title: "x", Status: StatusPending, Type: TypeRole, Created: ts, Updated: ts},
	}
	for i, tp := range bad {
		if _, err := MarshalTopic(tp); err == nil {
			t.Errorf("case %d: expected error", i)
		}
	}
}

func TestYAMLQuote(t *testing.T) {
	for _, s := range []string{"", "plain", `"q"`, `\`, "a\nb", "tab\t", "\x1b[31m", " ", "\ufeff", "ñ€😀", "#x", "- y", ": z", "!tag", "&a", "*b", "'"} {
		src := "k: " + yamlQuote(s) + "\n"
		n, err := parseYAMLMapping([]byte(src))
		if err != nil {
			t.Fatalf("%q: %v", s, err)
		}
		f, _ := mappingFields(n, "", "k")
		got, err := yamlString("k", f["k"])
		if err != nil || got != s {
			t.Fatalf("%q: got %q, %v", s, got, err)
		}
	}
}

func TestErrorFormatting(t *testing.T) {
	e := &Error{Code: CodeLimit, Path: "sections", Limit: LimitSections, Max: 500}
	if !strings.Contains(e.Error(), "sections") || !strings.Contains(e.Error(), "500") {
		t.Fatal(e.Error())
	}
	if !e.Is(&Error{Code: CodeLimit}) || e.Is(&Error{Code: CodeIO}) {
		t.Fatal("Is")
	}
}
