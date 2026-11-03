import { describe, expect, it } from 'vitest';
import { FormatError } from '../errors';
import { emitYaml, parseStrictYaml, quote } from '../yaml';

function code(fn: () => unknown): string {
  try {
    fn();
  } catch (e) {
    if (e instanceof FormatError) return e.code;
    throw e;
  }
  return 'ok';
}

describe('parseStrictYaml', () => {
  it('parses a plain mapping with exact types', () => {
    const m = parseStrictYaml('a: 1\nb: "x"\nc: true\nd: plain text\ne: 2026-11-03\n');
    expect(m.entries.get('a')).toEqual({ kind: 'int', value: 1 });
    expect(m.entries.get('b')).toEqual({ kind: 'str', value: 'x' });
    expect(m.entries.get('c')).toEqual({ kind: 'bool', value: true });
    expect(m.entries.get('d')).toEqual({ kind: 'str', value: 'plain text' });
    // YAML 1.2 core: dates and timestamps stay strings.
    expect(m.entries.get('e')).toEqual({ kind: 'str', value: '2026-11-03' });
  });

  it('does not coerce "yes", quoted numbers or quoted booleans', () => {
    const m = parseStrictYaml('a: yes\nb: "1"\nc: "true"\nd: no\n');
    expect(m.entries.get('a')).toEqual({ kind: 'str', value: 'yes' });
    expect(m.entries.get('b')).toEqual({ kind: 'str', value: '1' });
    expect(m.entries.get('c')).toEqual({ kind: 'str', value: 'true' });
    expect(m.entries.get('d')).toEqual({ kind: 'str', value: 'no' });
  });

  it('accepts only decimal integers and lowercase booleans', () => {
    for (const v of ['0x10', '0o7', '+1', '1.0', '1e3', '007', '.inf', '.nan']) {
      expect(parseStrictYaml(`a: ${v}\n`).entries.get('a')?.kind, v).toBe('other');
    }
    for (const v of ['True', 'FALSE']) {
      expect(parseStrictYaml(`a: ${v}\n`).entries.get('a')?.kind, v).toBe('other');
    }
    expect(parseStrictYaml('a: -3\n').entries.get('a')).toEqual({ kind: 'int', value: -3 });
  });

  it('marks integers beyond the safe range', () => {
    expect(parseStrictYaml('a: 99999999999999999999\n').entries.get('a')).toEqual({
      kind: 'int',
      value: Number.POSITIVE_INFINITY,
    });
  });

  it('keeps nulls as nulls', () => {
    const m = parseStrictYaml('a: ~\nb:\nc: null\n');
    expect(m.entries.get('a')?.kind).toBe('null');
    expect(m.entries.get('b')?.kind).toBe('null');
    expect(m.entries.get('c')?.kind).toBe('null');
  });

  it('rejects anchors', () => {
    expect(code(() => parseStrictYaml('a: &x 1\n'))).toBe('yaml-anchor');
    expect(code(() => parseStrictYaml('a: &x\n  b: 1\n'))).toBe('yaml-anchor');
  });

  it('rejects aliases', () => {
    expect(code(() => parseStrictYaml('a: *x\n'))).toBe('yaml-alias');
    expect(code(() => parseStrictYaml('a: [1]\nb:\n  - *a\n'))).toBe('yaml-alias');
  });

  it('rejects tags, including standard ones', () => {
    expect(code(() => parseStrictYaml('a: !!str 1\n'))).toBe('yaml-tag');
    expect(code(() => parseStrictYaml('a: !custom x\n'))).toBe('yaml-tag');
    expect(code(() => parseStrictYaml('a: !!binary aGVsbG8=\n'))).toBe('yaml-tag');
    expect(code(() => parseStrictYaml('!!map\na: 1\n'))).toBe('yaml-tag');
  });

  it('rejects merge keys', () => {
    expect(code(() => parseStrictYaml('<<: {a: 1}\nb: 2\n'))).toBe('yaml-merge-key');
  });

  it('rejects multiple documents', () => {
    expect(code(() => parseStrictYaml('a: 1\n---\nb: 2\n'))).toBe('yaml-multiple-documents');
  });

  it('rejects duplicate keys', () => {
    expect(code(() => parseStrictYaml('a: 1\na: 2\n'))).toBe('yaml-duplicate-key');
    expect(code(() => parseStrictYaml('a: 1\n"a": 2\n'))).toBe('yaml-duplicate-key');
  });

  it('rejects directives', () => {
    expect(code(() => parseStrictYaml('%YAML 1.2\n---\na: 1\n'))).toBe('yaml-directive');
    expect(code(() => parseStrictYaml('%TAG !e! tag:example.com,2000:\n---\na: 1\n'))).toBe('yaml-directive');
  });

  it('rejects complex keys, non-mappings and empty input', () => {
    expect(code(() => parseStrictYaml('? [a]\n: 1\n'))).toBe('yaml-complex-key');
    expect(code(() => parseStrictYaml('- a\n- b\n'))).toBe('yaml-not-mapping');
    expect(code(() => parseStrictYaml('just text\n'))).toBe('yaml-not-mapping');
    expect(code(() => parseStrictYaml(''))).toBe('yaml-not-mapping');
    expect(code(() => parseStrictYaml('# only a comment\n'))).toBe('yaml-not-mapping');
  });

  it('rejects syntax errors', () => {
    expect(code(() => parseStrictYaml('a: "unterminated\n'))).toBe('yaml-syntax');
    expect(code(() => parseStrictYaml('a: 1\n  b: 2\n'))).toBe('yaml-syntax');
  });
});

describe('emitYaml', () => {
  it('round-trips through the strict parser', () => {
    const text = emitYaml([
      ['title', { quoted: 'He said "hi" \\ \n\t\u0001\u007f\u0085 ñ 🙂' }],
      ['order', { int: 4 }],
      ['flag', { bool: true }],
      ['created', { plain: '2026-11-03T09:41:00Z' }],
      ['skip', undefined],
      ['nested', { map: [['id', { quoted: 'x' }]] }],
      [
        'list',
        {
          seqOfMaps: [
            [
              ['title', { quoted: 'A' }],
              ['url', { quoted: 'https://example.com/a' }],
            ],
            [['url', { quoted: 'https://example.com/b' }]],
          ],
        },
      ],
    ]);
    const m = parseStrictYaml(text);
    expect(m.entries.get('title')).toEqual({ kind: 'str', value: 'He said "hi" \\ \n\t\u0001\u007f\u0085 ñ 🙂' });
    expect(m.entries.get('order')).toEqual({ kind: 'int', value: 4 });
    expect(m.entries.get('flag')).toEqual({ kind: 'bool', value: true });
    expect(m.entries.has('skip')).toBe(false);
    const list = m.entries.get('list');
    expect(list?.kind).toBe('seq');
    if (list?.kind === 'seq') expect(list.items).toHaveLength(2);
  });

  it('quotes strings that would otherwise change type', () => {
    for (const s of ['true', 'null', '123', '~', '', '- x', '#c', 'a: b']) {
      expect(parseStrictYaml(`k: ${quote(s)}\n`).entries.get('k')).toEqual({ kind: 'str', value: s });
    }
  });
});
