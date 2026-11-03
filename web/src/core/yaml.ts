/**
 * The restricted YAML subset used by frontmatter and `bearings.yaml` (§4).
 *
 * Parsing uses the `yaml` package's AST (YAML 1.2 core schema) and then walks
 * it, rejecting anchors, aliases, tags, merge keys, directives, multiple
 * documents, duplicate keys and complex keys. Scalars keep their exact type:
 * integers must be written as plain decimal numbers and booleans as plain
 * `true`/`false`; nothing is coerced.
 *
 * Writing uses a small emitter that produces only what this module accepts.
 */
import {
  isAlias,
  isMap,
  isPair,
  isScalar,
  isSeq,
  parseAllDocuments,
  type Document,
  type Node as YNode,
} from 'yaml';
import { FormatError, type ErrorCode } from './errors';

export type Value =
  | { kind: 'str'; value: string }
  | { kind: 'int'; value: number }
  | { kind: 'bool'; value: boolean }
  | { kind: 'null' }
  | { kind: 'other' }
  | { kind: 'map'; entries: Map<string, Value> }
  | { kind: 'seq'; items: Value[] };

export type MapValue = Extract<Value, { kind: 'map' }>;

const INT_SOURCE = /^-?(?:0|[1-9][0-9]*)$/;

/** Parses a YAML text that must hold exactly one mapping. */
export function parseStrictYaml(text: string): MapValue {
  const docs = parseAllDocuments(text, {
    version: '1.2',
    schema: 'core',
    uniqueKeys: true,
    merge: false,
    strict: true,
    prettyErrors: false,
    keepSourceTokens: false,
  });
  if (!Array.isArray(docs)) {
    // EmptyStream: no document at all.
    throw new FormatError('yaml-not-mapping');
  }
  if (docs.length === 0) throw new FormatError('yaml-not-mapping');
  if (docs.length > 1) throw new FormatError('yaml-multiple-documents');
  const doc = docs[0] as Document.Parsed;

  const tags = doc.directives.tags;
  const tagHandles = Object.keys(tags);
  if (doc.directives.yaml.explicit || tagHandles.length !== 1 || tags['!!'] !== 'tag:yaml.org,2002:') {
    throw new FormatError('yaml-directive');
  }

  // Walk first: a specific violation (alias, anchor…) is more useful than the
  // generic parse error it may also cause.
  const violation = findViolation(doc.contents);
  if (violation) throw new FormatError(violation);

  if (doc.errors.length > 0) {
    const dup = doc.errors.some((e) => e.code === 'DUPLICATE_KEY');
    throw new FormatError(dup ? 'yaml-duplicate-key' : 'yaml-syntax');
  }
  if (doc.warnings.length > 0) {
    const tag = doc.warnings.some((w) => w.code === 'TAG_RESOLVE_FAILED');
    throw new FormatError(tag ? 'yaml-tag' : 'yaml-syntax');
  }

  const root = convert(doc.contents);
  if (root.kind !== 'map') throw new FormatError('yaml-not-mapping');
  return root;
}

function findViolation(node: unknown): ErrorCode | null {
  if (node === null || node === undefined) return null;
  if (isAlias(node)) return 'yaml-alias';
  if (isPair(node)) {
    if (isScalar(node.key) && node.key.value === '<<' && node.key.type === 'PLAIN') return 'yaml-merge-key';
    return findViolation(node.key) ?? findViolation(node.value);
  }
  const n = node as YNode;
  if (n.anchor) return 'yaml-anchor';
  if (n.tag) return 'yaml-tag';
  if (isMap(n) || isSeq(n)) {
    for (const item of n.items) {
      const v = findViolation(item);
      if (v) return v;
    }
  }
  return null;
}

function convert(node: unknown): Value {
  if (node === null || node === undefined) return { kind: 'null' };
  if (isScalar(node)) {
    const v: unknown = node.value;
    if (v === null) return { kind: 'null' };
    if (typeof v === 'string') return { kind: 'str', value: v };
    const plain = node.type === 'PLAIN' ? (node.source ?? '') : '';
    if (typeof v === 'boolean') {
      if (plain !== 'true' && plain !== 'false') return { kind: 'other' };
      return { kind: 'bool', value: v };
    }
    if (typeof v === 'number') {
      if (INT_SOURCE.test(plain) && Number.isSafeInteger(v)) return { kind: 'int', value: v };
      if (INT_SOURCE.test(plain)) return { kind: 'int', value: Number.POSITIVE_INFINITY };
      return { kind: 'other' };
    }
    // bigint or anything else the schema might produce.
    return { kind: 'other' };
  }
  if (isMap(node)) {
    const entries = new Map<string, Value>();
    for (const pair of node.items) {
      if (!isScalar(pair.key)) throw new FormatError('yaml-complex-key');
      const key: unknown = pair.key.value;
      const name = typeof key === 'string' ? key : (pair.key.source ?? String(key));
      if (entries.has(name)) throw new FormatError('yaml-duplicate-key', { key: name });
      entries.set(name, convert(pair.value));
    }
    return { kind: 'map', entries };
  }
  if (isSeq(node)) {
    return { kind: 'seq', items: node.items.map((item) => convert(item)) };
  }
  throw new FormatError('yaml-syntax');
}

// ---------------------------------------------------------------------------
// Emitter

export type EmitValue =
  | { quoted: string }
  | { plain: string }
  | { int: number }
  | { bool: boolean }
  | { map: EmitEntry[] }
  | { seqOfMaps: EmitEntry[][] };

export type EmitEntry = readonly [key: string, value: EmitValue | undefined];

/** Emits a block mapping. Entries with an `undefined` value are skipped. */
export function emitYaml(entries: readonly EmitEntry[], indent = ''): string {
  let out = '';
  for (const [key, value] of entries) {
    if (value === undefined) continue;
    out += `${indent}${key}:`;
    if ('map' in value) {
      out += '\n' + emitYaml(value.map, indent + '  ');
    } else if ('seqOfMaps' in value) {
      out += '\n';
      for (const item of value.seqOfMaps) {
        const body = emitYaml(item, indent + '    ');
        // Replace the first line's indentation with the "- " marker.
        out += `${indent}  - ${body.slice(indent.length + 4)}`;
      }
    } else {
      out += ` ${scalar(value)}\n`;
    }
  }
  return out;
}

function scalar(v: Exclude<EmitValue, { map: unknown } | { seqOfMaps: unknown }>): string {
  if ('quoted' in v) return quote(v.quoted);
  if ('plain' in v) return v.plain;
  if ('int' in v) return String(v.int);
  return v.bool ? 'true' : 'false';
}

/** YAML double-quoted scalar. Escapes everything outside the printable set. */
export function quote(s: string): string {
  let out = '"';
  for (const ch of s) {
    const c = ch.codePointAt(0) ?? 0;
    if (ch === '"') out += '\\"';
    else if (ch === '\\') out += '\\\\';
    else if (c === 0x0a) out += '\\n';
    else if (c === 0x09) out += '\\t';
    else if (c === 0x0d) out += '\\r';
    else if (c < 0x20 || c === 0x7f || (c >= 0x80 && c <= 0x9f) || c === 0xfeff || (c >= 0xd800 && c <= 0xdfff)) {
      out += '\\u' + c.toString(16).padStart(4, '0');
    } else out += ch;
  }
  return out + '"';
}
