import { useEffect, useId, useRef, type FormEvent, type ReactNode } from 'react';
import { useI18n } from '../i18n';

interface ModalProps {
  title: string;
  onClose: () => void;
  children: ReactNode;
  /** `alert` for destructive confirmations. */
  role?: 'dialog' | 'alertdialog';
}

/** A modal dialog built on the native <dialog> element (focus trap, Escape, inert background). */
export function Modal({ title, onClose, children, role = 'dialog' }: ModalProps) {
  const { t } = useI18n();
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();
  const closeRef = useRef(onClose);
  useEffect(() => {
    closeRef.current = onClose;
  });

  useEffect(() => {
    const dlg = ref.current;
    if (!dlg) return;
    if (typeof dlg.showModal === 'function') dlg.showModal();
    else dlg.setAttribute('open', '');
    dlg.querySelector<HTMLElement>('[data-autofocus]')?.focus();
    return () => {
      if (typeof dlg.close === 'function' && dlg.open) dlg.close();
    };
  }, []);

  return (
    // The click handler only adds a mouse shortcut (click on the backdrop);
    // keyboard users close the dialog with Escape or the Close button.
    // eslint-disable-next-line jsx-a11y/no-noninteractive-element-interactions, jsx-a11y/click-events-have-key-events
    <dialog
      ref={ref}
      className="modal"
      role={role}
      aria-labelledby={titleId}
      onCancel={(e: FormEvent) => {
        e.preventDefault();
        closeRef.current();
      }}
      onClick={(e) => {
        if (e.target === ref.current) closeRef.current();
      }}
    >
      <div className="modal-body">
        <div className="modal-head">
          <h2 id={titleId}>{title}</h2>
          <button type="button" className="btn btn-quiet btn-icon" onClick={onClose} aria-label={t('common.close')}>
            <span aria-hidden="true">×</span>
          </button>
        </div>
        {children}
      </div>
    </dialog>
  );
}
