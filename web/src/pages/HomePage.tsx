import { api } from '../api/client';
import type { Counts } from '../api/types';
import { useApp } from '../app/AppContext';
import { ErrorNotice } from '../components/ErrorNotice';
import { EntryCard } from '../components/EntryCard';
import { Loading } from '../components/Loading';
import { ProgressBar } from '../components/ProgressBar';
import { QuickEntry } from '../components/QuickEntry';
import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { useOutline } from '../lib/useOutline';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';

function CountsText({ counts }: { counts: Counts }) {
  const { t } = useI18n();
  return (
    <ul className="counts" aria-label={t('progress.chart', { mastered: counts.mastered, inProgress: counts.in_progress, pending: counts.pending })}>
      <li className="counts-mastered">{t('progress.mastered', { count: counts.mastered })}</li>
      <li className="counts-progress">{t('progress.inProgress', { count: counts.in_progress })}</li>
      <li className="counts-pending">{t('progress.pending', { count: counts.pending })}</li>
    </ul>
  );
}

export function HomePage() {
  const { t, tn } = useI18n();
  const { readOnly, info } = useApp();
  usePageTitle(t('home.title'));
  const progress = useResource((s) => api.progress(s), []);
  const recent = useResource((s) => api.journal({ limit: 3 }, s), []);
  const outline = useOutline();
  const p = progress.data;

  return (
    <div className="page">
      <h1>
        {t('home.greeting')}
        {info.name ? <span className="h1-sub">{info.name}</span> : null}
      </h1>

      {!readOnly ? <QuickEntry /> : null}

      <section className="card" aria-labelledby="progress-h">
        <h2 id="progress-h">{t('home.progressTitle')}</h2>
        <ErrorNotice error={progress.error} onRetry={progress.reload} />
        {!p ? (
          progress.error === undefined ? <Loading /> : null
        ) : p.all.total === 0 ? (
          <p>{t('home.noTopics')}</p>
        ) : (
          <>
            <p className="big-number">{t('home.progressSummary', { mastered: p.all.mastered, total: p.all.total })}</p>
            <ProgressBar counts={p.all} large />
            <CountsText counts={p.all} />
            <ul className="plain facts">
              <li>{t('home.roleSummary', { mastered: p.role.mastered, total: p.role.total })}</li>
              <li>{t('home.personalSummary', { mastered: p.personal.mastered, total: p.personal.total })}</li>
              <li>{tn('home.entries', p.journal_entries)}</li>
              {p.suggested > 0 ? <li>{tn('home.suggested', p.suggested)}</li> : null}
            </ul>
            <h3>{t('home.bySection')}</h3>
            <ul className="plain section-bars">
              {p.sections.map((s) => (
                <li key={s.slug}>
                  <a href={links.section(s.slug)}>{s.title}</a>
                  <ProgressBar counts={s.all} />
                  <span className="muted small">
                    {s.all.mastered}/{s.all.total}
                  </span>
                </li>
              ))}
            </ul>
          </>
        )}
      </section>

      <section aria-labelledby="recent-h">
        <div className="row-between">
          <h2 id="recent-h">{t('home.recent')}</h2>
          <a href={links.journal()}>{t('home.seeAll')}</a>
        </div>
        <ErrorNotice error={recent.error} onRetry={recent.reload} />
        {recent.data && recent.data.entries.length === 0 ? <p className="muted">{t('home.noEntries')}</p> : null}
        {recent.data?.entries.map((e) => (
          <EntryCard key={e.id} entry={e} lookup={outline.lookup} compact showDate />
        ))}
      </section>
    </div>
  );
}
