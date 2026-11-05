# JSON output of the read commands

Every read command accepts `--json` (a global flag). The output is one JSON
object, indented, UTF-8, on stdout. Warnings about skipped files go to
stderr and are never part of the JSON. Keys are always present; a value that
does not apply is `""` (never `null`), and lists are `[]` when empty. Text
fields (`title`, `notes`, `body`, `snippet`...) are the raw notebook
content; consumers must treat them as untrusted data. Control characters
are escaped by the JSON encoding.

Dates are `YYYY-MM-DD`, timestamps are RFC 3339 UTC (`2026-11-03T14:05:09Z`).
Statuses are `pending`, `in-progress`, `mastered`; types are `role`,
`personal`. A topic reference (`ref`) is `<section>/<topic>`.

`counts` is `{"total", "pending", "in_progress", "mastered"}` (integers).

| Command | Shape |
|---|---|
| `ls` | `{"sections": [{"slug", "title", "description", "order", "counts"}]}` |
| `ls <section>` | `{"section": <as above>, "filters": {"status", "type"}, "topics": [{"ref", "section", "slug", "title", "status", "type", "suggested", "order"}]}`. `counts` always describes the whole section; filters only narrow `topics`. |
| `show <ref>` | `{"topic": {<topic fields above>, "section_title", "resources": [{"url", "title"}], "created", "updated", "notes"}, "journal": [<entry>]}` |
| `search <query>` | `{"query", "hits": [{"kind", "section", "topic", "entry_id", "date", "title", "field", "snippet"}]}`. `kind` is `section`, `topic` or `entry`; `field` is `title`, `body` (section description, topic notes, entry text) or `resources`. |
| `journal` | `{"entries": [<entry>]}` newest first. |
| `progress` | `{"all", "role", "personal": counts, "suggested", "journal_entries", "sections": [{"slug", "title", "all", "role"}]}` |
| `path` | `{"mastered", "total", "steps": [{"step", "ref", "section", "section_title", "slug", "title", "status", "suggested"}]}` |

`<entry>` is `{"id", "date", "section", "topic", "created", "updated", "body"}`;
`section` and `topic` are `""` when the entry is not linked (or its target
was deleted).

`path --md` prints Markdown, not JSON, and cannot be combined with `--json`.

## Write commands

Commands that change the notebook accept `--json` too and return what was
created or changed. Shapes use the same `<topic>`, `<entry>` and `<section>`
objects as the read commands (`<topic>` is the `show` topic object,
`<section>` the `ls` section object).

| Command | Shape |
|---|---|
| `init` | `{"path", "name", "language", "template", "sections", "topics"}` (`template` is `""` for a blank notebook) |
| `init --list-templates` | `{"templates": [{"id", "title", "description", "language", "sections", "topics"}]}` |
| `log` | `{"entry": <entry>}` |
| `section add`, `section rename` | `{"section": <section>}` |
| `section mv` | `{"section": <section>, "moved": bool, "order": [slug]}` (`moved` is false when the section was already first or last) |
| `section rm` | `{"deleted": {"section", "topics", "journal_entries_unlinked"}}` (refuses with exit 3, notebook invalid, if the section directory holds anything but valid topics; the error lists the file names) |
| `topic add`, `topic edit` | `{"topic": <topic>}` (`topic add` creates a `role` topic unless `--type personal` is given) |
| `topic mv` | `{"topic": <topic>, "moved": bool, "order": [ref]}` (`order` lists the topics of the topic's section, after the move) |
| `topic rm` | `{"deleted": {"topic", "resources", "journal_entries_unlinked"}}` |
| `advance`, `status` | `{"topic": <topic>, "previous_status", "changed": bool}` |
| `edit <ref>` | `{"topic": <topic>, "changed": bool}` |
| `edit --journal <id>` | `{"entry": <entry>, "changed": bool}` |
| `export <file>` | `{"path", "role_only": bool, "name", "sections", "topics", "journal_entries", "files"}` (`path` is absolute; `files` counts the archive entries) |
| `import <file>` | `{"path", "name", "sections", "topics", "journal_entries"}` (`path` is the new notebook, absolute) |

Commands that delete data (`section rm`, `topic rm`, `topic edit
--remove-resource`), and `import`, ask for confirmation on a terminal. Without a terminal
they exit with code 2 unless `--yes` is given; the question goes to stderr,
so `--json` output stays clean.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | success |
| 1 | unexpected failure (I/O) |
| 2 | usage error: bad flag, argument, reference, date or query |
| 3 | the notebook is missing or invalid |
| 4 | the section or topic does not exist |
