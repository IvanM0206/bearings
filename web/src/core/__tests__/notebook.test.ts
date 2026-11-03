import { describe, expect, it } from 'vitest';
import { NotebookErrors } from '../errors';
import { isEntryId, isSlug, newEntryId, slugify } from '../ids';
import { MAX_ENTRIES, MAX_SECTIONS, MAX_TOPICS_PER_SECTION } from '../limits';
import { classifyPath, entryPath, loadNotebook, notebookFiles, type FileInput } from '../notebook';
import { compareEntries, rolePath } from '../ordering';
import { addDays, isDate, isTimestamp, nowTimestamp } from '../time';
import { enc, ENTRY_ID, entryFile, minimalNotebook, sectionFile, topicFile, TS } from './helpers';

function errors(files: FileInput[]): NotebookErrors {
  try {
    loadNotebook(files);
  } catch (e) {
    if (e instanceof NotebookErrors) return e;
    throw e;
  }
  throw new Error('expected NotebookErrors');
}

const config = (): FileInput => ({ path: 'bearings.yaml', bytes: enc('format: 1\n') });

describe('identifiers (§3)', () => {
  it('validates slugs', () => {
    for (const ok of ['a', 'a1', 'technical', 'a-b', '0', 'a'.repeat(64)]) expect(isSlug(ok), ok).toBe(true);
    for (const bad of ['', '-a', 'a-', 'A', 'a_b', 'a.b', 'á', 'a'.repeat(65), 'a/b', 'con', 'nul', 'com1', 'lpt9', 'aux', 'prn']) {
      expect(isSlug(bad), bad).toBe(false);
    }
    expect(isSlug('com10')).toBe(true);
    expect(isSlug('com0')).toBe(true);
  });
  it('derives slugs from titles', () => {
    expect(slugify('Gestión de riesgo', new Set(), 'x')).toBe('gestion-de-riesgo');
    expect(slugify('  Año Ñandú!! ', new Set(), 'x')).toBe('ano-nandu');
    expect(slugify('Team', new Set(['team']), 'x')).toBe('team-2');
    expect(slugify('Team', new Set(['team', 'team-2']), 'x')).toBe('team-3');
    expect(slugify('日本語', new Set(), 'section')).toBe('section');
    expect(slugify('CON', new Set(), 'x')).toBe('con-2');
    const long = slugify('a'.repeat(100), new Set(['a'.repeat(64)]), 'x');
    expect(long).toBe(`${'a'.repeat(62)}-2`);
    expect(isSlug(slugify(`${'a'.repeat(63)}-b`, new Set(), 'x'))).toBe(true);
    // Never changes for a fixed set of inputs.
    expect(slugify('a-'.repeat(40), new Set(), 'x')).toMatch(/^[a-z0-9-]+$/);
  });
  it('generates entry ids from 50 random bits', () => {
    for (let i = 0; i < 200; i++) expect(isEntryId(newEntryId())).toBe(true);
    expect(newEntryId((b) => b.fill(255))).toBe('zzzzzzzzzz');
    expect(newEntryId((b) => b.fill(0))).toBe('0000000000');
    for (const bad of ['7k2mq9xr4', '7k2mq9xr4tt', '7K2MQ9XR4T', 'iiiiiiiiii', 'llllllllll', 'oooooooooo', 'uuuuuuuuuu']) {
      expect(isEntryId(bad), bad).toBe(false);
    }
  });
});

describe('dates and timestamps', () => {
  it('validates dates', () => {
    expect(isDate('2024-02-29')).toBe(true);
    expect(isDate('2023-02-29')).toBe(false);
    expect(isDate('1900-02-29')).toBe(false);
    expect(isDate('2000-02-29')).toBe(true);
    expect(isDate('1970-01-01')).toBe(true);
    expect(isDate('9999-12-31')).toBe(true);
    expect(isDate('1969-12-31')).toBe(false);
    expect(isDate('2026-13-01')).toBe(false);
    expect(isDate('2026-00-10')).toBe(false);
    expect(isDate('2026-1-01')).toBe(false);
  });
  it('validates and produces timestamps', () => {
    expect(isTimestamp(TS)).toBe(true);
    expect(isTimestamp(nowTimestamp(new Date('2026-11-03T14:05:09.987Z')))).toBe(true);
    expect(nowTimestamp(new Date('2026-11-03T14:05:09.987Z'))).toBe('2026-11-03T14:05:09Z');
    expect(addDays('2026-03-01', -1)).toBe('2026-02-28');
  });
});

describe('paths (§2)', () => {
  it('classifies the four patterns', () => {
    expect(classifyPath('bearings.yaml')).toEqual({ kind: 'config' });
    expect(classifyPath('sections/team/_section.md')).toEqual({ kind: 'section', section: 'team' });
    expect(classifyPath('sections/team/who.md')).toEqual({ kind: 'topic', section: 'team', topic: 'who' });
    expect(classifyPath(`journal/2026/11/2026-11-03-${ENTRY_ID}.md`)).toMatchObject({ kind: 'entry', id: ENTRY_ID });
  });
  it('ignores everything else', () => {
    for (const p of [
      'README.md',
      '.git/config',
      '.DS_Store',
      'sections/team/.who.md.swp',
      'sections/Team/_section.md',
      'sections/team/Who.md',
      'sections/team/notes.txt',
      'sections/team/sub/x.md',
      'sections/_section.md',
      'journal/2026/11/notes.md',
      'journal/2026/11/2026-11-03-UPPERCASE1.md',
      'journal/2026/2026-11-03-7k2mq9xr4t.md',
      'bearings.yml',
      'x/bearings.yaml',
    ]) {
      expect(classifyPath(p).kind, p).toBe('ignored');
    }
  });
  it('rejects reserved names instead of silently ignoring them', () => {
    expect(() => classifyPath('sections/con/_section.md')).toThrow();
    expect(() => classifyPath('sections/team/nul.md')).toThrow();
  });
});

describe('loadNotebook', () => {
  it('loads a valid notebook and round-trips it', () => {
    const { notebook, warnings, ignored } = loadNotebook([...minimalNotebook(), { path: 'README.md', bytes: enc('hi') }]);
    expect(notebook.sections).toHaveLength(1);
    expect(notebook.topics).toHaveLength(1);
    expect(notebook.entries).toHaveLength(1);
    expect(warnings).toEqual([]);
    expect(ignored).toBe(1);
    const again = loadNotebook(notebookFiles(notebook)).notebook;
    expect(again).toEqual(notebook);
  });
  it('requires bearings.yaml', () => {
    const e = errors(minimalNotebook().filter((f) => f.path !== 'bearings.yaml'));
    expect(e.errors[0]?.code).toBe('missing-config');
  });
  it('reports a topic in a section with no _section.md', () => {
    const e = errors([config(), { path: 'sections/ghost/a.md', bytes: enc(topicFile()) }]);
    expect(e.errors[0]?.code).toBe('missing-section-file');
    expect(e.errors[0]?.details.file).toBe('sections/ghost/a.md');
  });
  it('checks journal directories against the date', () => {
    const e = errors([config(), { path: `journal/2026/10/2026-11-03-${ENTRY_ID}.md`, bytes: enc(entryFile()) }]);
    expect(e.errors[0]?.code).toBe('path-date-mismatch');
    const e2 = errors([config(), { path: `journal/2025/11/2026-11-03-${ENTRY_ID}.md`, bytes: enc(entryFile()) }]);
    expect(e2.errors[0]?.code).toBe('path-date-mismatch');
  });
  it('rejects duplicate entry ids and duplicate paths', () => {
    const e = errors([
      config(),
      { path: `journal/2026/11/2026-11-03-${ENTRY_ID}.md`, bytes: enc(entryFile()) },
      { path: `journal/2026/11/2026-11-04-${ENTRY_ID}.md`, bytes: enc(entryFile({ date: '2026-11-04' })) },
    ]);
    expect(e.errors[0]?.code).toBe('duplicate-id');
    expect(errors([config(), config()]).errors[0]?.code).toBe('duplicate-id');
  });
  it('treats dangling links as unlinked, with a warning', () => {
    const { warnings } = loadNotebook([
      config(),
      { path: `journal/2026/11/2026-11-03-${ENTRY_ID}.md`, bytes: enc(entryFile({ section: 'gone' })) },
      { path: 'sections/technical/_section.md', bytes: enc(sectionFile()) },
      { path: 'journal/2026/11/2026-11-03-0000000000.md', bytes: enc(entryFile({ section: 'technical', topic: 'technical/gone' })) },
    ]);
    expect(warnings.map((w) => w.code).sort()).toEqual(['dangling-section', 'dangling-topic']);
  });
  it('reports several errors with file names, not contents', () => {
    const e = errors([
      config(),
      { path: 'sections/a/_section.md', bytes: enc(sectionFile({ title: '"SECRET-CONTENT\\u0001"' })) },
      { path: 'sections/b/_section.md', bytes: enc('no frontmatter') },
    ]);
    expect(e.errors.map((x) => x.details.file)).toEqual(['sections/a/_section.md', 'sections/b/_section.md']);
    expect(e.message).not.toContain('SECRET');
  });
  it('enforces the per-file size limit on every recognized file', () => {
    const e = errors([config(), { path: 'sections/a/_section.md', bytes: new Uint8Array(1024 * 1024 + 1) }]);
    expect(e.errors[0]?.code).toBe('file-too-large');
  });
  it('enforces the directory depth limit (4)', () => {
    expect(loadNotebook([config(), { path: 'a/b/c/d/x', bytes: enc('') }]).ignored).toBe(1);
    const e = errors([config(), { path: 'a/b/c/d/e/x', bytes: enc('') }]);
    expect(e.errors[0]?.details.limit).toBe('depth');
  });
});

describe('notebook limits (§7)', () => {
  it(`allows ${String(MAX_SECTIONS)} sections and rejects one more`, () => {
    const files = (n: number): FileInput[] => [
      config(),
      ...Array.from({ length: n }, (_, i) => ({ path: `sections/s${String(i)}/_section.md`, bytes: enc(sectionFile()) })),
    ];
    expect(loadNotebook(files(MAX_SECTIONS)).notebook.sections).toHaveLength(MAX_SECTIONS);
    const e = errors(files(MAX_SECTIONS + 1));
    expect(e.errors[0]?.code).toBe('limit-exceeded');
    expect(e.errors[0]?.details.limit).toBe('sections');
  });
  it(`allows ${String(MAX_TOPICS_PER_SECTION)} topics per section and rejects one more`, () => {
    const files = (n: number): FileInput[] => [
      config(),
      { path: 'sections/s/_section.md', bytes: enc(sectionFile()) },
      ...Array.from({ length: n }, (_, i) => ({ path: `sections/s/t${String(i)}.md`, bytes: enc(topicFile()) })),
    ];
    expect(loadNotebook(files(MAX_TOPICS_PER_SECTION)).notebook.topics).toHaveLength(MAX_TOPICS_PER_SECTION);
    const e = errors(files(MAX_TOPICS_PER_SECTION + 1));
    expect(e.errors[0]?.details.limit).toBe('topics-per-section');
  });
  it(`rejects more than ${String(MAX_ENTRIES)} journal entries`, () => {
    const body = enc(entryFile());
    function* files(n: number): Generator<FileInput> {
      yield config();
      for (let i = 0; i < n; i++) {
        let id = '';
        for (let n = i, k = 0; k < 10; k++, n = Math.floor(n / 32)) id = '0123456789abcdefghjkmnpqrstvwxyz'.charAt(n % 32) + id;
        yield { path: `journal/2026/11/2026-11-03-${id}.md`, bytes: body };
      }
    }
    const e = errors([...files(MAX_ENTRIES + 1)]);
    expect(e.errors.some((x) => x.details.limit === 'entries')).toBe(true);
  }, 60_000);
});

describe('ordering (§6)', () => {
  it('orders the journal by date desc, created desc, then id', () => {
    const mk = (id: string, date: string, created: string) => ({ id, date, created, updated: created, body: 'x' });
    const list = [
      mk('b000000000', '2026-11-02', '2026-11-02T10:00:00Z'),
      mk('a000000000', '2026-11-03', '2026-11-03T09:00:00Z'),
      mk('c000000000', '2026-11-03', '2026-11-03T11:00:00Z'),
      mk('d000000000', '2026-11-03', '2026-11-03T11:00:00Z'),
    ];
    expect([...list].sort(compareEntries).map((e) => e.id)).toEqual(['c000000000', 'd000000000', 'a000000000', 'b000000000']);
  });
  it('numbers role path steps across sections', () => {
    const s = (slug: string, order: number) => ({ slug, title: slug, order, created: TS, updated: TS, description: '' });
    const t = (section: string, slug: string, order: number, type: 'role' | 'personal' = 'role') => ({
      section,
      slug,
      title: slug,
      order,
      type,
      status: 'pending' as const,
      suggested: false,
      resources: [],
      created: TS,
      updated: TS,
      notes: '',
    });
    const path = rolePath(
      [s('b', 2), s('a', 1), s('c', 3)],
      [t('b', 'y', 1), t('a', 'z', 2), t('a', 'x', 2), t('a', 'p', 1, 'personal'), t('b', 'w', 0)],
    );
    expect(path.map((p) => p.section.slug)).toEqual(['a', 'b']);
    expect(path.flatMap((p) => p.steps.map((st) => `${String(st.number)}:${st.topic.slug}`))).toEqual(['1:x', '2:z', '3:w', '4:y']);
  });
});

describe('entryPath', () => {
  it('builds the journal path', () => {
    expect(entryPath({ id: ENTRY_ID, date: '2026-01-09' })).toBe(`journal/2026/01/2026-01-09-${ENTRY_ID}.md`);
  });
});
