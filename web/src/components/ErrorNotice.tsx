import { ApiError } from '../api/client';
import { describeError, useI18n } from '../i18n';

interface Props {
  error: unknown;
  /** Offered for conflicts: loads the latest version of what changed elsewhere. */
  onReload?: () => void;
  onRetry?: () => void;
}

/** A translated, content-free description of what went wrong. */
export function ErrorNotice({ error, onReload, onRetry }: Props) {
  const i18n = useI18n();
  if (error === undefined || error === null) return null;
  const kind = error instanceof ApiError ? error.kind : undefined;
  return (
    <div className="notice notice-error" role="alert">
      <p>{describeError(i18n, error)}</p>
      {kind === 'conflict' && onReload ? (
        <button type="button" className="btn btn-secondary" onClick={onReload}>
          {i18n.t('error.conflictReload')}
        </button>
      ) : null}
      {kind !== 'conflict' && kind !== 'unauthorized' && kind !== 'read_only' && onRetry ? (
        <button type="button" className="btn btn-secondary" onClick={onRetry}>
          {i18n.t('app.retry')}
        </button>
      ) : null}
    </div>
  );
}
