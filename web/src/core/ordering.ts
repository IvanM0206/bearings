/** Ordering rules (§6). */
import type { Entry, Section, Topic } from './model';

const cmp = (a: string, b: string): number => (a < b ? -1 : a > b ? 1 : 0);

export function compareSections(a: Section, b: Section): number {
  return a.order - b.order || cmp(a.slug, b.slug);
}

export function compareTopics(a: Topic, b: Topic): number {
  return a.order - b.order || cmp(a.slug, b.slug);
}

/** Journal: date descending, then created descending, then id. */
export function compareEntries(a: Entry, b: Entry): number {
  return cmp(b.date, a.date) || cmp(b.created, a.created) || cmp(a.id, b.id);
}

export function sortSections(list: readonly Section[]): Section[] {
  return [...list].sort(compareSections);
}

export function sortTopics(list: readonly Topic[]): Topic[] {
  return [...list].sort(compareTopics);
}

export function sortEntries(list: readonly Entry[]): Entry[] {
  return [...list].sort(compareEntries);
}

export interface RolePathStep {
  number: number;
  topic: Topic;
}

export interface RolePathSection {
  section: Section;
  steps: RolePathStep[];
}

/**
 * Role path: sections in section order; within each, `type: role` topics in
 * topic order, numbered 1…n across the whole path. Sections with no role
 * topics are left out.
 */
export function rolePath(sections: readonly Section[], topics: readonly Topic[]): RolePathSection[] {
  const bySection = new Map<string, Topic[]>();
  for (const t of topics) {
    if (t.type !== 'role') continue;
    const list = bySection.get(t.section);
    if (list) list.push(t);
    else bySection.set(t.section, [t]);
  }
  let n = 0;
  const out: RolePathSection[] = [];
  for (const section of sortSections(sections)) {
    const list = bySection.get(section.slug);
    if (!list) continue;
    out.push({ section, steps: sortTopics(list).map((topic) => ({ number: ++n, topic })) });
  }
  return out;
}
