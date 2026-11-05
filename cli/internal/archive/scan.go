package archive

import (
	"encoding/binary"
	"errors"
	"hash/fnv"
	"io"
	"sort"
)

// The pre-scan walks the central directory with constant memory before
// archive/zip loads it. archive/zip keeps one *zip.File per record and
// reads records until it finds a bad one, whatever count the archive
// declares, so without this an archive of a few hundred MiB of tiny
// records could make it allocate gigabytes. It also checks what
// archive/zip does not: that each local header agrees with its central
// directory record, and that no two entries share data (the
// "overlapping files" zip bomb).
//
// It locates the end of central directory record exactly as archive/zip
// does (same search, same zip64 handling, same base offset correction),
// and the caller checks afterwards that archive/zip found the same
// records, so a disagreement is refused instead of trusted.

const (
	sigLocal     = 0x04034b50
	sigCentral   = 0x02014b50
	sigEnd       = 0x06054b50
	sigEnd64     = 0x06064b50
	sigEnd64Loc  = 0x07064b50
	lenLocal     = 30
	lenCentral   = 46
	lenEnd       = 22
	lenEnd64     = 56
	lenEnd64Loc  = 20
	zip64ExtraID = 0x0001
)

// scanResult summarizes the central directory.
type scanResult struct {
	hashes []uint64 // FNV-1a of each record's name, in directory order
}

type dirEnd struct {
	records uint64
	size    uint64
	offset  uint64
}

func readAt(r io.ReaderAt, off int64, n int) ([]byte, error) {
	buf := make([]byte, n)
	k, err := r.ReadAt(buf, off)
	if k == n {
		return buf, nil // io.ReaderAt may report io.EOF along with a full read
	}
	if err == nil || errors.Is(err, io.EOF) {
		err = io.ErrUnexpectedEOF
	}
	return nil, err
}

// findEnd mirrors archive/zip's readDirectoryEnd.
func findEnd(r io.ReaderAt, size int64) (d dirEnd, endOffset int64, err error) {
	var buf []byte
	for i, bLen := range []int64{1024, 65 * 1024} {
		bLen = min(bLen, size)
		buf = make([]byte, int(bLen))
		if _, err := r.ReadAt(buf, size-bLen); err != nil && !errors.Is(err, io.EOF) {
			return d, 0, err
		}
		if p := findEndSignature(buf); p >= 0 {
			buf = buf[p:]
			endOffset = size - bLen + int64(p)
			break
		}
		if i == 1 || bLen == size {
			return d, 0, errFormat
		}
	}
	le := binary.LittleEndian
	d.records = uint64(le.Uint16(buf[10:]))
	d.size = uint64(le.Uint32(buf[12:]))
	d.offset = uint64(le.Uint32(buf[16:]))
	if d.records == 0xffff || d.size == 0xffff || d.offset == 0xffffffff {
		p, err := findEnd64(r, endOffset)
		if err == nil && p >= 0 {
			endOffset = p
			b, err := readAt(r, p, lenEnd64)
			if err != nil {
				return d, 0, err
			}
			if le.Uint32(b) != sigEnd64 {
				return d, 0, errFormat
			}
			d.records = le.Uint64(b[32:])
			d.size = le.Uint64(b[40:])
			d.offset = le.Uint64(b[48:])
		}
		if err != nil {
			return d, 0, err
		}
	}
	const maxInt64 = 1<<63 - 1
	if d.size > maxInt64 || d.offset > maxInt64 {
		return d, 0, errFormat
	}
	return d, endOffset, nil
}

func findEndSignature(b []byte) int {
	for i := len(b) - lenEnd; i >= 0; i-- {
		if b[i] == 'P' && b[i+1] == 'K' && b[i+2] == 0x05 && b[i+3] == 0x06 {
			n := int(b[i+lenEnd-2]) | int(b[i+lenEnd-1])<<8
			if n+lenEnd+i > len(b) {
				return -1
			}
			return i
		}
	}
	return -1
}

func findEnd64(r io.ReaderAt, endOffset int64) (int64, error) {
	loc := endOffset - lenEnd64Loc
	if loc < 0 {
		return -1, nil
	}
	b, err := readAt(r, loc, lenEnd64Loc)
	if err != nil {
		return -1, err
	}
	le := binary.LittleEndian
	if le.Uint32(b) != sigEnd64Loc || le.Uint32(b[4:]) != 0 || le.Uint32(b[16:]) != 1 {
		return -1, nil
	}
	p := le.Uint64(b[8:])
	if p > 1<<62 {
		return -1, errFormat
	}
	return int64(p), nil
}

var errFormat = errors.New("not a valid ZIP archive")

// record is one central directory record, as far as the pre-scan needs it.
type record struct {
	next     int64 // offset of the following record
	name     []byte
	method   uint16
	flags    uint16
	csize    uint64
	localOff int64
}

// readRecord reads the central directory record at off; ok is false where
// archive/zip would stop reading records.
func readRecord(r io.ReaderAt, size, off int64) (rec record, ok bool, err error) {
	if off < 0 || off > size-lenCentral {
		return rec, false, nil
	}
	h, err := readAt(r, off, lenCentral)
	if err != nil {
		return rec, false, err
	}
	le := binary.LittleEndian
	if le.Uint32(h) != sigCentral {
		return rec, false, nil
	}
	n, m, k := int64(le.Uint16(h[28:])), int64(le.Uint16(h[30:])), int64(le.Uint16(h[32:]))
	if off+lenCentral+n+m+k > size {
		return rec, false, nil
	}
	rec.next = off + lenCentral + n + m + k
	if n > MaxNameLen {
		return rec, true, newErr(CodeBadName, "")
	}
	v, err := readAt(r, off+lenCentral, int(n+m))
	if err != nil {
		return rec, false, err
	}
	rec.name = v[:n]
	rec.flags = le.Uint16(h[8:])
	rec.method = le.Uint16(h[10:])
	rec.csize = uint64(le.Uint32(h[20:]))
	usize := uint64(le.Uint32(h[24:]))
	localOff := uint64(le.Uint32(h[42:]))
	// zip64 extra: the fields that are 0xffffffff come from it, in order.
	for extra := v[n:]; len(extra) >= 4; {
		id, sz := le.Uint16(extra), int(le.Uint16(extra[2:]))
		extra = extra[4:]
		if sz > len(extra) {
			break
		}
		field := extra[:sz]
		extra = extra[sz:]
		if id != zip64ExtraID {
			continue
		}
		next := func(v *uint64) bool {
			if *v != 0xffffffff {
				return true
			}
			if len(field) < 8 {
				return false
			}
			*v = le.Uint64(field)
			field = field[8:]
			return true
		}
		if !next(&usize) || !next(&rec.csize) || !next(&localOff) {
			return rec, true, errFormat
		}
	}
	if localOff > 1<<62 {
		return rec, true, errFormat
	}
	rec.localOff = int64(localOff)
	return rec, true, nil
}

// scan pre-scans the central directory. See the comment at the top.
func scan(r io.ReaderAt, size int64) (*scanResult, error) {
	d, endOffset, err := findEnd(r, size)
	if err != nil {
		return nil, formatErr(err)
	}
	base := endOffset - toI64(d.size) - toI64(d.offset)
	if o := base + toI64(d.offset); o < 0 || o >= size {
		return nil, formatErr(errFormat)
	}
	if base > 0 {
		if _, ok, err := readRecord(r, size, toI64(d.offset)); err == nil && ok {
			base = 0
		}
	}
	start := base + toI64(d.offset)

	type span struct{ from, to int64 }
	var spans []span
	res := &scanResult{}
	for off := start; ; {
		rec, ok, err := readRecord(r, size, off)
		if err != nil {
			return nil, asArchiveErr(err)
		}
		if !ok {
			break
		}
		if len(res.hashes) >= maxEntries {
			return nil, limitErr(CodeTooMany, "", int64(maxEntries))
		}
		if rec.next-start > MaxDirectorySize {
			return nil, limitErr(CodeDirectorySize, "", MaxDirectorySize)
		}
		h := fnv.New64a()
		_, _ = h.Write(rec.name)
		res.hashes = append(res.hashes, h.Sum64())

		from, to, err := checkLocal(r, size, base, rec)
		if err != nil {
			return nil, err
		}
		spans = append(spans, span{from, to})
		off = rec.next
	}
	// No two entries may share bytes, and no entry may reach into the
	// central directory.
	sort.Slice(spans, func(i, j int) bool { return spans[i].from < spans[j].from })
	for i, s := range spans {
		if s.to > start || (i > 0 && s.from < spans[i-1].to) {
			return nil, newErr(CodeFormat, "")
		}
	}
	return res, nil
}

// checkLocal checks the local header of rec against the central directory
// and returns the byte range the entry occupies (header and data).
func checkLocal(r io.ReaderAt, size, base int64, rec record) (from, to int64, err error) {
	name := string(rec.name)
	off := base + rec.localOff
	if off < 0 || off > size-lenLocal {
		return 0, 0, newErr(CodeFormat, name)
	}
	h, err := readAt(r, off, lenLocal)
	if err != nil {
		return 0, 0, asArchiveErr(err)
	}
	le := binary.LittleEndian
	if le.Uint32(h) != sigLocal {
		return 0, 0, newErr(CodeFormat, name)
	}
	n, m := int64(le.Uint16(h[26:])), int64(le.Uint16(h[28:]))
	if off+lenLocal+n+m > size || n != int64(len(rec.name)) {
		return 0, 0, newErr(CodeFormat, name)
	}
	ln, err := readAt(r, off+lenLocal, int(n))
	if err != nil {
		return 0, 0, asArchiveErr(err)
	}
	const encrypted = 0x1 | 0x40
	if string(ln) != name || le.Uint16(h[8:]) != rec.method || le.Uint16(h[6:])&encrypted != rec.flags&encrypted {
		return 0, 0, newErr(CodeFormat, name)
	}
	data := off + lenLocal + n + m
	if rec.csize > toU64(size) || data+toI64(rec.csize) > size {
		return 0, 0, newErr(CodeFormat, name)
	}
	return off, data + toI64(rec.csize), nil
}

func formatErr(err error) *Error {
	e := newErr(CodeFormat, "")
	if !errors.Is(err, errFormat) {
		e.Err = err
	}
	return e
}

func asArchiveErr(err error) error {
	var ae *Error
	if errors.As(err, &ae) {
		return err
	}
	return formatErr(err)
}
