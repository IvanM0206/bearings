import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FakeApi, apiError } from './fakeApi';
import { renderApp } from './render';

afterEach(() => vi.unstubAllGlobals());

describe('sections', () => {
  it('lists sections with counts and renders descriptions as Markdown', async () => {
    new FakeApi().install();
    renderApp('#/sections');
    const first = (await screen.findByRole('heading', { name: 'Technical' })).closest('li') as HTMLElement;
    expect(within(first).getByText('work', { selector: 'strong' })).toBeInTheDocument();
    expect(within(first).getByText(/2 topics/)).toBeInTheDocument();
  });

  it('adds a section from a dialog', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/sections');
    await userEvent.click(await screen.findByRole('button', { name: 'Add a section' }));
    const dialog = await screen.findByRole('dialog', { name: 'New section' });
    await userEvent.type(within(dialog).getByRole('textbox', { name: 'Section name' }), 'Processes');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    expect(await screen.findByRole('heading', { name: 'Processes' })).toBeInTheDocument();
    expect(api.callsTo('POST', '/sections')[0]?.body).toEqual({ title: 'Processes', description: '' });
  });

  it('renames a section sending the version it read', async () => {
    const api = new FakeApi();
    api.on((c) => (c.method === 'PATCH' && c.path === '/sections/team' ? { body: { ...api.sections[1] } } : undefined));
    api.install();
    renderApp('#/sections');
    const card = (await screen.findByRole('heading', { name: 'Team' })).closest('li') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Edit/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Edit section' });
    const name = within(dialog).getByRole('textbox', { name: 'Section name' });
    await userEvent.clear(name);
    await userEvent.type(name, 'People');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.callsTo('PATCH', '/sections/team')).toHaveLength(1));
    expect(api.callsTo('PATCH', '/sections/team')[0]?.body).toEqual({ version: '1-bb', title: 'People', description: '' });
  });

  it('asks for confirmation before deleting, and Cancel deletes nothing', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/sections');
    const card = (await screen.findByRole('heading', { name: 'Team' })).closest('li') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Delete/ }));
    const dialog = await screen.findByRole('alertdialog', { name: 'Delete this section?' });
    expect(dialog).toHaveTextContent('The section "Team" and its 1 topic will be deleted');
    await userEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));
    expect(screen.queryByRole('alertdialog')).toBeNull();
    expect(api.callsTo('DELETE', '/sections')).toHaveLength(0);

    await userEvent.click(within(card).getByRole('button', { name: /Delete/ }));
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete section' }));
    await waitFor(() => expect(screen.queryByRole('heading', { name: 'Team' })).toBeNull());
    expect(api.callsTo('DELETE', '/sections/team')).toHaveLength(1);
  });

  it('shows why a delete failed and keeps the dialog open', async () => {
    const api = new FakeApi();
    api.on((c) => (c.method === 'DELETE' ? apiError(409, 'section_not_deletable') : undefined));
    api.install();
    renderApp('#/sections');
    const card = (await screen.findByRole('heading', { name: 'Team' })).closest('li') as HTMLElement;
    await userEvent.click(within(card).getByRole('button', { name: /Delete/ }));
    await userEvent.click(within(await screen.findByRole('alertdialog')).getByRole('button', { name: 'Delete section' }));
    expect(await screen.findByRole('alert')).toHaveTextContent('files that are not valid topics');
    expect(screen.getByRole('alertdialog')).toBeInTheDocument();
  });

  it('reorders sections and disables impossible moves', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/sections');
    const first = (await screen.findByRole('heading', { name: 'Technical' })).closest('li') as HTMLElement;
    const last = screen.getByRole('heading', { name: 'Team' }).closest('li') as HTMLElement;
    expect(within(first).getByRole('button', { name: /Move up/ })).toBeDisabled();
    expect(within(last).getByRole('button', { name: /Move down/ })).toBeDisabled();
    await userEvent.click(within(last).getByRole('button', { name: /Move up/ }));
    await waitFor(() => expect(api.callsTo('POST', '/sections/team/reorder')).toHaveLength(1));
    expect(api.callsTo('POST', '/sections/team/reorder')[0]?.body).toEqual({ delta: -1 });
  });
});

describe('section page', () => {
  it('lists topics with status chips and filters by status', async () => {
    new FakeApi().install();
    renderApp('#/sections/technical');
    expect(await screen.findByRole('heading', { level: 1, name: 'Technical' })).toBeInTheDocument();
    const list = screen.getByRole('list', { name: 'Topics in Technical' });
    expect(within(list).getAllByRole('listitem')).toHaveLength(2);
    expect(within(list).getByText('In progress', { selector: '.chip' })).toBeInTheDocument();
    expect(within(list).getByText('Suggested')).toBeInTheDocument();

    await userEvent.click(screen.getByRole('button', { name: 'Pending' }));
    expect(within(screen.getByRole('list', { name: 'Topics in Technical' })).getAllByRole('listitem')).toHaveLength(1);
    expect(screen.getByRole('button', { name: 'Pending' })).toHaveAttribute('aria-pressed', 'true');
    await userEvent.click(screen.getByRole('button', { name: 'Mastered' }));
    expect(screen.getByText('No topics with this status.')).toBeInTheDocument();
  });

  it('advances a topic and adds a new one', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/sections/technical');
    const row = (await screen.findByRole('heading', { name: 'Backtesting' })).closest('li') as HTMLElement;
    await userEvent.click(within(row).getByRole('button', { name: /Mark as In progress/ }));
    await waitFor(() => expect(api.callsTo('POST', '/topics/technical/backtesting/advance')).toHaveLength(1));
    expect(await within(row).findByText('In progress', { selector: '.chip' })).toBeInTheDocument();

    await userEvent.type(screen.getByRole('textbox', { name: 'What do you need to learn?' }), 'Order types');
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'personal');
    await userEvent.click(screen.getByRole('button', { name: 'Add a topic' }));
    expect(await screen.findByRole('heading', { name: 'Order types' })).toBeInTheDocument();
    expect(api.callsTo('POST', '/sections/technical/topics')[0]?.body).toEqual({ title: 'Order types', type: 'personal', status: 'pending' });
  });

  it('says so when the section does not exist', async () => {
    new FakeApi().install();
    renderApp('#/sections/nope');
    expect(await screen.findByText('This section does not exist.')).toBeInTheDocument();
  });
});
