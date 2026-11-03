/** Derived views over a notebook: progress, search, role path export, templates. */
import { topicRef } from './ids';
import type { Entry, Notebook, NotebookConfig, Section, Topic, TopicStatus } from './model';
import { rolePath } from './ordering';
import { foldForSearch } from './text';
import { nowTimestamp } from './time';

export interface Progress {
  total: number;
  mastered: number;
  inProgress: number;
  pending: number;
  /** 0–100, rounded down. */
  percent: number;
}

export function progress(topics: readonly Topic[]): Progress {
  const p = { total: topics.length, mastered: 0, inProgress: 0, pending: 0, percent: 0 };
  for (const t of topics) {
    if (t.status === 'mastered') p.mastered++;
    else if (t.status === 'in-progress') p.inProgress++;
    else p.pending++;
  }
  p.percent = p.total === 0 ? 0 : Math.floor((p.mastered * 100) / p.total);
  return p;
}

export function nextStatus(s: TopicStatus): TopicStatus {
  return s === 'pending' ? 'in-progress' : s === 'in-progress' ? 'mastered' : 'pending';
}

// ---------------------------------------------------------------------------
// Search

export type SearchHit =
  | { kind: 'section'; section: Section; field: 'title' | 'description'; snippet: Snippet }
  | { kind: 'topic'; topic: Topic; field: 'title' | 'notes' | 'resource'; snippet: Snippet }
  | { kind: 'entry'; entry: Entry; snippet: Snippet };

/** Text split around the match so the UI can highlight it without HTML. */
export interface Snippet {
  before: string;
  match: string;
  after: string;
}

const SNIPPET_CONTEXT = 40;

/**
 * Case- and accent-insensitive substring match. Returns the snippet around the
 * first match, or null. Folding works per code point so offsets map back to
 * the original text.
 */
export function findMatch(text: string, foldedQuery: string): Snippet | null {
  if (foldedQuery === '' || !foldForSearch(text).includes(foldedQuery)) return null;
  const chars = [...text];
  let folded = '';
  const owner: number[] = [];
  chars.forEach((ch, i) => {
    const f = foldForSearch(ch);
    folded += f;
    for (let k = 0; k < f.length; k++) owner.push(i);
  });
  const idx = folded.indexOf(foldedQuery);
  if (idx === -1) return null;
  const startChar = owner[idx] ?? 0;
  const endChar = (owner[idx + foldedQuery.length - 1] ?? startChar) + 1;
  const from = Math.max(0, startChar - SNIPPET_CONTEXT);
  const to = Math.min(chars.length, endChar + SNIPPET_CONTEXT);
  const oneLine = (x: string): string => x.replace(/\s+/g, ' ');
  return {
    before: (from > 0 ? '…' : '') + oneLine(chars.slice(from, startChar).join('')),
    match: oneLine(chars.slice(startChar, endChar).join('')),
    after: oneLine(chars.slice(endChar, to).join('')) + (to < chars.length ? '…' : ''),
  };
}

export const MAX_SEARCH_RESULTS = 200;

export function search(nb: Pick<Notebook, 'sections' | 'topics' | 'entries'>, query: string): SearchHit[] {
  const q = foldForSearch(query.trim());
  if (q === '') return [];
  const hits: SearchHit[] = [];
  const add = (h: SearchHit): boolean => {
    hits.push(h);
    return hits.length >= MAX_SEARCH_RESULTS;
  };
  for (const section of nb.sections) {
    const t = findMatch(section.title, q);
    if (t && add({ kind: 'section', section, field: 'title', snippet: t })) return hits;
    if (t) continue;
    const d = findMatch(section.description, q);
    if (d && add({ kind: 'section', section, field: 'description', snippet: d })) return hits;
  }
  for (const topic of nb.topics) {
    let hit: SearchHit | null = null;
    const t = findMatch(topic.title, q);
    if (t) hit = { kind: 'topic', topic, field: 'title', snippet: t };
    if (!hit) {
      const n = findMatch(topic.notes, q);
      if (n) hit = { kind: 'topic', topic, field: 'notes', snippet: n };
    }
    if (!hit) {
      for (const r of topic.resources) {
        const m = findMatch(r.title ?? '', q) ?? findMatch(r.url, q);
        if (m) {
          hit = { kind: 'topic', topic, field: 'resource', snippet: m };
          break;
        }
      }
    }
    if (hit && add(hit)) return hits;
  }
  for (const entry of nb.entries) {
    const m = findMatch(entry.body, q);
    if (m && add({ kind: 'entry', entry, snippet: m })) return hits;
  }
  return hits;
}

// ---------------------------------------------------------------------------
// Role path as Markdown

export interface RolePathLabels {
  heading: string;
  intro: string;
  status: Record<TopicStatus, string>;
  resources: string;
  empty: string;
}

/** Escapes text so CommonMark renders it literally (no HTML, no markup). */
export function escapeMarkdown(text: string): string {
  // Inline constructs only; every escaped line starts with our own prefix.
  return text.replace(/[\\`*_{}[\]()<>#!|~&]/g, (c) => `\\${c}`);
}

function markdownUrl(url: string): string {
  return url.replace(/[()<>\s]/g, (c) => `%${c.charCodeAt(0).toString(16).toUpperCase().padStart(2, '0')}`);
}

/** The role path (§6) as a Markdown guide for whoever takes the role next. */
export function rolePathMarkdown(nb: Pick<Notebook, 'sections' | 'topics'>, title: string, labels: RolePathLabels): string {
  const path = rolePath(nb.sections, nb.topics);
  const lines: string[] = [`# ${escapeMarkdown(labels.heading)}: ${escapeMarkdown(title)}`, '', escapeMarkdown(labels.intro), ''];
  if (path.length === 0) lines.push(escapeMarkdown(labels.empty), '');
  for (const { section, steps } of path) {
    lines.push(`## ${escapeMarkdown(section.title)}`, '');
    for (const { number, topic } of steps) {
      const mark = topic.status === 'mastered' ? ' ✓' : '';
      lines.push(`### ${String(number)}. ${escapeMarkdown(topic.title)}${mark}`, '');
      lines.push(`*${escapeMarkdown(labels.status[topic.status])}*`, '');
      if (topic.resources.length > 0) {
        lines.push(`${escapeMarkdown(labels.resources)}:`, '');
        for (const r of topic.resources) {
          lines.push(`- [${escapeMarkdown(r.title ?? r.url)}](${markdownUrl(r.url)})`);
        }
        lines.push('');
      }
      if (topic.notes.trim() !== '') lines.push(topic.notes.trim(), '');
    }
  }
  return lines.join('\n').replace(/\n+$/, '') + '\n';
}

// ---------------------------------------------------------------------------
// Templates

/**
 * A new notebook started from a template: same sections and topics, fresh
 * timestamps, no journal, and no `template` key (it is a notebook now).
 */
export function notebookFromTemplate(template: Notebook, now: Date = new Date()): Notebook {
  const ts = nowTimestamp(now);
  const config: NotebookConfig = { format: 1 };
  const name = template.config.name ?? template.config.template?.title;
  if (name !== undefined) config.name = name;
  if (template.config.language !== undefined) config.language = template.config.language;
  return {
    config,
    sections: template.sections.map((s) => ({ ...s, created: ts, updated: ts })),
    topics: template.topics.map((t) => ({ ...t, resources: t.resources.map((r) => ({ ...r })), created: ts, updated: ts })),
    entries: [],
  };
}

/** Entries linked to a section, or to one topic of it. */
export function linkedEntries(entries: readonly Entry[], section: string, topic?: string): Entry[] {
  const ref = topic === undefined ? undefined : topicRef(section, topic);
  return entries.filter((e) => (ref === undefined ? e.section === section : e.topic === ref));
}
