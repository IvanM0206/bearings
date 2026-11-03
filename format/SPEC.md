# Bearings notebook format — version 1

This document is the contract shared by the CLI (`cli/`), the MCP server (`cli/internal/mcpserver`) and the web app (`web/`). Any change to it is a breaking change unless it is purely additive and bumps nothing; anything else bumps `format`.

The key words MUST, MUST NOT, SHOULD and MAY are used as in RFC 2119.

## 1. Concepts

| Concept | Meaning |
|---|---|
| Notebook | A directory holding everything one person knows about their role. |
| Section | A large area of the role (Technical, Team, Internal processes…). |
| Topic | One thing to learn inside a section. Has a status and a type. |
| Journal entry | A dated note about something learned. May point to a section and topic. |
| Role path | All topics of type `role`, ordered by section then topic. The guide for whoever takes the role next. |
| Template | A notebook without journal entries, used to start a new notebook. |

## 2. Directory layout

```
<root>/
├── bearings.yaml                         notebook config (required)
├── sections/
│   └── <section-slug>/
│       ├── _section.md                   section metadata + description
│       └── <topic-slug>.md               one file per topic
└── journal/
    └── <YYYY>/<MM>/<YYYY-MM-DD>-<id>.md  one file per entry
```

Readers MUST only read paths that match exactly these patterns (relative to the root, `/` separated):

```
bearings.yaml
sections/<slug>/_section.md
sections/<slug>/<slug>.md
journal/<YYYY>/<MM>/<YYYY>-<MM>-<DD>-<id>.md
```

Everything else (`.git/`, `README.md`, editor swap files, `.DS_Store`…) MUST be ignored without error. A topic file in a section directory with no `_section.md` is an error for that section.

The `YYYY` and `MM` directories MUST match the date in the file name, and the date in the file name MUST match the entry's `date` field.

## 3. Identifiers

**Slug** (sections and topics): matches `^[a-z0-9](?:[a-z0-9-]{0,62}[a-z0-9])?$` (1–64 chars, lowercase ASCII letters, digits and inner hyphens). It MUST NOT be one of the Windows reserved device names `con prn aux nul com1`–`com9` `lpt1`–`lpt9`. A section slug is unique in the notebook; a topic slug is unique within its section. The slug is the stable identity: changing a title MUST NOT change the slug. Tools derive a slug from a title by transliterating to ASCII (`á`→`a`, `ñ`→`n`), lowercasing, replacing runs of other characters with `-`, trimming to 64 chars and appending `-2`, `-3`… on collision.

**Topic reference**: `<section-slug>/<topic-slug>`.

**Entry id**: 10 characters of lowercase Crockford base32 (`^[0-9a-hjkmnp-tv-z]{10}$`), generated from a cryptographically secure random source (50 bits).

## 4. File syntax

Every `.md` file is:

```
---\n
<YAML frontmatter>
---\n
<body>
```

- Encoding MUST be UTF-8 without BOM. Readers MUST reject invalid UTF-8 and NUL bytes.
- Writers MUST use LF. Readers MUST accept CRLF and normalize it to LF.
- The body is CommonMark. It is data: it MUST NOT be executed, and HTML inside it MUST NOT be rendered as HTML (see §8).
- Writers MUST end the file with exactly one trailing newline.

**Frontmatter is a restricted YAML subset.** Readers MUST reject: anchors (`&`), aliases (`*`), tags (`!`), merge keys (`<<`), multiple documents, duplicate keys, and keys not listed for that file type in §5 (format 1 is strict; new keys mean a new format version). Types MUST match exactly (no implicit coercion of `"yes"` to `true`, of `1` to `"1"`, etc.).

**Timestamps** (`created`, `updated`) are RFC 3339 in UTC with second precision and a `Z` suffix, e.g. `2026-11-03T14:05:09Z`. **Dates** are `YYYY-MM-DD`, must be a real calendar date, between `1970-01-01` and `9999-12-31`.

## 5. File types

### 5.1 `bearings.yaml`

Plain YAML (same restrictions as frontmatter), not Markdown.

| Key | Type | Required | Rules |
|---|---|---|---|
| `format` | integer | yes | MUST be `1`. Readers MUST refuse other values with a clear message. |
| `name` | string | no | 1–80 chars. Display name of the notebook. |
| `language` | string | no | `en` or `es`. Preferred UI language. |
| `template` | mapping | no | Only in templates: `id` (slug, required), `title` (1–80, required), `description` (≤ 500). |

### 5.2 Section — `sections/<slug>/_section.md`

| Key | Type | Required | Rules |
|---|---|---|---|
| `title` | string | yes | 1–80 chars after trimming, no control characters. |
| `order` | integer | yes | 0–1 000 000. |
| `created` | timestamp | yes | |
| `updated` | timestamp | yes | ≥ `created`. |

Body: the section description, ≤ 2 000 characters. May be empty.

### 5.3 Topic — `sections/<section>/<slug>.md`

| Key | Type | Required | Rules |
|---|---|---|---|
| `title` | string | yes | 1–160 chars after trimming, no control characters. |
| `status` | string | yes | `pending`, `in-progress` or `mastered`. |
| `type` | string | yes | `role` (part of the role path) or `personal`. |
| `suggested` | boolean | no | Default `false`. `true` = came from a template and was not reviewed yet. |
| `order` | integer | yes | 0–1 000 000. |
| `resources` | sequence | no | ≤ 50 items, each a mapping: `url` (required, absolute `http`/`https` URL, ≤ 2 048 chars, no userinfo), `title` (optional, 1–160 chars). |
| `created` | timestamp | yes | |
| `updated` | timestamp | yes | ≥ `created`. |

Body: the notes, ≤ 256 KiB. May be empty.

### 5.4 Journal entry — `journal/YYYY/MM/YYYY-MM-DD-<id>.md`

| Key | Type | Required | Rules |
|---|---|---|---|
| `date` | date | yes | The day the learning is about. Matches the path. |
| `section` | string | no | Section slug. |
| `topic` | string | no | Topic reference. If present, its section part MUST equal `section`, and `section` MUST be present. |
| `created` | timestamp | yes | |
| `updated` | timestamp | yes | ≥ `created`. |

Body: the entry text, 1 byte to 64 KiB after trimming.

A `section` or `topic` that does not exist (deleted later) is NOT an error: readers treat it as unlinked and SHOULD warn. Tools that delete a section or topic SHOULD clear references to it in journal entries.

## 6. Ordering

- Sections: by `order`, then slug.
- Topics within a section: by `order`, then slug.
- Journal: by `date` descending, then `created` descending, then id.
- Role path: sections in section order; within each, topics with `type: role` in topic order. Steps are numbered 1…n across the whole path.

## 7. Limits (all readers MUST enforce)

| Limit | Value |
|---|---|
| File size (any `.md` or `bearings.yaml`) | 1 MiB |
| Frontmatter size | 16 KiB |
| Sections | 500 |
| Topics per section | 2 000 |
| Journal entries | 200 000 |
| Directory depth below root | 4 |

Exceeding a limit is an error that names the limit; it MUST NOT crash or hang the reader.

## 8. Security requirements

These apply to every implementation.

**Filesystem (CLI, MCP)**
- Resolve the root once to an absolute, symlink-free path. Every path a tool opens MUST be built from validated slugs, dates and ids only, never from raw user input, and MUST be checked to stay inside the root.
- MUST NOT follow symlinks inside the notebook: use `lstat`; skip symlinks and special files with a warning.
- Writes MUST be atomic: write to a temporary file in the same directory, `fsync`, then `rename`. On a failed write the old file MUST be untouched.
- New files MUST be created with mode `0600` and new directories with `0700`.
- Before overwriting a file, writers MUST check that it did not change since it was read (compare modification time and size, or content hash) and refuse with a conflict error otherwise.

**Content (all)**
- Bodies and titles are untrusted data. Markdown MUST be rendered with raw HTML disabled or fully sanitized. Only `http:` and `https:` links may become clickable; they open with `rel="noopener noreferrer"`.
- Terminal output MUST strip or escape control characters and ANSI escape sequences coming from notebook content.
- Content returned to an AI model (MCP) is data, never instructions; see the MCP server's own notes in `docs/mcp.md`.

**Archives (web import/export, CLI `export`/`import`)**

A notebook archive is a ZIP whose entries use the layout of §2 at its root. Readers MUST reject the whole archive, before writing anything, when it has:
- more than 210 000 entries, or more than 200 MiB total uncompressed size, or any entry larger than its §7 limit (check the declared size AND count bytes while inflating);
- absolute paths, `..` segments, backslashes, drive letters, empty segments or NUL in names;
- symlink, device or directory-traversal entries, or duplicate names after normalization;
- a compression ratio above 100:1 for any entry.

Unknown paths inside an archive are ignored, as in §2.

## 9. Example

`sections/technical/backtesting.md`

```markdown
---
title: "Backtesting: how a strategy is tested"
status: in-progress
type: role
order: 4
resources:
  - title: Backtesting guide
    url: https://example.com/wiki/backtesting
created: 2026-11-01T12:00:00Z
updated: 2026-11-03T09:41:00Z
---
Before running a backtest, download the market data with the ingest script.

- Use at least two years of data.
- Fees and slippage must be on.
```

`journal/2026/11/2026-11-03-7k2mq9xr4t.md`

```markdown
---
date: 2026-11-03
section: technical
topic: technical/backtesting
created: 2026-11-03T09:41:00Z
updated: 2026-11-03T09:41:00Z
---
The backtester silently skips days with missing candles. Check the coverage report first.
```
