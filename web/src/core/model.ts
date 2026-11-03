/** Domain model mirroring format/SPEC.md §1 and §5. */

export type Language = 'en' | 'es';
export const LANGUAGES: readonly Language[] = ['en', 'es'];

export type TopicStatus = 'pending' | 'in-progress' | 'mastered';
export const STATUSES: readonly TopicStatus[] = ['pending', 'in-progress', 'mastered'];

export type TopicType = 'role' | 'personal';
export const TOPIC_TYPES: readonly TopicType[] = ['role', 'personal'];

export interface TemplateInfo {
  id: string;
  title: string;
  description?: string;
}

/** `bearings.yaml` (§5.1). */
export interface NotebookConfig {
  format: 1;
  name?: string;
  language?: Language;
  template?: TemplateInfo;
}

/** `sections/<slug>/_section.md` (§5.2). */
export interface Section {
  slug: string;
  title: string;
  order: number;
  created: string;
  updated: string;
  /** Markdown body, ≤ 2 000 chars. */
  description: string;
}

export interface Resource {
  url: string;
  title?: string;
}

/** `sections/<section>/<slug>.md` (§5.3). */
export interface Topic {
  section: string;
  slug: string;
  title: string;
  status: TopicStatus;
  type: TopicType;
  suggested: boolean;
  order: number;
  resources: Resource[];
  created: string;
  updated: string;
  /** Markdown body, ≤ 256 KiB. */
  notes: string;
}

/** `journal/YYYY/MM/YYYY-MM-DD-<id>.md` (§5.4). */
export interface Entry {
  id: string;
  date: string;
  section?: string;
  /** Topic reference `<section>/<topic>`. */
  topic?: string;
  created: string;
  updated: string;
  body: string;
}

export interface Notebook {
  config: NotebookConfig;
  sections: Section[];
  topics: Topic[];
  entries: Entry[];
}

export interface Warning {
  code: 'dangling-section' | 'dangling-topic' | 'ignored-path';
  file: string;
}
