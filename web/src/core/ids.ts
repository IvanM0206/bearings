/** Identifiers (§3). */

export const SLUG_RE = /^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$/;
export const ENTRY_ID_RE = /^[0-9a-hjkmnp-tv-z]{10}$/;
export const MAX_SLUG_CHARS = 64;

const RESERVED = new Set([
  'con',
  'prn',
  'aux',
  'nul',
  ...Array.from({ length: 9 }, (_, i) => `com${String(i + 1)}`),
  ...Array.from({ length: 9 }, (_, i) => `lpt${String(i + 1)}`),
]);

export function isReservedName(slug: string): boolean {
  return RESERVED.has(slug);
}

/** Matches the slug syntax, ignoring the reserved-name rule. */
export function looksLikeSlug(value: string): boolean {
  return SLUG_RE.test(value);
}

export function isSlug(value: string): boolean {
  return SLUG_RE.test(value) && !RESERVED.has(value);
}

export function isEntryId(value: string): boolean {
  return ENTRY_ID_RE.test(value);
}

/** `<section-slug>/<topic-slug>`. */
export function isTopicRef(value: string): boolean {
  const parts = value.split('/');
  return parts.length === 2 && isSlug(parts[0] ?? '') && isSlug(parts[1] ?? '');
}

export function topicRef(section: string, topic: string): string {
  return `${section}/${topic}`;
}

export function splitTopicRef(ref: string): { section: string; topic: string } {
  const [section = '', topic = ''] = ref.split('/');
  return { section, topic };
}

/**
 * Derives a slug from a title (§3): transliterate to ASCII, lowercase, replace
 * runs of other characters with `-`, trim to 64 chars, then append `-2`, `-3`…
 * on collision. The suffix is fitted inside the 64-char limit. Titles with no
 * ASCII letters or digits fall back to `fallback`.
 */
export function slugify(title: string, taken: ReadonlySet<string>, fallback: string): string {
  const base =
    title
      .normalize('NFKD')
      .replace(/\p{M}+/gu, '')
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, '-')
      .replace(/^-+|-+$/g, '') || fallback;
  const fit = (stem: string, suffix: string): string =>
    stem.slice(0, MAX_SLUG_CHARS - suffix.length).replace(/-+$/, '') + suffix;
  let candidate = fit(base, '');
  for (let n = 2; !isSlug(candidate) || taken.has(candidate); n++) {
    candidate = fit(base, `-${String(n)}`);
  }
  return candidate;
}

const CROCKFORD = '0123456789abcdefghjkmnpqrstvwxyz';

/** 10 Crockford base32 characters from 50 cryptographically random bits (§3). */
export function newEntryId(random: (bytes: Uint8Array<ArrayBuffer>) => Uint8Array = defaultRandom): string {
  const bytes = random(new Uint8Array(10));
  let id = '';
  for (let i = 0; i < 10; i++) {
    // Each byte contributes its low 5 bits: 10 × 5 = 50 uniform bits.
    id += CROCKFORD.charAt((bytes[i] ?? 0) & 31);
  }
  return id;
}

function defaultRandom(bytes: Uint8Array<ArrayBuffer>): Uint8Array {
  return crypto.getRandomValues(bytes);
}
