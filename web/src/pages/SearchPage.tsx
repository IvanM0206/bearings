import { useId, useState, type FormEvent } from 'react';
import { api } from '../api/client';
import type { Hit } from '../api/types';
import { ErrorNotice } from '../components/ErrorNotice';
import { Highlight } from '../components/Highlight';
import { Loading } from '../components/Loading';
import { useI18n } from '../i18n';
import { links, navigate } from '../lib/router';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';

const MAX_QUERY = 200;

function hitLink(h: Hit): string {
  if (h.kind === 'entry') return links.entry(h.entry_id);
  if (h.kind === 'topic') return links.topic(h.topic);
  return links.section(h.section);
}

export function SearchPage({ query }: { query: URLSearchParams }) {
  const { t, tn, formatDay } = useI18n();
  usePageTitle(t('search.title'));
  const q = (query.get('q') ?? '').trim();
  const [text, setText] = useState(q);
  const inputId = useId();
  // Keep the box in step with the address (back button, links).
  const [seen, setSeen] = useState(q);
  if (seen !== q) {
    setSeen(q);
    setText(q);
  }

  const res = useResource(
    (s) => (q === '' ? Promise.resolve<Hit[]>([]) : api.search(q, s)),
    [q],
  );

  const submit = (e: FormEvent) => {
    e.preventDefault();
    navigate(links.search(text.trim()));
  };
  const hits = res.data ?? [];

  return (
    <div className="page">
      <h1>{t('search.title')}</h1>
      <form onSubmit={submit} role="search" className="search-form">
        <label htmlFor={inputId}>{t('search.label')}</label>
        <div className="search-row">
          <input
            id={inputId}
            type="search"
            value={text}
            maxLength={MAX_QUERY}
            placeholder={t('search.placeholder')}
            onChange={(e) => setText(e.target.value)}
          />
          <button type="submit" className="btn btn-primary" disabled={text.trim() === ''}>
            {t('search.submit')}
          </button>
        </div>
        <p className="hint">{t('search.hint')}</p>
      </form>

      <ErrorNotice error={res.error} onRetry={res.reload} />
      {q !== '' && res.loading && !res.data ? <Loading /> : null}
      {q !== '' && res.data ? (
        <>
          <p role="status" className="muted">
            {hits.length === 0 ? t('search.none', { query: q }) : tn('search.results', hits.length)}
          </p>
          <ul className="plain stack">
            {hits.map((h, i) => (
              <li key={`${h.kind}-${h.entry_id || h.topic || h.section}-${i}`} className="card hit">
                <p className="meta">
                  <span className="tag">{t(`search.kind.${h.kind}`)}</span>
                  <span className="muted small">{t('search.matchIn', { field: t(`search.field.${h.field}`) })}</span>
                </p>
                <h2>
                  <a href={hitLink(h)}>
                    {h.kind === 'entry' ? formatDay(h.date) : <Highlight text={h.title} query={q} />}
                  </a>
                </h2>
                {h.snippet && h.field !== 'title' ? (
                  <p className="snippet">
                    <Highlight text={h.snippet} query={q} />
                  </p>
                ) : null}
              </li>
            ))}
          </ul>
        </>
      ) : null}
    </div>
  );
}
