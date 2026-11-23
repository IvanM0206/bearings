import { useId, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Status, TopicSummary, TopicType } from '../api/types';
import { STATUSES } from '../api/types';
import { useApp } from '../app/AppContext';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { ProgressBar } from '../components/ProgressBar';
import { StatusChip } from '../components/StatusChip';
import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { useAction } from '../lib/useAction';
import { useOutline } from '../lib/useOutline';
import { usePageTitle } from '../lib/usePageTitle';
import { Markdown } from '../markdown/Markdown';
import { useMarkdownLabels } from '../markdown/useLabels';

const NEXT: Record<Status, Status | undefined> = { pending: 'in-progress', 'in-progress': 'mastered', mastered: undefined };

export function SectionPage({ slug }: { slug: string }) {
  const { t, tn } = useI18n();
  const { readOnly, bump, flash } = useApp();
  const labels = useMarkdownLabels();
  const res = useOutline();
  const item = res.data?.find((o) => o.section.slug === slug);
  usePageTitle(item?.section.title);
  const [filter, setFilter] = useState<Status | 'all'>('all');
  const write = useAction();
  const add = useAction();
  const ids = { title: useId(), type: useId() };
  const [title, setTitle] = useState('');
  const [type, setType] = useState<TopicType>('role');

  if (res.loading && !res.data) return <Loading />;
  if (res.error !== undefined && !res.data) return <div className="page"><ErrorNotice error={res.error} onRetry={res.reload} /></div>;
  if (!item) {
    return (
      <div className="page">
        <p>{t('sections.notFound')}</p>
        <a href={links.sections()}>{t('section.backToSections')}</a>
      </div>
    );
  }

  const all = item.topics;
  const shown = filter === 'all' ? all : all.filter((x) => x.status === filter);

  const advance = async (x: TopicSummary) => {
    const next = NEXT[x.status];
    if (!next) return;
    if (await write.run(() => api.advance(x.ref))) {
      bump();
      flash(t('status.changed', { status: t(`status.${next}`) }));
    }
  };
  const reorder = async (x: TopicSummary, delta: -1 | 1) => {
    if (await write.run(() => api.reorderTopic(x.ref, delta))) bump();
  };
  const submit = async (e: FormEvent) => {
    e.preventDefault();
    if (await add.run(() => api.addTopic(slug, { title, type, status: 'pending' }))) {
      setTitle('');
      bump();
    }
  };

  return (
    <div className="page">
      <nav aria-label={t('section.breadcrumb')} className="crumbs">
        <a href={links.sections()}>{t('nav.sections')}</a>
      </nav>
      <h1>{item.section.title}</h1>
      {item.section.description ? <Markdown source={item.section.description} labels={labels} className="muted" /> : null}
      <ProgressBar counts={item.section.counts ?? { pending: 0, in_progress: 0, mastered: 0, total: 0 }} />

      <div className="filters" role="group" aria-label={t('section.filter')}>
        {(['all', ...STATUSES] as const).map((s) => (
          <button key={s} type="button" className="filter-btn" aria-pressed={filter === s} onClick={() => setFilter(s)}>
            {s === 'all' ? t('common.all') : t(`status.${s}`)}
          </button>
        ))}
      </div>

      <ErrorNotice error={write.error} onReload={() => { write.clear(); bump(); }} />
      {all.length === 0 ? <p>{t('section.noTopics')}</p> : null}
      {all.length > 0 && shown.length === 0 ? <p>{t('section.noMatches')}</p> : null}
      <ul className="plain stack" aria-label={t('section.topicsList', { section: item.section.title })}>
        {shown.map((x) => {
          const idx = all.indexOf(x);
          const next = NEXT[x.status];
          return (
            <li key={x.ref} className="card topic-row">
              <div className="topic-main">
                <h2>
                  <a href={links.topic(x.ref)}>{x.title}</a>
                </h2>
                <p className="meta">
                  <StatusChip status={x.status} />
                  <span className="tag">{t(`type.${x.type}`)}</span>
                  {x.suggested ? <span className="tag tag-accent">{t('section.suggestedBadge')}</span> : null}
                  {x.resources > 0 ? <span className="muted small">{tn('section.resources', x.resources)}</span> : null}
                </p>
              </div>
              {!readOnly ? (
                <div className="row-actions">
                  {next ? (
                    <button type="button" className="btn btn-secondary btn-small" disabled={write.busy} onClick={() => void advance(x)}>
                      {t('status.advanceTo', { status: t(`status.${next}`) })}
                      <span className="sr-only"> — {x.title}</span>
                    </button>
                  ) : null}
                  {filter === 'all' ? (
                    <>
                      <button type="button" className="btn btn-quiet btn-small" disabled={idx === 0 || write.busy} onClick={() => void reorder(x, -1)}>
                        {t('common.moveUp')}
                        <span className="sr-only"> — {x.title}</span>
                      </button>
                      <button type="button" className="btn btn-quiet btn-small" disabled={idx === all.length - 1 || write.busy} onClick={() => void reorder(x, 1)}>
                        {t('common.moveDown')}
                        <span className="sr-only"> — {x.title}</span>
                      </button>
                    </>
                  ) : null}
                </div>
              ) : null}
            </li>
          );
        })}
      </ul>

      {!readOnly ? (
        <section className="card" aria-labelledby={ids.title + '-h'}>
          <h2 id={ids.title + '-h'}>{t('section.addTopicTitle')}</h2>
          <form onSubmit={(e) => void submit(e)} className="form">
            <div className="field">
              <label htmlFor={ids.title}>{t('section.topicTitleLabel')}</label>
              <input id={ids.title} value={title} required maxLength={160} onChange={(e) => setTitle(e.target.value)} />
            </div>
            <div className="field">
              <label htmlFor={ids.type}>{t('type.label')}</label>
              <select id={ids.type} value={type} onChange={(e) => setType(e.target.value as TopicType)}>
                <option value="role">{t('type.role')}</option>
                <option value="personal">{t('type.personal')}</option>
              </select>
              <p className="hint">{type === 'role' ? t('type.roleHint') : t('type.personalHint')}</p>
            </div>
            <ErrorNotice error={add.error} />
            <div className="actions actions-start">
              <button type="submit" className="btn btn-primary" disabled={add.busy || title.trim() === ''}>
                {add.busy ? t('common.saving') : t('section.addTopic')}
              </button>
            </div>
          </form>
        </section>
      ) : null}
    </div>
  );
}
