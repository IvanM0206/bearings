import { useEffect, useMemo, useState } from 'react';

// Hash routing: the server only ever serves index.html and the assets, and a
// hash never reaches it (nor any proxy log).

export interface Route {
  segments: string[];
  query: URLSearchParams;
}

function decode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

export function parseHash(hash: string): Route {
  const raw = hash.startsWith('#') ? hash.slice(1) : hash;
  const [path = '', ...rest] = raw.split('?');
  const segments = path.split('/').filter((s) => s !== '').map(decode);
  return { segments, query: new URLSearchParams(rest.join('?')) };
}

export function useRoute(): Route {
  const [hash, setHash] = useState(() => window.location.hash);
  useEffect(() => {
    const onChange = () => setHash(window.location.hash);
    window.addEventListener('hashchange', onChange);
    return () => window.removeEventListener('hashchange', onChange);
  }, []);
  return useMemo(() => parseHash(hash), [hash]);
}

const enc = encodeURIComponent;

export const links = {
  home: () => '#/',
  sections: () => '#/sections',
  section: (slug: string) => `#/sections/${enc(slug)}`,
  topic: (ref: string) => {
    const [section = '', topic = ''] = ref.split('/');
    return `#/topics/${enc(section)}/${enc(topic)}`;
  },
  journal: (params?: Record<string, string | undefined>) => {
    const q = new URLSearchParams();
    for (const [k, v] of Object.entries(params ?? {})) if (v) q.set(k, v);
    const s = q.toString();
    return s ? `#/journal?${s}` : '#/journal';
  },
  entry: (id: string) => `#/journal/${enc(id)}`,
  path: () => '#/path',
  search: (q?: string) => (q ? `#/search?q=${enc(q)}` : '#/search'),
};

export function navigate(to: string): void {
  window.location.hash = to.startsWith('#') ? to : `#${to}`;
}
