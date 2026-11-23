import { useI18n } from '../i18n';

export function Loading() {
  const { t } = useI18n();
  return (
    <p className="loading" role="status">
      {t('app.loading')}
    </p>
  );
}
