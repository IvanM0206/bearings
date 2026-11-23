import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FakeApi, apiError } from './fakeApi';
import { renderApp } from './render';

afterEach(() => vi.unstubAllGlobals());

describe('journal', () => {
  it('groups entries by day, newest first, with their links', async () => {
    new FakeApi().install();
    renderApp('#/journal');
    const headings = await screen.findAllByRole('heading', { level: 2 });
    expect(headings.map((h) => h.textContent)).toEqual(['Tuesday, November 3, 2026', 'Sunday, November 1, 2026']);
    const day = screen.getByRole('region', { name: 'Tuesday, November 3, 2026' });
    expect(within(day).getAllByRole('article')).toHaveLength(2);
    expect(within(day).getByRole('link', { name: 'Risk basics' })).toHaveAttribute('href', '#/topics/technical/risk-basics');
    expect(within(day).getByRole('link', { name: 'Technical' })).toHaveAttribute('href', '#/sections/technical');
  });

  it('filters by section through the address and the API', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal');
    await screen.findByText('Second note');
    await userEvent.selectOptions(screen.getByLabelText('Section'), 'team');
    await waitFor(() => expect(window.location.hash).toBe('#/journal?section=team'));
    expect(await screen.findByText('An older note')).toBeInTheDocument();
    await waitFor(() => expect(screen.queryByText('Second note')).toBeNull());
    expect(api.callsTo('GET', '/journal').some((c) => c.query.get('section') === 'team')).toBe(true);
    await userEvent.click(screen.getByRole('link', { name: 'Clear filters' }));
    expect(await screen.findByText('Second note')).toBeInTheDocument();
  });

  it('sends the date range', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal?from=2026-11-02&to=2026-11-30');
    await screen.findByText('Second note');
    const last = api.callsTo('GET', '/journal').at(-1);
    expect(last?.query.get('from')).toBe('2026-11-02');
    expect(last?.query.get('to')).toBe('2026-11-30');
    expect(screen.getByLabelText('From')).toHaveValue('2026-11-02');
  });

  it('adds an entry for a past day linked to a topic', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal');
    await userEvent.click((await screen.findAllByRole('button', { name: 'Add note' }))[0] as HTMLElement);
    const dialog = await screen.findByRole('dialog', { name: 'New note' });
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'What did you learn?' }), 'Backfilled');
    await userEvent.type(within(dialog).getByLabelText(/^Day/), '2026-10-30');
    await userEvent.selectOptions(within(dialog).getByLabelText(/^Section/), 'technical');
    await userEvent.selectOptions(within(dialog).getByLabelText(/^Topic/), 'technical/risk-basics');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.callsTo('POST', '/journal')).toHaveLength(1));
    expect(api.callsTo('POST', '/journal')[0]?.body).toEqual({ body: 'Backfilled', date: '2026-10-30', section: 'technical', topic: 'technical/risk-basics' });
  });

  it('edits an entry with its version', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal');
    const card = (await screen.findByText('Second note')).closest('article') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Edit/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit note' });
    const box = within(dialog).getByRole('textbox', { name: 'What did you learn?' });
    expect(box).toHaveValue('Second note');
    await userEvent.type(box, ' (edited)');
    await waitFor(() => expect(within(dialog).getByLabelText(/^Section/)).toBeEnabled());
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.callsTo('PATCH', '/journal/bbbbbbbbbb')).toHaveLength(1));
    expect(api.callsTo('PATCH', '/journal/bbbbbbbbbb')[0]?.body).toEqual({ version: '1-bbbbbbbbbb', body: 'Second note (edited)', section: '', topic: '' });
    expect(await screen.findByText('Second note (edited)')).toBeInTheDocument();
  });

  it('deletes only after confirmation', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal');
    const card = (await screen.findByText('Second note')).closest('article') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Delete/ }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this note?' });
    expect(api.callsTo('DELETE', '/journal')).toHaveLength(0);
    await userEvent.click(within(dialog).getByRole('button', { name: 'Delete note' }));
    await waitFor(() => expect(screen.queryByText('Second note')).toBeNull());
    expect(api.callsTo('DELETE', '/journal/bbbbbbbbbb')).toHaveLength(1);
  });

  it('shows the conflict flow when an entry changed elsewhere', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/journal');
    const card = (await screen.findByText('Second note')).closest('article') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Edit/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit note' });
    api.on((c) => (c.method === 'PATCH' ? apiError(409, 'conflict') : undefined));
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'What did you learn?' }), '!');
    await waitFor(() => expect(within(dialog).getByLabelText(/^Section/)).toBeEnabled());
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    const alert = await within(dialog).findByRole('alert');
    expect(alert).toHaveTextContent('changed elsewhere');
    const entry = api.entries.find((e) => e.id === 'bbbbbbbbbb');
    if (entry) entry.body = 'Changed in the terminal';
    await userEvent.click(within(alert).getByRole('button', { name: 'Reload latest version' }));
    await waitFor(() => expect(within(dialog).getByRole('textbox', { name: 'What did you learn?' })).toHaveValue('Changed in the terminal'));
  });

  it('shows hostile entry text as text', async () => {
    const api = new FakeApi();
    api.entries = [api.entry('dddddddddd', '2026-11-04', '<img src=x onerror=alert(1)> [x](javascript:alert(1))', '', '')];
    api.install();
    const { container } = renderApp('#/journal');
    await screen.findByText(/<img src=x/);
    expect(container.querySelector('img')).toBeNull();
    expect(container.querySelector('a[href^="javascript"]')).toBeNull();
  });

  it('opens a single entry', async () => {
    new FakeApi().install();
    renderApp('#/journal/aaaaaaaaaa');
    expect(await screen.findByText('First note about risk')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Back to the journal' })).toBeInTheDocument();
  });
});

describe('role path', () => {
  it('numbers the steps of role topics only', async () => {
    new FakeApi().install();
    renderApp('#/path');
    const main = await screen.findByRole('main');
    const steps = await within(main).findAllByRole('listitem');
    expect(steps).toHaveLength(2);
    expect(within(steps[0] as HTMLElement).getByRole('link', { name: 'Risk basics' })).toBeInTheDocument();
    expect(steps[0]).toHaveTextContent('1');
    expect(steps[1]).toHaveTextContent('2');
    expect(screen.queryByText('Team onboarding')).toBeNull(); // personal topic
    expect(screen.getByText('0 of 2 steps mastered')).toBeInTheDocument();
    expect(within(main).getByRole('list').tagName).toBe('OL');
  });
});

describe('search', () => {
  it('highlights matches safely and links to topics and entries', async () => {
    const api = new FakeApi();
    const first = api.entries[0];
    if (first) first.body = 'Check <b>risk</b> limits first';
    api.install();
    const { container } = renderApp('#/search');
    await userEvent.type(await screen.findByRole('searchbox', { name: 'Search your notebook' }), 'RISK{Enter}');
    expect(await screen.findByText('2 results')).toBeInTheDocument();
    expect(window.location.hash).toBe('#/search?q=RISK');
    const marks = [...container.querySelectorAll('mark')];
    expect(marks.length).toBeGreaterThan(0);
    expect(marks.every((m) => m.textContent?.toLowerCase() === 'risk')).toBe(true);
    // The raw HTML of the snippet is text, not markup.
    expect(container.querySelector('li b')).toBeNull();
    expect(screen.getByText(/Check <b>/)).toBeInTheDocument();
    expect(screen.getByRole('link', { name: /Risk basics/ })).toHaveAttribute('href', '#/topics/technical/risk-basics');
    expect(container.querySelector('a[href="#/journal/aaaaaaaaaa"]')).not.toBeNull();
  });

  it('says when nothing matches', async () => {
    new FakeApi().install();
    renderApp('#/search?q=zzz');
    expect(await screen.findByText('Nothing found for "zzz".')).toBeInTheDocument();
  });

  it('explains an invalid query', async () => {
    const api = new FakeApi();
    api.on((c) => (c.path === '/search' ? apiError(400, 'invalid_query', 'query') : undefined));
    api.install();
    renderApp('#/search?q=a');
    expect(await screen.findByRole('alert')).toHaveTextContent('That search is not valid.');
  });
});
