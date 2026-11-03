import { describe, expect, it } from 'vitest';
import { FormatError } from '../errors';
import {
  parseConfig,
  parseEntry,
  parseSection,
  parseTopic,
  serializeConfig,
  serializeEntry,
  serializeSection,
  serializeTopic,
  splitFrontmatter,
} from '../files';
import { KIB, MIB } from '../limits';
import { decodeFile } from '../text';
import { enc, ENTRY_ID, entryFile, sectionFile, topicFile, TS } from './helpers';

function err(fn: () => unknown): FormatError {
  try {
    fn();
  } catch (e) {
    if (e instanceof FormatError) return e;
    throw e;
  }
  throw new Error('expected a FormatError');
}

const section = (text: string): ReturnType<typeof parseSection> => parseSection(enc(text), 'technical');
const topic = (text: string): ReturnType<typeof parseTopic> => parseTopic(enc(text), 'technical', 'backtesting');
const entry = (text: string, date = '2026-11-03'): ReturnType<typeof parseEntry> => parseEntry(enc(text), ENTRY_ID, date);

describe('encoding (§4)', () => {
  it('rejects invalid UTF-8', () => {
    expect(err(() => decodeFile(new Uint8Array([0x2d, 0xff, 0xfe]))).code).toBe('invalid-utf8');
    // Overlong encoding of "/".
    expect(err(() => decodeFile(new Uint8Array([0xc0, 0xaf]))).code).toBe('invalid-utf8');
    // Encoded surrogate.
    expect(err(() => decodeFile(new Uint8Array([0xed, 0xa0, 0x80]))).code).toBe('invalid-utf8');
  });
  it('rejects a BOM', () => {
    expect(err(() => decodeFile(new Uint8Array([0xef, 0xbb, 0xbf, 0x61]))).code).toBe('bom');
  });
  it('rejects NUL bytes', () => {
    expect(err(() => decodeFile(enc('a\0b'))).code).toBe('nul-byte');
  });
  it('normalizes CRLF to LF', () => {
    expect(decodeFile(enc('a\r\nb\r\n'))).toBe('a\nb\n');
    const s = section(sectionFile().replace(/\n/g, '\r\n') + 'Line one\r\nLine two\r\n');
    expect(s.description).toBe('Line one\nLine two');
  });
  it('rejects files over 1 MiB and accepts exactly 1 MiB', () => {
    const e = err(() => decodeFile(new Uint8Array(MIB + 1).fill(0x61)));
    expect(e.code).toBe('file-too-large');
    expect(e.details.limit).toBe('file-size');
    expect(decodeFile(new Uint8Array(MIB).fill(0x61))).toHaveLength(MIB);
  });
});

describe('frontmatter', () => {
  it('requires the opening and closing delimiters', () => {
    expect(err(() => splitFrontmatter('title: x\n')).code).toBe('frontmatter-missing');
    expect(err(() => splitFrontmatter('---\ntitle: x\n')).code).toBe('frontmatter-missing');
    expect(err(() => splitFrontmatter(' ---\ntitle: x\n---\n')).code).toBe('frontmatter-missing');
  });
  it('splits body and strips trailing newlines', () => {
    expect(splitFrontmatter('---\na: 1\n---\nbody\n\n')).toEqual({ frontmatter: 'a: 1\n', body: 'body' });
    expect(splitFrontmatter('---\na: 1\n---')).toEqual({ frontmatter: 'a: 1\n', body: '' });
    expect(splitFrontmatter('---\n---\n')).toEqual({ frontmatter: '', body: '' });
  });
  it('keeps later --- lines in the body', () => {
    expect(splitFrontmatter('---\na: 1\n---\nx\n---\ny\n').body).toBe('x\n---\ny');
  });
  it('limits frontmatter to 16 KiB', () => {
    const big = `---\na: "${'x'.repeat(16 * KIB)}"\n---\n`;
    const e = err(() => splitFrontmatter(big));
    expect(e.code).toBe('frontmatter-too-large');
    expect(e.details.limit).toBe('frontmatter-size');
    const ok = `---\na: "${'x'.repeat(16 * KIB - 10)}"\n---\n`;
    expect(splitFrontmatter(ok).frontmatter.length).toBeLessThanOrEqual(16 * KIB);
  });
});

describe('bearings.yaml (§5.1)', () => {
  it('parses a full config', () => {
    const c = parseConfig(
      enc('format: 1\nname: "Mine"\nlanguage: es\ntemplate:\n  id: my-role\n  title: "Role"\n  description: "Desc"\n'),
    );
    expect(c).toEqual({ format: 1, name: 'Mine', language: 'es', template: { id: 'my-role', title: 'Role', description: 'Desc' } });
  });
  it('requires format 1 exactly', () => {
    expect(err(() => parseConfig(enc('name: x\n'))).code).toBe('missing-key');
    expect(err(() => parseConfig(enc('format: 2\n'))).code).toBe('unsupported-format');
    expect(err(() => parseConfig(enc('format: "1"\n'))).code).toBe('wrong-type');
    expect(err(() => parseConfig(enc('format: 1.0\n'))).code).toBe('wrong-type');
  });
  it('rejects unknown keys', () => {
    const e = err(() => parseConfig(enc('format: 1\ntheme: dark\n')));
    expect(e.code).toBe('unknown-key');
    expect(e.details.key).toBe('theme');
    expect(err(() => parseConfig(enc('format: 1\ntemplate:\n  id: a\n  title: b\n  extra: c\n'))).details.key).toBe(
      'template.extra',
    );
  });
  it('validates name, language and template', () => {
    expect(err(() => parseConfig(enc(`format: 1\nname: "${'a'.repeat(81)}"\n`))).code).toBe('too-long');
    expect(parseConfig(enc(`format: 1\nname: "${'a'.repeat(80)}"\n`)).name).toHaveLength(80);
    expect(err(() => parseConfig(enc('format: 1\nname: "  "\n'))).code).toBe('too-short');
    expect(err(() => parseConfig(enc('format: 1\nlanguage: fr\n'))).code).toBe('invalid-value');
    expect(err(() => parseConfig(enc('format: 1\ntemplate:\n  title: b\n'))).code).toBe('missing-key');
    expect(err(() => parseConfig(enc('format: 1\ntemplate:\n  id: Bad_Id\n  title: b\n'))).code).toBe('invalid-value');
    expect(err(() => parseConfig(enc('format: 1\ntemplate:\n  id: a\n  title: b\n  description: "' + 'x'.repeat(501) + '"\n'))).code).toBe(
      'too-long',
    );
    expect(err(() => parseConfig(enc('format: 1\ntemplate: x\n'))).code).toBe('wrong-type');
  });
  it('round-trips', () => {
    const c = { format: 1 as const, name: 'N "q"', language: 'en' as const, template: { id: 'true', title: 'T' } };
    expect(parseConfig(enc(serializeConfig(c)))).toEqual(c);
  });
});

describe('section (§5.2)', () => {
  it('parses the example', () => {
    expect(section(sectionFile({}, 'Stack.\n'))).toEqual({
      slug: 'technical',
      title: 'Technical',
      order: 1,
      created: TS,
      updated: TS,
      description: 'Stack.',
    });
  });
  it('requires every key', () => {
    for (const key of ['title', 'order', 'created', 'updated']) {
      const e = err(() => section(sectionFile({ [key]: '' })));
      expect(e.code, key).toBe('missing-key');
      expect(e.details.key).toBe(key);
    }
  });
  it('rejects unknown keys', () => {
    expect(err(() => section(sectionFile({ status: 'pending' }))).code).toBe('unknown-key');
  });
  it('validates the title', () => {
    expect(section(sectionFile({ title: `"  ${'a'.repeat(80)}  "` })).title).toHaveLength(80);
    expect(err(() => section(sectionFile({ title: `"${'a'.repeat(81)}"` }))).code).toBe('too-long');
    expect(err(() => section(sectionFile({ title: '"   "' }))).code).toBe('too-short');
    expect(err(() => section(sectionFile({ title: '"a\\u0007b"' }))).code).toBe('control-characters');
    expect(err(() => section(sectionFile({ title: '"a\\nb"' }))).code).toBe('control-characters');
    expect(err(() => section(sectionFile({ title: '"a\\x85b"' }))).code).toBe('control-characters');
    expect(err(() => section(sectionFile({ title: '123' }))).code).toBe('wrong-type');
    // Code points, not UTF-16 units: 80 emoji are fine.
    expect(section(sectionFile({ title: `"${'🙂'.repeat(80)}"` })).title).toHaveLength(160);
  });
  it('validates order range and type', () => {
    expect(section(sectionFile({ order: '0' })).order).toBe(0);
    expect(section(sectionFile({ order: '1000000' })).order).toBe(1_000_000);
    expect(err(() => section(sectionFile({ order: '1000001' }))).code).toBe('out-of-range');
    expect(err(() => section(sectionFile({ order: '-1' }))).code).toBe('out-of-range');
    expect(err(() => section(sectionFile({ order: '"1"' }))).code).toBe('wrong-type');
    expect(err(() => section(sectionFile({ order: '1.5' }))).code).toBe('wrong-type');
    expect(err(() => section(sectionFile({ order: '99999999999999999999' }))).code).toBe('out-of-range');
  });
  it('validates timestamps', () => {
    for (const bad of [
      '2026-11-03T14:05:09+00:00',
      '2026-11-03T14:05:09.123Z',
      '2026-11-03 14:05:09Z',
      '2026-02-30T00:00:00Z',
      '2026-11-03T24:00:00Z',
      '2026-11-03T23:60:00Z',
      '2026-11-03T23:59:60Z',
      '1969-12-31T23:59:59Z',
      '2026-11-03t14:05:09z',
    ]) {
      expect(err(() => section(sectionFile({ created: bad, updated: bad }))).code, bad).toBe('invalid-value');
    }
    expect(err(() => section(sectionFile({ created: '2026-11-03T10:00:00Z', updated: '2026-11-03T09:00:00Z' }))).details.key).toBe(
      'updated',
    );
    expect(section(sectionFile({ created: '"2026-11-03T10:00:00Z"', updated: '2026-11-04T00:00:00Z' })).updated).toBe(
      '2026-11-04T00:00:00Z',
    );
  });
  it('limits the description to 2000 chars', () => {
    expect(section(sectionFile({}, 'é'.repeat(2000))).description).toHaveLength(2000);
    expect(err(() => section(sectionFile({}, 'é'.repeat(2001)))).code).toBe('too-long');
  });
  it('round-trips', () => {
    const s = section(sectionFile({ title: '"Équipe: \\"core\\""' }, 'Desc\n\nMore'));
    const text = serializeSection(s);
    expect(text.endsWith('More\n')).toBe(true);
    expect(text.endsWith('\n\n')).toBe(false);
    expect(section(text)).toEqual(s);
    const empty = { ...s, description: '' };
    expect(serializeSection(empty).endsWith('---\n')).toBe(true);
    expect(section(serializeSection(empty))).toEqual(empty);
  });
});

describe('topic (§5.3)', () => {
  it('parses the spec example', () => {
    const t = topic(
      '---\ntitle: "Backtesting: how a strategy is tested"\nstatus: in-progress\ntype: role\norder: 4\nresources:\n  - title: Backtesting guide\n    url: https://example.com/wiki/backtesting\ncreated: 2026-11-01T12:00:00Z\nupdated: 2026-11-03T09:41:00Z\n---\nBefore running.\n',
    );
    expect(t.title).toBe('Backtesting: how a strategy is tested');
    expect(t.suggested).toBe(false);
    expect(t.resources).toEqual([{ title: 'Backtesting guide', url: 'https://example.com/wiki/backtesting' }]);
    expect(t.notes).toBe('Before running.');
  });
  it('validates status, type and suggested', () => {
    expect(err(() => topic(topicFile({ status: 'done' }))).code).toBe('invalid-value');
    expect(err(() => topic(topicFile({ type: 'team' }))).code).toBe('invalid-value');
    expect(topic(topicFile({ suggested: 'true' })).suggested).toBe(true);
    expect(err(() => topic(topicFile({ suggested: 'yes' }))).code).toBe('wrong-type');
    expect(err(() => topic(topicFile({ suggested: '"true"' }))).code).toBe('wrong-type');
    expect(err(() => topic(topicFile({ suggested: '~' }))).code).toBe('wrong-type');
  });
  it('validates the title length (160)', () => {
    expect(topic(topicFile({ title: `"${'a'.repeat(160)}"` })).title).toHaveLength(160);
    expect(err(() => topic(topicFile({ title: `"${'a'.repeat(161)}"` }))).code).toBe('too-long');
  });
  it('validates resources', () => {
    const res = (items: string): string => topicFile({ resources: `\n${items}` });
    expect(err(() => topic(res('  - title: x\n'))).details.key).toBe('resources.0.url');
    expect(err(() => topic(res('  - url: https://a.example\n    extra: 1\n'))).details.key).toBe('resources.0.extra');
    expect(err(() => topic(res('  - https://a.example\n'))).code).toBe('wrong-type');
    expect(err(() => topic(topicFile({ resources: 'https://a.example' }))).code).toBe('wrong-type');
    for (const bad of [
      'javascript:alert(1)',
      'ftp://example.com/x',
      'data:text/html,hi',
      '//example.com/x',
      '/relative',
      'https://user:pw@example.com/',
      'https://user@example.com/',
      'https://@example.com/',
      'https:///nohost',
      '"https://exa mple.com/"',
      '"https://example.com/\\t"',
    ]) {
      expect(err(() => topic(res(`  - url: ${bad}\n`))).code, bad).toBe('invalid-value');
    }
    expect(topic(res('  - url: HTTPS://Example.com/Path?q=1#h\n')).resources[0]?.url).toBe('HTTPS://Example.com/Path?q=1#h');
    const long = `https://example.com/${'a'.repeat(2048 - 20)}`;
    expect(long).toHaveLength(2048);
    expect(topic(res(`  - url: ${long}\n`)).resources).toHaveLength(1);
    expect(err(() => topic(res(`  - url: ${long}a\n`))).code).toBe('too-long');
    expect(err(() => topic(res(`  - url: https://a.example\n    title: "${'t'.repeat(161)}"\n`))).code).toBe('too-long');
    expect(err(() => topic(res('  - url: https://a.example\n    title: ""\n'))).code).toBe('too-short');
  });
  it('allows at most 50 resources', () => {
    const items = (n: number): string => Array.from({ length: n }, (_, i) => `  - url: https://example.com/${String(i)}\n`).join('');
    expect(topic(topicFile({ resources: `\n${items(50)}` })).resources).toHaveLength(50);
    const e = err(() => topic(topicFile({ resources: `\n${items(51)}` })));
    expect(e.code).toBe('limit-exceeded');
    expect(e.details.limit).toBe('resources');
  });
  it('limits notes to 256 KiB', () => {
    expect(topic(topicFile({}, 'a'.repeat(256 * KIB))).notes).toHaveLength(256 * KIB);
    expect(err(() => topic(topicFile({}, 'a'.repeat(256 * KIB + 1)))).code).toBe('too-long');
    // Bytes, not chars.
    expect(err(() => topic(topicFile({}, 'é'.repeat(128 * KIB + 1)))).code).toBe('too-long');
  });
  it('round-trips with resources', () => {
    const t = topic(
      topicFile({ suggested: 'true', resources: '\n  - title: "A: b"\n    url: "https://example.com/a?x=1&y=2"\n  - url: https://example.com/b\n' }, '# Notes\n\n- one'),
    );
    expect(topic(serializeTopic(t))).toEqual(t);
  });
});

describe('journal entry (§5.4)', () => {
  it('parses an entry with links', () => {
    const e = entry(entryFile({ section: 'technical', topic: 'technical/backtesting' }));
    expect(e).toMatchObject({ id: ENTRY_ID, date: '2026-11-03', section: 'technical', topic: 'technical/backtesting' });
    expect(e.body).toBe('Learned something.');
  });
  it('requires the date to match the path', () => {
    expect(err(() => entry(entryFile({ date: '2026-11-04' }))).code).toBe('path-date-mismatch');
    expect(err(() => entry(entryFile({ date: '2026-02-30' }), '2026-02-30')).code).toBe('invalid-value');
    expect(err(() => entry(entryFile({ date: '"2026-11-3"' }))).code).toBe('invalid-value');
    expect(err(() => entry(entryFile({ date: '1969-12-31' }), '1969-12-31')).code).toBe('invalid-value');
  });
  it('validates links', () => {
    expect(err(() => entry(entryFile({ topic: 'technical/backtesting' }))).details.key).toBe('section');
    expect(err(() => entry(entryFile({ section: 'team', topic: 'technical/backtesting' }))).details.key).toBe('topic');
    expect(err(() => entry(entryFile({ section: 'Technical' }))).code).toBe('invalid-value');
    expect(err(() => entry(entryFile({ section: 'con' }))).code).toBe('invalid-value');
    expect(err(() => entry(entryFile({ section: 'technical', topic: 'technical' }))).code).toBe('invalid-value');
    expect(err(() => entry(entryFile({ section: 'technical', topic: 'technical/a/b' }))).code).toBe('invalid-value');
  });
  it('requires 1 byte to 64 KiB of text after trimming', () => {
    expect(err(() => entry(entryFile({}, ''))).code).toBe('too-short');
    expect(err(() => entry(entryFile({}, '   \n\t\n'))).code).toBe('too-short');
    expect(entry(entryFile({}, 'x')).body).toBe('x');
    expect(entry(entryFile({}, `  ${'a'.repeat(64 * KIB)}  \n`)).body.trim()).toHaveLength(64 * KIB);
    expect(err(() => entry(entryFile({}, 'a'.repeat(64 * KIB + 1)))).code).toBe('too-long');
  });
  it('rejects unknown keys', () => {
    expect(err(() => entry(entryFile({ title: 'x' }))).code).toBe('unknown-key');
  });
  it('round-trips', () => {
    const e = entry(entryFile({ section: '"123"', topic: '"123/null"' }, 'Body *md*'));
    expect(e.section).toBe('123');
    expect(entry(serializeEntry(e))).toEqual(e);
  });
});
