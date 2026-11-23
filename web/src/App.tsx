import { useCallback, useEffect, useMemo, useState } from 'react';
import { api } from './api/client';
import type { Info } from './api/types';
import { AppProvider, type App as AppValue, type NotePrefill } from './app/AppContext';
import { ErrorNotice } from './components/ErrorNotice';
import { Layout } from './components/Layout';
import { Loading } from './components/Loading';
import { NoteDialog } from './components/NoteDialog';
import { useI18n } from './i18n';
import { useRoute, type Route } from './lib/router';
import { EntryPage } from './pages/EntryPage';
import { HomePage } from './pages/HomePage';
import { JournalPage } from './pages/JournalPage';
import { NotFoundPage } from './pages/NotFoundPage';
import { RolePathPage } from './pages/RolePathPage';
import { SearchPage } from './pages/SearchPage';
import { SectionPage } from './pages/SectionPage';
import { SectionsPage } from './pages/SectionsPage';
import { TopicPage } from './pages/TopicPage';

function renderRoute(route: Route) {
  const [a, b, c] = route.segments;
  const n = route.segments.length;
  if (n === 0) return <HomePage />;
  if (a === 'sections' && n === 1) return <SectionsPage />;
  if (a === 'sections' && n === 2 && b) return <SectionPage slug={b} />;
  if (a === 'topics' && n === 3 && b && c) return <TopicPage section={b} topic={c} />;
  if (a === 'journal' && n === 1) return <JournalPage query={route.query} />;
  if (a === 'journal' && n === 2 && b) return <EntryPage id={b} />;
  if (a === 'path' && n === 1) return <RolePathPage />;
  if (a === 'search' && n === 1) return <SearchPage query={route.query} />;
  return <NotFoundPage />;
}

const FLASH_MS = 5000;

export function App() {
  const i18n = useI18n();
  const { t, suggestLanguage } = i18n;
  const route = useRoute();
  const [info, setInfo] = useState<Info | undefined>();
  const [error, setError] = useState<unknown>();
  const [revision, setRevision] = useState(0);
  const [toast, setToast] = useState({ message: '', id: 0 });
  const [note, setNote] = useState<NotePrefill | null>(null);

  const loadInfo = useCallback(() => {
    api.info().then(
      (i) => {
        setInfo(i);
        suggestLanguage(i.language);
      },
      (e: unknown) => setError(e),
    );
  }, [suggestLanguage]);

  useEffect(() => {
    loadInfo();
  }, [loadInfo]);

  // Clears the status message after a while.
  useEffect(() => {
    if (toast.id === 0) return;
    const timer = setTimeout(() => setToast((t) => (t.id === toast.id ? { ...t, message: '' } : t)), FLASH_MS);
    return () => clearTimeout(timer);
  }, [toast.id]);

  const retry = useCallback(() => {
    setError(undefined);
    loadInfo();
  }, [loadInfo]);
  const bump = useCallback(() => setRevision((n) => n + 1), []);
  const flash = useCallback((message: string) => setToast((t) => ({ message, id: t.id + 1 })), []);
  const openNote = useCallback((prefill?: NotePrefill) => setNote(prefill ?? {}), []);

  const value = useMemo<AppValue | undefined>(
    () => (info ? { info, readOnly: info.read_only, revision, bump, flash, openNote } : undefined),
    [info, revision, bump, flash, openNote],
  );

  if (!value) {
    return (
      <div className="boot">
        <h1 className="boot-title">{t('app.name')}</h1>
        {error === undefined ? <Loading /> : <ErrorNotice error={error} onRetry={retry} />}
      </div>
    );
  }

  return (
    <AppProvider value={value}>
      <Layout route={route}>{renderRoute(route)}</Layout>
      <div className="toast" role="status" aria-live="polite">
        {toast.message}
      </div>
      {note && !value.readOnly ? <NoteDialog prefill={note} onClose={() => setNote(null)} /> : null}
    </AppProvider>
  );
}
