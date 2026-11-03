import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { FormatError, NotebookErrors } from '../core/errors';
import { LANGUAGES, type Language } from '../core/model';
import { dateAtNoonUtc } from '../core/time';
import { en, type MessageKey, type Messages } from './en';
import { es } from './es';

const CATALOGS: Record<Language, Messages> = { en, es };
const STORAGE_KEY = 'bearings.language';

export type Params = Record<string, string | number>;

export interface I18n {
  lang: Language;
  setLang: (lang: Language) => void;
  t: (key: MessageKey, params?: Params) => string;
  /** Like `t`, picking the `_one` form when `count` is singular. */
  tn: (key: MessageKey, count: number, params?: Params) => string;
  formatDay: (date: string, options: Intl.DateTimeFormatOptions) => string;
  formatNumber: (n: number) => string;
  formatBytes: (n: number) => string;
}

export function interpolate(template: string, params: Params = {}): string {
  return template.replace(/\{(\w+)\}/g, (whole, name: string) =>
    Object.prototype.hasOwnProperty.call(params, name) ? String(params[name]) : whole,
  );
}

export function isLanguage(value: unknown): value is Language {
  return typeof value === 'string' && (LANGUAGES as readonly string[]).includes(value);
}

function readStoredLanguage(): Language | null {
  try {
    const v = globalThis.localStorage?.getItem(STORAGE_KEY);
    return isLanguage(v) ? v : null;
  } catch {
    return null;
  }
}

function storeLanguage(lang: Language): void {
  try {
    globalThis.localStorage?.setItem(STORAGE_KEY, lang);
  } catch {
    // Storage blocked: the choice lasts for this visit only.
  }
}

export function hasStoredLanguage(): boolean {
  return readStoredLanguage() !== null;
}

export function makeI18n(lang: Language, setLang: (l: Language) => void): I18n {
  const messages = CATALOGS[lang];
  const plural = new Intl.PluralRules(lang);
  const number = new Intl.NumberFormat(lang);
  const t = (key: MessageKey, params?: Params): string => interpolate(messages[key], params);
  return {
    lang,
    setLang,
    t,
    tn: (key, count, params) => {
      const oneKey = `${key}_one` as MessageKey;
      const useOne = plural.select(count) === 'one' && oneKey in messages;
      return interpolate(messages[useOne ? oneKey : key], { count: number.format(count), ...params });
    },
    formatDay: (date, options) =>
      new Intl.DateTimeFormat(lang, { ...options, timeZone: 'UTC' }).format(dateAtNoonUtc(date)),
    formatNumber: (n) => number.format(n),
    formatBytes: (n) => {
      const units = ['B', 'KB', 'MB', 'GB', 'TB'];
      let v = n;
      let u = 0;
      while (v >= 1024 && u < units.length - 1) {
        v /= 1024;
        u++;
      }
      return `${new Intl.NumberFormat(lang, { maximumFractionDigits: u === 0 ? 0 : 1 }).format(v)} ${units[u] ?? ''}`;
    },
  };
}

const I18nContext = createContext<I18n>(makeI18n('en', () => undefined));

export function I18nProvider({ children, initial }: { children: ReactNode; initial?: Language }) {
  const [lang, setLangState] = useState<Language>(() => initial ?? readStoredLanguage() ?? 'en');
  const setLang = useCallback((l: Language) => {
    storeLanguage(l);
    setLangState(l);
  }, []);
  useEffect(() => {
    document.documentElement.lang = lang;
    document.title = CATALOGS[lang]['app.name'];
  }, [lang]);
  const value = useMemo(() => makeI18n(lang, setLang), [lang, setLang]);
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>;
}

export function useI18n(): I18n {
  return useContext(I18nContext);
}

/** Longest file path shown in an error; paths come from untrusted archives. */
const MAX_SHOWN_PATH = 120;

function shorten(s: string): string {
  return s.length > MAX_SHOWN_PATH ? `${s.slice(0, MAX_SHOWN_PATH)}…` : s;
}

/** A translated, content-free description of one error. */
export function describeError(i18n: I18n, e: unknown): string {
  if (!(e instanceof FormatError)) return i18n.t('error.generic');
  const d = e.details;
  const params: Params = {
    max: d.max === undefined ? '' : i18n.formatNumber(d.max),
    min: d.min === undefined ? '' : i18n.formatNumber(d.min),
    limit: d.limit === undefined ? '' : i18n.t(`limit.${d.limit}`),
  };
  const parts: string[] = [];
  if (d.file !== undefined) parts.push(i18n.t('error.inFile', { file: shorten(d.file) }));
  parts.push(i18n.t(`error.${e.code}`, params));
  if (d.key !== undefined) parts.push(i18n.t('error.atKey', { key: shorten(d.key) }));
  return parts.join(' ');
}

/** All errors from a failed import or save, translated. */
export function describeErrors(i18n: I18n, e: unknown): string[] {
  if (e instanceof NotebookErrors) return e.errors.map((x) => describeError(i18n, x));
  return [describeError(i18n, e)];
}
