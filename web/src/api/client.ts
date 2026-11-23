import type {
  Entry,
  Hit,
  Info,
  JournalPage,
  JournalQuery,
  OutlineItem,
  Progress,
  ResourceIn,
  RoleStep,
  Section,
  Status,
  Topic,
  TopicType,
} from './types';

const BASE = '/api/v1';

/** What went wrong, in terms the interface reacts to. */
export type ErrorKind =
  | 'unauthorized' // 401: the session cookie is missing or expired
  | 'read_only' // 403 read_only: the notebook was opened read-only
  | 'forbidden' // any other 403 / 421
  | 'not_found'
  | 'conflict' // 409 conflict: the file changed elsewhere
  | 'invalid' // 400 / 409 / 415 / 422: the server refused the values
  | 'too_large'
  | 'network' // the server could not be reached
  | 'server'; // 5xx or an unreadable answer

export class ApiError extends Error {
  readonly kind: ErrorKind;
  readonly status: number;
  /** Stable machine code from the server (never shown verbatim). */
  readonly code: string;
  readonly field: string | undefined;

  constructor(kind: ErrorKind, status: number, code: string, field?: string) {
    super(code);
    this.name = 'ApiError';
    this.kind = kind;
    this.status = status;
    this.code = code;
    this.field = field;
  }
}

export function kindOf(status: number, code: string): ErrorKind {
  switch (status) {
    case 401:
      return 'unauthorized';
    case 403:
      return code === 'read_only' ? 'read_only' : 'forbidden';
    case 404:
      return 'not_found';
    case 409:
      return code === 'conflict' ? 'conflict' : 'invalid';
    case 413:
      return 'too_large';
    case 421:
      return 'forbidden';
    case 400:
    case 415:
    case 422:
      return 'invalid';
    default:
      return 'server';
  }
}

interface ErrorBody {
  error?: { code?: unknown; field?: unknown };
}

async function errorFrom(res: Response): Promise<ApiError> {
  let code = 'unknown';
  let field: string | undefined;
  try {
    const body = (await res.json()) as ErrorBody;
    if (typeof body.error?.code === 'string') code = body.error.code;
    if (typeof body.error?.field === 'string') field = body.error.field;
  } catch {
    // Not JSON: keep the generic code.
  }
  return new ApiError(kindOf(res.status, code), res.status, code, field);
}

async function request<T>(method: string, path: string, body?: unknown, signal?: AbortSignal): Promise<T> {
  const init: RequestInit = {
    method,
    credentials: 'same-origin',
    headers: body === undefined ? { Accept: 'application/json' } : { Accept: 'application/json', 'Content-Type': 'application/json' },
    ...(body === undefined ? {} : { body: JSON.stringify(body) }),
    ...(signal ? { signal } : {}),
  };
  let res: Response;
  try {
    res = await fetch(BASE + path, init);
  } catch (e) {
    if (e instanceof DOMException && e.name === 'AbortError') throw e;
    throw new ApiError('network', 0, 'network');
  }
  if (!res.ok) throw await errorFrom(res);
  if (res.status === 204) return undefined as T;
  try {
    return (await res.json()) as T;
  } catch {
    throw new ApiError('server', res.status, 'bad_response');
  }
}

const get = <T>(path: string, signal?: AbortSignal) => request<T>('GET', path, undefined, signal);
const enc = encodeURIComponent;

function topicPath(ref: string): string {
  const [section = '', slug = ''] = ref.split('/');
  return `/topics/${enc(section)}/${enc(slug)}`;
}

function query(params: Record<string, string | number | undefined>): string {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== '') q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : '';
}

export interface NewTopicInput {
  title: string;
  type: TopicType;
  status?: Status;
}

export interface TopicPatch {
  version: string;
  title?: string;
  status?: Status;
  type?: TopicType;
  suggested?: boolean;
  notes?: string;
  resources?: ResourceIn[];
}

export interface EntryInput {
  body: string;
  date?: string;
  section?: string;
  topic?: string;
}

export const api = {
  info: (signal?: AbortSignal) => get<Info>('/info', signal),
  progress: (signal?: AbortSignal) => get<Progress>('/progress', signal),
  sections: async (signal?: AbortSignal) => (await get<{ sections: Section[] }>('/sections', signal)).sections,
  outline: async (signal?: AbortSignal) => (await get<{ outline: OutlineItem[] }>('/outline', signal)).outline,
  topic: (ref: string, signal?: AbortSignal) => get<Topic>(topicPath(ref), signal),
  rolePath: async (signal?: AbortSignal) => (await get<{ steps: RoleStep[] }>('/role-path', signal)).steps,
  journal: (q: JournalQuery = {}, signal?: AbortSignal) =>
    get<JournalPage>(`/journal${query({ ...q })}`, signal),
  entry: (id: string, signal?: AbortSignal) => get<Entry>(`/journal/${enc(id)}`, signal),
  search: async (q: string, signal?: AbortSignal) =>
    (await get<{ hits: Hit[] }>(`/search${query({ q })}`, signal)).hits,

  addSection: (input: { title: string; description?: string }) => request<Section>('POST', '/sections', input),
  updateSection: (slug: string, patch: { version: string; title?: string; description?: string }) =>
    request<Section>('PATCH', `/sections/${enc(slug)}`, patch),
  deleteSection: (slug: string) => request<void>('DELETE', `/sections/${enc(slug)}`),
  reorderSection: (slug: string, delta: -1 | 1) =>
    request<{ moved: boolean }>('POST', `/sections/${enc(slug)}/reorder`, { delta }),

  addTopic: (section: string, input: NewTopicInput) =>
    request<Topic>('POST', `/sections/${enc(section)}/topics`, input),
  updateTopic: (ref: string, patch: TopicPatch) => request<Topic>('PATCH', topicPath(ref), patch),
  deleteTopic: (ref: string) => request<void>('DELETE', topicPath(ref)),
  reorderTopic: (ref: string, delta: -1 | 1) =>
    request<{ moved: boolean }>('POST', `${topicPath(ref)}/reorder`, { delta }),
  setStatus: (ref: string, status: Status, version?: string) =>
    request<Topic>('POST', `${topicPath(ref)}/status`, { status, ...(version ? { version } : {}) }),
  advance: (ref: string, version?: string) =>
    request<Topic>('POST', `${topicPath(ref)}/advance`, version ? { version } : undefined),

  addEntry: (input: EntryInput) => request<Entry>('POST', '/journal', input),
  updateEntry: (id: string, patch: { version: string; body?: string; section?: string; topic?: string }) =>
    request<Entry>('PATCH', `/journal/${enc(id)}`, patch),
  deleteEntry: (id: string) => request<void>('DELETE', `/journal/${enc(id)}`),
};
