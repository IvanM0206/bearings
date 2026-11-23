import { useMemo } from 'react';
import { api } from '../api/client';
import type { OutlineItem } from '../api/types';
import { useResource, type Resource } from './useResource';

export interface Lookup {
  sectionTitle: (slug: string) => string | undefined;
  topicTitle: (ref: string) => string | undefined;
}

export function useOutline(): Resource<OutlineItem[]> & { lookup: Lookup } {
  const res = useResource((s) => api.outline(s), []);
  const lookup = useMemo<Lookup>(() => {
    const sections = new Map<string, string>();
    const topics = new Map<string, string>();
    for (const o of res.data ?? []) {
      sections.set(o.section.slug, o.section.title);
      for (const t of o.topics) topics.set(t.ref, t.title);
    }
    return { sectionTitle: (s) => sections.get(s), topicTitle: (r) => topics.get(r) };
  }, [res.data]);
  return { ...res, lookup };
}
