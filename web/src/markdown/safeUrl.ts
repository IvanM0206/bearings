const ALLOWED_PROTOCOLS = new Set(['http:', 'https:', 'mailto:']);

/**
 * Returns the normalized URL when it is safe to make clickable, else null.
 * Only absolute http, https and mailto URLs pass. Relative links, fragments
 * and every other scheme (javascript:, data:, file:, vbscript: ...) do not.
 * Parsing first means tricks such as "java\tscript:" are judged by what the
 * browser would actually do with them.
 */
export function safeHref(raw: string): string | null {
  const trimmed = raw.trim();
  if (trimmed === '' || trimmed.length > 4096) return null;
  let url: URL;
  try {
    url = new URL(trimmed);
  } catch {
    return null;
  }
  if (!ALLOWED_PROTOCOLS.has(url.protocol)) return null;
  if (url.protocol !== 'mailto:' && (url.username !== '' || url.password !== '')) return null;
  return url.href;
}
