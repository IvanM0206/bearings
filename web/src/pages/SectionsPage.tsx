import { useState } from 'react';
import { api } from '../api/client';
import type { Section } from '../api/types';
import { useApp } from '../app/AppContext';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { ProgressBar } from '../components/ProgressBar';
import { SectionDialog } from '../components/SectionDialog';
import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { useAction } from '../lib/useAction';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';
import { Markdown } from '../markdown/Markdown';
import { useMarkdownLabels } from '../markdown/useLabels';

const EMPTY_COUNTS = { pending: 0, in_progress: 0, mastered: 0, total: 0 };

export function SectionsPage() {
  const { t, tn } = useI18n();
  const { readOnly, bump, flash } = useApp();
  const labels = useMarkdownLabels();
  usePageTitle(t('sections.title'));
  const res = useResource((s) => api.sections(s), []);
  const move = useAction();
  const [dialog, setDialog] = useState<{ section?: Section } | null>(null);
  const [deleting, setDeleting] = useState<Section | null>(null);
  const sections = res.data ?? [];

  const reorder = async (s: Section, delta: -1 | 1) => {
    if (await move.run(() => api.reorderSection(s.slug, delta))) {
      bump();
      flash(t('sections.reordered'));
    }
  };

  return (
    <div className="page">
      <div className="row-between">
        <h1>{t('sections.title')}</h1>
        {!readOnly ? (
          <button type="button" className="btn btn-primary" onClick={() => setDialog({})}>
            {t('sections.add')}
          </button>
        ) : null}
      </div>
      <p className="muted">{t('sections.intro')}</p>
      <ErrorNotice error={res.error ?? move.error} onRetry={res.reload} onReload={res.reload} />
      {res.loading && !res.data ? <Loading /> : null}
      {res.data && sections.length === 0 ? <p>{t('sections.empty')}</p> : null}
      <ul className="plain stack">
        {sections.map((s, i) => {
          const counts = s.counts ?? EMPTY_COUNTS;
          return (
            <li key={s.slug} className="card section-card">
              <h2>
                <a href={links.section(s.slug)}>{s.title}</a>
              </h2>
              {s.description ? <Markdown source={s.description} labels={labels} className="muted" /> : null}
              <p className="muted small">
                {tn('sections.topicCount', counts.total)} · {t('progress.mastered', { count: counts.mastered })}
              </p>
              <ProgressBar counts={counts} />
              {!readOnly ? (
                <div className="row-actions">
                  <button type="button" className="btn btn-quiet btn-small" disabled={i === 0 || move.busy} onClick={() => void reorder(s, -1)}>
                    {t('common.moveUp')}
                    <span className="sr-only"> — {s.title}</span>
                  </button>
                  <button type="button" className="btn btn-quiet btn-small" disabled={i === sections.length - 1 || move.busy} onClick={() => void reorder(s, 1)}>
                    {t('common.moveDown')}
                    <span className="sr-only"> — {s.title}</span>
                  </button>
                  <button type="button" className="btn btn-quiet btn-small" onClick={() => setDialog({ section: s })}>
                    {t('common.edit')}
                    <span className="sr-only"> — {s.title}</span>
                  </button>
                  <button type="button" className="btn btn-quiet btn-small" onClick={() => setDeleting(s)}>
                    {t('common.delete')}
                    <span className="sr-only"> — {s.title}</span>
                  </button>
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>
      {dialog ? <SectionDialog {...(dialog.section ? { section: dialog.section } : {})} onClose={() => setDialog(null)} /> : null}
      {deleting ? (
        <ConfirmDialog
          title={t('sections.deleteTitle')}
          body={tn('sections.deleteBody', deleting.counts?.total ?? 0, { title: deleting.title })}
          confirmLabel={t('sections.deleteConfirm')}
          onClose={() => setDeleting(null)}
          onConfirm={async () => {
            await api.deleteSection(deleting.slug);
            bump();
          }}
        />
      ) : null}
    </div>
  );
}
