import { FormatError } from './errors';
import { MAX_FILE_BYTES } from './limits';

const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
const encoder = new TextEncoder();

/**
 * Decodes a notebook file (§4): strict UTF-8, no BOM, no NUL bytes, at most
 * 1 MiB. CRLF is normalized to LF.
 */
export function decodeFile(bytes: Uint8Array): string {
  if (bytes.length > MAX_FILE_BYTES) {
    throw new FormatError('file-too-large', { limit: 'file-size', max: MAX_FILE_BYTES });
  }
  if (bytes.length >= 3 && bytes[0] === 0xef && bytes[1] === 0xbb && bytes[2] === 0xbf) {
    throw new FormatError('bom');
  }
  if (bytes.includes(0)) throw new FormatError('nul-byte');
  let text: string;
  try {
    text = decoder.decode(bytes);
  } catch {
    throw new FormatError('invalid-utf8');
  }
  return text.replace(/\r\n/g, '\n');
}

export function encodeText(text: string): Uint8Array {
  return encoder.encode(text);
}

export function byteLength(text: string): number {
  return encoder.encode(text).length;
}

/** Length in Unicode code points (what the spec calls "chars"). */
export function charLength(text: string): number {
  let n = 0;
  for (const _ of text) n++;
  return n;
}

/** C0 controls, DEL and C1 controls (Unicode category Cc). */
// eslint-disable-next-line no-control-regex
const CONTROL = /[\u0000-\u001f\u007f-\u009f]/;

export function hasControlChars(text: string): boolean {
  return CONTROL.test(text);
}

/** True when the string has no lone surrogates. */
export function isWellFormed(text: string): boolean {
  return text.isWellFormed();
}

/** Removes trailing LF characters (files end with exactly one newline, §4). */
export function stripTrailingNewlines(text: string): string {
  let end = text.length;
  while (end > 0 && text.charCodeAt(end - 1) === 0x0a) end--;
  return text.slice(0, end);
}

/** Lowercase, accent-free form used for searching. */
export function foldForSearch(text: string): string {
  return text.normalize('NFD').replace(/\p{M}+/gu, '').toLowerCase();
}
