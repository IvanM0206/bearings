package archive

import (
	"bytes"
	"compress/flate"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"strings"
	"testing"
	"time"

	"github.com/IvanM0206/bearings/cli/internal/notebook"
)

var t0 = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

// fixtureTree is a small invented notebook:
//
//	tech: build (role), private (personal)
//	team: rituals (role)
//	journal: two entries
func fixtureTree(t testing.TB) *notebook.Tree {
	t.Helper()
	d1, _ := notebook.ParseDate("2026-11-03")
	d0, _ := notebook.ParseDate("2026-10-20")
	return &notebook.Tree{
		Config: notebook.Config{Format: 1, Name: "Example role", Language: "en"},
		Sections: []notebook.Section{
			{Slug: "tech", Title: "Technical", Order: 0, Created: t0, Updated: t0, Description: "Systems and tools."},
			{Slug: "team", Title: "Team", Order: 1, Created: t0, Updated: t0},
		},
		Topics: []notebook.Topic{
			{Section: "tech", Slug: "build", Title: "Build", Status: notebook.StatusInProgress, Type: notebook.TypeRole, Order: 0,
				Resources: []notebook.Resource{{URL: "https://example.com/build", Title: "Build guide"}},
				Created:   t0, Updated: t0, Notes: "Run the build script.\n\n" + string(bytes.Repeat([]byte("-"), 5000))},
			{Section: "tech", Slug: "private", Title: "My shortcuts", Status: notebook.StatusPending, Type: notebook.TypePersonal, Order: 1,
				Created: t0, Updated: t0, Notes: "Personal notes."},
			{Section: "team", Slug: "rituals", Title: "Rituals", Status: notebook.StatusMastered, Type: notebook.TypeRole, Order: 0,
				Created: t0, Updated: t0},
		},
		Entries: []notebook.Entry{
			{ID: "0123456789", Date: d1, Section: "tech", Topic: notebook.TopicRef{Section: "tech", Topic: "build"}, Created: t0, Updated: t0, Body: "The build needs a cache."},
			{ID: "abcdefghjk", Date: d0, Created: t0, Updated: t0, Body: "First day."},
		},
	}
}

func fixtureFiles(t testing.TB) map[string][]byte {
	t.Helper()
	files, err := fixtureTree(t).Files()
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// exportBytes builds an archive from files with the real writer.
func exportBytes(t testing.TB, files map[string][]byte, opts Options) []byte {
	t.Helper()
	p, err := Prepare(files, opts)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if _, err := p.WriteTo(&buf); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readBytes(data []byte) (*notebook.Tree, error) {
	return Read(bytes.NewReader(data), int64(len(data)))
}

func codeOf(err error) Code {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return ""
}

// ---- A raw ZIP builder, for archives archive/zip would never write ------

const minimalConfig = "format: 1\n"

// rawEntry describes one entry. Zero fields get sensible defaults.
type rawEntry struct {
	name      string // central directory name
	localName *string
	method    uint16
	flags     uint16
	data      []byte // bytes stored in the archive (already compressed)
	crc       *uint32
	usize     *uint32 // declared uncompressed size (default: len(plain))
	csize     *uint32 // declared compressed size (default: len(data))
	plain     []byte  // uncompressed content, for the defaults
	creator   *uint16
	extAttrs  *uint32
	extra     []byte
	shareWith int // 1-based index of an earlier entry whose local header and data this one reuses
}

func u32(v uint32) *uint32 { return &v }
func u16(v uint16) *uint16 { return &v }

func stored(name, content string) rawEntry {
	return rawEntry{name: name, method: 0, data: []byte(content), plain: []byte(content)}
}

func deflateBytes(b []byte) []byte {
	var buf bytes.Buffer
	w, _ := flate.NewWriter(&buf, flate.BestCompression)
	_, _ = w.Write(b)
	_ = w.Close()
	return buf.Bytes()
}

func deflated(name string, content []byte) rawEntry {
	return rawEntry{name: name, method: 8, data: deflateBytes(content), plain: content}
}

func buildRaw(entries []rawEntry) []byte {
	var out bytes.Buffer
	le := binary.LittleEndian
	w16 := func(b *bytes.Buffer, v uint16) { _ = binary.Write(b, le, v) }
	w32 := func(b *bytes.Buffer, v uint32) { _ = binary.Write(b, le, v) }
	offsets := make([]uint32, len(entries))
	for i, e := range entries {
		if e.shareWith > 0 {
			offsets[i] = offsets[e.shareWith-1]
			continue
		}
		offsets[i] = le32(out.Len())
		name := e.name
		if e.localName != nil {
			name = *e.localName
		}
		w32(&out, 0x04034b50)
		w16(&out, 20)
		w16(&out, e.flags)
		w16(&out, e.method)
		w16(&out, 0)
		w16(&out, 0x21)
		w32(&out, e.crcValue())
		w32(&out, e.csizeValue())
		w32(&out, e.usizeValue())
		w16(&out, le16(len(name)))
		w16(&out, 0)
		out.WriteString(name)
		out.Write(e.data)
	}
	cdStart := out.Len()
	for i, e := range entries {
		creator := uint16(3<<8 | 20)
		if e.creator != nil {
			creator = *e.creator
		}
		ext := uint32(0o100600) << 16
		if strings.HasSuffix(e.name, "/") {
			ext = uint32(0o040700) << 16
		}
		if e.extAttrs != nil {
			ext = *e.extAttrs
		}
		w32(&out, 0x02014b50)
		w16(&out, creator)
		w16(&out, 20)
		w16(&out, e.flags)
		w16(&out, e.method)
		w16(&out, 0)
		w16(&out, 0x21)
		w32(&out, e.crcValue())
		w32(&out, e.csizeValue())
		w32(&out, e.usizeValue())
		w16(&out, le16(len(e.name)))
		w16(&out, le16(len(e.extra)))
		w16(&out, 0)
		w16(&out, 0)
		w16(&out, 0)
		w32(&out, ext)
		w32(&out, offsets[i])
		out.WriteString(e.name)
		out.Write(e.extra)
	}
	cdSize := out.Len() - cdStart
	w32(&out, 0x06054b50)
	w16(&out, 0)
	w16(&out, 0)
	w16(&out, le16(len(entries)%0x10000))
	w16(&out, le16(len(entries)%0x10000))
	w32(&out, le32(cdSize))
	w32(&out, le32(cdStart))
	w16(&out, 0)
	return out.Bytes()
}

func (e rawEntry) crcValue() uint32 {
	if e.crc != nil {
		return *e.crc
	}
	return crc32.ChecksumIEEE(e.plain)
}

func (e rawEntry) usizeValue() uint32 {
	if e.usize != nil {
		return *e.usize
	}
	return le32(len(e.plain))
}

func (e rawEntry) csizeValue() uint32 {
	if e.csize != nil {
		return *e.csize
	}
	return le32(len(e.data))
}

// withConfig prepends a valid bearings.yaml, so that a refusal is due to
// the entries under test and not to a missing configuration.
func withConfig(entries ...rawEntry) []rawEntry {
	return append([]rawEntry{stored("bearings.yaml", minimalConfig)}, entries...)
}

// randomBytes returns incompressible data.
func randomBytes(t *testing.T, n int) []byte {
	t.Helper()
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func le16(n int) uint16 {
	if n < 0 || n > 0xffff {
		panic("value does not fit in 16 bits")
	}
	return uint16(n)
}

func le32(n int) uint32 {
	if n < 0 || n > 0xffffffff {
		panic("value does not fit in 32 bits")
	}
	return uint32(n)
}
