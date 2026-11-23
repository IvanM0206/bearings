/** Lowercases and strips accents, keeping the index of each folded unit in the source. */
function fold(text: string): { folded: string; index: number[] } {
  let folded = '';
  const index: number[] = [];
  let i = 0;
  for (const ch of text) {
    const f = ch.normalize('NFD').replace(/\p{M}/gu, '').toLowerCase();
    for (const unit of f) {
      folded += unit;
      for (let k = 0; k < unit.length; k++) index.push(i);
    }
    i += ch.length;
  }
  return { folded, index };
}

/**
 * Finds where `query` occurs in `text`, ignoring case and accents like the
 * server's search does. Returns [start, end) ranges in `text`, never overlapping.
 */
export function matchRanges(text: string, query: string): Array<[number, number]> {
  const needle = fold(query.trim()).folded;
  if (needle === '') return [];
  const { folded, index } = fold(text);
  const out: Array<[number, number]> = [];
  let from = 0;
  for (;;) {
    const at = folded.indexOf(needle, from);
    if (at < 0) break;
    const start = index[at] ?? 0;
    const lastUnit = index[at + needle.length - 1] ?? start;
    // Extend to the end of the source character that holds the last unit.
    const cp = text.codePointAt(lastUnit);
    const end = lastUnit + (cp !== undefined && cp > 0xffff ? 2 : 1);
    const prev = out[out.length - 1];
    if (!prev || start >= prev[1]) out.push([start, end]);
    from = at + needle.length;
  }
  return out;
}

export interface Piece {
  text: string;
  match: boolean;
}

/** Splits `text` into plain and matching pieces. */
export function highlightPieces(text: string, query: string): Piece[] {
  const pieces: Piece[] = [];
  let pos = 0;
  for (const [s, e] of matchRanges(text, query)) {
    if (s > pos) pieces.push({ text: text.slice(pos, s), match: false });
    pieces.push({ text: text.slice(s, e), match: true });
    pos = e;
  }
  if (pos < text.length) pieces.push({ text: text.slice(pos), match: false });
  return pieces;
}
