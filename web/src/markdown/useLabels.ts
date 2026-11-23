import { useMemo } from 'react';
import { useI18n } from '../i18n';
import type { MarkdownLabels } from './Markdown';

export function useMarkdownLabels(): MarkdownLabels {
  const { t } = useI18n();
  return useMemo(() => ({ image: t('common.imageAlt') }), [t]);
}
