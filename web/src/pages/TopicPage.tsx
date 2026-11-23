import { useEffect, useId, useRef, useState, type FormEvent } from 'react';
import { ApiError, api } from '../api/client';
import type { Resource as TopicResource, Status, Topic, TopicType } from '../api/types';
import { STATUSES } from '../api/types';
import { useApp } from '../app/AppContext';
import { ConfirmDialog } from '../components/ConfirmDialog';
import { EntryCard } from '../components/EntryCard';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { MarkdownEditor } from '../components/MarkdownEditor';
import { StatusChip } from '../components/StatusChip';
import { useI18n } from '../i18n';
import { links, navigate } from '../lib/router';
import { useAction } from '../lib/useAction';
import { useOutline } from '../lib/useOutline';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';
import { Markdown } from '../markdown/Markdown';
import { safeHref } from '../markdown/safeUrl';
import { useMarkdownLabels } from '../markdown/useLabels';

const NEXT: Record<Status, Status | undefined> = { pending: 'in-progress', 'in-progress': 'mastered', mastered: undefined };
const TYPES: readonly TopicType[] = ['role', 'personal'];

export function TopicPage({ section, topic: slug }: { section: string; topic: string }) {
  const ref = `${section}/${slug}`;
  const { t } = useI18n();
  const res = useResource((s) => api.topic(ref, s), [ref]);
  usePageTitle(res.data?.title);

  if (res.loading && !res.data) return <Loading />;
  if (!res.data) {
    const gone = res.error instanceof ApiError && res.error.kind === 'not_found';
    return (
      <div className="page">
        {gone ? <p>{t('topic.notFound')}</p> : <ErrorNotice error={res.error} onRetry={res.reload} />}
        <a href={links.section(section)}>{t('nav.back')}</a>
      </div>
    );
  }
  return <TopicView key={res.data.ref} topic={res.data} setTopic={res.setData} reload={res.reload} />;
}

function TopicView({ topic, setTopic, reload }: { topic: Topic; setTopic: (t: Topic) => void; reload: () => void }) {
  const { t, formatDay } = useI18n();
  const { readOnly, bump, flash, openNote } = useApp();
  const labels = useMarkdownLabels();
  const action = useAction();
  const outline = useOutline();
  const entries = useResource((s) => api.journal({ topic: topic.ref, limit: 50 }, s), [topic.ref]);
  const ids = { title: useId(), url: useId(), rtitle: useId() };

  const [editingTitle, setEditingTitle] = useState(false);
  const [title, setTitle] = useState(topic.title);
  const [editingNotes, setEditingNotes] = useState(false);
  const [notes, setNotes] = useState(topic.notes);
  const [url, setUrl] = useState('');
  const [rtitle, setRtitle] = useState('');
  const [deleting, setDeleting] = useState(false);
  const titleRef = useRef<HTMLInputElement>(null);
  useEffect(() => {
    if (editingTitle) titleRef.current?.focus();
  }, [editingTitle]);

  /** Applies a change to the topic and keeps the new version for the next one. */
  const patch = async (changes: Omit<Parameters<typeof api.updateTopic>[1], 'version'>, message?: string): Promise<boolean> => {
    const ok = await action.run(async () => {
      const updated = await api.updateTopic(topic.ref, { version: topic.version, ...changes });
      setTopic(updated);
      bump();
      if (message) flash(message);
    });
    return ok;
  };

  const setStatus = async (status: Status) => {
    if (status === topic.status || action.busy) return;
    await action.run(async () => {
      const updated = await api.setStatus(topic.ref, status, topic.version);
      setTopic(updated);
      bump();
      flash(t('status.changed', { status: t(`status.${status}`) }));
    });
  };
  const advance = async () => {
    const next = NEXT[topic.status];
    if (!next) return;
    await action.run(async () => {
      const updated = await api.advance(topic.ref, topic.version);
      setTopic(updated);
      bump();
      flash(t('status.changed', { status: t(`status.${next}`) }));
    });
  };

  const saveTitle = async (e: FormEvent) => {
    e.preventDefault();
    if (await patch({ title })) setEditingTitle(false);
  };
  const saveNotes = async () => {
    if (await patch({ notes }, t('topic.notesSaved'))) setEditingNotes(false);
  };
  const addResource = async (e: FormEvent) => {
    e.preventDefault();
    const next: TopicResource[] = [...topic.resources, { url: url.trim(), title: rtitle.trim() }];
    if (await patch({ resources: next })) {
      setUrl('');
      setRtitle('');
    }
  };
  const removeResource = (i: number) => patch({ resources: topic.resources.filter((_, k) => k !== i) });

  const next = NEXT[topic.status];

  return (
    <div className="page">
      <nav aria-label={t('section.breadcrumb')} className="crumbs">
        <a href={links.sections()}>{t('nav.sections')}</a>
        <span aria-hidden="true"> / </span>
        <a href={links.section(topic.section)}>{topic.section_title || topic.section}</a>
      </nav>

      {editingTitle ? (
        <form onSubmit={(e) => void saveTitle(e)} className="form">
          <div className="field">
            <label htmlFor={ids.title}>{t('topic.titleLabel')}</label>
            <input id={ids.title} value={title} required maxLength={160} onChange={(e) => setTitle(e.target.value)} ref={titleRef} />
          </div>
          <div className="actions actions-start">
            <button type="submit" className="btn btn-primary" disabled={action.busy || title.trim() === ''}>
              {t('common.save')}
            </button>
            <button type="button" className="btn btn-secondary" onClick={() => { setEditingTitle(false); setTitle(topic.title); }}>
              {t('common.cancel')}
            </button>
          </div>
        </form>
      ) : (
        <div className="row-between">
          <h1>{topic.title}</h1>
          {!readOnly ? (
            <button type="button" className="btn btn-quiet btn-small" onClick={() => { setTitle(topic.title); setEditingTitle(true); }}>
              {t('topic.rename')}
            </button>
          ) : null}
        </div>
      )}
      <p className="muted small">{t('topic.updated', { date: formatDay(topic.updated.slice(0, 10)) })}</p>

      <ErrorNotice error={action.error} onReload={() => { action.clear(); setEditingNotes(false); reload(); }} />

      {topic.suggested ? (
        <div className="notice notice-info">
          <p>
            <strong>{t('topic.suggestedBadge')}.</strong> {t('topic.reviewedHint')}
          </p>
          {!readOnly ? (
            <button type="button" className="btn btn-secondary" disabled={action.busy} onClick={() => void patch({ suggested: false })}>
              {t('topic.markReviewed')}
            </button>
          ) : null}
        </div>
      ) : null}

      <section className="card" aria-labelledby="status-h">
        <h2 id="status-h" className="sr-only">
          {t('status.label')}
        </h2>
        <div className="props">
          <fieldset className="radio-group" disabled={readOnly}>
            <legend>{t('status.label')}</legend>
            <div className="seg-radio">
              {STATUSES.map((s) => (
                <label key={s} className={`seg-radio-item seg-${s}`}>
                  <input type="radio" name="status" value={s} checked={topic.status === s} onChange={() => void setStatus(s)} />
                  <span>{t(`status.${s}`)}</span>
                </label>
              ))}
            </div>
            {!readOnly ? (
              <p>
                {next ? (
                  <button type="button" className="btn btn-primary btn-small" onClick={() => void advance()} disabled={action.busy}>
                    {t('status.advanceTo', { status: t(`status.${next}`) })}
                  </button>
                ) : (
                  <span className="inline-ok">
                    <StatusChip status="mastered" /> {t('status.done')}
                  </span>
                )}
              </p>
            ) : null}
          </fieldset>
          <fieldset className="radio-group" disabled={readOnly}>
            <legend>{t('type.label')}</legend>
            <div className="seg-radio">
              {TYPES.map((ty) => (
                <label key={ty} className="seg-radio-item">
                  <input type="radio" name="type" value={ty} checked={topic.type === ty} onChange={() => void patch({ type: ty }, t('topic.typeSaved'))} />
                  <span>{t(`type.${ty}`)}</span>
                </label>
              ))}
            </div>
            <p className="hint">{topic.type === 'role' ? t('type.roleHint') : t('type.personalHint')}</p>
          </fieldset>
        </div>
      </section>

      <section className="card" aria-labelledby="notes-h">
        <div className="row-between">
          <h2 id="notes-h">{t('topic.notes')}</h2>
          {!readOnly && !editingNotes ? (
            <button type="button" className="btn btn-quiet btn-small" onClick={() => { setNotes(topic.notes); setEditingNotes(true); }}>
              {t('topic.notesEdit')}
            </button>
          ) : null}
        </div>
        {editingNotes ? (
          <>
            <MarkdownEditor label={t('topic.notes')} value={notes} onChange={setNotes} rows={12} placeholder={t('topic.notesPlaceholder')} initialFocus />
            <div className="actions actions-start">
              <button type="button" className="btn btn-primary" disabled={action.busy} onClick={() => void saveNotes()}>
                {action.busy ? t('common.saving') : t('common.save')}
              </button>
              <button type="button" className="btn btn-secondary" onClick={() => setEditingNotes(false)}>
                {t('common.cancel')}
              </button>
            </div>
          </>
        ) : topic.notes.trim() === '' ? (
          <p className="muted">{t('topic.notesEmpty')}</p>
        ) : (
          <Markdown source={topic.notes} labels={labels} />
        )}
      </section>

      <section className="card" aria-labelledby="res-h">
        <h2 id="res-h">{t('topic.resources')}</h2>
        {topic.resources.length === 0 ? <p className="muted">{t('topic.resourcesEmpty')}</p> : null}
        <ul className="plain resources">
          {topic.resources.map((r, i) => {
            const href = safeHref(r.url);
            const label = r.title || r.url;
            return (
              <li key={`${i}-${r.url}`}>
                {href ? (
                  <a href={href} target="_blank" rel="noopener noreferrer nofollow" aria-label={t('topic.resourceOpen', { title: label })}>
                    {label}
                  </a>
                ) : (
                  <span>{label}</span>
                )}
                {r.title ? <span className="muted small resource-url">{r.url}</span> : null}
                {!readOnly ? (
                  <button type="button" className="btn btn-quiet btn-small" disabled={action.busy} onClick={() => void removeResource(i)}>
                    {t('common.delete')}
                    <span className="sr-only"> — {t('topic.resourceRemove', { title: label })}</span>
                  </button>
                ) : null}
              </li>
            );
          })}
        </ul>
        {!readOnly ? (
          <form onSubmit={(e) => void addResource(e)} className="form">
            <div className="field-row">
              <div className="field">
                <label htmlFor={ids.url}>{t('topic.resourceUrl')}</label>
                <input id={ids.url} type="url" inputMode="url" required value={url} placeholder="https://" onChange={(e) => setUrl(e.target.value)} />
              </div>
              <div className="field">
                <label htmlFor={ids.rtitle}>
                  {t('topic.resourceTitle')} <span className="muted">({t('common.optional')})</span>
                </label>
                <input id={ids.rtitle} value={rtitle} maxLength={160} onChange={(e) => setRtitle(e.target.value)} />
              </div>
            </div>
            <p className="hint">{t('topic.resourceUrlHint')}</p>
            <div className="actions actions-start">
              <button type="submit" className="btn btn-secondary" disabled={action.busy || url.trim() === ''}>
                {t('topic.resourceAdd')}
              </button>
            </div>
          </form>
        ) : null}
      </section>

      <section aria-labelledby="entries-h">
        <div className="row-between">
          <h2 id="entries-h">{t('topic.entries')}</h2>
          {!readOnly ? (
            <button type="button" className="btn btn-secondary btn-small" onClick={() => openNote({ section: topic.section, topic: topic.ref })}>
              {t('topic.addEntry')}
            </button>
          ) : null}
        </div>
        <ErrorNotice error={entries.error} onRetry={entries.reload} />
        {entries.data && entries.data.entries.length === 0 ? <p className="muted">{t('topic.entriesEmpty')}</p> : null}
        {entries.data?.entries.map((e) => (
          <EntryCard key={e.id} entry={e} lookup={outline.lookup} showDate />
        ))}
      </section>

      {!readOnly ? (
        <p className="danger-zone">
          <button type="button" className="btn btn-danger-quiet" onClick={() => setDeleting(true)}>
            {t('topic.delete')}
          </button>
        </p>
      ) : null}
      {deleting ? (
        <ConfirmDialog
          title={t('topic.deleteTitle')}
          body={t('topic.deleteBody', { title: topic.title })}
          confirmLabel={t('topic.deleteConfirm')}
          onClose={() => setDeleting(false)}
          onConfirm={async () => {
            await api.deleteTopic(topic.ref);
            bump();
            navigate(links.section(topic.section));
          }}
        />
      ) : null}
    </div>
  );
}
