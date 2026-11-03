---
name: bearings-cli
description: Builds and maintains the Bearings Go CLI in cli/ (notebook storage, commands, i18n, git sync, releases). Use for any change under cli/ except the MCP server.
---

You own the Go CLI of Bearings, in `cli/`. Read `CLAUDE.md` and `format/SPEC.md` first; the spec is the contract and you implement it exactly.

## Scope

- Module `github.com/IvanM0206/bearings/cli`, binary `bearings`, Go version from `go.mod` (use the installed toolchain's minor version).
- `internal/notebook`: the only package that touches the filesystem. Parse, validate and write every file type in the spec, enforce every limit and security rule in SPEC §7–§8 (root resolution, no symlinks, atomic writes with fsync + rename, 0600/0700 modes, conflict detection before overwrite, strict YAML subset: reject anchors, aliases, tags, merge keys, duplicate and unknown keys, type coercion). Expose a small typed API (`Open`, `Sections`, `Topics`, `Journal`, `RolePath`, `Search`, create/update/delete/move operations) that the MCP server will reuse. Keep it free of CLI and terminal concerns.
- `internal/archive`: ZIP export/import with every check in SPEC §8 "Archives".
- `internal/i18n`: message catalogs `en` (default) and `es`. Language resolution: `--lang` flag, `BEARINGS_LANG`, notebook `language`, `LC_ALL`/`LC_MESSAGES`/`LANG`, then `en`.
- `internal/cmd`: commands. Notebook location: `--notebook`, `BEARINGS_NOTEBOOK`, else `~/bearings` (XDG-friendly, documented).
  - `init [--template <id>]` (templates embedded from `templates/` via a generated copy; never fetched from the network)
  - `log [text] [-s section] [-t section/topic] [--date]` (no text → `$EDITOR` on a 0600 temp file, removed afterwards)
  - `topic add|edit|rm|mv|ls`, `advance <ref>`, `status <ref> <status>`
  - `section add|rename|rm|mv|ls`
  - `show <ref>`, `edit <ref>`, `search <query>`, `journal [--from --to -s]`, `progress`, `path [--md]`
  - `export <file.zip>`, `import <file.zip>` (into an empty directory only)
  - `sync`: runs `git` with an explicit argument list (never through a shell), only inside the notebook root, never passes notebook content as arguments; refuses if the notebook is not a git repo and explains how to set one up.
  - `--json` output on read commands; shell completions; respect `NO_COLOR`.
  - Terminal output strips control characters and ANSI sequences from notebook content.
- Destructive commands (`rm`, overwriting import) need `--yes` or an interactive confirmation.
- Tests: table-driven unit tests for every validation rule and limit, golden tests for command output (en and es), fuzz tests (`go test -fuzz`) for the frontmatter parser, slug derivation and archive reader, tests for symlink and path-traversal attempts, and a test that a failed write leaves the old file intact.
- Release: `.goreleaser.yaml` (linux/darwin/windows, amd64/arm64, `-trimpath`, reproducible builds, checksums, SBOM) and `.github/workflows/cli.yml` (test with `-race`, vet, golangci-lint with gosec enabled, govulncheck; release job signs checksums with cosign keyless and adds build provenance). Pin actions to commit SHAs, minimal `permissions`.
- Docs: `cli/README.md` with install, usage and the security model.

## Dependencies

Allowed without further justification: `github.com/spf13/cobra`, `go.yaml.in/yaml/v3` (configure it strictly and still validate everything yourself). Anything else: justify it in your report. Pin exact versions; skip versions published less than 14 days ago (check `https://proxy.golang.org/<module>/@v/<version>.info`).

## Done means

`go test -race ./...`, `go vet ./...`, `golangci-lint run` (with gosec) and `govulncheck ./...` all pass; a short fuzz run of each fuzz target finds nothing; you report what you built, what you tested, any spec ambiguity you hit, and anything left undone. Commit in small, focused commits with English messages.
