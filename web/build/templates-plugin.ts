/**
 * Bundles the starter notebooks from the repository's `templates/` directory
 * at build time, as the virtual module `virtual:bearings-templates`.
 *
 * Every template is read with the app's own `src/core` parser, so a template
 * that breaks the format fails the build instead of reaching users. Nothing
 * is fetched at runtime.
 */
import { lstatSync, readdirSync, readFileSync } from 'node:fs';
import { join, relative, sep } from 'node:path';
import type { Plugin } from 'vite';
import { NotebookErrors } from '../src/core/errors';
import { MAX_DEPTH } from '../src/core/limits';
import type { Notebook } from '../src/core/model';
import { classifyPath, loadNotebook, type FileInput } from '../src/core/notebook';

const VIRTUAL_ID = 'virtual:bearings-templates';
const RESOLVED_ID = '\0' + VIRTUAL_ID;

export interface BundledTemplate {
  id: string;
  title: string;
  description: string;
  language: string;
  notebook: Notebook;
}

function collect(root: string, dir: string, depth: number, out: FileInput[]): void {
  for (const name of readdirSync(dir).sort()) {
    const full = join(dir, name);
    const st = lstatSync(full);
    const rel = relative(root, full).split(sep).join('/');
    if (st.isSymbolicLink()) throw new Error(`templates: symlinks are not allowed (${rel})`);
    if (st.isDirectory()) {
      if (depth >= MAX_DEPTH) throw new Error(`templates: directory too deep (${rel})`);
      collect(root, full, depth + 1, out);
    } else if (st.isFile()) {
      if (classifyPath(rel).kind !== 'ignored') out.push({ path: rel, bytes: new Uint8Array(readFileSync(full)) });
    } else {
      throw new Error(`templates: special file not allowed (${rel})`);
    }
  }
}

export function loadTemplates(templatesDir: string): BundledTemplate[] {
  const out: BundledTemplate[] = [];
  for (const name of readdirSync(templatesDir).sort()) {
    const dir = join(templatesDir, name);
    const st = lstatSync(dir);
    if (!st.isDirectory() || st.isSymbolicLink()) continue;
    const files: FileInput[] = [];
    collect(dir, dir, 0, files);
    let notebook: Notebook;
    try {
      notebook = loadNotebook(files).notebook;
    } catch (e) {
      const detail = e instanceof NotebookErrors ? e.errors.map((x) => x.message).join('; ') : String(e);
      throw new Error(`templates/${name}: invalid notebook: ${detail}`);
    }
    const info = notebook.config.template;
    if (info === undefined) throw new Error(`templates/${name}: bearings.yaml has no template key`);
    if (info.id !== name) throw new Error(`templates/${name}: template.id must match the directory name`);
    if (notebook.entries.length > 0) throw new Error(`templates/${name}: templates must not have journal entries`);
    out.push({
      id: info.id,
      title: info.title,
      description: info.description ?? '',
      language: notebook.config.language ?? 'en',
      notebook,
    });
  }
  if (out.length === 0) throw new Error('templates: none found');
  return out;
}

export function templatesPlugin(templatesDir: string): Plugin {
  return {
    name: 'bearings-templates',
    resolveId(id) {
      return id === VIRTUAL_ID ? RESOLVED_ID : null;
    },
    load(id) {
      if (id !== RESOLVED_ID) return null;
      const templates = loadTemplates(templatesDir);
      for (const file of readdirSync(templatesDir, { recursive: true })) {
        this.addWatchFile(join(templatesDir, String(file)));
      }
      // JSON inside JSON.parse: the data can never be evaluated as code.
      return `export default JSON.parse(${JSON.stringify(JSON.stringify(templates))});\n`;
    },
  };
}
