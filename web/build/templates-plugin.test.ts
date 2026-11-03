import { mkdirSync, mkdtempSync, rmSync, symlinkSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { afterEach, describe, expect, it } from 'vitest';
import { loadTemplates } from './templates-plugin';

const repoTemplates = fileURLToPath(new URL('../../templates', import.meta.url));

describe('bundled templates', () => {
  it('loads the repository templates (en and es) with the core parser', () => {
    const list = loadTemplates(repoTemplates);
    const langs = list.map((t) => t.language).sort();
    expect(langs).toContain('en');
    expect(langs).toContain('es');
    for (const t of list) {
      expect(t.notebook.sections.length).toBeGreaterThan(0);
      expect(t.notebook.topics.length).toBeGreaterThan(0);
      expect(t.notebook.entries).toEqual([]);
    }
  });
});

describe('template validation', () => {
  let dir = '';
  afterEach(() => {
    if (dir) rmSync(dir, { recursive: true, force: true });
  });
  const make = (): string => {
    dir = mkdtempSync(join(tmpdir(), 'bearings-tpl-'));
    const t = join(dir, 'sample');
    mkdirSync(join(t, 'sections', 'a'), { recursive: true });
    writeFileSync(join(t, 'bearings.yaml'), 'format: 1\ntemplate:\n  id: sample\n  title: "Sample"\n');
    writeFileSync(
      join(t, 'sections', 'a', '_section.md'),
      '---\ntitle: "A"\norder: 0\ncreated: 2026-11-03T00:00:00Z\nupdated: 2026-11-03T00:00:00Z\n---\n',
    );
    return t;
  };
  it('accepts a minimal template', () => {
    make();
    expect(loadTemplates(dir)).toHaveLength(1);
  });
  it('fails the build on invalid content', () => {
    const t = make();
    writeFileSync(join(t, 'sections', 'a', '_section.md'), '---\ntitle: &x "A"\n---\n');
    expect(() => loadTemplates(dir)).toThrow(/invalid notebook/);
  });
  it('fails the build on symlinks', () => {
    const t = make();
    symlinkSync('/etc/hostname', join(t, 'sections', 'a', 'link.md'));
    expect(() => loadTemplates(dir)).toThrow(/symlinks/);
  });
  it('requires the template key and no journal', () => {
    const t = make();
    writeFileSync(join(t, 'bearings.yaml'), 'format: 1\n');
    expect(() => loadTemplates(dir)).toThrow(/no template key/);
  });
});
