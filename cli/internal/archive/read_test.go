package archive

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

func TestReadValidMinimal(t *testing.T) {
	tree, err := readBytes(buildRaw(withConfig()))
	if err != nil {
		t.Fatal(err)
	}
	if tree.Config.Format != 1 || len(tree.Sections) != 0 {
		t.Fatalf("%+v", tree)
	}
}

// TestZipSlip covers every unsafe name of SPEC §8 and the encoded
// variants. Each archive is otherwise valid.
func TestZipSlip(t *testing.T) {
	refused := []string{
		"../evil.md",
		"../../etc/passwd",
		"sections/../../evil.md",
		"sections/tech/../../../evil.md",
		"..",
		"/etc/passwd",
		"/bearings.yaml",
		"//server/share/x",
		`..\evil.md`,
		`sections\tech\_section.md`,
		`C:\Windows\evil`,
		"C:/evil.md",
		"C:evil.md",
		"c:",
		"bearings.yaml:stream",
		"sections//tech/_section.md",
		"sections/tech//x.md",
		"./bearings.yaml",
		"sections/./tech/_section.md",
		"sections/tech/.",
		"bearings.yaml\x00.txt",
		"\x00",
		"sections/tech\x1b[31m/x.md",
		"sections/tech\n/x.md",
		"sections/tech\u0085/x.md", // C1 control (NEL)
		"sections/tech\x7f/x.md",
		"\xc0\xae\xc0\xae/evil.md", // overlong UTF-8 for ".."
		"\xff\xfe/evil.md",         // invalid UTF-8
		"sections/\xed\xa0\x80/x",  // UTF-16 surrogate encoded in UTF-8
		".. /evil.md",              // Windows drops the trailing space
		"../",
		"... /x",
		strings.Repeat("a/", 2) + strings.Repeat("b", MaxNameLen),
	}
	for _, name := range refused {
		t.Run(fmt.Sprintf("%q", name), func(t *testing.T) {
			_, err := readBytes(buildRaw(withConfig(stored(name, "x"))))
			if c := codeOf(err); c != CodeBadName {
				t.Fatalf("got %v (%s), want %s", err, c, CodeBadName)
			}
		})
	}

	// Look-alikes that are harmless literal names: accepted, but not part
	// of the layout, so they are ignored and never written anywhere.
	ignored := []string{
		"%2e%2e/evil.md",
		"%2e%2e%2fevil.md",
		"..%c0%afevil.md",
		"\u2025/evil.md",       // two dot leader
		"\uff0e\uff0e/evil.md", // fullwidth full stops
		"sections/tech/..x.md", // dots inside a segment
		"...x/evil.md",         // leading dots
		"sections/te.ch/_section.md",
		"sections/tech/x. /y", // trailing dot and space inside
	}
	for _, name := range ignored {
		t.Run(fmt.Sprintf("ignored %q", name), func(t *testing.T) {
			tree, err := readBytes(buildRaw(withConfig(stored(name, "x"))))
			if err != nil {
				t.Fatal(err)
			}
			if len(tree.Sections)+len(tree.Topics)+len(tree.Entries) != 0 {
				t.Fatalf("%+v", tree)
			}
		})
	}
}

func TestDepthLimit(t *testing.T) {
	_, err := readBytes(buildRaw(withConfig(stored("a/b/c/d/e.md", "x"))))
	if codeOf(err) != CodeDepth {
		t.Fatalf("%v", err)
	}
	if _, err := readBytes(buildRaw(withConfig(stored("a/b/c/d.md", "x")))); err != nil {
		t.Fatalf("depth 4 must be accepted: %v", err)
	}
}

func TestDuplicates(t *testing.T) {
	sec := "---\ntitle: T\norder: 0\ncreated: 2026-11-03T12:00:00Z\nupdated: 2026-11-03T12:00:00Z\n---\n"
	cases := map[string][]rawEntry{
		"exact":               {stored("bearings.yaml", minimalConfig), stored("bearings.yaml", minimalConfig)},
		"case":                {stored("bearings.yaml", minimalConfig), stored("BEARINGS.yaml", minimalConfig)},
		"case in a directory": withConfig(stored("sections/tech/_section.md", sec), stored("Sections/tech/_section.md", sec)),
		"kelvin sign":         withConfig(stored("sections/key/_section.md", sec), stored("sections/\u212aey/_section.md", sec)),
		"unicode case":        withConfig(stored("Ä/x", "x"), stored("ä/x", "x")),
		"trailing dot":        {stored("bearings.yaml", minimalConfig), stored("bearings.yaml.", minimalConfig)},
		"trailing space":      withConfig(stored("sections/tech/_section.md", sec), stored("sections/tech /_section.md", sec)),
		"file then directory": withConfig(stored("sections/tech", "x"), stored("sections/tech/_section.md", sec)),
		"directory then file": withConfig(stored("sections/tech/_section.md", sec), stored("sections/tech", "x")),
		"file and dir entry":  withConfig(stored("sections/tech", "x"), stored("sections/tech/", "")),
		"duplicate dir entry": withConfig(stored("sections/", ""), stored("sections/", "")),
		"dir entry case":      withConfig(stored("sections/", ""), stored("SECTIONS/", "")),
	}
	for name, entries := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readBytes(buildRaw(entries)); codeOf(err) != CodeDuplicate {
				t.Fatalf("got %v", err)
			}
		})
	}
	// A directory entry after the files it contains is fine.
	ok := withConfig(stored("sections/tech/_section.md", sec), stored("sections/tech/", ""), stored("sections/", ""))
	if _, err := readBytes(buildRaw(ok)); err != nil {
		t.Fatal(err)
	}
}

func TestEntryTypes(t *testing.T) {
	unixMode := func(m uint32) *uint32 { return u32(m << 16) }
	cases := map[string]rawEntry{
		"symlink":            {name: "link.md", data: []byte("/etc/passwd"), plain: []byte("/etc/passwd"), extAttrs: unixMode(0o120777)},
		"symlink to topic":   {name: "sections/tech/x.md", data: []byte("../../x"), plain: []byte("../../x"), extAttrs: unixMode(0o120644)},
		"char device":        {name: "dev", extAttrs: unixMode(0o020600)},
		"block device":       {name: "dev", extAttrs: unixMode(0o060600)},
		"fifo":               {name: "fifo", extAttrs: unixMode(0o010600)},
		"socket":             {name: "sock", extAttrs: unixMode(0o140600)},
		"directory bits":     {name: "notadir", extAttrs: unixMode(0o040700)},
		"symlink from FAT":   {name: "link", creator: u16(0), extAttrs: u32(0o120777 << 16)},
		"symlink from NTFS":  {name: "link", creator: u16(11 << 8), extAttrs: u32(0o120777 << 16)},
		"dos directory flag": {name: "notadir", creator: u16(0), extAttrs: u32(0x10)},
		"dir with symlink":   {name: "dir/", extAttrs: unixMode(0o120777)},
		"dir with content":   {name: "dir/", data: []byte("x"), plain: []byte("x"), extAttrs: unixMode(0o040700)},
	}
	for name, e := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeEntryType {
				t.Fatalf("got %v", err)
			}
		})
	}
	// The same entry written by archive/zip with SetMode is refused too.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("bearings.yaml")
	_, _ = w.Write([]byte(minimalConfig))
	fh := &zip.FileHeader{Name: "sections/tech/x.md"}
	fh.SetMode(fs.ModeSymlink | 0o777)
	w, _ = zw.CreateHeader(fh)
	_, _ = w.Write([]byte("/etc/passwd"))
	_ = zw.Close()
	if _, err := readBytes(buf.Bytes()); codeOf(err) != CodeEntryType {
		t.Fatalf("got %v", err)
	}
}

func TestEncryptedAndMethods(t *testing.T) {
	enc := func(flag uint16) rawEntry {
		e := stored("x.md", "x")
		e.flags = flag
		return e
	}
	for _, f := range []uint16{0x1, 0x40, 0x2000, 0x41} {
		if _, err := readBytes(buildRaw(withConfig(enc(f)))); codeOf(err) != CodeEncrypted {
			t.Errorf("flag %#x: %v", f, err)
		}
	}
	for _, m := range []uint16{1, 6, 9, 12, 14, 93, 95, 98, 99} {
		e := stored("x.md", "x")
		e.method = m
		if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeMethod {
			t.Errorf("method %d: %v", m, err)
		}
	}
}

func TestEntrySizeLimit(t *testing.T) {
	big := bytes.Repeat([]byte("abcdefghij0123456789"), MaxEntrySize/20+1)
	// Declared honestly.
	if _, err := readBytes(buildRaw(withConfig(stored("sections/tech/x.md", string(big))))); codeOf(err) != CodeEntrySize {
		t.Fatalf("declared: %v", err)
	}
	// Declared small, actually large: caught while inflating.
	e := deflated("notes.txt", big)
	e.usize = u32(100)
	e.csize = nil
	_, err := readBytes(buildRaw(withConfig(e)))
	if c := codeOf(err); c != CodeEntrySize && c != CodeFormat {
		t.Fatalf("lying size: %v", err)
	}
	// Exactly at the limit is fine.
	exact := randomBytes(t, MaxEntrySize)
	if _, err := readBytes(buildRaw(withConfig(deflated("notes.txt", exact)))); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
}

func TestRatioLimit(t *testing.T) {
	zeros := make([]byte, 1<<20)
	// Honest headers: refused before inflating.
	if _, err := readBytes(buildRaw(withConfig(deflated("notes.txt", zeros)))); codeOf(err) != CodeRatio {
		t.Fatalf("declared: %v", err)
	}
	// Lying headers (declared 1:1): refused while inflating.
	e := deflated("notes.txt", zeros)
	e.usize = u32(le32(len(e.data)))
	_, err := readBytes(buildRaw(withConfig(e)))
	if c := codeOf(err); c != CodeRatio && c != CodeFormat {
		t.Fatalf("lying: %v", err)
	}
	// Compressed size zero with content.
	e = stored("notes.txt", "")
	e.usize = u32(10)
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeRatio {
		t.Fatalf("zero compressed size: %v", err)
	}
	// The boundary: 100:1 passes the header check (the bogus data then
	// fails to inflate), 101:1 does not.
	e = rawEntry{name: "notes.txt", method: 8, data: make([]byte, 10), usize: u32(1000)}
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) == CodeRatio {
		t.Fatalf("100:1 must pass the header check: %v", err)
	}
	e.usize = u32(1001)
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeRatio {
		t.Fatalf("101:1: %v", err)
	}
}

// TestTotalSizeLimit uses the real 200 MiB limit with declared sizes
// (refused before anything is inflated), then a lowered limit to show the
// real bytes are counted.
func TestTotalSizeLimit(t *testing.T) {
	var entries []rawEntry
	for i := range MaxTotalSize/MaxEntrySize + 1 {
		// Declared 1 MiB, ratio exactly 100:1, data never inflated.
		data := make([]byte, MaxEntrySize/MaxRatio+1)
		entries = append(entries, rawEntry{name: fmt.Sprintf("junk/%d", i), method: 8, data: data, usize: u32(MaxEntrySize), crc: u32(0)})
	}
	if _, err := readBytes(buildRaw(withConfig(entries...))); codeOf(err) != CodeTotalSize {
		t.Fatalf("declared: %v", err)
	}

	defer func(v int64) { maxTotalSize = v }(maxTotalSize)
	maxTotalSize = 3 << 20
	entries = nil
	for i := range 4 {
		entries = append(entries, deflated(fmt.Sprintf("junk/%d", i), randomBytes(t, MaxEntrySize)))
	}
	if _, err := readBytes(buildRaw(withConfig(entries...))); codeOf(err) != CodeTotalSize {
		t.Fatalf("real: %v", err)
	}
	// Lying headers: every entry declares 10 bytes.
	for i := range entries {
		entries[i].usize = u32(10)
	}
	_, err := readBytes(buildRaw(withConfig(entries...)))
	if c := codeOf(err); c != CodeTotalSize && c != CodeFormat && c != CodeRatio {
		t.Fatalf("lying: %v", err)
	}
}

func TestTooManyEntries(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a large archive")
	}
	entries := make([]rawEntry, 0, MaxEntries+1)
	entries = append(entries, stored("bearings.yaml", minimalConfig))
	for i := range MaxEntries {
		entries = append(entries, stored(fmt.Sprintf("j/%d", i), ""))
	}
	if _, err := readBytes(buildRaw(entries)); codeOf(err) != CodeTooMany {
		t.Fatalf("got %v", err)
	}
	if _, err := readBytes(buildRaw(entries[:MaxEntries])); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
}

func TestTooManyEntriesLowLimit(t *testing.T) {
	defer func(v int) { maxEntries = v }(maxEntries)
	maxEntries = 3
	if _, err := readBytes(buildRaw(withConfig(stored("a", ""), stored("b", ""), stored("c", "")))); codeOf(err) != CodeTooMany {
		t.Fatalf("got %v", err)
	}
}

func TestLocalHeaderMismatch(t *testing.T) {
	other := "bearings.yamx"
	e := stored("bearings.yaml", minimalConfig)
	e.localName = &other
	if _, err := readBytes(buildRaw([]rawEntry{e})); codeOf(err) != CodeFormat {
		t.Fatalf("name: %v", err)
	}
	e = stored("x.md", "x")
	e.flags = 0x1
	raw := buildRaw(withConfig(e))
	// Clear the encryption flag in the central directory only.
	i := bytes.LastIndex(raw, []byte("PK\x01\x02"))
	raw[i+8] = 0
	if _, err := readBytes(raw); codeOf(err) != CodeFormat {
		t.Fatalf("flags: %v", err)
	}
}

func TestOverlappingEntries(t *testing.T) {
	content := bytes.Repeat([]byte("0123456789abcdef"), 4000)
	first := deflated("a.txt", content)
	second := first
	second.name = "b.txt"
	second.shareWith = 2 // index 1 after withConfig, 1-based
	if _, err := readBytes(buildRaw(withConfig(first, second))); codeOf(err) != CodeFormat {
		t.Fatalf("got %v", err)
	}
}

func TestUnicodePathExtra(t *testing.T) {
	e := stored("notes.txt", "x")
	e.extra = []byte{0x75, 0x70, 6, 0, 1, 0, 0, 0, 0, 'x'}
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeBadName {
		t.Fatalf("got %v", err)
	}
	e.extra = []byte{0x01, 0x02, 9, 0}
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeFormat {
		t.Fatalf("malformed extra: %v", err)
	}
}

func TestCorruptData(t *testing.T) {
	e := stored("notes.txt", "hello")
	e.crc = u32(1)
	if _, err := readBytes(buildRaw(withConfig(e))); codeOf(err) != CodeFormat {
		t.Fatalf("checksum: %v", err)
	}
	for _, data := range [][]byte{nil, []byte("PK"), []byte("not a zip file at all, really"), make([]byte, 100)} {
		if _, err := readBytes(data); codeOf(err) != CodeFormat {
			t.Fatalf("%q: %v", data, err)
		}
	}
}

func TestInvalidNotebookFileInArchive(t *testing.T) {
	files := fixtureFiles(t)
	files["sections/tech/build.md"] = []byte("---\ntitle: x\nunknown: 1\n---\n")
	var entries []rawEntry
	for p, d := range files {
		entries = append(entries, stored(p, string(d)))
	}
	_, err := readBytes(buildRaw(entries))
	var ne *notebook.Error
	if !errors.As(err, &ne) || ne.Path != "sections/tech/build.md" {
		t.Fatalf("got %v", err)
	}
	if strings.Contains(err.Error(), "unknown: 1") {
		t.Fatal("the error must not contain file contents")
	}
	// A journal entry under the wrong month.
	entries = withConfig(stored("journal/2026/12/2026-11-03-0123456789.md", "x"))
	if _, err := readBytes(buildRaw(entries)); notebook.CodeOf(err) != notebook.CodePathMismatch {
		t.Fatalf("got %v", err)
	}
	// No bearings.yaml.
	if _, err := readBytes(buildRaw([]rawEntry{stored("README.md", "x")})); notebook.CodeOf(err) != notebook.CodeNotNotebook {
		t.Fatalf("got %v", err)
	}
}

func TestUnknownPathsAreIgnored(t *testing.T) {
	files := fixtureFiles(t)
	var entries []rawEntry
	for p, d := range files {
		entries = append(entries, deflated(p, d))
	}
	entries = append(entries, stored(".git/config", "[core]"), stored("README.md", "hi"), stored("sections/tech/notes.txt", "x"),
		stored("sections/", ""))
	tree, err := readBytes(buildRaw(entries))
	if err != nil {
		t.Fatal(err)
	}
	got, err := tree.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(files) {
		t.Fatalf("got %d files, want %d", len(got), len(files))
	}
}

func TestReadFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "good.zip")
	if err := os.WriteFile(good, exportBytes(t, fixtureFiles(t), Options{}), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(good); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(filepath.Join(dir, "missing.zip")); codeOf(err) != CodeIO {
		t.Fatalf("missing: %v", err)
	}
	if _, err := ReadFile(dir); codeOf(err) != CodeNotRegular {
		t.Fatalf("directory: %v", err)
	}
	// Bounded before opening: a sparse file just over the limit.
	huge := filepath.Join(dir, "huge.zip")
	f, err := os.Create(huge)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(MaxArchiveSize + 1); err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := ReadFile(huge); codeOf(err) != CodeTooLarge {
		t.Fatalf("huge: %v", err)
	}
	if _, err := Read(bytes.NewReader(nil), MaxArchiveSize+1); codeOf(err) != CodeTooLarge {
		t.Fatalf("Read: %v", err)
	}
	if runtime.GOOS != "windows" {
		link := filepath.Join(dir, "link.zip")
		if err := os.Symlink(good, link); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadFile(link); err != nil {
			t.Fatalf("a symlink the user names is followed: %v", err)
		}
	}
}

func TestErrorQuotesName(t *testing.T) {
	_, err := readBytes(buildRaw(withConfig(stored("evil\x1b]0;title\x07/../x", "x"))))
	var e *Error
	if !errors.As(err, &e) {
		t.Fatal(err)
	}
	q := e.QuotedName()
	for _, r := range q {
		if r < 0x20 || r > 0x7e {
			t.Fatalf("quoted name %q has a non-printable character", q)
		}
	}
}
