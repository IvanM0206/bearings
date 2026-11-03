import { deflateSync } from 'fflate';
import { describe, expect, it } from 'vitest';
import { readArchive, writeArchive } from '../archive';
import { crc32 } from '../crc32';
import { FormatError, NotebookErrors } from '../errors';
import { MAX_ARCHIVE_ENTRIES, MIB } from '../limits';
import { loadNotebook } from '../notebook';
import { checkEntryName, listZip, readZip, writeZip } from '../zip';
import { enc, filesToRaw, minimalNotebook, rawZip, sectionFile, topicFile, type RawEntry } from './helpers';

function fail(fn: () => unknown): FormatError {
  try {
    fn();
  } catch (e) {
    if (e instanceof FormatError) return e;
    if (e instanceof NotebookErrors && e.errors[0]) return e.errors[0];
    throw e;
  }
  throw new Error('expected the archive to be rejected');
}

const valid = (): RawEntry[] => filesToRaw(minimalNotebook());

describe('archive round trip', () => {
  it('exports and re-imports a notebook', () => {
    const nb = loadNotebook(minimalNotebook()).notebook;
    const zip = writeArchive(nb, new Date('2026-11-03T10:00:00Z'));
    const back = readArchive(zip);
    expect(back.notebook).toEqual(nb);
    expect(back.summary).toEqual({ name: 'My notebook', sections: 1, topics: 1, entries: 1, isTemplate: false });
  });
  it('reads archives made by other tools (directory entries, unknown files)', () => {
    const zip = rawZip([
      { name: 'sections/' },
      { name: 'sections/technical/' },
      { name: 'journal/2026/11/' },
      ...valid(),
      { name: 'README.md', data: enc('# hello') },
      { name: '.git/HEAD', data: enc('ref: refs/heads/main\n') },
    ]);
    const r = readArchive(zip);
    expect(r.notebook.sections).toHaveLength(1);
    expect(r.ignored).toBeGreaterThan(0);
  });
  it('accepts stored entries and MS-DOS made-by entries', () => {
    const zip = rawZip(valid().map((e) => ({ ...e, method: 0, madeBy: 20, external: 0 })));
    expect(readArchive(zip).notebook.topics).toHaveLength(1);
  });
  it('stores highly compressible files so its own exports stay under the ratio limit', () => {
    const files = [{ path: 'a.md', bytes: new Uint8Array(512 * 1024).fill(0x61) }];
    const zip = writeZip(files);
    const listed = listZip(zip);
    expect(listed[0]?.method).toBe(0);
    expect(readZip(zip, { wanted: () => true }).files[0]?.bytes).toEqual(files[0]?.bytes);
  });
  it('writes ZIP64 end records for 65 535+ entries and reads them back', () => {
    const files = Array.from({ length: 70_000 }, (_, i) => ({ path: `x/${String(i)}`, bytes: new Uint8Array() }));
    const zip = writeZip(files);
    expect(listZip(zip)).toHaveLength(70_000);
  }, 60_000);
});

describe('entry names (§8)', () => {
  it.each([
    ['/etc/passwd', 'absolute path'],
    ['../evil.md', 'parent segment'],
    ['sections/../../evil.md', 'inner parent segment'],
    ['sections/./x.md', 'dot segment'],
    ['sections\\technical\\_section.md', 'backslash'],
    ['C:/evil.md', 'drive letter'],
    ['c:evil.md', 'drive-relative'],
    ['sections//x.md', 'empty segment'],
    ['sections/x.md\u0001', 'control character'],
  ])('rejects %s (%s)', (name) => {
    expect(fail(() => readArchive(rawZip([...valid(), { name, data: enc('x') }]))).code).toBe('archive-bad-name');
  });
  it('rejects NUL in names', () => {
    const name = new Uint8Array([...enc('sections/a.md'), 0, ...enc('.txt')]);
    expect(fail(() => readArchive(rawZip([...valid(), { name, data: enc('x') }]))).code).toBe('archive-bad-name');
  });
  it('rejects names that are not valid UTF-8', () => {
    const name = new Uint8Array([...enc('sections/'), 0xff, 0xfe]);
    expect(fail(() => readArchive(rawZip([...valid(), { name, data: enc('x') }]))).code).toBe('archive-bad-name');
  });
  it('allows a trailing slash only on directories', () => {
    expect(checkEntryName('sections/', true)).toBe('sections');
    expect(() => checkEntryName('/', true)).toThrow();
  });
  it('rejects a local header whose name differs from the central directory', () => {
    const entries = valid();
    entries[1] = { ...entries[1]!, localName: '../../evil.md' };
    expect(fail(() => readArchive(rawZip(entries))).code).toBe('archive-malformed');
  });
});

describe('duplicates (§8)', () => {
  it('rejects exact duplicates', () => {
    const v = valid();
    expect(fail(() => readArchive(rawZip([...v, v[1]!]))).code).toBe('archive-duplicate-name');
  });
  it('rejects names equal after case folding', () => {
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'BEARINGS.yaml', data: enc('x') }]))).code).toBe(
      'archive-duplicate-name',
    );
  });
  it('rejects names equal after Unicode normalization', () => {
    const zip = rawZip([...valid(), { name: 'notes/caf\u00e9.md', data: enc('x') }, { name: 'notes/cafe\u0301.md', data: enc('y') }]);
    expect(fail(() => readArchive(zip)).code).toBe('archive-duplicate-name');
  });
  it('rejects a directory entry and a file with the same name', () => {
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'notes/' }, { name: 'notes', data: enc('x') }]))).code).toBe(
      'archive-duplicate-name',
    );
  });
});

describe('special entries (§8)', () => {
  it('rejects symlinks', () => {
    const link: RawEntry = { name: 'sections/technical/link.md', data: enc('/etc/passwd'), method: 0, external: ((0o120777 << 16) >>> 0) };
    expect(fail(() => readArchive(rawZip([...valid(), link]))).code).toBe('archive-special-entry');
  });
  it('rejects symlinks with an ignored path too', () => {
    const link: RawEntry = { name: 'README.md', data: enc('/etc/passwd'), method: 0, external: ((0o120777 << 16) >>> 0) };
    expect(fail(() => readArchive(rawZip([...valid(), link]))).code).toBe('archive-special-entry');
  });
  it.each([
    ['character device', 0o020644],
    ['block device', 0o060644],
    ['fifo', 0o010644],
    ['socket', 0o140644],
  ])('rejects a %s', (_label, mode) => {
    const e: RawEntry = { name: 'dev', data: new Uint8Array(), method: 0, external: ((mode << 16) >>> 0) };
    expect(fail(() => readArchive(rawZip([...valid(), e]))).code).toBe('archive-special-entry');
  });
  it('rejects directory entries without a trailing slash and directories with data', () => {
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'notes', external: ((0o040755 << 16) >>> 0), method: 0 }]))).code).toBe(
      'archive-special-entry',
    );
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'notes/', data: enc('x'), method: 0 }]))).code).toBe(
      'archive-special-entry',
    );
  });
  it('rejects encrypted entries', () => {
    const v = valid();
    v[0] = { ...v[0]!, flags: 1 };
    expect(fail(() => readArchive(rawZip(v))).code).toBe('archive-encrypted');
  });
  it('rejects unsupported compression methods', () => {
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'x.bin', data: enc('x'), method: 12 }]))).code).toBe(
      'archive-unsupported',
    );
  });
});

describe('size limits and zip bombs (§8)', () => {
  it('rejects an entry whose declared size exceeds the 1 MiB file limit', () => {
    const data = new Uint8Array(MIB + 1);
    for (let i = 0; i < data.length; i++) data[i] = (i * 7919) & 0xff;
    const e = fail(() => readArchive(rawZip([...valid(), { name: 'sections/technical/big.md', data }])));
    expect(e.code).toBe('limit-exceeded');
    expect(e.details.limit).toBe('archive-entry-size');
  });
  it('rejects a ratio above 100:1 from the declared sizes', () => {
    const data = new Uint8Array(200_000); // zeros compress ~1000:1
    const e = fail(() => readArchive(rawZip([...valid(), { name: 'sections/technical/bomb.md', data }])));
    expect(e.code).toBe('archive-ratio');
  });
  it('rejects a ratio above 100:1 even for ignored files', () => {
    const data = new Uint8Array(200_000);
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'README.md', data }]))).code).toBe('archive-ratio');
  });
  it('counts bytes while inflating: a lying size field cannot inflate more', () => {
    // Declares 1 000 bytes; the stream really inflates to 50 000.
    const real = new Uint8Array(50_000);
    for (let i = 0; i < real.length; i++) real[i] = i % 251;
    const payload = deflateSync(real);
    const e = fail(() =>
      readArchive(rawZip([...valid(), { name: 'sections/technical/liar.md', data: real, payload, declaredSize: 1000 }])),
    );
    expect(e.code).toBe('archive-size-mismatch');
  });
  it('rejects a stream shorter than its declared size', () => {
    const real = enc(topicFile());
    const e = fail(() =>
      readArchive(rawZip([...valid(), { name: 'sections/technical/short.md', data: real, declaredSize: real.length + 10 }])),
    );
    expect(e.code).toBe('archive-size-mismatch');
  });
  it('rejects a truncated deflate stream', () => {
    const real = new Uint8Array(5000).map((_, i) => (i * 31) & 0xff);
    const full = deflateSync(real);
    const payload = full.subarray(0, full.length - 20);
    const e = fail(() =>
      readArchive(rawZip([...valid(), { name: 'sections/technical/cut.md', data: real, payload, declaredSize: real.length }])),
    );
    expect(['archive-malformed', 'archive-size-mismatch', 'archive-ratio']).toContain(e.code);
  });
  it('rejects a bad CRC', () => {
    const v = valid();
    v[1] = { ...v[1]!, crc: (crc32(v[1]!.data!) ^ 1) >>> 0 };
    expect(fail(() => readArchive(rawZip(v))).code).toBe('archive-crc');
  });
  it('rejects a stored entry whose sizes disagree', () => {
    const data = enc(sectionFile());
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'x.md', data, method: 0, declaredSize: data.length - 1 }]))).code).toBe(
      'archive-size-mismatch',
    );
  });
  it('rejects more than 200 MiB in total (declared), before inflating anything', () => {
    // 201 entries of 1 MiB each, stored. Built lazily to keep memory sane:
    // declare sizes without materializing the payload by pointing many entries at
    // one payload is an overlap, so build real entries of 1 MiB.
    const chunk = new Uint8Array(MIB);
    for (let i = 0; i < chunk.length; i++) chunk[i] = (i * 2654435761) >>> 24;
    const entries: RawEntry[] = [...valid()];
    for (let i = 0; i < 201; i++) entries.push({ name: `blob/${String(i)}`, data: chunk, method: 0 });
    const e = fail(() => readArchive(rawZip(entries)));
    expect(e.code).toBe('limit-exceeded');
    expect(e.details.limit).toBe('archive-total-size');
  }, 60_000);
  it(`rejects more than ${String(MAX_ARCHIVE_ENTRIES)} entries`, () => {
    const files = Array.from({ length: MAX_ARCHIVE_ENTRIES + 1 }, (_, i) => ({ path: `x/${String(i)}`, bytes: new Uint8Array() }));
    const e = fail(() => readArchive(writeZip(files)));
    expect(e.code).toBe('limit-exceeded');
    expect(e.details.limit).toBe('archive-entries');
  }, 60_000);
  it('rejects overlapping entries (one payload referenced many times)', () => {
    const v = valid();
    const zip = rawZip([...v, { name: 'copy.md', data: v[1]!.data!, reuseOffsetOf: 1 }]);
    expect(['archive-malformed']).toContain(fail(() => readArchive(zip)).code);
  });
  it('rejects an entry whose data runs into the next entry', () => {
    const v = valid().map((e) => ({ ...e, method: 0, flags: 8 }));
    const zip = rawZip(v);
    const view = new DataView(zip.buffer);
    const cd = view.getUint32(zip.length - 22 + 16, true);
    // Grow entry 0's declared sizes so its data covers entry 1's local header.
    const size = view.getUint32(cd + 24, true) + 40;
    view.setUint32(cd + 20, size, true);
    view.setUint32(cd + 24, size, true);
    expect(fail(() => readArchive(zip)).code).toBe('archive-malformed');
  });
  it('rejects entries deeper than 4 directories', () => {
    const e = fail(() => readArchive(rawZip([...valid(), { name: 'a/b/c/d/e/f.txt', data: enc('x') }])));
    expect(e.details.limit).toBe('depth');
    expect(readArchive(rawZip([...valid(), { name: 'a/b/c/d/f.txt', data: enc('x') }])).notebook).toBeDefined();
  });
  it('rejects archives that are too large to read', () => {
    const big = { length: 300 * MIB } as unknown as Uint8Array;
    expect(fail(() => readArchive(big)).code).toBe('archive-too-large');
  });
});

describe('malformed archives', () => {
  it('rejects garbage, truncation and lying entry counts', () => {
    expect(fail(() => readArchive(enc('not a zip at all, sorry'))).code).toBe('archive-malformed');
    expect(fail(() => readArchive(new Uint8Array()))).toBeInstanceOf(FormatError);
    const zip = rawZip(valid());
    expect(fail(() => readArchive(zip.subarray(0, zip.length - 30))).code).toBe('archive-malformed');
    expect(fail(() => readArchive(rawZip(valid(), { countOverride: 2 }))).code).toBe('archive-malformed');
    expect(fail(() => readArchive(rawZip(valid(), { countOverride: 9 }))).code).toBe('archive-malformed');
  });
  it('rejects data after the end record', () => {
    expect(fail(() => readArchive(rawZip(valid(), { trailing: enc('junk') }))).code).toBe('archive-malformed');
  });
  it('validates notebook content before returning anything', () => {
    const v = valid();
    v[1] = { name: v[1]!.name, data: enc(sectionFile({ title: '"x"', extra: 'nope' })) };
    expect(fail(() => readArchive(rawZip(v))).code).toBe('unknown-key');
  });
  it('rejects reserved slugs in archive paths', () => {
    expect(fail(() => readArchive(rawZip([...valid(), { name: 'sections/aux/_section.md', data: enc(sectionFile()) }]))).code).toBe(
      'reserved-slug',
    );
  });
});
