import { describe, expect, it } from 'vitest';
import { links, parseHash } from './router';

describe('hash routes', () => {
  it('parses segments and query', () => {
    const r = parseHash('#/topics/technical/risk-basics');
    expect(r.segments).toEqual(['topics', 'technical', 'risk-basics']);
    const j = parseHash('#/journal?from=2026-01-01&section=tech');
    expect(j.segments).toEqual(['journal']);
    expect(j.query.get('section')).toBe('tech');
  });

  it('treats an empty hash as home and survives bad escapes', () => {
    expect(parseHash('').segments).toEqual([]);
    expect(parseHash('#/').segments).toEqual([]);
    expect(parseHash('#/a/%E0%A4%A').segments).toEqual(['a', '%E0%A4%A']);
  });

  it('builds links that round-trip', () => {
    expect(parseHash(links.topic('a b/c')).segments).toEqual(['topics', 'a b', 'c']);
    expect(links.journal({ from: '', section: 'x' })).toBe('#/journal?section=x');
    expect(parseHash(links.search('a&b=c')).query.get('q')).toBe('a&b=c');
  });
});
