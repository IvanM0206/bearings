import { useEffect, useRef, type ReactNode } from 'react';
import { useApp } from '../app/AppContext';
import { LANGUAGES, useI18n, type Language } from '../i18n';
import { links, type Route } from '../lib/router';
import { Icon, type IconName } from './Icon';
import type { MessageKey } from '../i18n/en';

interface NavItem {
  icon: IconName;
  href: string;
  label: MessageKey;
  short?: MessageKey;
  active: (first: string | undefined) => boolean;
}

const NAV: NavItem[] = [
  { icon: 'home', href: links.home(), label: 'nav.home', active: (f) => f === undefined },
  { icon: 'sections', href: links.sections(), label: 'nav.sections', active: (f) => f === 'sections' || f === 'topics' },
  { icon: 'journal', href: links.journal(), label: 'nav.journal', active: (f) => f === 'journal' },
  { icon: 'path', href: links.path(), label: 'nav.path', short: 'nav.pathShort', active: (f) => f === 'path' },
  { icon: 'search', href: links.search(), label: 'nav.search', active: (f) => f === 'search' },
];

export function Layout({ route, children }: { route: Route; children: ReactNode }) {
  const { t, lang, setLang } = useI18n();
  const { info, readOnly, openNote } = useApp();
  const main = useRef<HTMLElement>(null);
  const first = route.segments[0];
  const routeKey = route.segments.join('/');
  const initial = useRef(true);

  // Move focus to the page after a navigation, so keyboard and screen reader users start there.
  useEffect(() => {
    if (initial.current) {
      initial.current = false;
      return;
    }
    main.current?.focus({ preventScroll: false });
    window.scrollTo(0, 0);
  }, [routeKey]);

  return (
    <div className="shell">
      <a className="skip-link" href="#main" onClick={(e) => { e.preventDefault(); main.current?.focus(); }}>
        {t('app.skipToContent')}
      </a>
      <nav className="nav" aria-label={t('nav.main')}>
        <a className="brand" href={links.home()}>
          <span className="brand-mark" aria-hidden="true" />
          <span className="brand-name">{t('app.name')}</span>
        </a>
        <ul>
          {NAV.map((n) => (
            <li key={n.href}>
              <a href={n.href} aria-current={n.active(first) ? 'page' : undefined}>
                <Icon name={n.icon} />
                <span className="nav-label">{t(n.label)}</span>
                {n.short ? <span className="nav-short">{t(n.short)}</span> : null}
              </a>
            </li>
          ))}
        </ul>
        <p className="nav-privacy">{t('app.privacy')}</p>
      </nav>
      <div className="column">
        <header className="topbar">
          <p className="notebook-name">
            <span className="sr-only">{t('app.notebook')}: </span>
            {info.name || t('app.name')}
          </p>
          <div className="topbar-tools">
            {readOnly ? <span className="badge">{t('app.readOnlyBadge')}</span> : null}
            <label className="lang">
              <span className="sr-only">{t('nav.language')}</span>
              <select value={lang} onChange={(e) => setLang(e.target.value as Language)}>
                {LANGUAGES.map((l) => (
                  <option key={l} value={l} lang={l}>
                    {t(`language.${l}`)}
                  </option>
                ))}
              </select>
            </label>
          </div>
        </header>
        <main id="main" ref={main} tabIndex={-1}>
          {readOnly ? (
            <p className="notice notice-info" role="status">
              {t('app.readOnly')}
            </p>
          ) : null}
          {children}
        </main>
      </div>
      {!readOnly ? (
        <button type="button" className="fab" onClick={() => openNote()}>
          <Icon name="plus" />
          <span>{t('nav.addNote')}</span>
        </button>
      ) : null}
    </div>
  );
}
