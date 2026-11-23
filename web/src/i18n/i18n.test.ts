import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '../api/client';
import { en } from './en';
import { es } from './es';
import { CATALOGS, describeError, detectLanguage, interpolate, languageOf, makeI18n } from './index';

const placeholders = (s: string): string[] => [...s.matchAll(/\{(\w+)\}/g)].map((m) => m[1] ?? '').sort();

describe('message catalogs', () => {
  it('have exactly the same keys', () => {
    expect(Object.keys(es).sort()).toEqual(Object.keys(en).sort());
  });

  it('use the same placeholders in every message', () => {
    for (const key of Object.keys(en) as Array<keyof typeof en>) {
      expect(placeholders(es[key]), key).toEqual(placeholders(en[key]));
    }
  });

  it('have no empty messages', () => {
    for (const [lang, catalog] of Object.entries(CATALOGS)) {
      for (const [key, value] of Object.entries(catalog)) {
        expect(value.trim(), `${lang}:${key}`).not.toBe('');
      }
    }
  });

  it('give every plural key a singular form in both languages, with {count}', () => {
    for (const catalog of [en, es] as Array<Record<string, string>>) {
      for (const key of Object.keys(catalog).filter((k) => k.endsWith('_one'))) {
        const base = key.slice(0, -4);
        expect(catalog[base], base).toBeDefined();
        expect(catalog[key]).toContain('{count}');
        expect(catalog[base]).toContain('{count}');
      }
    }
  });

  it('never contain markup', () => {
    for (const catalog of [en, es]) {
      for (const value of Object.values(catalog)) expect(value).not.toMatch(/<\/?[a-z]/i);
    }
  });

  it('translate every error code the server can send', () => {
    // Codes of cli/internal/server/errors.go (notebookErrors and request errors).
    const codes = [
      'invalid_slug', 'invalid_reference', 'invalid_entry_id', 'invalid_date', 'invalid_query', 'empty', 'too_long',
      'control_character', 'invalid_value', 'out_of_range', 'topic_section_mismatch', 'limit_exceeded', 'not_found',
      'already_exists', 'conflict', 'ambiguous', 'section_not_deletable',
    ];
    const withoutMessage = codes.filter((c) => !(`error.code.${c}` in en) && !['not_found', 'conflict', 'invalid_entry_id'].includes(c));
    expect(withoutMessage).toEqual([]);
  });
});

describe('interpolate and plurals', () => {
  it('fills known placeholders and leaves unknown ones', () => {
    expect(interpolate('{a} and {b}', { a: 1 })).toBe('1 and {b}');
  });

  it('does not treat parameter values as templates', () => {
    expect(interpolate('Hi {name}', { name: '{evil}' })).toBe('Hi {evil}');
  });

  it('picks singular and plural forms per language', () => {
    expect(makeI18n('en').tn('sections.topicCount', 1)).toBe('1 topic');
    expect(makeI18n('en').tn('sections.topicCount', 3)).toBe('3 topics');
    expect(makeI18n('es').tn('sections.topicCount', 1)).toBe('1 tema');
    expect(makeI18n('es').tn('sections.topicCount', 0)).toBe('0 temas');
  });

  it('formats dates in the chosen language and survives bad ones', () => {
    expect(makeI18n('en').formatDay('2026-11-03')).toContain('November');
    expect(makeI18n('es').formatDay('2026-11-03')).toContain('noviembre');
    expect(makeI18n('en').formatDay('nonsense')).toBe('nonsense');
  });
});

describe('language detection', () => {
  afterEach(() => vi.unstubAllGlobals());

  it('takes the language of a locale tag', () => {
    expect(languageOf('es-MX')).toBe('es');
    expect(languageOf('en_GB')).toBe('en');
    expect(languageOf('fr-FR')).toBeNull();
    expect(languageOf(undefined)).toBeNull();
  });

  it('uses the first supported browser language, else English', () => {
    vi.stubGlobal('navigator', { languages: ['fr-FR', 'es-ES', 'en'], language: 'fr-FR' });
    expect(detectLanguage()).toBe('es');
    vi.stubGlobal('navigator', { languages: ['de'], language: 'de' });
    expect(detectLanguage()).toBe('en');
  });
});

describe('describeError', () => {
  const i18n = makeI18n('en');

  it('explains an expired session with the printed URL', () => {
    const msg = describeError(i18n, new ApiError('unauthorized', 401, 'unauthorized'));
    expect(msg).toContain('bearings serve');
    expect(msg).toContain('/auth?token=');
  });

  it('explains conflicts', () => {
    expect(describeError(i18n, new ApiError('conflict', 409, 'conflict'))).toContain('changed elsewhere');
  });

  it('uses the code message and names the field', () => {
    const msg = describeError(i18n, new ApiError('invalid', 422, 'too_long', 'title'));
    expect(msg).toContain('too long');
    expect(msg).toContain('title');
  });

  it('falls back to the kind for unknown codes and to a generic message for other errors', () => {
    expect(describeError(i18n, new ApiError('server', 500, 'notebook_error'))).toBe(en['error.server']);
    expect(describeError(i18n, new ApiError('invalid', 400, 'brand_new_code', 'weird'))).toContain(en['error.invalid']);
    expect(describeError(i18n, new Error('boom'))).toBe(en['error.generic']);
  });

  it('translates', () => {
    expect(describeError(makeI18n('es'), new ApiError('network', 0, 'network'))).toContain('servidor local');
  });
});
