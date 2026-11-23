import type { Status } from '../api/types';
import { useI18n } from '../i18n';

/** Status shown with text and a shape, never by colour alone. */
export function StatusChip({ status }: { status: Status }) {
  const { t } = useI18n();
  return <span className={`chip chip-${status}`}>{t(`status.${status}`)}</span>;
}
