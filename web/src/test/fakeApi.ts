import { vi } from 'vitest';
import type { Entry, Info, OutlineItem, Section, Topic, TopicSummary } from '../api/types';

export interface Call {
  method: string;
  path: string;
  query: URLSearchParams;
  body: unknown;
}

export interface Reply {
  status?: number;
  body?: unknown;
}

type Override = (call: Call) => Reply | undefined;

const ts = '2026-11-03T09:41:00Z';

function counts(topics: Array<{ status: string }>) {
  return {
    pending: topics.filter((t) => t.status === 'pending').length,
    in_progress: topics.filter((t) => t.status === 'in-progress').length,
    mastered: topics.filter((t) => t.status === 'mastered').length,
    total: topics.length,
  };
}

/**
 * A small in-memory stand-in for the local API (cli/internal/server). It is
 * stateful for the writes the tests check, and returns the same JSON shapes.
 */
export class FakeApi {
  info: Info = { name: 'Example role', language: 'en', format: 1, read_only: false };
  sections: Section[] = [
    { slug: 'technical', title: 'Technical', description: 'How things **work**.', order: 0, created: ts, updated: ts, version: '1-aa' },
    { slug: 'team', title: 'Team', description: '', order: 1, created: ts, updated: ts, version: '1-bb' },
  ];
  topics: Topic[] = [
    this.topic('technical', 'risk-basics', 'Risk basics', 'in-progress', 'role', 0, 'Notes about **risk**.'),
    this.topic('technical', 'backtesting', 'Backtesting', 'pending', 'role', 1, '', true),
    this.topic('team', 'onboarding', 'Team onboarding', 'mastered', 'personal', 0),
  ];
  entries: Entry[] = [
    this.entry('aaaaaaaaaa', '2026-11-03', 'First note about risk', 'technical', 'technical/risk-basics'),
    this.entry('bbbbbbbbbb', '2026-11-03', 'Second note', '', ''),
    this.entry('cccccccccc', '2026-11-01', 'An older note', 'team', ''),
  ];
  calls: Call[] = [];
  private overrides: Override[] = [];
  private seq = 0;

  topic(section: string, slug: string, title: string, status: Topic['status'], type: Topic['type'], order: number, notes = '', suggested = false): Topic {
    return {
      ref: `${section}/${slug}`, section, section_title: section === 'team' ? 'Team' : 'Technical', slug, title, status, type,
      suggested, order, resources: [], created: ts, updated: ts, notes, version: `1-${slug}`,
    };
  }

  entry(id: string, date: string, body: string, section: string, topic: string): Entry {
    return { id, date, section, topic, created: ts, updated: ts, body, version: `1-${id}` };
  }

  /** Answers matching calls with a fixed reply, before the normal handlers. */
  on(fn: Override): void {
    this.overrides.unshift(fn);
  }

  install(): void {
    vi.stubGlobal('fetch', vi.fn((url: string, init: RequestInit = {}) => Promise.resolve(this.handle(url, init))));
  }

  callsTo(method: string, pathPrefix: string): Call[] {
    return this.calls.filter((c) => c.method === method && c.path.startsWith(pathPrefix));
  }

  private summary(t: Topic): TopicSummary {
    return { ref: t.ref, section: t.section, slug: t.slug, title: t.title, status: t.status, type: t.type, suggested: t.suggested, order: t.order, resources: t.resources.length, updated: t.updated };
  }

  private outline(): OutlineItem[] {
    return this.sections.map((s) => {
      const topics = this.topics.filter((t) => t.section === s.slug);
      return { section: { ...s, counts: counts(topics) }, topics: topics.map((t) => this.summary(t)) };
    });
  }

  private handle(url: string, init: RequestInit): Response {
    const u = new URL(url, 'http://127.0.0.1');
    const method = init.method ?? 'GET';
    const body: unknown = typeof init.body === 'string' ? JSON.parse(init.body) : undefined;
    const call: Call = { method, path: u.pathname.replace('/api/v1', ''), query: u.searchParams, body };
    this.calls.push(call);
    for (const o of this.overrides) {
      const r = o(call);
      if (r) return json(r.status ?? 200, r.body);
    }
    const r = this.route(call);
    return json(r.status ?? 200, r.body);
  }

  private route(c: Call): Reply {
    const { method, path } = c;
    const b = c.body as Record<string, unknown> | undefined;
    let m: RegExpExecArray | null;
    if (method === 'GET') {
      if (path === '/info') return { body: this.info };
      if (path === '/outline') return { body: { outline: this.outline() } };
      if (path === '/sections') return { body: { sections: this.outline().map((o) => o.section) } };
      if (path === '/progress') return { body: this.progress() };
      if (path === '/role-path') {
        const steps = this.topics.filter((t) => t.type === 'role').map((t, i) => ({ step: i + 1, section_title: t.section_title, topic: t }));
        return { body: { steps } };
      }
      if (path === '/journal') {
        let es = [...this.entries];
        const section = c.query.get('section');
        const topic = c.query.get('topic');
        if (section) es = es.filter((e) => e.section === section);
        if (topic) es = es.filter((e) => e.topic === topic);
        es.sort((a, z) => z.date.localeCompare(a.date));
        return { body: { entries: es, has_more: false } };
      }
      if ((m = /^\/journal\/(\w+)$/.exec(path))) {
        const e = this.entries.find((x) => x.id === m?.[1]);
        return e ? { body: e } : notFound();
      }
      if ((m = /^\/topics\/([^/]+)\/([^/]+)$/.exec(path))) {
        const t = this.topics.find((x) => x.ref === `${m?.[1] ?? ''}/${m?.[2] ?? ''}`);
        return t ? { body: t } : notFound();
      }
      if (path === '/search') return { body: { hits: this.search(c.query.get('q') ?? '') } };
    }
    if (method === 'POST' && path === '/journal') {
      const e = this.entry(`new${++this.seq}`.padEnd(10, 'x'), '2026-11-05', str(b, 'body'), str(b, 'section'), str(b, 'topic'));
      this.entries.unshift(e);
      return { status: 201, body: e };
    }
    if (method === 'PATCH' && (m = /^\/journal\/(\w+)$/.exec(path))) {
      const e = this.entries.find((x) => x.id === m?.[1]);
      if (!e) return notFound();
      if (typeof b?.['body'] === 'string') e.body = b['body'];
      e.version = `2-${e.id}`;
      return { body: e };
    }
    if (method === 'DELETE' && (m = /^\/journal\/(\w+)$/.exec(path))) {
      this.entries = this.entries.filter((x) => x.id !== m?.[1]);
      return { status: 204 };
    }
    if (method === 'POST' && path === '/sections') {
      const s: Section = { slug: str(b, 'title').toLowerCase().replace(/\W+/g, '-'), title: str(b, 'title'), description: str(b, 'description'), order: this.sections.length, created: ts, updated: ts, version: '1-new' };
      this.sections.push(s);
      return { status: 201, body: s };
    }
    if (method === 'DELETE' && (m = /^\/sections\/([^/]+)$/.exec(path))) {
      this.sections = this.sections.filter((s) => s.slug !== m?.[1]);
      this.topics = this.topics.filter((t) => t.section !== m?.[1]);
      return { status: 204 };
    }
    if (method === 'POST' && (m = /^\/sections\/([^/]+)\/reorder$/.exec(path))) return { body: { moved: true } };
    if (method === 'POST' && (m = /^\/sections\/([^/]+)\/topics$/.exec(path))) {
      const t = this.topic(m[1] ?? '', str(b, 'title').toLowerCase().replace(/\W+/g, '-'), str(b, 'title'), 'pending', (b?.['type'] as Topic['type']) ?? 'role', 9);
      this.topics.push(t);
      return { status: 201, body: t };
    }
    if ((m = /^\/topics\/([^/]+)\/([^/]+)\/(status|advance)$/.exec(path)) && method === 'POST') {
      const t = this.topics.find((x) => x.ref === `${m?.[1] ?? ''}/${m?.[2] ?? ''}`);
      if (!t) return notFound();
      if (m[3] === 'advance') t.status = t.status === 'pending' ? 'in-progress' : 'mastered';
      else t.status = b?.['status'] as Topic['status'];
      t.version = `${t.version}+`;
      return { body: t };
    }
    if (method === 'PATCH' && (m = /^\/topics\/([^/]+)\/([^/]+)$/.exec(path))) {
      const t = this.topics.find((x) => x.ref === `${m?.[1] ?? ''}/${m?.[2] ?? ''}`);
      if (!t) return notFound();
      Object.assign(t, { ...b, version: `${t.version}+` });
      return { body: t };
    }
    if (method === 'DELETE' && (m = /^\/topics\/([^/]+)\/([^/]+)$/.exec(path))) {
      this.topics = this.topics.filter((x) => x.ref !== `${m?.[1] ?? ''}/${m?.[2] ?? ''}`);
      return { status: 204 };
    }
    return notFound();
  }

  private progress() {
    const role = this.topics.filter((t) => t.type === 'role');
    const personal = this.topics.filter((t) => t.type === 'personal');
    return {
      all: counts(this.topics), role: counts(role), personal: counts(personal),
      suggested: this.topics.filter((t) => t.suggested).length, journal_entries: this.entries.length,
      sections: this.sections.map((s) => ({ slug: s.slug, title: s.title, all: counts(this.topics.filter((t) => t.section === s.slug)), role: counts(role.filter((t) => t.section === s.slug)) })),
    };
  }

  private search(q: string) {
    const needle = q.toLowerCase();
    const hits = [];
    for (const t of this.topics) {
      if (t.title.toLowerCase().includes(needle)) hits.push({ kind: 'topic', section: t.section, topic: t.ref, entry_id: '', date: '', title: t.title, field: 'title', snippet: t.title });
      else if (t.notes.toLowerCase().includes(needle)) hits.push({ kind: 'topic', section: t.section, topic: t.ref, entry_id: '', date: '', title: t.title, field: 'body', snippet: t.notes });
    }
    for (const e of this.entries) {
      if (e.body.toLowerCase().includes(needle)) hits.push({ kind: 'entry', section: e.section, topic: e.topic, entry_id: e.id, date: e.date, title: '', field: 'body', snippet: e.body });
    }
    return hits;
  }
}

function str(b: Record<string, unknown> | undefined, key: string): string {
  const v = b?.[key];
  return typeof v === 'string' ? v : '';
}

function notFound(): Reply {
  return { status: 404, body: { error: { code: 'not_found', message: 'Not found.' } } };
}

function json(status: number, body: unknown): Response {
  return new Response(status === 204 || body === undefined ? null : JSON.stringify(body), {
    status,
    headers: { 'Content-Type': 'application/json' },
  });
}

export function apiError(status: number, code: string, field?: string): Reply {
  return { status, body: { error: { code, message: 'x', ...(field ? { field } : {}) } } };
}
