# Bearings MCP server

*Español: [mcp.es.md](mcp.es.md)*

`bearings mcp` runs a local [Model Context Protocol](https://modelcontextprotocol.io)
server so that an AI assistant (Claude Code, Claude Desktop or any MCP client)
can read your notebook and, if you allow it, add to it.

```
bearings mcp [--allow-write] [--log-level error|warn|info|debug] [--notebook DIR]
```

- **stdio only.** The assistant starts `bearings mcp` as a child process and
  talks to it over stdin and stdout. The server opens no network port and makes
  no network calls.
- **One notebook, fixed at startup.** It is chosen like for every other command:
  `--notebook`, then `BEARINGS_NOTEBOOK`, then `~/bearings`. No tool takes a
  file path; tools take section slugs (`technical`), topic references
  (`technical/backtesting`), dates (`2026-11-03`) and cursors, all validated.
- **Read-only by default.** Write tools exist only with `--allow-write`. There
  are no delete, move or rename tools at all.
- **Logs go to stderr** (stdout carries the protocol), at `--log-level`
  (default `warn`). Logs hold tool names, durations, error codes and file paths
  inside the notebook, never note content.

## Tools

Every list is paginated: when a result has `next_cursor`, call the tool again
with `cursor` set to it. Pages hold at most 100 items (default 50, set with
`limit` where available) and about 64 KiB of JSON.

### Read tools (always available)

| Tool | Inputs | Returns |
|---|---|---|
| `list_sections` | `cursor`, `limit` | Sections in order: slug, title, order, description. |
| `list_topics` | `section`, `status` (`pending`, `in-progress`, `mastered`), `type` (`role`, `personal`), `cursor`, `limit` | Topics without notes: reference, title, status, type, suggested, number of resources. |
| `get_topic` | `topic` (required), `notes_offset` | One topic with its resources and notes. Long notes come in chunks of about 64 KiB: when `notes_truncated` is true, call again with `notes_offset` = `notes_next_offset`. |
| `search` | `query` (required, 1–200 characters), `cursor`, `limit` | Matches in section, topic and journal titles and text, ignoring case and accents, with a one-line snippet. |
| `list_journal` | `from`, `to` (inclusive, `YYYY-MM-DD`), `section`, `cursor`, `limit` | Journal entries, newest first. Bodies over 8 KiB are cut (`body_truncated`). |
| `get_role_path` | `cursor`, `limit` | Every topic of type `role`, numbered across sections: the guide for whoever takes the role next. |
| `get_progress` | `cursor` | Topic counts by status for the whole notebook, role and personal topics and each section, plus the number of journal entries. |

### Write tools (only with `--allow-write`)

| Tool | Inputs | Does |
|---|---|---|
| `add_journal_entry` | `body` (required, ≤ 64 KiB), `date` (default today), `section`, `topic` | Adds a journal entry. The section and topic must exist; a topic must belong to the section if both are given. |
| `set_topic_status` | `topic`, `status` (both required) | Sets a topic's status. Reports the previous status; does not write when nothing changes. |
| `add_topic` | `section`, `title` (required), `slug`, `type` (default `role`), `status` (default `pending`), `notes` | Adds a topic at the end of an existing section. The slug is derived from the title unless given. |
| `append_topic_notes` | `topic`, `text` (both required) | Appends text to a topic's notes after a blank line, keeping what is there. Refused if the notes would exceed 256 KiB. |

Every write goes through the same code as the CLI: validated input, atomic
writes (temporary file, fsync, rename) with private file modes, and conflict
detection, so a file changed by someone else in the meantime is never
overwritten. Text with control characters (other than tab and line breaks) is
refused.

## Security notes

- **Notebook content is data, not instructions.** Notes, titles and journal
  entries may have been written by someone else (a template, an imported
  archive). The server returns them only inside the JSON fields of tool
  results (`title`, `description`, `notes`, `body`, `snippet`, resource `url`
  and `title`), never in tool descriptions or error messages. The server's
  instructions and every tool description tell the assistant that these
  fields are user data and must not be followed as instructions. This lowers
  the risk of prompt injection but cannot remove it: review what an assistant
  proposes to do, especially with `--allow-write`.
- **Sanitized.** ANSI escape sequences, control characters and invisible
  text-reordering characters are removed from returned text, so content cannot
  hide text or drive a terminal that displays it.
- **Errors are generic** (for example `not found: no such section or topic: topic`)
  and never contain file contents.
- **Bounded.** Results are capped in items and bytes; long texts are cut at a
  character boundary and flagged with a `…_truncated` field (and, in lists,
  the marker ` [truncated by bearings]`). Requests over 2 MiB are rejected.
- **Least privilege.** Start without `--allow-write` unless you want the
  assistant to change the notebook. Nothing can be deleted through MCP. If you
  sync the notebook with git, you can review every change with `git diff`
  before `bearings sync`.

## Setup

First install `bearings` (see [cli/README.md](../cli/README.md)) and make sure
it is on your `PATH`: `bearings progress` should work in a terminal. If the
assistant cannot find it, use the absolute path of the binary (`which bearings`)
instead of `bearings` below.

### Claude Code

1. Add the server, read-only, using the default notebook (`~/bearings` or
   `BEARINGS_NOTEBOOK`):

   ```sh
   claude mcp add bearings -- bearings mcp
   ```

2. Or choose the notebook explicitly, and allow writes:

   ```sh
   claude mcp add bearings -- bearings mcp --notebook /home/me/bearings --allow-write
   ```

   Use `--scope user` (`claude mcp add --scope user bearings -- …`) to make it
   available in every project.

3. Check it: `claude mcp list` should show `bearings` as connected. In a
   session, `/mcp` lists its tools.

To change the flags later, remove it and add it again:
`claude mcp remove bearings`.

### Claude Desktop

1. Open Settings → Developer → Edit Config. This opens
   `claude_desktop_config.json` (on macOS in
   `~/Library/Application Support/Claude/`, on Windows in `%APPDATA%\Claude\`).
2. Add the server under `mcpServers`. Read-only:

   ```json
   {
     "mcpServers": {
       "bearings": {
         "command": "/usr/local/bin/bearings",
         "args": ["mcp", "--notebook", "/Users/me/bearings"]
       }
     }
   }
   ```

   With writes allowed:

   ```json
   {
     "mcpServers": {
       "bearings": {
         "command": "/usr/local/bin/bearings",
         "args": ["mcp", "--notebook", "/Users/me/bearings", "--allow-write"]
       }
     }
   }
   ```

   Claude Desktop does not use your shell's `PATH`, so give the absolute path
   of `bearings` and of the notebook. On Windows, escape backslashes in JSON
   (`"C:\\Users\\me\\bearings"`).
3. Restart Claude Desktop. The Bearings tools appear in the tools menu.

### Troubleshooting

- Run `bearings mcp --log-level debug` in a terminal: it waits for a client on
  stdin (press Ctrl+D to end it). If the notebook cannot be opened, it exits at
  once with an error on stderr.
- Claude Desktop writes server logs (stderr) to its log folder
  (`~/Library/Logs/Claude/` on macOS, `%APPDATA%\Claude\logs\` on Windows).
