/**
 * Minimal, defensive ZIP reader and writer for notebook archives (§8).
 *
 * The reader trusts nothing: it parses the central directory itself, checks
 * every rule of §8 for every entry before inflating anything, cross-checks
 * each local header against the central directory, refuses overlapping
 * entries, and while inflating counts bytes so that a lying size field cannot
 * make it produce more than the declared size. Inflation uses fflate's
 * streaming `Inflate` fed in small chunks.
 */
import { deflateSync, Inflate } from 'fflate';
import { crc32, crc32Update } from './crc32';
import { FormatError } from './errors';
import { MAX_ARCHIVE_ENTRIES, MAX_ARCHIVE_TOTAL_BYTES, MAX_COMPRESSION_RATIO, MAX_FILE_BYTES } from './limits';
import type { FileInput } from './notebook';

const SIG_LOCAL = 0x04034b50;
const SIG_CENTRAL = 0x02014b50;
const SIG_EOCD = 0x06054b50;
const SIG_ZIP64_EOCD = 0x06064b50;
const SIG_ZIP64_LOCATOR = 0x07064b50;

const EOCD_SIZE = 22;
const ZIP64_LOCATOR_SIZE = 20;
const ZIP64_EOCD_SIZE = 56;
const CENTRAL_SIZE = 46;
const LOCAL_SIZE = 30;
const MAX_COMMENT = 0xffff;

const FLAG_ENCRYPTED = 0x0001;
const FLAG_DATA_DESCRIPTOR = 0x0008;
const FLAG_STRONG_ENCRYPTION = 0x0040;
const FLAG_UTF8 = 0x0800;
const FLAG_MASKED_HEADERS = 0x2000;

const METHOD_STORED = 0;
const METHOD_DEFLATE = 8;

const HOST_UNIX = 3;
const HOST_DARWIN = 19;
const S_IFMT = 0o170000;
const S_IFREG = 0o100000;
const S_IFDIR = 0o040000;
const DOS_DIRECTORY = 0x10;

const INFLATE_CHUNK = 4096;

export interface ZipEntry {
  name: string;
  isDirectory: boolean;
  method: number;
  flags: number;
  crc: number;
  compressedSize: number;
  size: number;
  localOffset: number;
  /** Set once the local header has been checked. */
  dataOffset: number;
}

export interface ReadZipResult {
  files: FileInput[];
  /** Number of entries in the archive (files and directories). */
  entryCount: number;
}

export interface ReadZipOptions {
  /** Which file entries to inflate and return; others are only validated. */
  wanted: (name: string) => boolean;
  /** Per-entry size limit; defaults to the §7 file limit for every entry. */
  maxEntrySize?: number;
}

const utf8 = new TextDecoder('utf-8', { fatal: true });

function bad(code: 'archive-malformed' | 'archive-unsupported', file?: string): FormatError {
  return new FormatError(code, file === undefined ? {} : { file });
}

class Reader {
  readonly view: DataView;
  constructor(readonly data: Uint8Array) {
    this.view = new DataView(data.buffer, data.byteOffset, data.byteLength);
  }
  u16(off: number): number {
    this.need(off, 2);
    return this.view.getUint16(off, true);
  }
  u32(off: number): number {
    this.need(off, 4);
    return this.view.getUint32(off, true);
  }
  u64(off: number): number {
    this.need(off, 8);
    const v = this.view.getBigUint64(off, true);
    if (v > BigInt(Number.MAX_SAFE_INTEGER)) throw bad('archive-malformed');
    return Number(v);
  }
  bytes(off: number, len: number): Uint8Array {
    this.need(off, len);
    return this.data.subarray(off, off + len);
  }
  need(off: number, len: number): void {
    if (off < 0 || len < 0 || off + len > this.data.length) throw bad('archive-malformed');
  }
}

/** Validates a name from the archive (§8) and returns it without a trailing `/`. */
export function checkEntryName(name: string, isDirectory: boolean): string {
  const fail = (): FormatError => new FormatError('archive-bad-name', { file: name });
  if (name.length === 0) throw fail();
  // eslint-disable-next-line no-control-regex
  if (/[\u0000-\u001f\u007f]/.test(name)) throw fail();
  if (name.includes('\\')) throw fail();
  if (name.startsWith('/')) throw fail();
  const path = isDirectory ? name.slice(0, -1) : name;
  if (path.length === 0) throw fail();
  for (const seg of path.split('/')) {
    if (seg === '' || seg === '.' || seg === '..') throw fail();
    if (/^[A-Za-z]:/.test(seg)) throw fail();
  }
  return path;
}

/** Key used to detect duplicates: Unicode NFC, case-insensitive. */
function normalizedKey(path: string): string {
  return path.normalize('NFC').toLowerCase();
}

function findEocd(r: Reader): number {
  const len = r.data.length;
  if (len < EOCD_SIZE) throw bad('archive-malformed');
  const min = Math.max(0, len - EOCD_SIZE - MAX_COMMENT);
  for (let off = len - EOCD_SIZE; off >= min; off--) {
    if (r.u32(off) === SIG_EOCD && off + EOCD_SIZE + r.u16(off + 20) === len) return off;
  }
  throw bad('archive-malformed');
}

interface Directory {
  count: number;
  cdOffset: number;
  cdSize: number;
  /** Where the central directory must end (start of the end records). */
  cdLimit: number;
}

function readDirectory(r: Reader): Directory {
  const eocd = findEocd(r);
  const disk = r.u16(eocd + 4);
  const cdDisk = r.u16(eocd + 6);
  let countDisk = r.u16(eocd + 8);
  let count = r.u16(eocd + 10);
  let cdSize = r.u32(eocd + 12);
  let cdOffset = r.u32(eocd + 16);
  let cdLimit = eocd;
  const needs64 = count === 0xffff || countDisk === 0xffff || cdSize === 0xffffffff || cdOffset === 0xffffffff;
  const locator = eocd - ZIP64_LOCATOR_SIZE;
  if (locator >= 0 && r.u32(locator) === SIG_ZIP64_LOCATOR) {
    if (r.u32(locator + 4) !== 0 || r.u32(locator + 16) !== 1) throw bad('archive-unsupported');
    const rec = r.u64(locator + 8);
    if (rec + ZIP64_EOCD_SIZE > locator || r.u32(rec) !== SIG_ZIP64_EOCD) throw bad('archive-malformed');
    if (r.u32(rec + 16) !== 0 || r.u32(rec + 20) !== 0) throw bad('archive-unsupported');
    countDisk = r.u64(rec + 24);
    count = r.u64(rec + 32);
    cdSize = r.u64(rec + 40);
    cdOffset = r.u64(rec + 48);
    cdLimit = rec;
  } else if (needs64) {
    throw bad('archive-malformed');
  } else if (disk !== 0 || cdDisk !== 0) {
    throw bad('archive-unsupported');
  }
  if (countDisk !== count) throw bad('archive-unsupported');
  if (count > MAX_ARCHIVE_ENTRIES) {
    throw new FormatError('limit-exceeded', { limit: 'archive-entries', max: MAX_ARCHIVE_ENTRIES });
  }
  if (cdOffset + cdSize > cdLimit) throw bad('archive-malformed');
  return { count, cdOffset, cdSize, cdLimit };
}

interface Zip64Fields {
  size: number;
  compressedSize: number;
  localOffset: number;
}

function applyZip64(r: Reader, extraStart: number, extraLen: number, f: Zip64Fields, disk: number, name: string): void {
  const wantSize = f.size === 0xffffffff;
  const wantComp = f.compressedSize === 0xffffffff;
  const wantOff = f.localOffset === 0xffffffff;
  const wantDisk = disk === 0xffff;
  let off = extraStart;
  const end = extraStart + extraLen;
  let found = false;
  while (off + 4 <= end) {
    const id = r.u16(off);
    const len = r.u16(off + 2);
    const body = off + 4;
    if (body + len > end) throw bad('archive-malformed', name);
    if (id === 0x0001) {
      if (found) throw bad('archive-malformed', name);
      found = true;
      let p = body;
      const take = (): number => {
        if (p + 8 > body + len) throw bad('archive-malformed', name);
        const v = r.u64(p);
        p += 8;
        return v;
      };
      if (wantSize) f.size = take();
      if (wantComp) f.compressedSize = take();
      if (wantOff) f.localOffset = take();
      if (wantDisk && (p + 4 > body + len || r.u32(p) !== 0)) throw bad('archive-unsupported', name);
    }
    off = body + len;
  }
  if (off !== end) throw bad('archive-malformed', name);
  if ((wantSize || wantComp || wantOff || wantDisk) && !found) throw bad('archive-malformed', name);
}

function decodeName(raw: Uint8Array, flags: number): string {
  // Names are decoded as UTF-8 whether or not the UTF-8 flag is set: every
  // valid notebook path is ASCII, and refusing undecodable names (rather than
  // guessing CP437) keeps every reader's view of a name identical.
  void flags;
  try {
    return utf8.decode(raw);
  } catch {
    throw new FormatError('archive-bad-name');
  }
}

/** Parses and validates the central directory and every local header. */
export function listZip(data: Uint8Array, maxEntrySize = MAX_FILE_BYTES): ZipEntry[] {
  const r = new Reader(data);
  const dir = readDirectory(r);
  const entries: ZipEntry[] = [];
  const names = new Set<string>();
  let total = 0;
  let off = dir.cdOffset;

  for (let i = 0; i < dir.count; i++) {
    if (off + CENTRAL_SIZE > dir.cdOffset + dir.cdSize || r.u32(off) !== SIG_CENTRAL) {
      throw bad('archive-malformed');
    }
    const madeBy = r.u16(off + 4);
    const flags = r.u16(off + 8);
    const method = r.u16(off + 10);
    const crc = r.u32(off + 16);
    const f: Zip64Fields = { compressedSize: r.u32(off + 20), size: r.u32(off + 24), localOffset: r.u32(off + 42) };
    const nameLen = r.u16(off + 28);
    const extraLen = r.u16(off + 30);
    const commentLen = r.u16(off + 32);
    const disk = r.u16(off + 34);
    const external = r.u32(off + 38);
    const nameStart = off + CENTRAL_SIZE;
    const next = nameStart + nameLen + extraLen + commentLen;
    if (next > dir.cdOffset + dir.cdSize) throw bad('archive-malformed');

    const rawName = r.bytes(nameStart, nameLen);
    if (rawName.includes(0)) throw new FormatError('archive-bad-name');
    const name = decodeName(rawName, flags);
    applyZip64(r, nameStart + nameLen, extraLen, f, disk, name);
    if (disk !== 0 && disk !== 0xffff) throw bad('archive-unsupported', name);

    if (flags & (FLAG_ENCRYPTED | FLAG_STRONG_ENCRYPTION | FLAG_MASKED_HEADERS)) {
      throw new FormatError('archive-encrypted', { file: name });
    }

    // Entry type: only regular files and directories are allowed.
    const host = madeBy >>> 8;
    const endsWithSlash = name.endsWith('/');
    let isDirectory = endsWithSlash;
    if (host === HOST_UNIX || host === HOST_DARWIN) {
      const type = (external >>> 16) & S_IFMT;
      if (type === S_IFDIR) isDirectory = true;
      else if (type !== 0 && type !== S_IFREG) throw new FormatError('archive-special-entry', { file: name });
    }
    if (external & DOS_DIRECTORY) isDirectory = true;
    if (isDirectory !== endsWithSlash) throw new FormatError('archive-special-entry', { file: name });

    const path = checkEntryName(name, isDirectory);
    const key = normalizedKey(path);
    if (names.has(key)) throw new FormatError('archive-duplicate-name', { file: name });
    names.add(key);

    if (isDirectory) {
      if (f.size !== 0) throw new FormatError('archive-special-entry', { file: name });
    } else {
      if (method !== METHOD_STORED && method !== METHOD_DEFLATE) {
        throw new FormatError('archive-unsupported', { file: name });
      }
      if (f.size > maxEntrySize) {
        throw new FormatError('limit-exceeded', { file: name, limit: 'archive-entry-size', max: maxEntrySize });
      }
      if (method === METHOD_STORED && f.compressedSize !== f.size) {
        throw new FormatError('archive-size-mismatch', { file: name });
      }
      if (f.size > f.compressedSize * MAX_COMPRESSION_RATIO) {
        throw new FormatError('archive-ratio', { file: name, max: MAX_COMPRESSION_RATIO });
      }
      total += f.size;
      if (total > MAX_ARCHIVE_TOTAL_BYTES) {
        throw new FormatError('limit-exceeded', { limit: 'archive-total-size', max: MAX_ARCHIVE_TOTAL_BYTES });
      }
    }

    entries.push({
      name: path,
      isDirectory,
      method,
      flags,
      crc,
      compressedSize: f.compressedSize,
      size: f.size,
      localOffset: f.localOffset,
      dataOffset: -1,
    });
    off = next;
  }
  if (off !== dir.cdOffset + dir.cdSize) throw bad('archive-malformed');

  for (const e of entries) checkLocalHeader(r, e, dir.cdOffset);

  // No two entries may share bytes (overlapping entries are a zip-bomb trick).
  const byOffset = [...entries].sort((a, b) => a.localOffset - b.localOffset);
  let end = 0;
  for (const e of byOffset) {
    if (e.localOffset < end) throw bad('archive-malformed', e.name);
    end = e.dataOffset + e.compressedSize;
  }
  return entries;
}

function checkLocalHeader(r: Reader, e: ZipEntry, cdOffset: number): void {
  const off = e.localOffset;
  if (off + LOCAL_SIZE > cdOffset || r.u32(off) !== SIG_LOCAL) throw bad('archive-malformed', e.name);
  const flags = r.u16(off + 6);
  const method = r.u16(off + 8);
  const crc = r.u32(off + 14);
  const comp = r.u32(off + 18);
  const size = r.u32(off + 22);
  const nameLen = r.u16(off + 26);
  const extraLen = r.u16(off + 28);
  if (flags !== e.flags || method !== e.method) throw bad('archive-malformed', e.name);
  const raw = r.bytes(off + LOCAL_SIZE, nameLen);
  let localName: string;
  try {
    localName = utf8.decode(raw);
  } catch {
    throw new FormatError('archive-bad-name', { file: e.name });
  }
  const expected = e.isDirectory ? `${e.name}/` : e.name;
  if (localName !== expected) throw bad('archive-malformed', e.name);
  if (!(flags & FLAG_DATA_DESCRIPTOR)) {
    const comp64 = comp === 0xffffffff;
    const size64 = size === 0xffffffff;
    if (crc !== e.crc || (!comp64 && comp !== e.compressedSize) || (!size64 && size !== e.size)) {
      throw new FormatError('archive-size-mismatch', { file: e.name });
    }
  }
  e.dataOffset = off + LOCAL_SIZE + nameLen + extraLen;
  if (e.dataOffset + e.compressedSize > cdOffset) throw bad('archive-malformed', e.name);
}

/** Inflates one entry, never producing more than its declared size. */
export function extractEntry(data: Uint8Array, e: ZipEntry): Uint8Array {
  const src = data.subarray(e.dataOffset, e.dataOffset + e.compressedSize);
  let out: Uint8Array;
  if (e.method === METHOD_STORED) {
    out = src.slice();
  } else {
    out = new Uint8Array(e.size);
    let produced = 0;
    let finished = false;
    const inflater = new Inflate((chunk, final) => {
      if (produced + chunk.length > e.size) throw new FormatError('archive-size-mismatch', { file: e.name });
      out.set(chunk, produced);
      produced += chunk.length;
      if (produced > e.compressedSize * MAX_COMPRESSION_RATIO) {
        throw new FormatError('archive-ratio', { file: e.name, max: MAX_COMPRESSION_RATIO });
      }
      if (final) finished = true;
    });
    try {
      for (let p = 0; p < src.length; p += INFLATE_CHUNK) {
        const end = Math.min(p + INFLATE_CHUNK, src.length);
        inflater.push(src.subarray(p, end), end === src.length);
      }
      if (src.length === 0) inflater.push(src, true);
    } catch (err) {
      if (err instanceof FormatError) throw err;
      throw bad('archive-malformed', e.name);
    }
    if (!finished || produced !== e.size) throw new FormatError('archive-size-mismatch', { file: e.name });
  }
  if (crc32(out) !== e.crc) throw new FormatError('archive-crc', { file: e.name });
  return out;
}

/**
 * Reads a whole archive. Every entry is validated before anything is
 * inflated; then the wanted files are inflated and checked one by one.
 */
export function readZip(data: Uint8Array, options: ReadZipOptions): ReadZipResult {
  const entries = listZip(data, options.maxEntrySize);
  const files: FileInput[] = [];
  for (const e of entries) {
    if (e.isDirectory || !options.wanted(e.name)) continue;
    files.push({ path: e.name, bytes: extractEntry(data, e) });
  }
  return { files, entryCount: entries.length };
}

// ---------------------------------------------------------------------------
// Writer

function dosDateTime(d: Date): { time: number; date: number } {
  const year = Math.min(Math.max(d.getUTCFullYear(), 1980), 2107);
  return {
    time: (d.getUTCHours() << 11) | (d.getUTCMinutes() << 5) | (d.getUTCSeconds() >> 1),
    date: ((year - 1980) << 9) | ((d.getUTCMonth() + 1) << 5) | d.getUTCDate(),
  };
}

/**
 * Writes a ZIP with regular-file entries (mode 0600). An entry whose deflated
 * form would exceed the 100:1 ratio limit, or would not be smaller, is stored
 * uncompressed so the archive always passes {@link readZip}. ZIP64 end records
 * are written when there are 65 535 entries or more.
 */
export function writeZip(files: readonly FileInput[], mtime: Date = new Date()): Uint8Array {
  const enc = new TextEncoder();
  const { time, date } = dosDateTime(mtime);
  const chunks: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  let offset = 0;

  for (const file of files) {
    const name = enc.encode(file.path);
    const crc = crc32Update(0, file.bytes);
    const deflated = deflateSync(file.bytes, { level: 6 });
    const useDeflate =
      deflated.length < file.bytes.length && file.bytes.length <= deflated.length * MAX_COMPRESSION_RATIO;
    const payload = useDeflate ? deflated : file.bytes;
    const method = useDeflate ? METHOD_DEFLATE : METHOD_STORED;
    const flags = /[^\x20-\x7e]/.test(file.path) ? FLAG_UTF8 : 0;

    const local = new Uint8Array(LOCAL_SIZE + name.length);
    const lv = new DataView(local.buffer);
    lv.setUint32(0, SIG_LOCAL, true);
    lv.setUint16(4, 20, true);
    lv.setUint16(6, flags, true);
    lv.setUint16(8, method, true);
    lv.setUint16(10, time, true);
    lv.setUint16(12, date, true);
    lv.setUint32(14, crc, true);
    lv.setUint32(18, payload.length, true);
    lv.setUint32(22, file.bytes.length, true);
    lv.setUint16(26, name.length, true);
    lv.setUint16(28, 0, true);
    local.set(name, LOCAL_SIZE);

    const cen = new Uint8Array(CENTRAL_SIZE + name.length);
    const cv = new DataView(cen.buffer);
    cv.setUint32(0, SIG_CENTRAL, true);
    cv.setUint16(4, (HOST_UNIX << 8) | 20, true);
    cv.setUint16(6, 20, true);
    cv.setUint16(8, flags, true);
    cv.setUint16(10, method, true);
    cv.setUint16(12, time, true);
    cv.setUint16(14, date, true);
    cv.setUint32(16, crc, true);
    cv.setUint32(20, payload.length, true);
    cv.setUint32(24, file.bytes.length, true);
    cv.setUint16(28, name.length, true);
    cv.setUint32(38, ((S_IFREG | 0o600) << 16) >>> 0, true);
    cv.setUint32(42, offset, true);
    cen.set(name, CENTRAL_SIZE);

    chunks.push(local, payload);
    central.push(cen);
    offset += local.length + payload.length;
  }

  const cdOffset = offset;
  const cdSize = central.reduce((n, c) => n + c.length, 0);
  for (const c of central) chunks.push(c);
  const count = files.length;
  const zip64 = count >= 0xffff;

  if (zip64) {
    const rec = new Uint8Array(ZIP64_EOCD_SIZE);
    const rv = new DataView(rec.buffer);
    rv.setUint32(0, SIG_ZIP64_EOCD, true);
    rv.setBigUint64(4, BigInt(ZIP64_EOCD_SIZE - 12), true);
    rv.setUint16(12, (HOST_UNIX << 8) | 45, true);
    rv.setUint16(14, 45, true);
    rv.setBigUint64(24, BigInt(count), true);
    rv.setBigUint64(32, BigInt(count), true);
    rv.setBigUint64(40, BigInt(cdSize), true);
    rv.setBigUint64(48, BigInt(cdOffset), true);
    const loc = new Uint8Array(ZIP64_LOCATOR_SIZE);
    const locv = new DataView(loc.buffer);
    locv.setUint32(0, SIG_ZIP64_LOCATOR, true);
    locv.setBigUint64(8, BigInt(cdOffset + cdSize), true);
    locv.setUint32(16, 1, true);
    chunks.push(rec, loc);
  }

  const eocd = new Uint8Array(EOCD_SIZE);
  const ev = new DataView(eocd.buffer);
  ev.setUint32(0, SIG_EOCD, true);
  ev.setUint16(8, zip64 ? 0xffff : count, true);
  ev.setUint16(10, zip64 ? 0xffff : count, true);
  ev.setUint32(12, cdSize, true);
  ev.setUint32(16, cdOffset, true);
  chunks.push(eocd);

  const total = chunks.reduce((n, c) => n + c.length, 0);
  const out = new Uint8Array(total);
  let p = 0;
  for (const c of chunks) {
    out.set(c, p);
    p += c.length;
  }
  return out;
}
