import { useI18n } from '../i18n';
import { links } from '../lib/router';
import { usePageTitle } from '../lib/usePageTitle';

export function NotFoundPage() {
  const { t } = useI18n();
  usePageTitle(t('notFound.title'));
  return (
    <div className="page">
      <h1>{t('notFound.title')}</h1>
      <p>{t('notFound.body')}</p>
      <p>
        <a className="btn btn-secondary" href={links.home()}>
          {t('notFound.home')}
        </a>
      </p>
    </div>
  );
}
