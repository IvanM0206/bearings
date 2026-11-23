import { afterEach, describe, expect, it, vi } from 'vitest';
import { ApiError, api, kindOf } from './client';

function respond(status: number, body?: unknown): Response {
  return new Response(body === undefined ? null : JSON.stringify(body), {
    status,
    headers: body === undefined ? {} : { 'Content-Type': 'application/json' },
  });
}

function stub(fn: (url: string, init: RequestInit) => Response | Promise<Response>) {
  const spy = vi.fn((url: string, init: RequestInit) => Promise.resolve(fn(url, init)));
  vi.stubGlobal('fetch', spy);
  return spy;
}

afterEach(() => {
  vi.unstubAllGlobals();
});

async function failure(p: Promise<unknown>): Promise<ApiError> {
  try {
    await p;
  } catch (e) {
    if (e instanceof ApiError) return e;
    throw e;
  }
  throw new Error('expected a failure');
}

describe('kindOf', () => {
  it.each([
    [401, 'unauthorized', 'unauthorized'],
    [403, 'read_only', 'read_only'],
    [403, 'forbidden', 'forbidden'],
    [421, 'bad_host', 'forbidden'],
    [404, 'not_found', 'not_found'],
    [409, 'conflict', 'conflict'],
    [409, 'already_exists', 'invalid'],
    [409, 'section_not_deletable', 'invalid'],
    [400, 'invalid_slug', 'invalid'],
    [415, 'unsupported_media_type', 'invalid'],
    [422, 'too_long', 'invalid'],
    [413, 'payload_too_large', 'too_large'],
    [500, 'notebook_error', 'server'],
    [502, 'x', 'server'],
  ] as const)('maps %i %s to %s', (status, code, kind) => {
    expect(kindOf(status, code)).toBe(kind);
  });
});

describe('api client', () => {
  it('sends credentials, accepts JSON and parses the answer', async () => {
    const spy = stub(() => respond(200, { name: 'N', language: 'es', format: 1, read_only: false }));
    const info = await api.info();
    expect(info.language).toBe('es');
    const [url, init] = spy.mock.calls[0] ?? [];
    expect(url).toBe('/api/v1/info');
    expect(init?.credentials).toBe('same-origin');
    expect(init?.method).toBe('GET');
    expect((init?.headers as Record<string, string>)['Accept']).toBe('application/json');
  });

  it('sends JSON bodies with a content type', async () => {
    const spy = stub(() => respond(201, { id: 'x' }));
    await api.addEntry({ body: 'hello', section: 'tech' });
    const [url, init] = spy.mock.calls[0] ?? [];
    expect(url).toBe('/api/v1/journal');
    expect(init?.method).toBe('POST');
    expect((init?.headers as Record<string, string>)['Content-Type']).toBe('application/json');
    expect(JSON.parse(init?.body as string)).toEqual({ body: 'hello', section: 'tech' });
  });

  it('sends no body and no content type for an advance without version', async () => {
    const spy = stub(() => respond(200, {}));
    await api.advance('tech/risk');
    const [url, init] = spy.mock.calls[0] ?? [];
    expect(url).toBe('/api/v1/topics/tech/risk/advance');
    expect(init?.body).toBeUndefined();
    expect((init?.headers as Record<string, string>)['Content-Type']).toBeUndefined();
  });

  it('encodes path segments and query values', async () => {
    const spy = stub(() => respond(200, { hits: [] }));
    await api.search('a&b c');
    expect(spy.mock.calls[0]?.[0]).toBe('/api/v1/search?q=a%26b+c');
    await api.entry('a/b');
    expect(spy.mock.calls[1]?.[0]).toBe('/api/v1/journal/a%2Fb');
  });

  it('builds the journal query and skips empty values', async () => {
    const spy = stub(() => respond(200, { entries: [], has_more: false }));
    await api.journal({ from: '2026-01-01', to: '', section: 'tech', limit: 100, offset: 0 });
    expect(spy.mock.calls[0]?.[0]).toBe('/api/v1/journal?from=2026-01-01&section=tech&limit=100&offset=0');
  });

  it('returns undefined for 204', async () => {
    stub(() => respond(204));
    await expect(api.deleteEntry('0123456789')).resolves.toBeUndefined();
  });

  it('maps 401 to unauthorized', async () => {
    stub(() => respond(401, { error: { code: 'unauthorized', message: 'x' } }));
    const e = await failure(api.info());
    expect(e.kind).toBe('unauthorized');
    expect(e.status).toBe(401);
  });

  it('maps 403 read_only', async () => {
    stub(() => respond(403, { error: { code: 'read_only', message: 'x' } }));
    expect((await failure(api.deleteEntry('0123456789'))).kind).toBe('read_only');
  });

  it('maps 409 conflict', async () => {
    stub(() => respond(409, { error: { code: 'conflict', message: 'x' } }));
    const e = await failure(api.updateTopic('a/b', { version: 'v' }));
    expect(e.kind).toBe('conflict');
    expect(e.code).toBe('conflict');
  });

  it('keeps the code and field of validation errors', async () => {
    stub(() => respond(422, { error: { code: 'too_long', message: 'x', field: 'title' } }));
    const e = await failure(api.addSection({ title: 'x' }));
    expect(e).toMatchObject({ kind: 'invalid', code: 'too_long', field: 'title' });
  });

  it('survives an error answer that is not JSON', async () => {
    stub(() => new Response('<html>oops</html>', { status: 502 }));
    const e = await failure(api.info());
    expect(e).toMatchObject({ kind: 'server', code: 'unknown' });
  });

  it('maps a failed connection to network', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))));
    expect((await failure(api.info())).kind).toBe('network');
  });

  it('rejects a success answer that is not JSON', async () => {
    stub(() => new Response('nope', { status: 200 }));
    expect(await failure(api.info())).toMatchObject({ kind: 'server', code: 'bad_response' });
  });

  it('never puts server messages into the error text', async () => {
    stub(() => respond(500, { error: { code: 'notebook_error', message: 'secret path /home/x' } }));
    const e = await failure(api.info());
    expect(e.message).toBe('notebook_error');
  });

  it('passes the abort signal and rethrows aborts untouched', async () => {
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new DOMException('aborted', 'AbortError'))));
    await expect(api.info(new AbortController().signal)).rejects.toMatchObject({ name: 'AbortError' });
  });
});
