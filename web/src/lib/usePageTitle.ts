import { useEffect } from 'react';
import { useI18n } from '../i18n';

/** Sets the document title ("Page · Bearings") for the current screen. */
export function usePageTitle(title: string | undefined): void {
  const { t } = useI18n();
  useEffect(() => {
    document.title = title ? `${title} · ${t('app.name')}` : t('app.name');
  }, [title, t]);
}
