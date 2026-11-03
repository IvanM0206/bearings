import MarkdownIt from 'markdown-it';
import { describe, expect, it } from 'vitest';
import { escapeMarkdown, findMatch, nextStatus, notebookFromTemplate, progress, rolePathMarkdown, search } from '../derive';
import type { Notebook, Topic } from '../model';
import { loadNotebook } from '../notebook';
import { foldForSearch } from '../text';
import { minimalNotebook, TS } from './helpers';

const topic = (over: Partial<Topic>): Topic => ({
  section: 'technical',
  slug: 'a',
  title: 'A',
  status: 'pending',
  type: 'role',
  suggested: false,
  order: 0,
  resources: [],
  created: TS,
  updated: TS,
  notes: '',
  ...over,
});

describe('progress', () => {
  it('counts statuses and rounds the percentage down', () => {
    const p = progress([topic({ status: 'mastered' }), topic({ status: 'in-progress' }), topic({}), topic({ status: 'mastered' }), topic({}), topic({})]);
    expect(p).toEqual({ total: 6, mastered: 2, inProgress: 1, pending: 3, percent: 33 });
    expect(progress([]).percent).toBe(0);
  });
  it('cycles statuses', () => {
    expect(nextStatus('pending')).toBe('in-progress');
    expect(nextStatus('in-progress')).toBe('mastered');
    expect(nextStatus('mastered')).toBe('pending');
  });
});

describe('search', () => {
  const nb = (): Notebook => {
    const n = loadNotebook(minimalNotebook()).notebook;
    n.topics[0]!.resources = [{ url: 'https://example.com/wiki/backtesting', title: 'Guía de órdenes' }];
    return n;
  };
  it('matches case- and accent-insensitively and returns snippets', () => {
    const m = findMatch('La GESTIÓN de riesgo', foldForSearch('gestion'));
    expect(m).toEqual({ before: 'La ', match: 'GESTIÓN', after: ' de riesgo' });
  });
  it('maps matches back through decomposed characters', () => {
    const m = findMatch('café au lait', foldForSearch('café au'));
    expect(m?.match).toBe('café au');
  });
  it('searches sections, topics (title, notes, resources) and journal', () => {
    const kinds = (q: string): string[] => search(nb(), q).map((h) => (h.kind === 'topic' ? `topic:${h.field}` : h.kind));
    expect(kinds('technical')).toEqual(['section']);
    expect(kinds('stack')).toEqual(['section']);
    expect(kinds('backtest')).toEqual(['topic:title']);
    expect(kinds('notes here')).toEqual(['topic:notes']);
    expect(kinds('ordenes')).toEqual(['topic:resource']);
    expect(kinds('learned')).toEqual(['entry']);
    expect(kinds('   ')).toEqual([]);
    expect(kinds('zzz-nothing')).toEqual([]);
  });
  it('truncates long snippets', () => {
    const m = findMatch(`${'x'.repeat(100)}needle${'y'.repeat(100)}`, 'needle');
    expect(m?.before.startsWith('…')).toBe(true);
    expect(m?.after.endsWith('…')).toBe(true);
  });
});

describe('role path markdown', () => {
  const labels = {
    heading: 'Role path',
    intro: 'Steps for the next person.',
    status: { pending: 'Pending', 'in-progress': 'In progress', mastered: 'Mastered' },
    resources: 'Resources',
    empty: 'Nothing yet.',
  };
  it('numbers steps and marks mastered ones', () => {
    const nb = loadNotebook(minimalNotebook()).notebook;
    nb.topics[0]!.status = 'mastered';
    const md = rolePathMarkdown(nb, 'My notebook', labels);
    expect(md).toContain('### 1. Backtesting ✓');
    expect(md).toContain('## Technical');
    expect(md.endsWith('\n')).toBe(true);
  });
  it('escapes titles so they cannot inject HTML or links', () => {
    const md = new MarkdownIt({ html: true });
    const evil = '<img src=x onerror=alert(1)> [click](javascript:alert(1)) *x*';
    const html = md.render(escapeMarkdown(evil));
    expect(html).not.toContain('<img');
    expect(html).not.toContain('<a');
    expect(html).not.toContain('<em>');
  });
  it('percent-encodes characters that would break link syntax', () => {
    const nb = loadNotebook(minimalNotebook()).notebook;
    nb.topics[0]!.resources = [{ url: 'https://example.com/a_(b)<c>' }];
    expect(rolePathMarkdown(nb, 'x', labels)).toContain('(https://example.com/a_%28b%29%3Cc%3E)');
  });
  it('skips personal topics', () => {
    const nb = loadNotebook(minimalNotebook()).notebook;
    nb.topics[0]!.type = 'personal';
    expect(rolePathMarkdown(nb, 'x', labels)).toContain('Nothing yet.');
  });
});

describe('notebookFromTemplate', () => {
  it('copies sections and topics with fresh timestamps and drops the template key', () => {
    const tpl = loadNotebook(minimalNotebook()).notebook;
    tpl.config.template = { id: 'tpl', title: 'A template' };
    delete tpl.config.name;
    const nb = notebookFromTemplate(tpl, new Date('2027-01-02T03:04:05Z'));
    expect(nb.config).toEqual({ format: 1, name: 'A template', language: 'en' });
    expect(nb.entries).toEqual([]);
    expect(nb.sections[0]?.created).toBe('2027-01-02T03:04:05Z');
    expect(nb.topics[0]?.updated).toBe('2027-01-02T03:04:05Z');
  });
});
