import { describe, expect, it } from 'vitest';
import { highlightPieces, matchRanges } from './highlight';

describe('highlightPieces', () => {
  it('marks matches ignoring case', () => {
    expect(highlightPieces('Risk and risk', 'RISK')).toEqual([
      { text: 'Risk', match: true },
      { text: ' and ', match: false },
      { text: 'risk', match: true },
    ]);
  });

  it('ignores accents on both sides like the server search', () => {
    expect(highlightPieces('Reunión de gestión', 'gestion')).toEqual([
      { text: 'Reunión de ', match: false },
      { text: 'gestión', match: true },
    ]);
    expect(highlightPieces('gestion', 'gestión')).toEqual([{ text: 'gestion', match: true }]);
  });

  it('returns the text untouched without a match or with an empty query', () => {
    expect(highlightPieces('abc', 'x')).toEqual([{ text: 'abc', match: false }]);
    expect(highlightPieces('abc', '  ')).toEqual([{ text: 'abc', match: false }]);
    expect(highlightPieces('', 'a')).toEqual([]);
  });

  it('keeps HTML-looking text as plain data', () => {
    const pieces = highlightPieces('<b>bold</b> <script>', 'bold');
    expect(pieces.map((p) => p.text).join('')).toBe('<b>bold</b> <script>');
    expect(pieces.find((p) => p.match)?.text).toBe('bold');
  });

  it('handles astral characters without splitting them', () => {
    const [r] = matchRanges('a\u{1F600}b', 'b');
    expect(r).toEqual([3, 4]);
  });

  it('does not overlap matches', () => {
    expect(matchRanges('aaaa', 'aa')).toEqual([
      [0, 2],
      [2, 4],
    ]);
  });
});
