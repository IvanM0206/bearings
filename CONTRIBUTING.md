# Contributing

Thanks for helping! Bearings is small on purpose; please open an issue to
discuss larger changes before writing code.

- Code, comments and commit messages are in English. User-facing text goes
  through the message catalogs (English and Spanish).
- `format/SPEC.md` is the contract between the CLI, the MCP server and the web
  app. Changes to it must update all of them, with tests.
- Pin dependencies to exact versions and explain why a new one is needed.
- Run the checks for the part you touched (see `CLAUDE.md`, "Commands") before
  opening a pull request.
- Security issues: see [SECURITY.md](SECURITY.md), never a public issue.

## Contributing a template

A template is a folder under `templates/` that follows the notebook format,
with a `template` block in `bearings.yaml` and no journal. Keep it generic:
no company names, internal systems, people or confidential details.
