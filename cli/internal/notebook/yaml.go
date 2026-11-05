package notebook

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// This file implements the restricted YAML subset of SPEC §4. The YAML
// library is only used to build a node tree; every rule (no anchors,
// aliases, tags, merge keys, multiple documents, duplicate or unknown keys,
// exact types) is checked here on that tree.

type yamlNode = yaml.Node

const (
	yamlSequence = yaml.SequenceNode
	yamlAlias    = yaml.AliasNode
)

const (
	maxYAMLDepth = 8
	maxKeyInErr  = 40
)

const (
	tagStr       = "!!str"
	tagInt       = "!!int"
	tagBool      = "!!bool"
	tagTimestamp = "!!timestamp"
	tagMerge     = "!!merge"
)

var (
	yamlLineRe = regexp.MustCompile(`line ([0-9]+)`)
	decimalRe  = regexp.MustCompile(`^(?:0|[1-9][0-9]{0,15})$`)
)

// parseYAMLMapping parses src as exactly one YAML document whose root is a
// mapping and checks the structural rules of SPEC §4. An empty document
// yields an empty mapping.
func parseYAMLMapping(src []byte) (n *yaml.Node, err error) {
	defer func() {
		// The YAML library is not expected to panic, but a malformed file
		// must never crash the reader.
		if r := recover(); r != nil {
			n, err = nil, newErr(CodeYAMLSyntax, "", "")
		}
	}()
	dec := yaml.NewDecoder(bytes.NewReader(src))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, nil
		}
		return nil, yamlSyntaxErr(err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, yamlSyntaxErr(err)
		}
		return nil, &Error{Code: CodeYAMLMultipleDocs, Line: extra.Line}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 {
		return nil, newErr(CodeYAMLNotMapping, "", "")
	}
	lines := splitLines(src)
	if err := checkYAMLNode(&doc, lines, 0); err != nil {
		return nil, err
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeYAMLNotMapping, Line: root.Line}
	}
	return root, nil
}

// yamlSyntaxErr keeps only the line number of a YAML library error: its
// message may quote file content.
func yamlSyntaxErr(err error) error {
	e := &Error{Code: CodeYAMLSyntax}
	if m := yamlLineRe.FindStringSubmatch(err.Error()); m != nil {
		if l, convErr := strconv.Atoi(m[1]); convErr == nil {
			e.Line = l
		}
	}
	return e
}

func splitLines(src []byte) [][]byte { return bytes.Split(src, []byte("\n")) }

// checkYAMLNode rejects anchors, aliases, tags, merge keys and excessive
// nesting anywhere in the tree.
//
// Explicit tags are detected both through the library's TaggedStyle flag
// and by looking at the source text at the node position: the non-specific
// tag "!" is otherwise invisible in the node tree.
func checkYAMLNode(n *yaml.Node, lines [][]byte, depth int) error {
	if depth > maxYAMLDepth {
		return &Error{Code: CodeYAMLTooDeep, Line: n.Line}
	}
	if n.Kind == yaml.AliasNode {
		return &Error{Code: CodeYAMLAlias, Line: n.Line}
	}
	if n.Anchor != "" {
		return &Error{Code: CodeYAMLAnchor, Line: n.Line}
	}
	if n.Style&yaml.TaggedStyle != 0 || n.Tag == tagMerge {
		if n.Tag == tagMerge {
			return &Error{Code: CodeYAMLMergeKey, Line: n.Line}
		}
		return &Error{Code: CodeYAMLTag, Line: n.Line}
	}
	if n.Kind != yaml.DocumentNode {
		switch charAt(lines, n.Line, n.Column) {
		case '!':
			return &Error{Code: CodeYAMLTag, Line: n.Line}
		case '&':
			return &Error{Code: CodeYAMLAnchor, Line: n.Line}
		case '*':
			return &Error{Code: CodeYAMLAlias, Line: n.Line}
		}
	}
	for _, c := range n.Content {
		if err := checkYAMLNode(c, lines, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// charAt returns the character at the 1-based line and column (counted in
// characters), or 0 if out of range.
func charAt(lines [][]byte, line, col int) rune {
	if line < 1 || line > len(lines) || col < 1 {
		return 0
	}
	l := lines[line-1]
	for i := 1; len(l) > 0; i++ {
		r, size := utf8.DecodeRune(l)
		if i == col {
			return r
		}
		l = l[size:]
	}
	return 0
}

// fieldSet holds the values of a validated YAML mapping.
type fieldSet map[string]*yaml.Node

// mappingFields checks that n is a mapping whose keys are plain strings,
// unique, not merge keys and all listed in allowed.
func mappingFields(n *yaml.Node, field string, allowed ...string) (fieldSet, error) {
	if n.Kind != yaml.MappingNode {
		return nil, &Error{Code: CodeWrongType, Field: field, Line: n.Line}
	}
	out := make(fieldSet, len(n.Content)/2)
	for i := 0; i+1 < len(n.Content); i += 2 {
		k, v := n.Content[i], n.Content[i+1]
		if k.Kind != yaml.ScalarNode {
			return nil, &Error{Code: CodeYAMLUnknownKey, Field: field, Line: k.Line}
		}
		name := k.Value
		if k.Tag == tagMerge || name == "<<" {
			return nil, &Error{Code: CodeYAMLMergeKey, Line: k.Line}
		}
		qualified := qualify(field, name)
		if k.Tag != tagStr || !contains(allowed, name) {
			return nil, &Error{Code: CodeYAMLUnknownKey, Field: truncateKey(qualified), Line: k.Line}
		}
		if _, dup := out[name]; dup {
			return nil, &Error{Code: CodeYAMLDuplicateKey, Field: qualified, Line: k.Line}
		}
		out[name] = v
	}
	return out, nil
}

func qualify(parent, name string) string {
	if parent == "" {
		return name
	}
	return parent + "." + name
}

func truncateKey(s string) string {
	if !utf8.ValidString(s) {
		return "?"
	}
	if utf8.RuneCountInString(s) <= maxKeyInErr {
		return s
	}
	r := []rune(s)
	return string(r[:maxKeyInErr]) + "…"
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func wrongType(field string, n *yaml.Node) error {
	return &Error{Code: CodeWrongType, Field: field, Line: n.Line}
}

func invalid(field string, n *yaml.Node) error {
	return &Error{Code: CodeInvalidValue, Field: field, Line: n.Line}
}

// required returns the node for key or a CodeMissingField error.
func (f fieldSet) required(key, parent string) (*yaml.Node, error) {
	n, ok := f[key]
	if !ok {
		return nil, newErr(CodeMissingField, "", qualify(parent, key))
	}
	return n, nil
}

// yamlString returns the value of a string scalar (tag !!str, any style).
func yamlString(field string, n *yaml.Node) (string, error) {
	if n.Kind != yaml.ScalarNode || n.Tag != tagStr {
		return "", wrongType(field, n)
	}
	if !utf8.ValidString(n.Value) {
		return "", &Error{Code: CodeInvalidUTF8, Field: field, Line: n.Line}
	}
	return n.Value, nil
}

// yamlText is yamlString plus [checkText].
func yamlText(field string, n *yaml.Node, maxLen int, noControl bool) (string, error) {
	s, err := yamlString(field, n)
	if err != nil {
		return "", err
	}
	s, err = checkText(field, s, maxLen, noControl)
	if err != nil {
		var e *Error
		if errors.As(err, &e) {
			e.Line = n.Line
		}
		return "", err
	}
	return s, nil
}

// yamlInt returns a plain decimal integer scalar (tag !!int) in [min, max].
// Hexadecimal, octal, signed or underscore forms are rejected.
func yamlInt(field string, n *yaml.Node, lo, hi int) (int, error) {
	if n.Kind != yaml.ScalarNode || n.Tag != tagInt || n.Style != 0 {
		return 0, wrongType(field, n)
	}
	if !decimalRe.MatchString(n.Value) {
		return 0, invalid(field, n)
	}
	v, err := strconv.Atoi(n.Value)
	if err != nil {
		return 0, invalid(field, n)
	}
	if v < lo || v > hi {
		return 0, &Error{Code: CodeOutOfRange, Field: field, Max: int64(hi), Line: n.Line}
	}
	return v, nil
}

// yamlBool returns a boolean scalar (tag !!bool: true/True/TRUE,
// false/False/FALSE as in the YAML 1.2 core schema).
func yamlBool(field string, n *yaml.Node) (bool, error) {
	if n.Kind != yaml.ScalarNode || n.Tag != tagBool || n.Style != 0 {
		return false, wrongType(field, n)
	}
	switch n.Value {
	case "true", "True", "TRUE":
		return true, nil
	case "false", "False", "FALSE":
		return false, nil
	}
	return false, invalid(field, n)
}

// yamlTimestamp returns a SPEC §4 timestamp. The scalar may be plain
// (resolved by YAML as a timestamp) or quoted (a string): both spell the
// same value and other implementations may write either.
func yamlTimestamp(field string, n *yaml.Node) (time.Time, error) {
	if n.Kind != yaml.ScalarNode || (n.Tag != tagTimestamp && n.Tag != tagStr) {
		return time.Time{}, wrongType(field, n)
	}
	t, err := ParseTimestamp(n.Value)
	if err != nil {
		return time.Time{}, invalid(field, n)
	}
	return t, nil
}

// yamlDate returns a SPEC §4 date (plain or quoted, see yamlTimestamp).
func yamlDate(field string, n *yaml.Node) (Date, error) {
	if n.Kind != yaml.ScalarNode || (n.Tag != tagTimestamp && n.Tag != tagStr) {
		return Date{}, wrongType(field, n)
	}
	d, err := ParseDate(n.Value)
	if err != nil {
		return Date{}, &Error{Code: CodeInvalidDate, Field: field, Line: n.Line}
	}
	return d, nil
}

// yamlQuote encodes s as a YAML double-quoted scalar. Every character that
// could be ambiguous or invisible is escaped.
func yamlQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch {
		case r == '"':
			b.WriteString(`\"`)
		case r == '\\':
			b.WriteString(`\\`)
		case r == '\n':
			b.WriteString(`\n`)
		case r == '\t':
			b.WriteString(`\t`)
		case !yamlPrintable(r) || (r >= 0x7f && r <= 0x9f) || r == 0x2028 || r == 0x2029 || r == 0xfeff || r == utf8.RuneError:
			if r > 0xffff {
				fmt.Fprintf(&b, `\U%08x`, r)
			} else {
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// yamlPrintable reports whether r is in the YAML 1.2 printable set
// (excluding tab and line breaks, which are always escaped).
func yamlPrintable(r rune) bool {
	return (r >= 0x20 && r <= 0x7e) || r == 0x85 || (r >= 0xa0 && r <= 0xd7ff) ||
		(r >= 0xe000 && r <= 0xfffd) || (r >= 0x10000 && r <= 0x10ffff)
}
