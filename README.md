# Bearings

> Get your bearings in a new role.

Bearings helps you **record** what you learn in a new job, **track** what you
still need to learn, and **look it up** later. Everything that is part of the
role (not your personal notes) becomes a **role path**: a clear, ordered guide
for whoever takes the role after you.

[Leer en español](README.es.md)

## Parts

| Part | What it is | Status |
|---|---|---|
| [`cli/`](cli/) | `bearings` command-line tool (Go). Your notebook is a folder of Markdown files, synced with git. | In progress |
| [`cli/internal/mcpserver`](docs/mcp.md) | Local MCP server (`bearings mcp`) so AI assistants like Claude can read and update your notebook. | In progress |
| [`web/`](web/) | Installable web app (works offline, on desktop and phone). Data stays in your browser; import and export notebooks as archives. | In progress |
| [`format/`](format/SPEC.md) | The notebook file format shared by all parts. | v1 |
| [`templates/`](templates/) | Starter notebooks for specific roles. Contributions welcome. | |

## Privacy

Your notes never leave your devices unless you move them. No accounts, no
backend, no telemetry. See [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
