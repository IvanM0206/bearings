import { api } from '../api/client';
import { ErrorNotice } from '../components/ErrorNotice';
import { Loading } from '../components/Loading';
import { StatusChip } from '../components/StatusChip';
import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { usePageTitle } from '../lib/usePageTitle';
import { useResource } from '../lib/useResource';

export function RolePathPage() {
  const { t } = useI18n();
  usePageTitle(t('path.title'));
  const res = useResource((s) => api.rolePath(s), []);
  const steps = res.data ?? [];
  const mastered = steps.filter((s) => s.topic.status === 'mastered').length;

  return (
    <div className="page">
      <h1>{t('path.title')}</h1>
      <p className="muted">{t('path.intro')}</p>
      <ErrorNotice error={res.error} onRetry={res.reload} />
      {res.loading && !res.data ? <Loading /> : null}
      {res.data && steps.length === 0 ? <p>{t('path.empty')}</p> : null}
      {steps.length > 0 ? <p className="big-number">{t('path.summary', { mastered, total: steps.length })}</p> : null}
      <ol className="steps">
        {steps.map((s) => (
          <li key={s.topic.ref} className="step">
            <span className="step-num" aria-hidden="true">
              {s.step}
            </span>
            <div className="card step-card">
              <p className="small muted">
                <span className="sr-only">{t('common.stepOfTotal', { step: s.step })}. </span>
                {s.section_title}
              </p>
              <h2>
                <a href={links.topic(s.topic.ref)}>{s.topic.title}</a>
              </h2>
              <p className="meta">
                <StatusChip status={s.topic.status} />
                {s.topic.suggested ? <span className="tag tag-accent">{t('section.suggestedBadge')}</span> : null}
              </p>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
