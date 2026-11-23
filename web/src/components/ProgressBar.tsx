import type { Counts } from '../api/types';
import { useI18n } from '../i18n';

/** A segmented bar: mastered, in progress, pending. The widths are the only style values. */
export function ProgressBar({ counts, large = false }: { counts: Counts; large?: boolean }) {
  const { t } = useI18n();
  const label =
    counts.total === 0
      ? t('progress.empty')
      : t('progress.chart', { mastered: counts.mastered, inProgress: counts.in_progress, pending: counts.pending });
  return (
    <div className={large ? 'bar bar-large' : 'bar'} role="img" aria-label={label}>
      <span className="bar-mastered" style={{ flexGrow: counts.mastered }} />
      <span className="bar-progress" style={{ flexGrow: counts.in_progress }} />
      <span className="bar-pending" style={{ flexGrow: counts.pending }} />
    </div>
  );
}
