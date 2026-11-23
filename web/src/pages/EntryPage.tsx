import { api, ApiError } from '../api/client';
import { EntryCard } from '../components/EntryCard';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { useI18n } from '../i18n';
import { links, navigate } from '../lib/router';
import { useOutline } from '../lib/useOutline';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';

export function EntryPage({ id }: { id: string }) {
  const { t, formatDay } = useI18n();
  const res = useResource((s) => api.entry(id, s), [id]);
  const outline = useOutline();
  usePageTitle(res.data ? formatDay(res.data.date) : t('journal.title'));

  return (
    <div className="page">
      <nav aria-label={t('section.breadcrumb')} className="crumbs">
        <a href={links.journal()}>{t('journal.back')}</a>
      </nav>
      {res.loading && !res.data ? <Loading /> : null}
      {res.error instanceof ApiError && res.error.kind === 'not_found' ? (
        <p>{t('journal.entryNotFound')}</p>
      ) : (
        <ErrorNotice error={res.error} onRetry={res.reload} />
      )}
      {res.data ? (
        <>
          <h1>{formatDay(res.data.date, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}</h1>
          <EntryCard entry={res.data} lookup={outline.lookup} onDeleted={() => navigate(links.journal())} />
        </>
      ) : null}
    </div>
  );
}
