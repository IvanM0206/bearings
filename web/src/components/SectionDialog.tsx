import { useId, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Section } from '../api/types';
import { useApp } from '../app/AppContext';
import { useI18n } from '../i18n';
import { useAction } from '../lib/useAction';
import { ErrorNotice } from './ErrorNotice';
import { Modal } from './Modal';

/** Adds a section, or renames/edits one when `section` is given. */
export function SectionDialog({ section, onClose }: { section?: Section; onClose: () => void }) {
  const { t } = useI18n();
  const { bump } = useApp();
  const action = useAction();
  const ids = { title: useId(), description: useId() };
  const [current, setCurrent] = useState(section);
  const [title, setTitle] = useState(section?.title ?? '');
  const [description, setDescription] = useState(section?.description ?? '');

  const submit = async (e: FormEvent) => {
    e.preventDefault();
    const ok = await action.run(async () => {
      if (current) await api.updateSection(current.slug, { version: current.version, title, description });
      else await api.addSection({ title, description });
      bump();
    });
    if (ok) onClose();
  };

  const reload = async () => {
    if (!current) return;
    await action.run(async () => {
      const fresh = (await api.sections()).find((s) => s.slug === current.slug);
      if (!fresh) throw new Error('gone');
      setCurrent(fresh);
      setTitle(fresh.title);
      setDescription(fresh.description);
    });
  };

  return (
    <Modal title={current ? t('sections.renameTitle') : t('sections.addTitle')} onClose={onClose}>
      <form onSubmit={(e) => void submit(e)} className="form">
        <div className="field">
          <label htmlFor={ids.title}>{t('sections.titleLabel')}</label>
          <input id={ids.title} value={title} required maxLength={80} onChange={(e) => setTitle(e.target.value)} data-autofocus />
        </div>
        <div className="field">
          <label htmlFor={ids.description}>
            {t('sections.descriptionLabel')} <span className="muted">({t('common.optional')})</span>
          </label>
          <textarea id={ids.description} rows={4} value={description} onChange={(e) => setDescription(e.target.value)} />
        </div>
        <ErrorNotice error={action.error} onReload={() => void reload()} />
        <div className="actions">
          <button type="button" className="btn btn-secondary" onClick={onClose}>
            {t('common.cancel')}
          </button>
          <button type="submit" className="btn btn-primary" disabled={action.busy || title.trim() === ''}>
            {action.busy ? t('common.saving') : t('common.save')}
          </button>
        </div>
      </form>
    </Modal>
  );
}
