import { screen, waitFor, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { FakeApi, apiError } from './fakeApi';
import { renderApp } from './render';

afterEach(() => vi.unstubAllGlobals());

describe('topic page', () => {
  it('shows the topic with status, type, notes as Markdown and linked entries', async () => {
    new FakeApi().install();
    renderApp('#/topics/technical/risk-basics');
    expect(await screen.findByRole('heading', { level: 1, name: 'Risk basics' })).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'In progress' })).toBeChecked();
    expect(screen.getByRole('radio', { name: 'Role' })).toBeChecked();
    expect(screen.getByText('risk', { selector: 'strong' })).toBeInTheDocument();
    expect(await screen.findByText('First note about risk')).toBeInTheDocument();
    expect(screen.queryByText('Second note')).toBeNull();
    const crumbs = screen.getByRole('navigation', { name: 'Breadcrumb' });
    expect(within(crumbs).getByRole('link', { name: 'Technical' })).toHaveAttribute('href', '#/sections/technical');
  });

  it('changes the status with the version it read, and advances', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/technical/risk-basics');
    await userEvent.click(await screen.findByRole('radio', { name: 'Pending' }));
    await waitFor(() => expect(api.callsTo('POST', '/topics/technical/risk-basics/status')).toHaveLength(1));
    expect(api.callsTo('POST', '/topics/technical/risk-basics/status')[0]?.body).toEqual({ status: 'pending', version: '1-risk-basics' });
    expect(await screen.findByText('Status changed to Pending.')).toBeInTheDocument();
    expect(screen.getByRole('radio', { name: 'Pending' })).toBeChecked();

    // The next change uses the version returned by the previous one.
    await userEvent.click(screen.getByRole('button', { name: 'Mark as In progress' }));
    await waitFor(() => expect(api.callsTo('POST', '/topics/technical/risk-basics/advance')).toHaveLength(1));
    expect(api.callsTo('POST', '/topics/technical/risk-basics/advance')[0]?.body).toEqual({ version: '1-risk-basics+' });
  });

  it('edits notes with a preview and saves them', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/technical/backtesting');
    await userEvent.click(await screen.findByRole('button', { name: 'Edit notes' }));
    const area = screen.getByRole('textbox', { name: 'Notes' });
    await userEvent.type(area, 'Use **two years** of data');
    await userEvent.click(screen.getByRole('button', { name: 'Preview' }));
    expect(screen.getByText('two years', { selector: 'strong' })).toBeInTheDocument();
    await userEvent.click(screen.getByRole('button', { name: 'Write' }));
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    await waitFor(() => expect(api.callsTo('PATCH', '/topics/technical/backtesting')).toHaveLength(1));
    expect(api.callsTo('PATCH', '/topics/technical/backtesting')[0]?.body).toEqual({ version: '1-backtesting', notes: 'Use **two years** of data' });
    expect(await screen.findByText('two years', { selector: 'strong' })).toBeInTheDocument();
    expect(screen.queryByRole('textbox', { name: 'Notes' })).toBeNull();
  });

  it('renders hostile notes as text', async () => {
    const api = new FakeApi();
    const t = api.topics[0];
    if (t) t.notes = '<script>window.pwned=1</script>\n\n<img src=x onerror=alert(1)>\n\n[click](javascript:alert(1))';
    api.install();
    const { container } = renderApp('#/topics/technical/risk-basics');
    await screen.findByRole('heading', { level: 1, name: 'Risk basics' });
    expect(container.querySelector('script, img')).toBeNull();
    expect(screen.queryByRole('link', { name: 'click' })).toBeNull();
    expect(container.querySelector('a[href^="javascript"]')).toBeNull();
    expect(container.textContent).toContain('<script>window.pwned=1</script>');
  });

  it('adds and removes resources', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/technical/risk-basics');
    await userEvent.type(await screen.findByLabelText('Web address'), 'https://example.com/guide');
    await userEvent.type(screen.getByLabelText(/^Name/), 'Guide');
    await userEvent.click(screen.getByRole('button', { name: 'Add resource' }));
    const link = await screen.findByRole('link', { name: /Guide/ });
    expect(link).toHaveAttribute('href', 'https://example.com/guide');
    expect(link).toHaveAttribute('target', '_blank');
    expect(link).toHaveAttribute('rel', 'noopener noreferrer nofollow');
    expect(api.callsTo('PATCH', '/topics/technical/risk-basics')[0]?.body).toEqual({
      version: '1-risk-basics',
      resources: [{ url: 'https://example.com/guide', title: 'Guide' }],
    });

    await userEvent.click(within(link.closest('li') as HTMLElement).getByRole('button', { name: /Delete/ }));
    await waitFor(() => expect(screen.queryByRole('link', { name: /Guide/ })).toBeNull());
  });

  it('does not make an unsafe resource URL clickable', async () => {
    const api = new FakeApi();
    const t = api.topics[0];
    if (t) t.resources = [{ url: 'javascript:alert(1)', title: 'Bad' }];
    api.install();
    renderApp('#/topics/technical/risk-basics');
    await screen.findByText('Bad');
    expect(screen.queryByRole('link', { name: /Bad/ })).toBeNull();
  });

  it('marks a suggested topic as reviewed', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/technical/backtesting');
    await userEvent.click(await screen.findByRole('button', { name: 'Mark as reviewed' }));
    await waitFor(() => expect(api.callsTo('PATCH', '/topics/technical/backtesting')[0]?.body).toEqual({ version: '1-backtesting', suggested: false }));
    await waitFor(() => expect(screen.queryByRole('button', { name: 'Mark as reviewed' })).toBeNull());
  });

  it('offers to reload when the topic changed elsewhere (409)', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/technical/risk-basics');
    await userEvent.click(await screen.findByRole('button', { name: 'Edit notes' }));
    await userEvent.type(screen.getByRole('textbox', { name: 'Notes' }), ' more');
    api.on((c) => (c.method === 'PATCH' ? apiError(409, 'conflict') : undefined));
    await userEvent.click(screen.getByRole('button', { name: 'Save' }));
    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent('changed elsewhere');

    // Someone else edited the file: reloading shows their version.
    const t = api.topics[0];
    if (t) {
      t.notes = 'Edited elsewhere';
      t.version = '2-risk-basics';
    }
    await userEvent.click(within(alert).getByRole('button', { name: 'Reload latest version' }));
    expect(await screen.findByText('Edited elsewhere')).toBeInTheDocument();
    expect(screen.queryByRole('alert')).toBeNull();
    expect(screen.queryByRole('textbox', { name: 'Notes' })).toBeNull();
  });

  it('confirms before deleting a topic and returns to its section', async () => {
    const api = new FakeApi();
    api.install();
    renderApp('#/topics/team/onboarding');
    await userEvent.click(await screen.findByRole('button', { name: 'Delete topic' }));
    const dialog = await screen.findByRole('alertdialog');
    expect(api.callsTo('DELETE', '/topics')).toHaveLength(0);
    await userEvent.click(within(dialog).getByRole('button', { name: 'Delete topic' }));
    await waitFor(() => expect(window.location.hash).toBe('#/sections/team'));
    expect(api.callsTo('DELETE', '/topics/team/onboarding')).toHaveLength(1);
  });

  it('says so when the topic does not exist', async () => {
    new FakeApi().install();
    renderApp('#/topics/technical/nope');
    expect(await screen.findByText('This topic does not exist.')).toBeInTheDocument();
  });
});
