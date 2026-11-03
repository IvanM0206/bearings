/**
 * All writes to the local notebook. Every write validates with the same rules
 * as the file readers (src/core), so the stored notebook can always be
 * exported as a valid archive.
 */
import { FormatError } from '../core/errors';
import { isSlug, isTopicRef, newEntryId, slugify, splitTopicRef, topicRef } from '../core/ids';
import { MAX_ENTRIES, MAX_ORDER, MAX_SECTIONS, MAX_TOPICS_PER_SECTION } from '../core/limits';
import type { Entry, Language, Notebook, NotebookConfig, Resource, Section, Topic, TopicStatus, TopicType } from '../core/model';
import { STATUSES, TOPIC_TYPES } from '../core/model';
import { loadNotebook, notebookFiles } from '../core/notebook';
import { sortEntries, sortSections, sortTopics } from '../core/ordering';
import { isDate, nowTimestamp } from '../core/time';
import {
  checkEntryBody,
  checkNotebookName,
  checkResources,
  checkSectionDescription,
  checkSectionTitle,
  checkTopicNotes,
  checkTopicTitle,
} from '../core/validate';
import type { BearingsDB } from './db';

/** `updated` never goes before `created`, even if the clock moved back. */
function touch(created: string): string {
  const now = nowTimestamp();
  return now < created ? created : now;
}

/** Text typed by the user: drop lone surrogates, normalize line endings. */
function clean(text: string): string {
  return text.toWellFormed().replace(/\r\n?/g, '\n');
}

export async function readNotebook(db: BearingsDB): Promise<Notebook | null> {
  return db.transaction('r', db.meta, db.sections, db.topics, db.entries, async () => {
    const meta = await db.meta.get('config');
    if (!meta) return null;
    const [sections, topics, entries] = await Promise.all([db.sections.toArray(), db.topics.toArray(), db.entries.toArray()]);
    return {
      config: meta.value,
      sections: sortSections(sections),
      topics: sortTopics(topics),
      entries: sortEntries(entries),
    };
  });
}

/**
 * Replaces the whole notebook (import, template, reset). The notebook is
 * validated through a full serialize-and-parse cycle first; on any error
 * nothing is changed.
 */
export async function replaceNotebook(db: BearingsDB, nb: Notebook): Promise<void> {
  const { notebook } = loadNotebook(notebookFiles(nb));
  await db.transaction('rw', db.meta, db.sections, db.topics, db.entries, async () => {
    await Promise.all([db.meta.clear(), db.sections.clear(), db.topics.clear(), db.entries.clear()]);
    await db.meta.put({ key: 'config', value: notebook.config });
    await db.sections.bulkAdd(notebook.sections);
    await db.topics.bulkAdd(notebook.topics);
    await db.entries.bulkAdd(notebook.entries);
  });
}

export async function deleteEverything(db: BearingsDB): Promise<void> {
  await db.transaction('rw', db.meta, db.sections, db.topics, db.entries, async () => {
    await Promise.all([db.meta.clear(), db.sections.clear(), db.topics.clear(), db.entries.clear()]);
  });
}

export async function updateConfig(db: BearingsDB, patch: { name?: string | null; language?: Language }): Promise<void> {
  await db.transaction('rw', db.meta, async () => {
    const row = await db.meta.get('config');
    if (!row) throw new FormatError('not-found');
    const config: NotebookConfig = { ...row.value };
    if (patch.name === null || patch.name?.trim() === '') delete config.name;
    else if (patch.name !== undefined) config.name = checkNotebookName(clean(patch.name));
    if (patch.language !== undefined) config.language = patch.language;
    await db.meta.put({ key: 'config', value: config });
  });
}

// ---------------------------------------------------------------------------
// Ordering helpers

function nextOrder(list: readonly { order: number }[]): number {
  return list.reduce((max, x) => Math.max(max, x.order + 1), 0);
}

/** Moves item `index` by `delta` and renumbers 0…n-1. Returns changed items. */
function reorder<T extends { order: number }>(sorted: T[], index: number, delta: -1 | 1): T[] {
  const target = index + delta;
  if (index < 0 || target < 0 || target >= sorted.length) return [];
  const list = [...sorted];
  const [item] = list.splice(index, 1);
  if (item === undefined) return [];
  list.splice(target, 0, item);
  const changed: T[] = [];
  list.forEach((x, i) => {
    if (x.order !== i) changed.push({ ...x, order: i });
  });
  return changed;
}

// ---------------------------------------------------------------------------
// Sections

export interface SectionInput {
  title: string;
  description: string;
}

export async function createSection(db: BearingsDB, input: SectionInput): Promise<string> {
  const title = checkSectionTitle(clean(input.title));
  const description = checkSectionDescription(clean(input.description).trim());
  return db.transaction('rw', db.sections, async () => {
    const all = await db.sections.toArray();
    if (all.length >= MAX_SECTIONS) throw new FormatError('limit-exceeded', { limit: 'sections', max: MAX_SECTIONS });
    const slug = slugify(title, new Set(all.map((s) => s.slug)), 'section');
    let order = nextOrder(all);
    if (order > MAX_ORDER) {
      const sorted = sortSections(all);
      await db.sections.bulkPut(sorted.map((s, i) => ({ ...s, order: i })));
      order = sorted.length;
    }
    const ts = nowTimestamp();
    await db.sections.add({ slug, title, order, created: ts, updated: ts, description });
    return slug;
  });
}

export async function updateSection(db: BearingsDB, slug: string, input: SectionInput): Promise<void> {
  const title = checkSectionTitle(clean(input.title));
  const description = checkSectionDescription(clean(input.description).trim());
  await db.transaction('rw', db.sections, async () => {
    const s = await db.sections.get(slug);
    if (!s) throw new FormatError('not-found');
    await db.sections.put({ ...s, title, description, updated: touch(s.created) });
  });
}

export async function moveSection(db: BearingsDB, slug: string, delta: -1 | 1): Promise<void> {
  await db.transaction('rw', db.sections, async () => {
    const sorted = sortSections(await db.sections.toArray());
    const changed = reorder(sorted, sorted.findIndex((s) => s.slug === slug), delta);
    await db.sections.bulkPut(changed.map((s) => ({ ...s, updated: touch(s.created) })));
  });
}

/** What deleting a section affects, for the confirmation message. */
export function sectionDeleteImpact(nb: Notebook, slug: string): { topics: number; entries: number } {
  return {
    topics: nb.topics.filter((t) => t.section === slug).length,
    entries: nb.entries.filter((e) => e.section === slug).length,
  };
}

/** Deletes a section and its topics; journal entries stay but lose the link (§5.4). */
export async function deleteSection(db: BearingsDB, slug: string): Promise<void> {
  await db.transaction('rw', db.sections, db.topics, db.entries, async () => {
    await db.sections.delete(slug);
    await db.topics.where('section').equals(slug).delete();
    const linked = await db.entries.where('section').equals(slug).toArray();
    await db.entries.bulkPut(linked.map((e) => unlink(e)));
  });
}

function unlink(e: Entry, keepSection = false): Entry {
  const { section, topic: _topic, ...rest } = e;
  void _topic;
  const out: Entry = { ...rest, updated: touch(e.created) };
  if (keepSection && section !== undefined) out.section = section;
  return out;
}

// ---------------------------------------------------------------------------
// Topics

export interface TopicInput {
  title: string;
  type: TopicType;
}

export async function createTopic(db: BearingsDB, section: string, input: TopicInput): Promise<string> {
  const title = checkTopicTitle(clean(input.title));
  if (!TOPIC_TYPES.includes(input.type)) throw new FormatError('invalid-value', { key: 'type' });
  return db.transaction('rw', db.sections, db.topics, async () => {
    if (!(await db.sections.get(section))) throw new FormatError('not-found');
    const siblings = await db.topics.where('section').equals(section).toArray();
    if (siblings.length >= MAX_TOPICS_PER_SECTION) {
      throw new FormatError('limit-exceeded', { limit: 'topics-per-section', max: MAX_TOPICS_PER_SECTION });
    }
    const slug = slugify(title, new Set(siblings.map((t) => t.slug)), 'topic');
    let order = nextOrder(siblings);
    if (order > MAX_ORDER) {
      const sorted = sortTopics(siblings);
      await db.topics.bulkPut(sorted.map((t, i) => ({ ...t, order: i })));
      order = sorted.length;
    }
    const ts = nowTimestamp();
    await db.topics.add({
      section,
      slug,
      title,
      status: 'pending',
      type: input.type,
      suggested: false,
      order,
      resources: [],
      created: ts,
      updated: ts,
      notes: '',
    });
    return slug;
  });
}

export interface TopicPatch {
  title?: string;
  status?: TopicStatus;
  type?: TopicType;
  suggested?: boolean;
  notes?: string;
  resources?: Resource[];
}

export async function updateTopic(db: BearingsDB, section: string, slug: string, patch: TopicPatch): Promise<void> {
  const next: Partial<Topic> = {};
  if (patch.title !== undefined) next.title = checkTopicTitle(clean(patch.title));
  if (patch.status !== undefined) {
    if (!STATUSES.includes(patch.status)) throw new FormatError('invalid-value', { key: 'status' });
    next.status = patch.status;
  }
  if (patch.type !== undefined) {
    if (!TOPIC_TYPES.includes(patch.type)) throw new FormatError('invalid-value', { key: 'type' });
    next.type = patch.type;
  }
  if (patch.suggested !== undefined) next.suggested = patch.suggested;
  if (patch.notes !== undefined) next.notes = checkTopicNotes(clean(patch.notes).replace(/\n+$/, ''));
  if (patch.resources !== undefined) {
    next.resources = checkResources(
      patch.resources.map((r) => {
        const res: Resource = { url: r.url.trim() };
        const title = r.title === undefined ? '' : clean(r.title).trim();
        if (title !== '') res.title = title;
        return res;
      }),
    );
  }
  await db.transaction('rw', db.topics, async () => {
    const t = await db.topics.get([section, slug]);
    if (!t) throw new FormatError('not-found');
    await db.topics.put({ ...t, ...next, updated: touch(t.created) });
  });
}

export async function moveTopic(db: BearingsDB, section: string, slug: string, delta: -1 | 1): Promise<void> {
  await db.transaction('rw', db.topics, async () => {
    const sorted = sortTopics(await db.topics.where('section').equals(section).toArray());
    const changed = reorder(sorted, sorted.findIndex((t) => t.slug === slug), delta);
    await db.topics.bulkPut(changed.map((t) => ({ ...t, updated: touch(t.created) })));
  });
}

/** Deletes a topic; journal entries keep their section but lose the topic link. */
export async function deleteTopic(db: BearingsDB, section: string, slug: string): Promise<void> {
  await db.transaction('rw', db.topics, db.entries, async () => {
    await db.topics.delete([section, slug]);
    const linked = await db.entries.where('topic').equals(topicRef(section, slug)).toArray();
    await db.entries.bulkPut(linked.map((e) => unlink(e, true)));
  });
}

// ---------------------------------------------------------------------------
// Journal

export interface EntryInput {
  date: string;
  body: string;
  section?: string;
  /** Topic reference `<section>/<topic>`. */
  topic?: string;
}

async function checkEntryInput(db: BearingsDB, input: EntryInput): Promise<Omit<Entry, 'id' | 'created' | 'updated'>> {
  if (!isDate(input.date)) throw new FormatError('invalid-value', { key: 'date' });
  const body = checkEntryBody(clean(input.body).trim());
  const out: Omit<Entry, 'id' | 'created' | 'updated'> = { date: input.date, body };
  if (input.section !== undefined && input.section !== '') {
    if (!isSlug(input.section) || !(await db.sections.get(input.section))) {
      throw new FormatError('invalid-value', { key: 'section' });
    }
    out.section = input.section;
  }
  if (input.topic !== undefined && input.topic !== '') {
    if (!isTopicRef(input.topic) || out.section === undefined || splitTopicRef(input.topic).section !== out.section) {
      throw new FormatError('invalid-value', { key: 'topic' });
    }
    const { section, topic } = splitTopicRef(input.topic);
    if (!(await db.topics.get([section, topic]))) throw new FormatError('invalid-value', { key: 'topic' });
    out.topic = input.topic;
  }
  return out;
}

export async function addEntry(db: BearingsDB, input: EntryInput): Promise<string> {
  return db.transaction('rw', db.entries, db.sections, db.topics, async () => {
    const fields = await checkEntryInput(db, input);
    if ((await db.entries.count()) >= MAX_ENTRIES) {
      throw new FormatError('limit-exceeded', { limit: 'entries', max: MAX_ENTRIES });
    }
    let id = newEntryId();
    while (await db.entries.get(id)) id = newEntryId();
    const ts = nowTimestamp();
    await db.entries.add({ id, ...fields, created: ts, updated: ts });
    return id;
  });
}

export async function updateEntry(db: BearingsDB, id: string, input: EntryInput): Promise<void> {
  await db.transaction('rw', db.entries, db.sections, db.topics, async () => {
    const e = await db.entries.get(id);
    if (!e) throw new FormatError('not-found');
    const fields = await checkEntryInput(db, input);
    await db.entries.put({ id, ...fields, created: e.created, updated: touch(e.created) });
  });
}

export async function deleteEntry(db: BearingsDB, id: string): Promise<void> {
  await db.entries.delete(id);
}
