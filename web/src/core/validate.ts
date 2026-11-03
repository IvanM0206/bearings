/**
 * Field rules from §5, shared by the file readers and by the app when the user
 * edits data, so both apply exactly the same limits.
 */
import { FormatError } from './errors';
import {
  MAX_ENTRY_BYTES,
  MAX_NAME_CHARS,
  MAX_ORDER,
  MAX_RESOURCE_TITLE_CHARS,
  MAX_RESOURCES,
  MAX_SECTION_DESCRIPTION_CHARS,
  MAX_SECTION_TITLE_CHARS,
  MAX_TEMPLATE_DESCRIPTION_CHARS,
  MAX_TOPIC_NOTES_BYTES,
  MAX_TOPIC_TITLE_CHARS,
  MAX_URL_CHARS,
  MIN_ORDER,
} from './limits';
import type { Resource } from './model';
import { byteLength, charLength, hasControlChars, isWellFormed } from './text';
import { isTimestamp } from './time';

/**
 * A single-line title: trimmed, 1–`max` code points, no control characters.
 * Returns the trimmed value.
 */
export function checkTitle(value: string, max: number, key = 'title'): string {
  const v = value.trim();
  if (!isWellFormed(v)) throw new FormatError('invalid-value', { key });
  if (hasControlChars(v)) throw new FormatError('control-characters', { key });
  const n = charLength(v);
  if (n < 1) throw new FormatError('too-short', { key, min: 1 });
  if (n > max) throw new FormatError('too-long', { key, max });
  return v;
}

export const checkSectionTitle = (v: string): string => checkTitle(v, MAX_SECTION_TITLE_CHARS);
export const checkTopicTitle = (v: string): string => checkTitle(v, MAX_TOPIC_TITLE_CHARS);
export const checkNotebookName = (v: string): string => checkTitle(v, MAX_NAME_CHARS, 'name');

export function checkOrder(value: number, key = 'order'): number {
  if (!Number.isSafeInteger(value) || value < MIN_ORDER || value > MAX_ORDER) {
    throw new FormatError('out-of-range', { key, min: MIN_ORDER, max: MAX_ORDER });
  }
  return value;
}

export function checkTimestamps(created: string, updated: string): void {
  if (!isTimestamp(created)) throw new FormatError('invalid-value', { key: 'created' });
  if (!isTimestamp(updated)) throw new FormatError('invalid-value', { key: 'updated' });
  // Fixed-width UTC format: string order is time order.
  if (updated < created) throw new FormatError('invalid-value', { key: 'updated' });
}

function checkBodyText(body: string, key: string): void {
  if (!isWellFormed(body)) throw new FormatError('invalid-value', { key });
  if (body.includes('\0')) throw new FormatError('nul-byte', { key });
}

export function checkSectionDescription(body: string): string {
  checkBodyText(body, 'body');
  if (charLength(body) > MAX_SECTION_DESCRIPTION_CHARS) {
    throw new FormatError('too-long', { key: 'body', max: MAX_SECTION_DESCRIPTION_CHARS });
  }
  return body;
}

export function checkTopicNotes(body: string): string {
  checkBodyText(body, 'body');
  if (byteLength(body) > MAX_TOPIC_NOTES_BYTES) {
    throw new FormatError('too-long', { key: 'body', max: MAX_TOPIC_NOTES_BYTES });
  }
  return body;
}

/** Entry text: 1 byte to 64 KiB after trimming. */
export function checkEntryBody(body: string): string {
  checkBodyText(body, 'body');
  const n = byteLength(body.trim());
  if (n < 1) throw new FormatError('too-short', { key: 'body', min: 1 });
  if (n > MAX_ENTRY_BYTES) throw new FormatError('too-long', { key: 'body', max: MAX_ENTRY_BYTES });
  return body;
}

export function checkTemplateDescription(value: string): string {
  if (!isWellFormed(value)) throw new FormatError('invalid-value', { key: 'template.description' });
  if (charLength(value) > MAX_TEMPLATE_DESCRIPTION_CHARS) {
    throw new FormatError('too-long', { key: 'template.description', max: MAX_TEMPLATE_DESCRIPTION_CHARS });
  }
  return value;
}

/**
 * Absolute `http`/`https` URL, ≤ 2 048 chars, no userinfo. Whitespace and
 * control characters are rejected (browsers silently strip some of them,
 * which would make the stored and the opened URL differ).
 */
export function checkUrl(value: string, key = 'url'): string {
  if (!isWellFormed(value) || hasControlChars(value) || /\s/u.test(value)) {
    throw new FormatError('invalid-value', { key });
  }
  if (charLength(value) > MAX_URL_CHARS) throw new FormatError('too-long', { key, max: MAX_URL_CHARS });
  const m = /^(https?):\/\/([^/?#]*)/i.exec(value);
  if (!m) throw new FormatError('invalid-value', { key });
  const authority = m[2] ?? '';
  if (authority.includes('@') || authority.length === 0) throw new FormatError('invalid-value', { key });
  let parsed: URL;
  try {
    parsed = new URL(value);
  } catch {
    throw new FormatError('invalid-value', { key });
  }
  if (
    (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') ||
    parsed.username !== '' ||
    parsed.password !== '' ||
    parsed.hostname === ''
  ) {
    throw new FormatError('invalid-value', { key });
  }
  return value;
}

/** True for a URL that may be rendered as a clickable link (§8). */
export function isSafeLink(value: string): boolean {
  try {
    checkUrl(value);
    return true;
  } catch {
    return false;
  }
}

export function checkResource(r: Resource, index: number): Resource {
  const url = checkUrl(r.url, `resources.${String(index)}.url`);
  if (r.title === undefined) return { url };
  const title = checkTitle(r.title, MAX_RESOURCE_TITLE_CHARS, `resources.${String(index)}.title`);
  return { url, title };
}

export function checkResources(list: readonly Resource[]): Resource[] {
  if (list.length > MAX_RESOURCES) {
    throw new FormatError('limit-exceeded', { key: 'resources', limit: 'resources', max: MAX_RESOURCES });
  }
  return list.map((r, i) => checkResource(r, i));
}
