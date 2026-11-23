import { useState } from 'react';
import { api } from '../api/client';
import type { Entry } from '../api/types';
import { useApp } from '../app/AppContext';
import { useI18n } from '../i18n';
import type { Lookup } from '../lib/useOutline';
import { links } from '../lib/router';
import { Markdown } from '../markdown/Markdown';
import { useMarkdownLabels } from '../markdown/useLabels';
import { ConfirmDialog } from './ConfirmDialog';
import { NoteDialog } from './NoteDialog';

interface Props {
  entry: Entry;
  lookup: Lookup;
  compact?: boolean;
  /** Called after the entry was deleted. */
  onDeleted?: () => void;
  showDate?: boolean;
}

export function EntryCard({ entry, lookup, compact = false, onDeleted, showDate = false }: Props) {
  const { t, formatDay } = useI18n();
  const { readOnly, bump, flash } = useApp();
  const labels = useMarkdownLabels();
  const [editing, setEditing] = useState(false);
  const [deleting, setDeleting] = useState(false);

  const sectionTitle = entry.section ? lookup.sectionTitle(entry.section) : undefined;
  const topicTitle = entry.topic ? lookup.topicTitle(entry.topic) : undefined;
  const brokenLink = (entry.section && sectionTitle === undefined) || (entry.topic && topicTitle === undefined);

  return (
    <article className={compact ? 'card entry entry-compact' : 'card entry'}>
      {showDate ? <p className="entry-date">{formatDay(entry.date)}</p> : null}
      <Markdown source={entry.body} labels={labels} className="entry-body" />
      <div className="entry-meta">
        {sectionTitle !== undefined ? (
          <a className="tag" href={links.section(entry.section)}>
            {sectionTitle}
          </a>
        ) : null}
        {topicTitle !== undefined ? (
          <a className="tag" href={links.topic(entry.topic)}>
            {topicTitle}
          </a>
        ) : null}
        {brokenLink ? <span className="tag tag-muted">{t('journal.unlinked')}</span> : null}
        {!readOnly ? (
          <span className="entry-actions">
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setEditing(true)}>
              {t('common.edit')}
              <span className="sr-only"> — {t('journal.edit')}</span>
            </button>
            <button type="button" className="btn btn-quiet btn-small" onClick={() => setDeleting(true)}>
              {t('common.delete')}
              <span className="sr-only"> — {t('journal.delete')}</span>
            </button>
          </span>
        ) : null}
      </div>
      {editing ? <NoteDialog entry={entry} onClose={() => setEditing(false)} /> : null}
      {deleting ? (
        <ConfirmDialog
          title={t('journal.deleteTitle')}
          body={t('journal.deleteBody')}
          confirmLabel={t('journal.deleteConfirm')}
          onClose={() => setDeleting(false)}
          onConfirm={async () => {
            await api.deleteEntry(entry.id);
            bump();
            flash(t('journal.deleted'));
            onDeleted?.();
          }}
        />
      ) : null}
    </article>
  );
}
