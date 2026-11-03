# Security policy

Bearings stores private work notes, so we treat security issues as top priority.

## Reporting a vulnerability

Please **do not open a public issue**. Report it privately through GitHub's
[private vulnerability reporting](https://github.com/IvanM0206/bearings/security/advisories/new)
for this repository. Include the affected component (CLI, MCP server, web app),
version or commit, steps to reproduce and the impact you expect.

You will get an acknowledgement within 7 days. Fixes are released as soon as
possible and credited in the release notes unless you prefer otherwise.

## Supported versions

Only the latest release receives security fixes while the project is below 1.0.

## Security model (summary)

- Your notebook never leaves your devices unless you move it yourself
  (git sync, archive export). The web app has no backend and makes no network
  requests besides loading itself. There is no telemetry.
- All notebook content (including templates and imported archives) is treated
  as untrusted input: strict parsing, size limits, no symlinks, no path
  traversal, sanitized Markdown, escaped terminal output.
- The MCP server runs locally over stdio, is read-only unless started with
  `--allow-write`, and has no delete operations.
- Releases are built reproducibly, with checksums signed using Sigstore cosign
  and build provenance attestations.

See [`format/SPEC.md` §7–§8](format/SPEC.md) for the exact rules.
