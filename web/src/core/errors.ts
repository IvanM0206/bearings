/**
 * Errors raised while reading or validating notebook data.
 *
 * Errors carry a machine-readable code plus the file path and key involved, so
 * the UI can show a translated message. They never include file contents.
 */
export type ErrorCode =
  // Encoding and file syntax (§4)
  | 'invalid-utf8'
  | 'bom'
  | 'nul-byte'
  | 'file-too-large'
  | 'frontmatter-missing'
  | 'frontmatter-too-large'
  // Restricted YAML subset (§4)
  | 'yaml-syntax'
  | 'yaml-anchor'
  | 'yaml-alias'
  | 'yaml-tag'
  | 'yaml-merge-key'
  | 'yaml-multiple-documents'
  | 'yaml-duplicate-key'
  | 'yaml-directive'
  | 'yaml-not-mapping'
  | 'yaml-complex-key'
  // Fields (§5)
  | 'unknown-key'
  | 'missing-key'
  | 'wrong-type'
  | 'invalid-value'
  | 'too-long'
  | 'too-short'
  | 'out-of-range'
  | 'control-characters'
  | 'unsupported-format'
  // Layout and identity (§2, §3)
  | 'missing-config'
  | 'missing-section-file'
  | 'reserved-slug'
  | 'path-date-mismatch'
  | 'duplicate-id'
  // Limits (§7)
  | 'limit-exceeded'
  // Archives (§8)
  | 'archive-too-large'
  | 'archive-malformed'
  | 'archive-unsupported'
  | 'archive-encrypted'
  | 'archive-bad-name'
  | 'archive-duplicate-name'
  | 'archive-special-entry'
  | 'archive-ratio'
  | 'archive-size-mismatch'
  | 'archive-crc'
  // Not found / conflicts in the local store
  | 'not-found'
  | 'already-exists';

export type LimitName =
  | 'file-size'
  | 'frontmatter-size'
  | 'sections'
  | 'topics-per-section'
  | 'entries'
  | 'depth'
  | 'archive-entries'
  | 'archive-total-size'
  | 'archive-entry-size'
  | 'resources';

export interface ErrorDetails {
  /** Path of the file, relative to the notebook root. */
  file?: string;
  /** Frontmatter key (dotted for nested keys, e.g. `resources.2.url`). */
  key?: string;
  /** Which §7/§8 limit was exceeded. */
  limit?: LimitName;
  /** A numeric bound (max length, max count) relevant to the error. */
  max?: number;
  min?: number;
}

export class FormatError extends Error {
  readonly code: ErrorCode;
  readonly details: ErrorDetails;

  constructor(code: ErrorCode, details: ErrorDetails = {}, message?: string) {
    super(message ?? describe(code, details));
    this.name = 'FormatError';
    this.code = code;
    this.details = details;
  }

  /** Returns a copy with the file path filled in, if it was missing. */
  inFile(file: string): FormatError {
    if (this.details.file !== undefined) return this;
    return new FormatError(this.code, { ...this.details, file });
  }
}

/** Several errors found while reading a notebook (reported together). */
export class NotebookErrors extends Error {
  readonly errors: readonly FormatError[];
  constructor(errors: readonly FormatError[]) {
    super(errors.map((e) => e.message).join('\n'));
    this.name = 'NotebookErrors';
    this.errors = errors;
  }
}

function describe(code: ErrorCode, d: ErrorDetails): string {
  const parts: string[] = [code];
  if (d.file !== undefined) parts.push(`file=${d.file}`);
  if (d.key !== undefined) parts.push(`key=${d.key}`);
  if (d.limit !== undefined) parts.push(`limit=${d.limit}`);
  if (d.max !== undefined) parts.push(`max=${String(d.max)}`);
  if (d.min !== undefined) parts.push(`min=${String(d.min)}`);
  return parts.join(' ');
}
