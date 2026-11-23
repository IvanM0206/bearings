import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { ApiError } from '../api/client';
import { en, type MessageKey, type Messages } from './en';
import { es } from './es';

export const LANGUAGES = ['en', 'es'] as const;
export type Language = (typeof LANGUAGES)[number];

export const CATALOGS: Record<Language, Messages> = { en, es };
const STORAGE_KEY = 'bearings.language';

export type Params = Record<string, string | number>;

export interface I18n {
  lang: Language;
  setLang: (lang: Language) => void;
  /** Uses the notebook's language unless the person already chose one. */
  suggestLanguage: (lang: string) => void;
  t: (key: MessageKey, params?: Params) => string;
  /** Like `t`, picking the `_one` form when `count` is singular. */
  tn: (key: MessageKey, count: number, params?: Params) => string;
  /** Formats a YYYY-MM-DD date. */
  formatDay: (date: string, options?: Intl.DateTimeFormatOptions) => string;
  formatNumber: (n: number) => string;
}

export function interpolate(template: string, params: Params = {}): string {
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : whole,
  );
}

export function isLanguage(value: unknown): value is Language {
  return typeof value === 'string' && (LANGUAGES as readonly string[]).includes(value);
}

/** The language of a locale tag such as "es-MX", or null when unsupported. */
export function languageOf(tag: string | undefined): Language | null {
  const base = tag?.toLowerCase().split(/[-_]/)[0];
  return isLanguage(base) ? base : null;
}

function readStoredLanguage(): Language | null {
  try {
    const v = globalThis.localStorage.getItem(STORAGE_KEY);
    return isLanguage(v) ? v : null;
  } catch {
    return null;
  }
}

function storeLanguage(lang: Language): void {
  try {
    globalThis.localStorage.setItem(STORAGE_KEY, lang);
  } catch {
    // Storage blocked: the choice lasts for this visit only.
  }
}

/** Stored choice, else browser language, else English. */
export function detectLanguage(): Language {
  const nav = typeof navigator === 'undefined' ? [] : (navigator.languages ?? [navigator.language]);
  for (const tag of nav) {
    const l = languageOf(tag);
    if (l) return l;
  }
  return 'en';
}

function dateAtNoonUtc(date: string): Date {
  return new Date(`${date}T12:00:00Z`);
}

export function makeI18n(
  lang: Language,
  setLang: (l: Language) => void = () => undefined,
  suggestLanguage: (l: string) => void = () => undefined,
): I18n {
  const messages = CATALOGS[lang];
  const plural = new Intl.PluralRules(lang);
  const number = new Intl.NumberFormat(lang);
  return {
    lang,
    setLang,
    suggestLanguage,
    t: (key, params) => interpolate(messages[key], params),
    tn: (key, count, params) => {
      const oneKey = `${key}_one` as MessageKey;
      const useOne = plural.select(count) === 'one' && oneKey in messages;
      return interpolate(messages[useOne ? oneKey : key], { count: number.format(count), ...params });
    },
    formatDay: (date, options = { year: 'numeric', month: 'long', day: 'numeric' }) => {
      const d = dateAtNoonUtc(date);
      return Number.isNaN(d.getTime()) ? date : new Intl.DateTimeFormat(lang, { ...options, timeZone: 'UTC' }).format(d);
    },
    formatNumber: (n) => number.format(n),
  };
}

const I18nContext = createContext<I18n>(makeI18n('en'));

export function I18nProvider({ children, initial }: { children: ReactNode; initial?: Language }) {
  // The person's own choice (kept in localStorage) always wins over the notebook's language.
  const [chosen, setChosen] = useState<Language | null>(readStoredLanguage);
  const [lang, setLangState] = useState<Language>(() => initial ?? chosen ?? detectLanguage());
  const setLang = useCallback((l: Language) => {
    storeLanguage(l);
    setChosen(l);
    setLangState(l);
  }, []);
  const suggestLanguage = useCallback(
    (l: string) => {
      if (chosen === null && isLanguage(l)) setLangState(l);
    },
    [chosen],
  );
  useEffect(() => {
    document.documentElement.lang = lang;
    document.title = CATALOGS[lang]['app.name'];
  }, [lang]);
  const value = useMemo(() => makeI18n(lang, setLang, suggestLanguage), [lang, setLang, suggestLanguage]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  return useContext(I18nContext);
}

function fieldLabel(i18n: I18n, field: string): string {
  const key = `error.field.${field}` as MessageKey;
  return key in CATALOGS[i18n.lang] ? i18n.t(key) : field;
}

/** A translated description of any error, never containing notebook content. */
export function describeError(i18n: I18n, e: unknown): string {
  if (!(e instanceof ApiError)) return i18n.t('error.generic');
  const byCode = `error.code.${e.code}` as MessageKey;
  const base = byCode in CATALOGS[i18n.lang] ? i18n.t(byCode) : i18n.t(`error.${e.kind}` as MessageKey);
  if (e.kind === 'invalid' && e.field) {
    return `${base} ${i18n.t('error.field', { field: fieldLabel(i18n, e.field) })}`;
  }
  return base;
}
