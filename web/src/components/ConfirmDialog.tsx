import { useAction } from '../lib/useAction';
import { useI18n } from '../i18n';
import { ErrorNotice } from './ErrorNotice';
import { Modal } from './Modal';

interface Props {
  title: string;
  body: string;
  confirmLabel: string;
  onConfirm: () => Promise<unknown>;
  onClose: () => void;
}

/** In-page confirmation for destructive actions. Cancel has the initial focus. */
export function ConfirmDialog({ title, body, confirmLabel, onConfirm, onClose }: Props) {
  const { t } = useI18n();
  const action = useAction();
  const confirm = async () => {
    if (await action.run(onConfirm)) onClose();
  };
  return (
    <Modal title={title} onClose={onClose} role="alertdialog">
      <p>{body}</p>
      <ErrorNotice error={action.error} />
      <div className="actions">
        <button type="button" className="btn btn-secondary" onClick={onClose} data-autofocus>
          {t('common.cancel')}
        </button>
        <button type="button" className="btn btn-danger" onClick={() => void confirm()} disabled={action.busy}>
          {action.busy ? t('common.saving') : confirmLabel}
        </button>
      </div>
    </Modal>
  );
}
