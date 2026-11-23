import { useId, useState } from 'react';
import { api } from '../api/client';
import type { Entry } from '../api/types';
import { useApp } from '../app/AppContext';
import { EntryCard } from '../components/EntryCard';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { useI18n } from '../i18n';
import { links, navigate } from '../lib/router';
import { useAction } from '../lib/useAction';
import { useOutline } from '../lib/useOutline';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';

const PAGE = 100;

function groupByDay(entries: Entry[]): Array<{ date: string; entries: Entry[] }> {
  const groups: Array<{ date: string; entries: Entry[] }> = [];
  for (const e of entries) {
    const last = groups[groups.length - 1];
    if (last && last.date === e.date) last.entries.push(e);
    else groups.push({ date: e.date, entries: [e] });
  }
  return groups;
}

export function JournalPage({ query }: { query: URLSearchParams }) {
  const { t, formatDay } = useI18n();
  const { readOnly, openNote } = useApp();
  usePageTitle(t('journal.title'));
  const outline = useOutline();
  const ids = { from: useId(), to: useId(), section: useId() };
  const from = query.get('from') ?? '';
  const to = query.get('to') ?? '';
  const section = query.get('section') ?? '';
  const filtered = from !== '' || to !== '' || section !== '';

  const first = useResource(
    (s) => api.journal({ from, to, section, limit: PAGE }, s),
    [from, to, section],
  );
  // Older pages loaded on demand. They belong to the first page they were
  // loaded after: a fresh first page (filters or data changed) drops them.
  const [older, setOlder] = useState<{ base: unknown; entries: Entry[]; hasMore: boolean } | null>(null);
  const more = useAction();
  const olderNow = older && older.base === first.data ? older : null;
  const hasMore = olderNow ? olderNow.hasMore : (first.data?.has_more ?? false);
  const entries = [...(first.data?.entries ?? []), ...(olderNow?.entries ?? [])];

  const loadMore = async () => {
    await more.run(async () => {
      const page = await api.journal({ from, to, section, limit: PAGE, offset: entries.length });
      setOlder({ base: first.data, entries: [...(olderNow?.entries ?? []), ...page.entries], hasMore: page.has_more });
    });
  };

  const setFilter = (key: 'from' | 'to' | 'section', value: string) => {
    navigate(links.journal({ from, to, section, [key]: value }));
  };

  return (
    <div className="page">
      <div className="row-between">
        <h1>{t('journal.title')}</h1>
        {!readOnly ? (
          <button type="button" className="btn btn-primary" onClick={() => openNote()}>
            {t('journal.add')}
          </button>
        ) : null}
      </div>
      <p className="muted">{t('journal.intro')}</p>

      <form className="card filters-form" role="search" aria-label={t('journal.filters')} onSubmit={(e) => e.preventDefault()}>
        <div className="field">
          <label htmlFor={ids.from}>{t('journal.from')}</label>
          <input id={ids.from} type="date" value={from} onChange={(e) => setFilter('from', e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor={ids.to}>{t('journal.to')}</label>
          <input id={ids.to} type="date" value={to} onChange={(e) => setFilter('to', e.target.value)} />
        </div>
        <div className="field">
          <label htmlFor={ids.section}>{t('journal.section')}</label>
          <select id={ids.section} value={section} onChange={(e) => setFilter('section', e.target.value)}>
            <option value="">{t('journal.allSections')}</option>
            {(outline.data ?? []).map((o) => (
              <option key={o.section.slug} value={o.section.slug}>
                {o.section.title}
              </option>
            ))}
          </select>
        </div>
        {filtered ? (
          <a className="btn btn-quiet" href={links.journal()}>
            {t('journal.clearFilters')}
          </a>
        ) : null}
      </form>

      <ErrorNotice error={first.error} onRetry={first.reload} />
      {first.loading && !first.data ? <Loading /> : null}
      {first.data && entries.length === 0 ? <p>{filtered ? t('journal.noMatches') : t('journal.empty')}</p> : null}
      {groupByDay(entries).map((g) => (
        <section key={g.date} aria-labelledby={`day-${g.date}`} className="day">
          <h2 id={`day-${g.date}`} className="day-heading">
            <time dateTime={g.date}>{formatDay(g.date, { weekday: 'long', year: 'numeric', month: 'long', day: 'numeric' })}</time>
          </h2>
          {g.entries.map((e) => (
            <EntryCard key={e.id} entry={e} lookup={outline.lookup} />
          ))}
        </section>
      ))}
      <ErrorNotice error={more.error} />
      {hasMore ? (
        <p className="center">
          <button type="button" className="btn btn-secondary" disabled={more.busy} onClick={() => void loadMore()}>
            {more.busy ? t('app.loading') : t('journal.loadMore')}
          </button>
        </p>
      ) : null}
    </div>
  );
}
