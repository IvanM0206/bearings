import { useId, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Entry } from '../api/types';
import { useApp, type NotePrefill } from '../app/AppContext';
import { useI18n } from '../i18n';
import { useAction } from '../lib/useAction';
import { useResource } from '../lib/useResource';
import { ErrorNotice } from './ErrorNotice';
import { MarkdownEditor } from './MarkdownEditor';
import { Modal } from './Modal';

interface Props {
  entry?: Entry;
  prefill?: NotePrefill;
  onClose: () => void;
  onSaved?: (entry: Entry) => void;
}

/** Creates or edits a journal entry. */
export function NoteDialog({ entry, prefill, onClose, onSaved }: Props) {
  const { t } = useI18n();
  const { bump, flash } = useApp();
  const outline = useResource((s) => api.outline(s), []);
  const action = useAction();
  const ids = { date: useId(), section: useId(), topic: useId() };

  const [current, setCurrent] = useState(entry);
  const [body, setBody] = useState(entry?.body ?? '');
  const [date, setDate] = useState('');
  const [section, setSection] = useState(entry?.section ?? prefill?.section ?? prefill?.topic?.split('/')[0] ?? '');
  const [topic, setTopic] = useState(entry?.topic ?? prefill?.topic ?? '');

  const sections = outline.data ?? [];
  const known = sections.some((o) => o.section.slug === section);
  const topics = sections.find((o) => o.section.slug === section)?.topics ?? [];
  // Links are only sent once the outline is known: until then they stay as they are.
  const linksReady = outline.data !== undefined;
  const topicOk = known && topics.some((x) => x.ref === topic);
  const links = linksReady
    ? { section: known ? section : '', topic: topicOk ? topic : '' }
    : {};

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const ok = await action.run(async () => {
      const saved = current
        ? await api.updateEntry(current.id, { version: current.version, body, ...links })
        : await api.addEntry({
            body,
            ...(date ? { date } : {}),
            ...(links.section ? { section: links.section } : {}),
            ...(links.topic ? { topic: links.topic } : {}),
          });
      bump();
      flash(t('journal.saved'));
      onSaved?.(saved);
    });
    if (ok) onClose();
  };

  const reload = async () => {
    if (!current) return;
    await action.run(async () => {
      const fresh = await api.entry(current.id);
      setCurrent(fresh);
      setBody(fresh.body);
      setSection(fresh.section);
      setTopic(fresh.topic);
    });
  };

  return (
    <Modal title={current ? t('journal.editTitle') : t('journal.newTitle')} onClose={onClose}>
      <form onSubmit={(e) => void submit(e)} className="form">
        <MarkdownEditor
          label={t('journal.bodyLabel')}
          value={body}
          onChange={setBody}
          rows={6}
          required
         
          initialFocus
          placeholder={t('home.quickPlaceholder')}
        />
        {!current ? (
          <div className="field">
            <label htmlFor={ids.date}>
              {t('journal.dateLabel')} <span className="muted">({t('common.optional')})</span>
            </label>
            <input id={ids.date} type="date" value={date} onChange={(e) => setDate(e.target.value)} />
            <p className="hint">{t('journal.dateHint')}</p>
          </div>
        ) : null}
        <div className="field-row">
          <div className="field">
            <label htmlFor={ids.section}>
              {t('journal.sectionLabel')} <span className="muted">({t('common.optional')})</span>
            </label>
            <select
              id={ids.section}
              value={known ? section : ''}
              onChange={(e) => {
                setSection(e.target.value);
                setTopic('');
              }}
            >
              <option value="">{t('journal.noLink')}</option>
              {sections.map((o) => (
                <option key={o.section.slug} value={o.section.slug}>
                  {o.section.title}
                </option>
              ))}
            </select>
          </div>
          <div className="field">
            <label htmlFor={ids.topic}>
              {t('journal.topicLabel')} <span className="muted">({t('common.optional')})</span>
            </label>
            <select id={ids.topic} value={topicOk ? topic : ''} onChange={(e) => setTopic(e.target.value)} disabled={!known}>
              <option value="">{t('journal.noTopic')}</option>
              {topics.map((x) => (
                <option key={x.ref} value={x.ref}>
                  {x.title}
                </option>
              ))}
            </select>
          </div>
        </div>
        <ErrorNotice error={action.error} onReload={() => void reload()} />
        <div className="actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn btn-primary" disabled={action.busy || body.trim() === ''}>
            {action.busy ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
