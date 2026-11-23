import { Fragment } from 'react';
import { highlightPieces } from '../lib/highlight';

/** Text with the search query marked. Built from React nodes, never from HTML. */
export function Highlight({ text, query }: { text: string; query: string }) {
  return (
    <>
      {highlightPieces(text, query).map((p, i) =>
        p.match ? <mark key={i}>{p.text}</mark> : <Fragment key={i}>{p.text}</Fragment>,
      )}
    </>
  );
}
