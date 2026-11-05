# bearings

`bearings` is the command-line tool of Bearings. It keeps a notebook of what
you learn in a new role: sections and topics with a status, a dated journal,
and a **role path** that becomes a guide for whoever takes the role next.

The notebook is a folder of plain Markdown files. It stays on your machine
unless you sync it with a git remote, export it, or share its role path.
There are no accounts, no network calls of its own and no telemetry.

Language: English. [Leer en español](README.es.md).

## Install

You need Go 1.25 or later (see `go.mod`).

From the Go module proxy:

```sh
go install github.com/IvanM0206/bearings/cli/cmd/bearings@latest
```

From a clone of the repository:

```sh
git clone https://github.com/IvanM0206/bearings.git
cd bearings/cli
go build -trimpath -o bearings ./cmd/bearings
./bearings --help
```

Put the binary somewhere on your `PATH`. `bearings sync` also needs `git`
2.26 or later.

## Quick start

```sh
# 1. Create a notebook from a built-in template (English or Spanish).
bearings init --template trading-bots-developer --name "My first role"

# 2. See the sections and their progress.
bearings ls

# 3. Look at a section, then at one topic.
bearings ls technical
bearings show technical/local-setup

# 4. Write a journal entry linked to a topic.
bearings log "Ran the project locally for the first time" -t technical/local-setup

# 5. Move a topic forward: pending -> in progress -> mastered.
bearings advance technical/local-setup

# 6. See the role path, the guide for whoever takes this role next.
bearings path
```

Topics that come from a template are marked as *suggested* until you review
them. `bearings ls <section>` shows the flag; `bearings topic edit <ref>
--clear-suggested` marks a topic as reviewed.

Run `bearings init --list-templates` to see the built-in templates. They are
part of the program and are never downloaded.

## Concepts

- **Notebook**: a directory with a `bearings.yaml` file, a `sections/`
  folder and a `journal/` folder.
- **Section**: a large area of the role (for example "Technical" or "Team").
  Its name in commands is a *slug*, derived from the title when it is created
  and never changed afterwards.
- **Topic**: something you need to learn, inside one section. Referred to as
  `<section>/<topic>`, for example `technical/backtesting`.
- **Status**: `pending`, `in-progress` or `mastered`.
- **Type**: `role` (part of the role path, the default) or `personal` (kept
  out of the role path and of shared guides).
- **Journal entry**: a dated note. It can be linked to a section or a topic.
- **Role path**: all `role` topics, in order, numbered across sections.

## Where the notebook is

The notebook directory is chosen in this order:

1. `--notebook <dir>`
2. the `BEARINGS_NOTEBOOK` environment variable
3. `~/bearings`

`init` creates it, and needs an empty or missing directory: it never creates a
notebook over existing files.

## Language

Messages are available in English (`en`, the default) and Spanish (`es`). The
language is taken from the first of these that is set:

1. `--lang en|es`
2. the `BEARINGS_LANG` environment variable
3. the `language` key of the notebook's `bearings.yaml`
4. the first of the `LC_ALL`, `LC_MESSAGES` and `LANG` variables that is set;
   `en_US.UTF-8` or `es_AR` select their language, and a locale without a
   catalog (such as `fr_FR`) falls back to `en`
5. `en`

An unknown value given with `--lang` is an error. Unknown values in
`BEARINGS_LANG` or in the notebook's `language` key are ignored. The
notebook's own language applies only when `--lang` and `BEARINGS_LANG` are
not set. Command names, flag names and file
names are never translated.

## Commands

Every command accepts the global flags `--notebook`, `--lang`, `--json` (on
read and write commands, see [JSON output](docs/json-output.md)), `--no-color`
and `--help`. Colors are also off when `NO_COLOR` is set, when `TERM=dumb`, or
when the output is not a terminal.

### Notebook and sections

| Command | What it does | Example |
|---|---|---|
| `init` | Create a notebook, blank or from a template. `--name`, `--language`, `--template`, `--list-templates`. | `bearings init --template trading-bots-developer-es --language es` |
| `section add <title>` | Add a section after the existing ones. `--description`. | `bearings section add "Customers" --description "Client work"` |
| `section rename <section> <title>` | Change a section's title; its slug stays. | `bearings section rename customers "Clients"` |
| `section mv <section> --up\|--down` | Move a section in the order. | `bearings section mv customers --up` |
| `section rm <section> [--yes]` | Delete a section and its topics. Journal entries keep their text but lose the link. | `bearings section rm customers --yes` |
| `section ls` | Same as `bearings ls`. | `bearings section ls` |

### Topics

| Command | What it does | Example |
|---|---|---|
| `topic add <section> <title>` | Add a topic (role and pending by default). `--type personal`, `--status`. | `bearings topic add team "Release calendar" --type role` |
| `topic edit <section>/<topic>` | Change `--title`, `--type`, `--clear-suggested`, `--add-resource <url>` with `--resource-title`, or `--remove-resource <url>`. | `bearings topic edit team/release-calendar --add-resource https://example.com/calendar --resource-title "Calendar"` |
| `topic mv <ref> --up\|--down\|--to-section <s>` | Reorder a topic, or move it to the end of another section. | `bearings topic mv team/release-calendar --up` |
| `topic rm <ref> [--yes]` | Delete a topic and its resources. | `bearings topic rm team/release-calendar --yes` |
| `topic ls <section> [--status] [--type]` | Same as `bearings ls <section>`. | `bearings topic ls technical --status pending` |
| `advance <ref>` | Move to the next status; a mastered topic stays mastered. | `bearings advance technical/backtesting` |
| `status <ref> <pending\|in-progress\|mastered>` | Set the status directly. | `bearings status technical/backtesting mastered` |

### Reading

| Command | What it does | Example |
|---|---|---|
| `ls` | Sections with the number of mastered topics. | `bearings ls` |
| `ls <section> [--status] [--type]` | Topics of a section, with status, type and whether they are suggested. | `bearings ls technical --type role` |
| `show <ref>` | One topic with its resources, notes and linked journal entries. | `bearings show technical/backtesting` |
| `search <query> [--limit n]` | Case-insensitive search (accents ignored) in sections, topics, resources and the journal. | `bearings search backtesting --limit 10` |
| `journal [--from] [--to] [-s section] [--limit n]` | Journal entries, newest first, grouped by date. Dates are `YYYY-MM-DD`, both bounds inclusive. | `bearings journal --from 2026-01-01 --section technical` |
| `progress` | Mastered topics per section and for the whole notebook, plus suggested topics not reviewed yet. | `bearings progress` |
| `path [--md]` | The role path. `--md` prints a Markdown guide with the role topics, their notes and resources, and no personal progress. | `bearings path --md > role-guide.md` |

`path --md` writes the notes as Markdown source. Raw HTML in the notes is not
escaped there, so review the guide, or render it with a viewer that disables
raw HTML, before publishing it.

### Journal

| Command | What it does | Example |
|---|---|---|
| `log [text\|-] [-s section] [-t section/topic] [--date YYYY-MM-DD]` | Add an entry. Give the text as an argument, `-` to read it from standard input, or no text to write it in your editor. | `echo "Shipped the first change" \| bearings log - -t technical/local-setup --date 2026-11-04` |
| `edit <section>/<topic>` | Edit a topic's notes in your editor (`$VISUAL` or `$EDITOR`). | `bearings edit technical/backtesting` |
| `edit --journal <id>` | Edit the text of a journal entry. The id is shown by `show` and `journal`. | `bearings edit --journal g85qj7ggwk` |

`edit` and `log` without text use a private temporary file (mode `0600`, in
a new directory) that is removed afterwards. If the notebook file changed
while `edit` was open, nothing is overwritten: your text is kept in a recovery
file and its path is printed.

### Export, import and sharing

| Command | What it does | Example |
|---|---|---|
| `export <file.zip> [--force] [--role-only]` | Write the notebook to a ZIP archive that `import` and the web app can read. `--role-only` leaves out the journal and personal topics and marks every remaining topic as pending and suggested, which makes a guide to share. | `bearings export ~/role-guide.zip --role-only` |
| `import <file.zip> [--into dir] [--yes]` | Create a new notebook from an archive. It never merges into or overwrites a notebook. | `bearings import ~/role-guide.zip --into ~/new-role --yes` |

`export` only includes the notebook's own files (`bearings.yaml`, `sections/`
and `journal/`), so a `.git` directory or other files in the folder are left
out. The archive cannot be written inside the notebook, and an existing file
is only replaced with `--force`.

`import` checks the whole archive before writing anything. If the archive is
refused, nothing is created. The new notebook goes to `--into`, or else to the
notebook location, which must be empty or missing.

### Sync

| Command | What it does | Example |
|---|---|---|
| `sync [--dry-run] [--verbose]` | Commit the changed notebook files to git. With an upstream, it then pulls with rebase and pushes. | `bearings sync --dry-run` |

See [docs/sync.md](docs/sync.md) (Spanish: [docs/sync.es.md](docs/sync.es.md))
for the setup and the full security model of `sync`. In short, the notebook
directory must be the top level of its own git repository, and the remote
should be private.

```sh
git -C ~/bearings init
bearings sync
git -C ~/bearings remote add origin <url of a private repository>
git -C ~/bearings push -u origin HEAD
```

`bearings sync` never resolves conflicts, resets, stashes or force-pushes. On a
conflict it stops and prints the git commands to look at it.

### Shell completion

```sh
bearings completion bash > /etc/bash_completion.d/bearings   # or zsh, fish, powershell
```

## Confirmation and scripts

Commands that delete data (`section rm`, `topic rm`, `topic edit
--remove-resource`) and `import` ask for confirmation in a terminal. Without a
terminal they stop with exit code 2 unless `--yes` (`-y`) is given, so a
script must say so on purpose.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Unexpected failure (for example an I/O error or a conflict with a change made elsewhere) |
| 2 | Usage error: a bad flag, argument, reference, date or query, or a confirmation that was not given |
| 3 | The notebook is missing or invalid |
| 4 | The section or topic does not exist |

Messages go to stderr; results go to stdout.

## JSON output

Read commands and write commands accept `--json`. The output is one JSON
object, and warnings go to stderr. The shapes are documented in
[docs/json-output.md](docs/json-output.md). Text fields are the raw content of
the notebook: treat them as untrusted data.

## Security model

`bearings` treats a notebook as untrusted input, because it may come from
someone else's template, archive or git repository. The rules below are the
ones in [format/SPEC.md](../format/SPEC.md) sections 7 and 8, as the CLI
implements them.

**Files and paths**

- The notebook root is resolved once to an absolute path with no symlinks.
  Every path the CLI opens is built from validated slugs, dates and ids, never
  from raw user input, and is checked to stay inside the root.
- Symbolic links are never followed inside the notebook. Symlinks and special
  files are skipped with a warning, and `import` refuses archives that contain
  them.
- Every write is atomic: the new content goes to a temporary file in the same
  directory, is `fsync`ed, and is then renamed over the old file. If a write
  fails, the old file is left as it was.
- New files are created with mode `0600` and new directories with mode `0700`.
- Before a file is overwritten, the CLI checks that it has not changed since it
  was read. If it has, the write is refused and your text is kept in a recovery
  file.
- The CLI never opens a file that is not one of the layout paths in the
  specification. Other files in the directory (`.git`, editor swap files and
  the like) are ignored.

**Limits** (every reader enforces them and names the limit when it stops)

| Limit | Value |
|---|---|
| File size (any `.md` or `bearings.yaml`) | 1 MiB |
| Frontmatter | 16 KiB |
| Sections | 500 |
| Topics per section | 2 000 |
| Journal entries | 200 000 |
| Directory depth below the root | 4 |

Frontmatter and `bearings.yaml` are parsed with a strict subset of YAML.
Anchors, aliases, tags, merge keys, multiple documents, duplicate keys, unknown
keys and values of the wrong type are all rejected.

**Archives** (`import`) are checked completely before anything is written. An
archive is refused when it has more than 210 000 entries, more than 200 MiB of
uncompressed data, an entry larger than a notebook file may be, an absolute
path, a `..` segment, a backslash, a drive letter, an empty segment or a NUL
byte in a name, a symbolic link, device or other special entry, two entries
with the same name after normalization, a compression ratio above 100:1, an
encrypted entry or an unsupported compression method. The data is counted while
it is decompressed, not only trusted from the declared sizes.

**Output**

- Notebook content is printed with control characters and ANSI escape
  sequences removed, so a note cannot change your terminal.
- Error messages name the file, field or limit, and never print file contents.
- No color is used when the output is not a terminal, or when `NO_COLOR` is set.

**Git (`sync`)**

- git always runs with an explicit argument list, never through a shell, and
  with `-C <notebook>`. No notebook content is ever an argument: the commit
  message is fixed, and the paths to commit go to git on standard input.
- Hooks, fsmonitor, submodule recursion and the `ext::` transport are turned
  off for every git command. Editors and pagers never run.
- A repository whose own configuration sets a setting that runs programs or
  sends your data elsewhere (for example `core.sshCommand`, a credential
  helper, a filter driver, or an `insteadOf` rewrite) is refused, with the names
  of the settings. `sync` reads only the names of the settings, never their
  values.
- `sync` never handles credentials: git's own helpers and SSH agent work as
  usual.

**Network and data**

- The CLI makes no network calls of its own. Only `bearings sync` talks to a
  remote, and only when you run it and the branch has an upstream.
- Templates are compiled into the binary. They are never downloaded.
- Nothing is sent anywhere else: no telemetry, no analytics.

Please report security problems privately, as described in
[SECURITY.md](../SECURITY.md).

## Development

```sh
cd cli
go test -race ./...
go vet ./...
golangci-lint run
govulncheck ./...
```

Golden files for the output of the commands live in
`internal/cmd/testdata/golden`, in English and Spanish. Regenerate them after
an intended change of output with `go test ./internal/cmd -run Golden -update`,
and review the diff.
