import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FakeApi } from './fakeApi';
import { renderApp } from './render';

afterEach(() => vi.unstubAllGlobals());

describe('home', () => {
  it('shows the progress overview with counts by status and per section', async () => {
    const api = new FakeApi();
    api.install();
    renderApp();
    expect(await screen.findByText('1 of 3 topics mastered')).toBeInTheDocument();
    expect(screen.getByText('Role path: 0 of 2 mastered')).toBeInTheDocument();
    expect(screen.getByText('3 journal entries')).toBeInTheDocument();
    expect(screen.getByText('1 topic comes from a template and is waiting for your review.')).toBeInTheDocument();
    const bars = screen.getAllByRole('img', { name: /mastered, .* in progress, .* pending/ });
    expect(bars[0]).toHaveAccessibleName('1 mastered, 1 in progress, 1 pending');
    const list = screen.getByRole('heading', { name: 'By section' }).nextElementSibling as HTMLElement;
    expect(within(list).getByRole('link', { name: 'Technical' })).toHaveAttribute('href', '#/sections/technical');
    expect(within(list).getByRole('link', { name: 'Team' })).toBeInTheDocument();
  });

  it('saves a quick entry with a section and a topic', async () => {
    const api = new FakeApi();
    api.install();
    renderApp();
    const box = await screen.findByRole('textbox', { name: 'What did you learn today?' });
    const save = screen.getByRole('button', { name: 'Save note' });
    expect(save).toBeDisabled();
    await userEvent.type(box, 'Fees matter');
    await userEvent.selectOptions(screen.getByLabelText(/^Section/), 'technical');
    await userEvent.selectOptions(screen.getByLabelText(/^Topic/), 'technical/backtesting');
    await userEvent.click(save);
    expect(await screen.findByText(/Note saved\./)).toBeInTheDocument();
    expect(api.callsTo('POST', '/journal')[0]?.body).toEqual({ body: 'Fees matter', section: 'technical', topic: 'technical/backtesting' });
    expect(box).toHaveValue('');
    // The recent notes list reloads.
    expect(await screen.findByText('Fees matter')).toBeInTheDocument();
  });

  it('opens the add-note dialog from the floating button', async () => {
    const api = new FakeApi();
    api.install();
    renderApp();
    await userEvent.click(await screen.findByRole('button', { name: 'Add note' }));
    const dialog = await screen.findByRole('dialog', { name: 'New note' });
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'What did you learn?' }), 'From the dialog');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull());
    expect(api.callsTo('POST', '/journal')[0]?.body).toEqual({ body: 'From the dialog' });
    expect(await screen.findByText('Note saved.')).toBeInTheDocument();
  });

  it('keeps the text and explains when saving fails', async () => {
    const api = new FakeApi();
    api.on((c) => (c.method === 'POST' && c.path === '/journal' ? { status: 422, body: { error: { code: 'too_long', message: 'x', field: 'body' } } } : undefined));
    api.install();
    renderApp();
    const box = await screen.findByRole('textbox', { name: 'What did you learn today?' });
    await userEvent.type(box, 'Long text');
    await userEvent.click(screen.getByRole('button', { name: 'Save note' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('too long');
    expect(box).toHaveValue('Long text');
  });
});
