/** Reading and writing the individual file types of §5. */
import { FormatError } from './errors';
import { isSlug, isTopicRef, splitTopicRef } from './ids';
import { MAX_FRONTMATTER_BYTES, MAX_TEMPLATE_TITLE_CHARS } from './limits';
import {
  LANGUAGES,
  STATUSES,
  TOPIC_TYPES,
  type Entry,
  type Language,
  type NotebookConfig,
  type Resource,
  type Section,
  type TemplateInfo,
  type Topic,
  type TopicStatus,
  type TopicType,
} from './model';
import { byteLength, decodeFile, stripTrailingNewlines } from './text';
import { isDate } from './time';
import {
  checkEntryBody,
  checkNotebookName,
  checkOrder,
  checkResources,
  checkSectionDescription,
  checkSectionTitle,
  checkTemplateDescription,
  checkTimestamps,
  checkTitle,
  checkTopicNotes,
  checkTopicTitle,
} from './validate';
import { emitYaml, parseStrictYaml, type EmitEntry, type MapValue, type Value } from './yaml';

// ---------------------------------------------------------------------------
// Frontmatter

export interface SplitFile {
  frontmatter: string;
  body: string;
}

/** Splits `---\n<yaml>---\n<body>` (§4). The body loses its trailing newlines. */
export function splitFrontmatter(text: string): SplitFile {
  if (!text.startsWith('---\n')) throw new FormatError('frontmatter-missing');
  let pos = 4;
  for (;;) {
    const nl = text.indexOf('\n', pos);
    const line = nl === -1 ? text.slice(pos) : text.slice(pos, nl);
    if (line === '---') {
      const frontmatter = text.slice(4, pos);
      if (byteLength(frontmatter) > MAX_FRONTMATTER_BYTES) {
        throw new FormatError('frontmatter-too-large', { limit: 'frontmatter-size', max: MAX_FRONTMATTER_BYTES });
      }
      const body = nl === -1 ? '' : text.slice(nl + 1);
      return { frontmatter, body: stripTrailingNewlines(body) };
    }
    if (nl === -1) throw new FormatError('frontmatter-missing');
    pos = nl + 1;
  }
}

function joinFrontmatter(yaml: string, body: string): string {
  const b = stripTrailingNewlines(body);
  return b === '' ? `---\n${yaml}---\n` : `---\n${yaml}---\n${b}\n`;
}

// ---------------------------------------------------------------------------
// Typed field access with strict key checking

class Fields {
  private readonly map: Map<string, Value>;
  private readonly prefix: string;

  constructor(value: MapValue, allowed: readonly string[], prefix = '') {
    this.map = value.entries;
    this.prefix = prefix;
    for (const key of this.map.keys()) {
      if (key === '<<') throw new FormatError('yaml-merge-key', { key: prefix + key });
      if (!allowed.includes(key)) throw new FormatError('unknown-key', { key: prefix + key });
    }
  }

  private get(key: string, required: boolean): Value | undefined {
    const v = this.map.get(key);
    if (v === undefined) {
      if (required) throw new FormatError('missing-key', { key: this.prefix + key });
      return undefined;
    }
    return v;
  }

  str(key: string): string;
  str(key: string, required: false): string | undefined;
  str(key: string, required = true): string | undefined {
    const v = this.get(key, required);
    if (v === undefined) return undefined;
    if (v.kind !== 'str') throw new FormatError('wrong-type', { key: this.prefix + key });
    return v.value;
  }

  int(key: string): number;
  int(key: string, required: false): number | undefined;
  int(key: string, required = true): number | undefined {
    const v = this.get(key, required);
    if (v === undefined) return undefined;
    if (v.kind !== 'int') throw new FormatError('wrong-type', { key: this.prefix + key });
    return v.value;
  }

  bool(key: string, required: false): boolean | undefined {
    const v = this.get(key, required);
    if (v === undefined) return undefined;
    if (v.kind !== 'bool') throw new FormatError('wrong-type', { key: this.prefix + key });
    return v.value;
  }

  map_(key: string, required: false): MapValue | undefined {
    const v = this.get(key, required);
    if (v === undefined) return undefined;
    if (v.kind !== 'map') throw new FormatError('wrong-type', { key: this.prefix + key });
    return v;
  }

  seq(key: string, required: false): Value[] | undefined {
    const v = this.get(key, required);
    if (v === undefined) return undefined;
    if (v.kind !== 'seq') throw new FormatError('wrong-type', { key: this.prefix + key });
    return v.items;
  }

  key(k: string): string {
    return this.prefix + k;
  }
}

function oneOf<T extends string>(value: string, allowed: readonly T[], key: string): T {
  if (!(allowed as readonly string[]).includes(value)) throw new FormatError('invalid-value', { key });
  return value as T;
}

function checkSlugField(value: string, key: string): string {
  if (!isSlug(value)) throw new FormatError('invalid-value', { key });
  return value;
}

// ---------------------------------------------------------------------------
// bearings.yaml (§5.1)

export function parseConfig(bytes: Uint8Array): NotebookConfig {
  const text = decodeFile(bytes);
  const f = new Fields(parseStrictYaml(text), ['format', 'name', 'language', 'template']);
  const format = f.int('format');
  if (format !== 1) throw new FormatError('unsupported-format', { key: 'format' });
  const config: NotebookConfig = { format: 1 };
  const name = f.str('name', false);
  if (name !== undefined) config.name = checkNotebookName(name);
  const language = f.str('language', false);
  if (language !== undefined) config.language = oneOf<Language>(language, LANGUAGES, 'language');
  const template = f.map_('template', false);
  if (template !== undefined) {
    const t = new Fields(template, ['id', 'title', 'description'], 'template.');
    const info: TemplateInfo = {
      id: checkSlugField(t.str('id'), 'template.id'),
      title: checkTitle(t.str('title'), MAX_TEMPLATE_TITLE_CHARS, 'template.title'),
    };
    const description = t.str('description', false);
    if (description !== undefined) info.description = checkTemplateDescription(description);
    config.template = info;
  }
  return config;
}

export function serializeConfig(config: NotebookConfig): string {
  const entries: EmitEntry[] = [
    ['format', { int: 1 }],
    ['name', config.name === undefined ? undefined : { quoted: config.name }],
    ['language', config.language === undefined ? undefined : { plain: config.language }],
    [
      'template',
      config.template === undefined
        ? undefined
        : {
            map: [
              ['id', { quoted: config.template.id }],
              ['title', { quoted: config.template.title }],
              [
                'description',
                config.template.description === undefined ? undefined : { quoted: config.template.description },
              ],
            ],
          },
    ],
  ];
  return emitYaml(entries);
}

// ---------------------------------------------------------------------------
// Section (§5.2)

export function parseSection(bytes: Uint8Array, slug: string): Section {
  const { frontmatter, body } = splitFrontmatter(decodeFile(bytes));
  const f = new Fields(parseStrictYaml(frontmatter), ['title', 'order', 'created', 'updated']);
  const title = checkSectionTitle(f.str('title'));
  const order = checkOrder(f.int('order'));
  const created = f.str('created');
  const updated = f.str('updated');
  checkTimestamps(created, updated);
  return { slug, title, order, created, updated, description: checkSectionDescription(body) };
}

export function serializeSection(s: Section): string {
  const yaml = emitYaml([
    ['title', { quoted: s.title }],
    ['order', { int: s.order }],
    ['created', { plain: s.created }],
    ['updated', { plain: s.updated }],
  ]);
  return joinFrontmatter(yaml, s.description);
}

// ---------------------------------------------------------------------------
// Topic (§5.3)

export function parseTopic(bytes: Uint8Array, section: string, slug: string): Topic {
  const { frontmatter, body } = splitFrontmatter(decodeFile(bytes));
  const f = new Fields(parseStrictYaml(frontmatter), [
    'title',
    'status',
    'type',
    'suggested',
    'order',
    'resources',
    'created',
    'updated',
  ]);
  const title = checkTopicTitle(f.str('title'));
  const status = oneOf<TopicStatus>(f.str('status'), STATUSES, 'status');
  const type = oneOf<TopicType>(f.str('type'), TOPIC_TYPES, 'type');
  const suggested = f.bool('suggested', false) ?? false;
  const order = checkOrder(f.int('order'));
  const rawResources = f.seq('resources', false) ?? [];
  const resources: Resource[] = rawResources.map((item, i) => {
    const prefix = `resources.${String(i)}.`;
    if (item.kind !== 'map') throw new FormatError('wrong-type', { key: `resources.${String(i)}` });
    const r = new Fields(item, ['url', 'title'], prefix);
    const res: Resource = { url: r.str('url') };
    const t = r.str('title', false);
    if (t !== undefined) res.title = t;
    return res;
  });
  const created = f.str('created');
  const updated = f.str('updated');
  checkTimestamps(created, updated);
  return {
    section,
    slug,
    title,
    status,
    type,
    suggested,
    order,
    resources: checkResources(resources),
    created,
    updated,
    notes: checkTopicNotes(body),
  };
}

export function serializeTopic(t: Topic): string {
  const yaml = emitYaml([
    ['title', { quoted: t.title }],
    ['status', { plain: t.status }],
    ['type', { plain: t.type }],
    ['suggested', t.suggested ? { bool: true } : undefined],
    ['order', { int: t.order }],
    [
      'resources',
      t.resources.length === 0
        ? undefined
        : {
            seqOfMaps: t.resources.map((r): EmitEntry[] => [
              ['title', r.title === undefined ? undefined : { quoted: r.title }],
              ['url', { quoted: r.url }],
            ]),
          },
    ],
    ['created', { plain: t.created }],
    ['updated', { plain: t.updated }],
  ]);
  return joinFrontmatter(yaml, t.notes);
}

// ---------------------------------------------------------------------------
// Journal entry (§5.4)

export function parseEntry(bytes: Uint8Array, id: string, pathDate: string): Entry {
  const { frontmatter, body } = splitFrontmatter(decodeFile(bytes));
  const f = new Fields(parseStrictYaml(frontmatter), ['date', 'section', 'topic', 'created', 'updated']);
  const date = f.str('date');
  if (!isDate(date)) throw new FormatError('invalid-value', { key: 'date' });
  if (date !== pathDate) throw new FormatError('path-date-mismatch', { key: 'date' });
  const entry: Entry = { id, date, created: '', updated: '', body: '' };
  const section = f.str('section', false);
  if (section !== undefined) entry.section = checkSlugField(section, f.key('section'));
  const topic = f.str('topic', false);
  if (topic !== undefined) {
    if (!isTopicRef(topic)) throw new FormatError('invalid-value', { key: 'topic' });
    if (section === undefined) throw new FormatError('missing-key', { key: 'section' });
    if (splitTopicRef(topic).section !== section) throw new FormatError('invalid-value', { key: 'topic' });
    entry.topic = topic;
  }
  entry.created = f.str('created');
  entry.updated = f.str('updated');
  checkTimestamps(entry.created, entry.updated);
  entry.body = checkEntryBody(body);
  return entry;
}

export function serializeEntry(e: Entry): string {
  const yaml = emitYaml([
    ['date', { plain: e.date }],
    ['section', e.section === undefined ? undefined : { quoted: e.section }],
    ['topic', e.topic === undefined ? undefined : { quoted: e.topic }],
    ['created', { plain: e.created }],
    ['updated', { plain: e.updated }],
  ]);
  return joinFrontmatter(yaml, e.body);
}
