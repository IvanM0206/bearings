/**
 * Local-first storage: the notebook lives in this browser's IndexedDB.
 * There is no server; nothing here makes network requests.
 */
import Dexie, { type Table } from 'dexie';
import type { Entry, NotebookConfig, Section, Topic } from '../core/model';

export interface MetaRow {
  key: 'config';
  value: NotebookConfig;
}

export class BearingsDB extends Dexie {
  sections!: Table<Section, string>;
  topics!: Table<Topic, [string, string]>;
  entries!: Table<Entry, string>;
  meta!: Table<MetaRow, string>;

  constructor(name = 'bearings') {
    super(name);
    this.version(1).stores({
      sections: '&slug',
      topics: '&[section+slug], section',
      entries: '&id, date, section, topic',
      meta: '&key',
    });
  }
}

let instance: BearingsDB | null = null;

export function getDb(): BearingsDB {
  instance ??= new BearingsDB();
  return instance;
}

/** For tests: use a fresh database. */
export function setDb(db: BearingsDB): void {
  instance = db;
}

export type PersistState = 'persisted' | 'denied' | 'unsupported';

/** Asks the browser not to evict our data under storage pressure. */
export async function requestPersistence(): Promise<PersistState> {
  try {
    if (typeof navigator === 'undefined' || !('storage' in navigator) || typeof navigator.storage.persist !== 'function') {
      return 'unsupported';
    }
    if (await navigator.storage.persisted()) return 'persisted';
    return (await navigator.storage.persist()) ? 'persisted' : 'denied';
  } catch {
    return 'unsupported';
  }
}

export async function persistState(): Promise<PersistState> {
  try {
    if (typeof navigator === 'undefined' || !('storage' in navigator) || typeof navigator.storage.persisted !== 'function') {
      return 'unsupported';
    }
    return (await navigator.storage.persisted()) ? 'persisted' : 'denied';
  } catch {
    return 'unsupported';
  }
}

export async function storageEstimate(): Promise<{ usage: number; quota: number } | null> {
  try {
    if (typeof navigator === 'undefined' || typeof navigator.storage?.estimate !== 'function') return null;
    const e = await navigator.storage.estimate();
    return { usage: e.usage ?? 0, quota: e.quota ?? 0 };
  } catch {
    return null;
  }
}
