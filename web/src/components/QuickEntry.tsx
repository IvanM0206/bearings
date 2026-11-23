import { useId, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import { useApp } from '../app/AppContext';
import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { useAction } from '../lib/useAction';
import { useOutline } from '../lib/useOutline';
import { ErrorNotice } from './ErrorNotice';

/** The "What did you learn today?" box of the home screen. */
export function QuickEntry() {
  const { t } = useI18n();
  const { bump } = useApp();
  const outline = useOutline();
  const action = useAction();
  const ids = { body: useId(), section: useId(), topic: useId() };
  const [body, setBody] = useState('');
  const [section, setSection] = useState('');
  const [topic, setTopic] = useState('');
  const [saved, setSaved] = useState(false);

  const sections = outline.data ?? [];
  const topics = sections.find((o) => o.section.slug === section)?.topics ?? [];

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const ok = await action.run(async () => {
      await api.addEntry({ body, ...(section ? { section } : {}), ...(topic ? { topic } : {}) });
      bump();
    });
    if (ok) {
      setBody('');
      setSaved(true);
    }
  };

  return (
    <section className="card quick" aria-labelledby={ids.body + '-h'}>
      <h2 id={ids.body + '-h'}>{t('home.quickTitle')}</h2>
      <p className="muted">{t('home.quickHint')}</p>
      <form onSubmit={(e) => void submit(e)} className="form">
        <div className="field">
          <label htmlFor={ids.body} className="sr-only">
            {t('home.quickTitle')}
          </label>
          <textarea
            id={ids.body}
            rows={3}
            value={body}
            placeholder={t('home.quickPlaceholder')}
            onChange={(e) => {
              setBody(e.target.value);
              setSaved(false);
            }}
          />
        </div>
        <div className="field-row">
          <div className="field">
            <label htmlFor={ids.section}>
              {t('journal.sectionLabel')} <span className="muted">({t('common.optional')})</span>
            </label>
            <select
              id={ids.section}
              value={section}
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
            <select id={ids.topic} value={topic} onChange={(e) => setTopic(e.target.value)} disabled={section === ''}>
              <option value="">{t('journal.noTopic')}</option>
              {topics.map((x) => (
                <option key={x.ref} value={x.ref}>
                  {x.title}
                </option>
              ))}
            </select>
          </div>
        </div>
        <ErrorNotice error={action.error} />
        <div className="actions actions-start">
          <button type="submit" className="btn btn-primary" disabled={action.busy || body.trim() === ''}>
            {action.busy ? t('common.saving') : t('home.quickSave')}
          </button>
          {saved ? (
            <p className="inline-ok" role="status">
              {t('home.quickSaved')} <a href={links.journal()}>{t('home.quickViewJournal')}</a>
            </p>
          ) : null}
        </div>
      </form>
    </section>
  );
}
