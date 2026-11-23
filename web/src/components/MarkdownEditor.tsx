import { useEffect, useId, useRef, useState } from 'react';
import { Markdown } from '../markdown/Markdown';
import { useI18n } from '../i18n';
import { useMarkdownLabels } from '../markdown/useLabels';

interface Props {
  label: string;
  value: string;
  onChange: (value: string) => void;
  rows?: number;
  placeholder?: string;
  required?: boolean;
  initialFocus?: boolean;
}

/** A textarea with a Write / Preview switch. */
export function MarkdownEditor({ label, value, onChange, rows = 8, placeholder, required, initialFocus }: Props) {
  const { t } = useI18n();
  const labels = useMarkdownLabels();
  const [preview, setPreview] = useState(false);
  const id = useId();
  const area = useRef<HTMLTextAreaElement>(null);
  useEffect(() => {
    if (initialFocus) area.current?.focus();
  }, [initialFocus]);
  return (
    <div className="field">
      <div className="field-head">
        <label htmlFor={id}>{label}</label>
        <div className="seg" role="group" aria-label={t('common.preview')}>
          <button type="button" className="seg-btn" aria-pressed={!preview} onClick={() => setPreview(false)}>
            {t('common.write')}
          </button>
          <button type="button" className="seg-btn" aria-pressed={preview} onClick={() => setPreview(true)}>
            {t('common.preview')}
          </button>
        </div>
      </div>
      {preview ? (
        <div className="preview" aria-live="polite">
          {value.trim() === '' ? <p className="muted">{t('common.nothingToPreview')}</p> : <Markdown source={value} labels={labels} />}
        </div>
      ) : (
        <textarea
          id={id}
          ref={area}
          value={value}
          rows={rows}
          required={required}
          placeholder={placeholder}
          onChange={(e) => onChange(e.target.value)}
          {...(initialFocus ? { 'data-autofocus': true } : {})}
        />
      )}
      <p className="hint">{t('common.markdownHint')}</p>
    </div>
  );
}
