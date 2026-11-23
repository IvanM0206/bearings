import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FakeApi, apiError } from './fakeApi';
import { renderApp } from './render';

afterEach(() => vi.unstubAllGlobals());

describe('app shell', () => {
  it('asks the person to reopen the printed URL when the session is not signed in', async () => {
    const api = new FakeApi();
    api.on((c) => (c.path === '/info' ? apiError(401, 'unauthorized') : undefined));
    api.install();
    renderApp();
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('bearings serve');
    expect(alert).toHaveTextContent('/auth?token=');
  });

  it('shows a retry when the server cannot be reached', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))));
    renderApp();
    expect(await screen.findByRole('alert')).toHaveTextContent('cannot reach the local server');
    expect(screen.getByRole('button', { name: 'Try again' })).toBeInTheDocument();
  });

  it('navigates with the keyboard-reachable tab bar and marks the current page', async () => {
    const api = new FakeApi();
    api.install();
    renderApp();
    const nav = await screen.findByRole('navigation', { name: 'Main' });
    expect(within(nav).getByRole('link', { name: /Home/ })).toHaveAttribute('aria-current', 'page');
    await userEvent.click(within(nav).getByRole('link', { name: /Journal/ }));
    expect(await screen.findByRole('heading', { level: 1, name: 'Journal' })).toBeInTheDocument();
    expect(within(nav).getByRole('link', { name: /Journal/ })).toHaveAttribute('aria-current', 'page');
  });

  it('shows a not-found page for unknown routes', async () => {
    new FakeApi().install();
    renderApp('#/nothing/here');
    expect(await screen.findByRole('heading', { name: 'Page not found' })).toBeInTheDocument();
  });

  it('uses the notebook language, and lets the person choose and remembers it', async () => {
    const api = new FakeApi();
    api.info.language = 'es';
    api.install();
    renderApp();
    expect(await screen.findByRole('heading', { level: 1, name: /Te damos la bienvenida/ })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('es');

    await userEvent.selectOptions(screen.getByRole('combobox', { name: 'Idioma' }), 'en');
    expect(await screen.findByRole('heading', { level: 1, name: /Welcome back/ })).toBeInTheDocument();
    expect(document.documentElement.lang).toBe('en');
    expect(localStorage.getItem('bearings.language')).toBe('en');
  });

  it('keeps the stored language over the notebook language', async () => {
    localStorage.setItem('bearings.language', 'en');
    const api = new FakeApi();
    api.info.language = 'es';
    api.install();
    renderApp();
    expect(await screen.findByRole('heading', { level: 1, name: /Welcome back/ })).toBeInTheDocument();
  });

  it('works when localStorage is blocked', async () => {
    const spy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      throw new DOMException('blocked', 'SecurityError');
    });
    const api = new FakeApi();
    api.install();
    renderApp();
    await userEvent.selectOptions(await screen.findByRole('combobox', { name: 'Language' }), 'es');
    expect(await screen.findByRole('heading', { level: 1, name: /Te damos la bienvenida/ })).toBeInTheDocument();
    spy.mockRestore();
  });

  it('hides every edit control when the notebook is read-only', async () => {
    const api = new FakeApi();
    api.info.read_only = true;
    api.install();
    renderApp();
    await screen.findByRole('heading', { level: 1, name: /Welcome back/ });
    expect(screen.getByText('Read-only', { selector: '.badge' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Add note' })).toBeNull();
    expect(screen.queryByRole('textbox')).toBeNull(); // no quick entry

    window.location.hash = '#/sections';
    await screen.findByRole('heading', { level: 1, name: 'Sections' });
    expect(screen.queryByRole('button', { name: /Add a section/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Delete/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Move up/ })).toBeNull();

    window.location.hash = '#/topics/technical/risk-basics';
    await screen.findByRole('heading', { level: 1, name: 'Risk basics' });
    expect(screen.queryByRole('button', { name: /Edit notes/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Delete topic/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /Add resource/ })).toBeNull();
    for (const radio of screen.getAllByRole('radio')) expect(radio).toBeDisabled();

    window.location.hash = '#/journal';
    await screen.findByText('First note about risk');
    expect(screen.queryByRole('button', { name: /^Edit/ })).toBeNull();
    expect(screen.queryByRole('button', { name: /^Delete/ })).toBeNull();
    expect(api.calls.filter((c) => c.method !== 'GET')).toHaveLength(0);
  });

  it('shows the server answer in the right words when a write is refused as read-only', async () => {
    const api = new FakeApi();
    api.on((c) => (c.method === 'POST' && c.path === '/journal' ? apiError(403, 'read_only') : undefined));
    api.install();
    renderApp();
    await userEvent.type(await screen.findByRole('textbox', { name: 'What did you learn today?' }), 'x');
    await userEvent.click(screen.getByRole('button', { name: 'Save note' }));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('read-only'));
  });
});
