/** Test helpers: build notebook files and hand-crafted (possibly hostile) ZIPs. */
import { deflateSync } from 'fflate';
import { crc32 } from '../crc32';
import type { FileInput } from '../notebook';

export const enc = (s: string): Uint8Array => new TextEncoder().encode(s);

export const TS = '2026-11-03T09:41:00Z';

export function sectionFile(fields: Record<string, string> = {}, body = ''): string {
  const f = { title: '"Technical"', order: '1', created: TS, updated: TS, ...fields };
  return `---\n${yamlLines(f)}---\n${body}`;
}

export function topicFile(fields: Record<string, string> = {}, body = ''): string {
  const f = { title: '"Backtesting"', status: 'in-progress', type: 'role', order: '4', created: TS, updated: TS, ...fields };
  return `---\n${yamlLines(f)}---\n${body}`;
}

export function entryFile(fields: Record<string, string> = {}, body = 'Learned something.\n'): string {
  const f = { date: '2026-11-03', created: TS, updated: TS, ...fields };
  return `---\n${yamlLines(f)}---\n${body}`;
}

function yamlLines(f: Record<string, string>): string {
  return Object.entries(f)
    .filter(([, v]) => v !== '')
    .map(([k, v]) => (v.startsWith('\n') ? `${k}:${v}\n` : `${k}: ${v}\n`))
    .join('');
}

export const ENTRY_ID = '7k2mq9xr4t';

export function minimalNotebook(): FileInput[] {
  return [
    { path: 'bearings.yaml', bytes: enc('format: 1\nname: "My notebook"\nlanguage: en\n') },
    { path: 'sections/technical/_section.md', bytes: enc(sectionFile({}, 'Stack and tools.\n')) },
    { path: 'sections/technical/backtesting.md', bytes: enc(topicFile({}, 'Notes here.\n')) },
    {
      path: `journal/2026/11/2026-11-03-${ENTRY_ID}.md`,
      bytes: enc(entryFile({ section: 'technical', topic: 'technical/backtesting' })),
    },
  ];
}

// ---------------------------------------------------------------------------
// Raw ZIP builder

export interface RawEntry {
  name: string | Uint8Array;
  data?: Uint8Array;
  /** 0 = stored, 8 = deflate (default 8 for files, 0 for names ending in "/"). */
  method?: number;
  /** Overrides the declared uncompressed size (central and local). */
  declaredSize?: number;
  /** Overrides the compressed payload. */
  payload?: Uint8Array;
  crc?: number;
  flags?: number;
  madeBy?: number;
  external?: number;
  /** Name written in the local header, if different. */
  localName?: string;
  /** Point this entry's local header at another entry's (overlap). */
  reuseOffsetOf?: number;
}

export function rawZip(entries: RawEntry[], opts: { countOverride?: number; trailing?: Uint8Array } = {}): Uint8Array {
  const chunks: Uint8Array[] = [];
  const central: Uint8Array[] = [];
  const offsets: number[] = [];
  let offset = 0;
  entries.forEach((e, i) => {
    const name = typeof e.name === 'string' ? enc(e.name) : e.name;
    const localName = e.localName === undefined ? name : enc(e.localName);
    const data = e.data ?? new Uint8Array();
    const isDir = typeof e.name === 'string' && e.name.endsWith('/');
    const method = e.method ?? (isDir ? 0 : 8);
    const payload = e.payload ?? (method === 8 ? deflateSync(data) : data);
    const crc = e.crc ?? crc32(data);
    const size = e.declaredSize ?? data.length;
    const flags = e.flags ?? 0;
    const myOffset = e.reuseOffsetOf !== undefined ? (offsets[e.reuseOffsetOf] ?? 0) : offset;
    offsets.push(myOffset);

    if (e.reuseOffsetOf === undefined) {
      const local = new Uint8Array(30 + localName.length);
      const lv = new DataView(local.buffer);
      lv.setUint32(0, 0x04034b50, true);
      lv.setUint16(4, 20, true);
      lv.setUint16(6, flags, true);
      lv.setUint16(8, method, true);
      lv.setUint32(14, crc, true);
      lv.setUint32(18, payload.length, true);
      lv.setUint32(22, size, true);
      lv.setUint16(26, localName.length, true);
      local.set(localName, 30);
      chunks.push(local, payload);
      offset += local.length + payload.length;
    }

    const cen = new Uint8Array(46 + name.length);
    const cv = new DataView(cen.buffer);
    cv.setUint32(0, 0x02014b50, true);
    cv.setUint16(4, e.madeBy ?? (3 << 8) | 20, true);
    cv.setUint16(6, 20, true);
    cv.setUint16(8, flags, true);
    cv.setUint16(10, method, true);
    cv.setUint32(16, crc, true);
    cv.setUint32(20, payload.length, true);
    cv.setUint32(24, size, true);
    cv.setUint16(28, name.length, true);
    cv.setUint32(38, e.external ?? (((isDir ? 0o040755 : 0o100644) << 16) >>> 0), true);
    cv.setUint32(42, myOffset, true);
    cen.set(name, 46);
    central.push(cen);
    void i;
  });
  const cdOffset = offset;
  const cdSize = central.reduce((n, c) => n + c.length, 0);
  const count = opts.countOverride ?? entries.length;
  const eocd = new Uint8Array(22);
  const ev = new DataView(eocd.buffer);
  ev.setUint32(0, 0x06054b50, true);
  ev.setUint16(8, count, true);
  ev.setUint16(10, count, true);
  ev.setUint32(12, cdSize, true);
  ev.setUint32(16, cdOffset, true);
  const parts = [...chunks, ...central, eocd, ...(opts.trailing ? [opts.trailing] : [])];
  const out = new Uint8Array(parts.reduce((n, c) => n + c.length, 0));
  let p = 0;
  for (const c of parts) {
    out.set(c, p);
    p += c.length;
  }
  return out;
}

export function filesToRaw(files: FileInput[]): RawEntry[] {
  return files.map((f) => ({ name: f.path, data: f.bytes }));
}
