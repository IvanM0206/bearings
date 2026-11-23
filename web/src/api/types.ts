// Wire types of the local JSON API (cli/internal/server/dto.go). Every string
// is notebook content and therefore untrusted: render it as text only.

export type Status = 'pending' | 'in-progress' | 'mastered';
export type TopicType = 'role' | 'personal';

export const STATUSES: readonly Status[] = ['pending', 'in-progress', 'mastered'];

export interface Info {
  name: string;
  language: string;
  format: number;
  read_only: boolean;
  template?: { id: string; title: string };
}

export interface Counts {
  pending: number;
  in_progress: number;
  mastered: number;
  total: number;
}

export interface Section {
  slug: string;
  title: string;
  description: string;
  order: number;
  created: string;
  updated: string;
  version: string;
  counts?: Counts;
}

export interface Resource {
  url: string;
  title: string;
}

export interface TopicSummary {
  ref: string;
  section: string;
  slug: string;
  title: string;
  status: Status;
  type: TopicType;
  suggested: boolean;
  order: number;
  resources: number;
  updated: string;
}

export interface Topic {
  ref: string;
  section: string;
  section_title: string;
  slug: string;
  title: string;
  status: Status;
  type: TopicType;
  suggested: boolean;
  order: number;
  resources: Resource[];
  created: string;
  updated: string;
  notes: string;
  version: string;
}

export interface OutlineItem {
  section: Section;
  topics: TopicSummary[];
}

export interface Entry {
  id: string;
  date: string;
  section: string;
  topic: string;
  created: string;
  updated: string;
  body: string;
  version: string;
}

export interface RoleStep {
  step: number;
  section_title: string;
  topic: Topic;
}

export interface SectionProgress {
  slug: string;
  title: string;
  all: Counts;
  role: Counts;
}

export interface Progress {
  all: Counts;
  role: Counts;
  personal: Counts;
  suggested: number;
  journal_entries: number;
  sections: SectionProgress[];
}

export type HitKind = 'section' | 'topic' | 'entry';

export interface Hit {
  kind: HitKind;
  section: string;
  topic: string;
  entry_id: string;
  date: string;
  title: string;
  field: 'title' | 'body' | 'resources';
  snippet: string;
}

export interface JournalPage {
  entries: Entry[];
  has_more: boolean;
}

export interface JournalQuery {
  from?: string;
  to?: string;
  section?: string;
  topic?: string;
  limit?: number;
  offset?: number;
}

export interface ResourceIn {
  url: string;
  title: string;
}
