/** Whole-notebook reading and writing over a set of files (§2, §7). */
import { FormatError, NotebookErrors } from './errors';
import { ENTRY_ID_RE, isReservedName, looksLikeSlug, topicRef } from './ids';
import { MAX_DEPTH, MAX_ENTRIES, MAX_FILE_BYTES, MAX_SECTIONS, MAX_TOPICS_PER_SECTION } from './limits';
import type { Entry, Notebook, Section, Topic, Warning } from './model';
import {
  parseConfig,
  parseEntry,
  parseSection,
  parseTopic,
  serializeConfig,
  serializeEntry,
  serializeSection,
  serializeTopic,
} from './files';
import { sortEntries, sortSections, sortTopics } from './ordering';
import { encodeText } from './text';
import { isDate } from './time';

export const CONFIG_PATH = 'bearings.yaml';

export type PathKind =
  | { kind: 'config' }
  | { kind: 'section'; section: string }
  | { kind: 'topic'; section: string; topic: string }
  | { kind: 'entry'; id: string; date: string; dirYear: string; dirMonth: string }
  | { kind: 'ignored' };

const SECTION_PATH = /^sections\/([^/]+)\/_section\.md$/;
const TOPIC_PATH = /^sections\/([^/]+)\/([^/]+)\.md$/;
const ENTRY_PATH = /^journal\/(\d{4})\/(\d{2})\/(\d{4}-\d{2}-\d{2})-([^/]+)\.md$/;

/**
 * Classifies a root-relative path against the patterns of §2. Anything else is
 * ignored. A path that has the right shape but uses a reserved slug, or whose
 * directories disagree with its date, is an error rather than ignored: it
 * would otherwise silently drop the user's data.
 */
export function classifyPath(path: string): PathKind {
  if (path === CONFIG_PATH) return { kind: 'config' };
  let m = SECTION_PATH.exec(path);
  if (m) {
    const section = m[1] ?? '';
    if (!looksLikeSlug(section)) return { kind: 'ignored' };
    if (isReservedName(section)) throw new FormatError('reserved-slug', { file: path });
    return { kind: 'section', section };
  }
  m = TOPIC_PATH.exec(path);
  if (m) {
    const section = m[1] ?? '';
    const topic = m[2] ?? '';
    if (!looksLikeSlug(section) || !looksLikeSlug(topic)) return { kind: 'ignored' };
    if (isReservedName(section) || isReservedName(topic)) throw new FormatError('reserved-slug', { file: path });
    return { kind: 'topic', section, topic };
  }
  m = ENTRY_PATH.exec(path);
  if (m) {
    const id = m[4] ?? '';
    const date = m[3] ?? '';
    if (!ENTRY_ID_RE.test(id)) return { kind: 'ignored' };
    return { kind: 'entry', id, date, dirYear: m[1] ?? '', dirMonth: m[2] ?? '' };
  }
  return { kind: 'ignored' };
}

/** Number of directories between the root and the file. */
export function pathDepth(path: string): number {
  let n = 0;
  for (const ch of path) if (ch === '/') n++;
  return n;
}

export function sectionPath(section: string): string {
  return `sections/${section}/_section.md`;
}

export function topicPath(section: string, topic: string): string {
  return `sections/${section}/${topic}.md`;
}

export function entryPath(e: Pick<Entry, 'id' | 'date'>): string {
  return `journal/${e.date.slice(0, 4)}/${e.date.slice(5, 7)}/${e.date}-${e.id}.md`;
}

export interface FileInput {
  path: string;
  bytes: Uint8Array;
}

export interface LoadResult {
  notebook: Notebook;
  warnings: Warning[];
  /** Files that matched no §2 pattern and were skipped. */
  ignored: number;
}

const MAX_REPORTED_ERRORS = 50;

/**
 * Reads a notebook from its files. Validates everything; on any error throws
 * {@link NotebookErrors} listing up to 50 problems. Never partially succeeds.
 */
export function loadNotebook(files: Iterable<FileInput>): LoadResult {
  const errors: FormatError[] = [];
  const fail = (e: unknown, file: string): void => {
    if (errors.length >= MAX_REPORTED_ERRORS) return;
    if (e instanceof FormatError) errors.push(e.inFile(file));
    else errors.push(new FormatError('invalid-value', { file }));
  };

  let config: Notebook['config'] | undefined;
  const sections = new Map<string, Section>();
  const topicFiles: { file: FileInput; section: string; topic: string }[] = [];
  const entries = new Map<string, Entry>();
  const topicsPerSection = new Map<string, number>();
  let ignored = 0;
  let entryCount = 0;
  const seen = new Set<string>();

  for (const file of files) {
    try {
      if (seen.has(file.path)) throw new FormatError('duplicate-id');
      seen.add(file.path);
      if (pathDepth(file.path) > MAX_DEPTH) {
        throw new FormatError('limit-exceeded', { limit: 'depth', max: MAX_DEPTH });
      }
      const kind = classifyPath(file.path);
      if (kind.kind === 'ignored') {
        ignored++;
        continue;
      }
      if (file.bytes.length > MAX_FILE_BYTES) {
        throw new FormatError('file-too-large', { limit: 'file-size', max: MAX_FILE_BYTES });
      }
      switch (kind.kind) {
        case 'config':
          config = parseConfig(file.bytes);
          break;
        case 'section':
          if (sections.size >= MAX_SECTIONS) {
            throw new FormatError('limit-exceeded', { limit: 'sections', max: MAX_SECTIONS });
          }
          sections.set(kind.section, parseSection(file.bytes, kind.section));
          break;
        case 'topic': {
          const n = (topicsPerSection.get(kind.section) ?? 0) + 1;
          if (n > MAX_TOPICS_PER_SECTION) {
            throw new FormatError('limit-exceeded', { limit: 'topics-per-section', max: MAX_TOPICS_PER_SECTION });
          }
          topicsPerSection.set(kind.section, n);
          topicFiles.push({ file, section: kind.section, topic: kind.topic });
          break;
        }
        case 'entry': {
          if (++entryCount > MAX_ENTRIES) {
            throw new FormatError('limit-exceeded', { limit: 'entries', max: MAX_ENTRIES });
          }
          if (!isDate(kind.date)) throw new FormatError('invalid-value', { key: 'date' });
          if (kind.dirYear !== kind.date.slice(0, 4) || kind.dirMonth !== kind.date.slice(5, 7)) {
            throw new FormatError('path-date-mismatch');
          }
          if (entries.has(kind.id)) throw new FormatError('duplicate-id');
          entries.set(kind.id, parseEntry(file.bytes, kind.id, kind.date));
          break;
        }
      }
    } catch (e) {
      fail(e, file.path);
      // Limits that apply to the whole notebook: stop instead of reporting each file.
      if (e instanceof FormatError && e.code === 'limit-exceeded' && e.details.limit !== 'depth') break;
    }
  }

  const topics: Topic[] = [];
  for (const { file, section, topic } of topicFiles) {
    try {
      if (!sections.has(section)) throw new FormatError('missing-section-file');
      topics.push(parseTopic(file.bytes, section, topic));
    } catch (e) {
      fail(e, file.path);
    }
  }

  if (config === undefined && errors.length === 0) {
    errors.push(new FormatError('missing-config', { file: CONFIG_PATH }));
  }
  if (errors.length > 0 || config === undefined) throw new NotebookErrors(errors);

  const topicRefs = new Set(topics.map((t) => topicRef(t.section, t.slug)));
  const warnings: Warning[] = [];
  for (const e of entries.values()) {
    const file = entryPath(e);
    if (e.section !== undefined && !sections.has(e.section)) warnings.push({ code: 'dangling-section', file });
    else if (e.topic !== undefined && !topicRefs.has(e.topic)) warnings.push({ code: 'dangling-topic', file });
  }

  return {
    notebook: {
      config,
      sections: sortSections([...sections.values()]),
      topics: sortTopics(topics),
      entries: sortEntries([...entries.values()]),
    },
    warnings,
    ignored,
  };
}

/** Serializes a notebook to its files in the §2 layout. */
export function notebookFiles(nb: Notebook): FileInput[] {
  const out: FileInput[] = [{ path: CONFIG_PATH, bytes: encodeText(serializeConfig(nb.config)) }];
  for (const s of sortSections(nb.sections)) {
    out.push({ path: sectionPath(s.slug), bytes: encodeText(serializeSection(s)) });
  }
  for (const t of sortTopics(nb.topics)) {
    out.push({ path: topicPath(t.section, t.slug), bytes: encodeText(serializeTopic(t)) });
  }
  for (const e of sortEntries(nb.entries)) {
    out.push({ path: entryPath(e), bytes: encodeText(serializeEntry(e)) });
  }
  return out;
}

/** Whether an entry's section/topic links resolve (§5.4: dangling = unlinked). */
export function resolveLinks(
  e: Entry,
  sectionSlugs: ReadonlySet<string>,
  topicRefs: ReadonlySet<string>,
): { section?: string; topic?: string } {
  if (e.section === undefined || !sectionSlugs.has(e.section)) return {};
  if (e.topic === undefined || !topicRefs.has(e.topic)) return { section: e.section };
  return { section: e.section, topic: e.topic };
}
