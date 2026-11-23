import { render } from '@testing-library/react';
import { App } from '../App';
import { I18nProvider, type Language } from '../i18n';

export function renderApp(hash = '', lang?: Language) {
  window.location.hash = hash;
  return render(
    <I18nProvider {...(lang ? { initial: lang } : {})}>
      <App />
    </I18nProvider>,
  );
}
