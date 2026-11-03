# Bearings — project instructions

Bearings helps someone in a new role record what they learn, track what they still need to learn, and look it up later. The role-related part becomes a guide (the role path) for whoever takes the role next.

## Layout

| Path | What | Owner agent |
|---|---|---|
| `format/SPEC.md` | Notebook file format, limits and security rules. The contract between all parts. | (shared — change only on purpose) |
| `cli/` | Go CLI `bearings` (module `github.com/IvanM0206/bearings/cli`). | `bearings-cli` |
| `cli/internal/mcpserver/`, `docs/mcp.md` | Local MCP server, started with `bearings mcp`. | `bearings-mcp` |
| `web/` | React + TypeScript PWA, deployed on Vercel. | `bearings-web` |
| `templates/` | Starter notebooks (en, es). | (shared) |

Agent definitions live in `.claude/agents/`.

## Conventions

- **Code is in English**: identifiers, comments, commit messages, file names, docs for contributors. User-facing text is translated: English is the default, Spanish (`es`) is fully supported. Never hard-code user-facing strings; put them in the message catalogs.
- Follow `format/SPEC.md` exactly. If something in it is ambiguous or wrong, stop and raise it instead of inventing a variant. Any change to the spec must update every implementation and its tests in the same change.
- Pin every dependency to an exact version. Prefer the standard library. Before adding a dependency, check it is maintained, widely used and has no open advisories; justify it in the PR description. Do not pick a version published less than 14 days ago.
- GitHub Actions: pin third-party actions to a full commit SHA (with the version in a comment), set `permissions:` to the minimum per job, never use `pull_request_target`.
- No telemetry, analytics or network calls that the user did not trigger.
- No secrets, tokens or personal data in the repo, tests or fixtures. Fixtures use invented, generic content.

## Security bar

Treat this as production software that holds private work notes. In particular:
- All notebook content is untrusted input (it may come from someone else's template or archive).
- Validate at the boundary, fail closed, give clear errors without leaking file contents.
- Every limit and security rule in SPEC §7–§8 needs a test that proves it.
- Run the security tooling for your part (see each agent definition) and fix findings before declaring work done.

## Commands

- CLI: `cd cli && go test -race ./... && go vet ./... && golangci-lint run && govulncheck ./...`
- Web: `cd web && npm ci && npm run typecheck && npm run lint && npm test && npm run build && npm audit --omit=dev`
