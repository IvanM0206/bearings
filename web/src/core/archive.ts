/** Notebook archives (§8): ZIP files with the §2 layout at their root. */
import { FormatError, NotebookErrors } from './errors';
import { MAX_ARCHIVE_FILE_BYTES, MAX_DEPTH } from './limits';
import type { Notebook } from './model';
import { classifyPath, loadNotebook, notebookFiles, type LoadResult } from './notebook';
import { listZip, extractEntry, writeZip } from './zip';

export interface ArchiveSummary {
  name?: string;
  sections: number;
  topics: number;
  entries: number;
  isTemplate: boolean;
}

export interface ImportResult extends LoadResult {
  summary: ArchiveSummary;
}

export function summarize(nb: Notebook): ArchiveSummary {
  const s: ArchiveSummary = {
    sections: nb.sections.length,
    topics: nb.topics.length,
    entries: nb.entries.length,
    isTemplate: nb.config.template !== undefined,
  };
  const name = nb.config.name ?? nb.config.template?.title;
  if (name !== undefined) s.name = name;
  return s;
}

/**
 * Reads and fully validates an archive. Throws {@link FormatError} for
 * archive-level problems and {@link NotebookErrors} for content problems.
 * Nothing is written anywhere; the caller decides what to do with the result.
 */
export function readArchive(data: Uint8Array): ImportResult {
  if (data.length > MAX_ARCHIVE_FILE_BYTES) {
    throw new FormatError('archive-too-large', { max: MAX_ARCHIVE_FILE_BYTES });
  }
  const entries = listZip(data);
  const errors: FormatError[] = [];
  const wanted = entries.filter((e) => {
    const depth = e.isDirectory ? e.name.split('/').length : e.name.split('/').length - 1;
    if (depth > MAX_DEPTH) {
      throw new FormatError('limit-exceeded', { file: e.name, limit: 'depth', max: MAX_DEPTH });
    }
    if (e.isDirectory) return false;
    try {
      return classifyPath(e.name).kind !== 'ignored';
    } catch (err) {
      if (err instanceof FormatError) errors.push(err.inFile(e.name));
      else throw err;
      return false;
    }
  });
  if (errors.length > 0) throw new NotebookErrors(errors);
  const files = wanted.map((e) => ({ path: e.name, bytes: extractEntry(data, e) }));
  const result = loadNotebook(files);
  return { ...result, ignored: result.ignored + entries.length - wanted.length, summary: summarize(result.notebook) };
}

/** Builds an archive the CLI can read. Validates the result before returning it. */
export function writeArchive(nb: Notebook, now: Date = new Date()): Uint8Array {
  const files = notebookFiles(nb);
  // Fail closed: never hand the user an archive we would refuse to import.
  loadNotebook(files);
  return writeZip(files, now);
}
