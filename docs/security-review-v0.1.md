# Security review — CLI and MCP server v0.1

Scope: `cli/internal/notebook`, `cli/internal/archive`, `cli/internal/gitsync`,
`cli/internal/cmd` (editor, `edit`, `log`, error output), `cli/internal/mcpserver`,
`cli/internal/output` and the dependencies in `cli/go.mod`, against
`format/SPEC.md` §7–§8. Reviewed at commit 2a95bb9 on branch `cli/v0.1`.

No Critical, High or Medium findings. The code already enforces the §7 limits
and the §8 filesystem, content and archive rules with tests; the findings below
are residual risks and hardening.

## Findings

| ID | Severity | Component | Location | Description | Status |
|---|---|---|---|---|---|
| BR-01 | Low | notebook | `cli/internal/notebook/fs.go:347` | Creating a new file renamed the temporary file over the target after a last existence check. A file another program (for example a concurrent `bearings mcp --allow-write`) created between that check and the rename was silently replaced. | Fixed in 2987e08: new files are placed with a hard link, which fails if the target exists (rename fallback where links are unsupported). Test `TestCreateDoesNotReplaceConcurrentFile`. |
| BR-02 | Low | notebook | `cli/internal/notebook/fs.go:341` | Overwrites check the content hash, then rename. A change made by another writer in the small window between the last check and the rename is lost. Inherent to check-then-rename without locking. | Open. Recommendation: take an advisory lock (`flock` on a lock file in the root) around read-check-write in every writer (CLI and MCP). |
| BR-03 | Low | notebook | `cli/internal/notebook/fs.go:115`, `:173` | Parent directories are checked with `lstat` one by one, then the full path is opened; `O_NOFOLLOW` protects only the last element. Someone who can write inside the notebook directory could swap a checked directory for a symlink between the check and the open. Requires local write access to the notebook (archives and templates cannot create symlinks; `git pull` can, but the swap still needs a racing local process). | Open. Recommendation: open through `os.Root` (Go 1.24+) or `openat2(RESOLVE_BENEATH\|RESOLVE_NO_SYMLINKS)` on Linux so containment is enforced by the kernel. |
| BR-04 | Info | notebook | `cli/internal/notebook/fs_other.go:10` | On non-unix platforms there is no `O_NOFOLLOW`; the code relies on `lstat` before and `os.SameFile` after opening. Reasonable, but untested on Windows (reparse points, junctions). | Open. Recommendation: run the symlink tests on a Windows CI runner. |
| BR-05 | Low | gitsync | `cli/internal/gitsync/gitsync.go:394` | Repository-local git config is checked against a denylist of keys that can run programs or redirect traffic (plus `-c` overrides for hooks and fsmonitor). Denylists age badly: a key added by a future git version, or one not listed (for example `remote.<name>.vcs`, which only runs `git-remote-<vcs>` from `PATH`), is accepted. No exploitable key was found for the commands sync runs (`status`, `add`, `commit`, `pull --rebase`, `push`). | Open. Recommendation: switch to an allowlist of local keys (`core.*` basics, `remote.*.url/fetch/pushurl`, `branch.*`, `user.*`) and refuse everything else. |
| BR-06 | Info | cmd | `cli/internal/cmd/app.go:108` | Error messages that echo arguments quote them with `%q`, so escape sequences were already neutralized; the final message was not sanitized again, so a future `%s` would let them through. | Fixed in aaea1e1 (defense in depth). Test `TestErrorMessagesAreSanitized`. |
| BR-07 | Info | cmd | `cli/internal/cmd/editor.go:250` | When saving edited text fails, a recovery copy stays in `$TMPDIR/bearings-recovery-*/` (dir 0700, file 0600) until the person deletes it. Intended, and the path is printed. | Accepted. Recommendation: mention it in the README security model. |
| BR-08 | Info | mcpserver | `cli/internal/cmd/mcp.go:56` | The slog logger is also handed to the MCP SDK. The server's own logs carry only tool names, ids and sanitized paths; what the SDK logs at `debug` level was not audited. Logs go to stderr only. | Open. Recommendation: check the SDK's debug logging for request payloads, or give the SDK a logger capped at `warn`. |
| BR-09 | Info | cmd, i18n | `cli/internal/i18n/es.go:252` | Spanish confirmations did not accept `y`/`yes`. | Fixed in 90580b5. |
| BR-10 | Info | docs | `cli/docs/json-output.md:47-48` | Two table rows had a third column. | Fixed in 90580b5. |

## Areas checked without findings

- Root resolution (`EvalSymlinks` once), path building from validated slugs and
  ids with a containment check, `lstat` of every parent, `O_NOFOLLOW|O_NONBLOCK`
  plus `fstat`/`SameFile`, size checked before reading and counted while reading.
- Atomic writes: temp file in the same directory, 0600, `fsync`, re-check,
  rename, directory `fsync`; temp file removed on failure. New directories 0700.
- YAML: anchors, aliases, explicit tags, merge keys, multiple documents,
  duplicate and unknown keys refused; typed accessors refuse coerced scalars.
- Archives: constant-memory pre-scan of the central directory (entry count,
  directory size, overlapping entries, local/central header agreement), name
  checks, entry types, encryption, methods, declared and inflated sizes, ratio,
  total size; import goes through a staging directory renamed into place.
- gitsync: no shell, fixed argv, paths passed on stdin with NUL separators,
  environment scrubbed of `GIT_DIR`-style variables, editors and pager disabled,
  hooks and fsmonitor disabled, unmerged and in-progress states refused.
- Editor: argv split without a shell, private temp dir, file read back with
  `lstat`, size and encoding checks; edits keep the version read before editing.
- MCP: write tools registered only with `--allow-write`; JSON-schema patterns
  plus server-side validation; request line cap, page and byte caps, truncation
  flags; errors mapped to fixed messages without content; data-not-instructions
  notes in the instructions and every tool description.
- Terminal output: titles, bodies, table cells, snippets, links, git stderr and
  warnings go through `output.Line`/`output.Text`.

## Dependencies (`cli/go.mod`)

| Module | Version | Note |
|---|---|---|
| github.com/spf13/cobra | v1.10.2 | allowed |
| go.yaml.in/yaml/v3 | v3.0.6 | allowed |
| github.com/modelcontextprotocol/go-sdk | v1.8.0 | official MCP SDK (MCP server) |
| github.com/google/jsonschema-go | v0.4.3 | used by the MCP SDK and for tool schemas |
| github.com/spf13/pflag | v1.0.9 | indirect, cobra |
| github.com/inconshreveable/mousetrap | v1.1.0 | indirect, cobra (Windows) |
| github.com/segmentio/encoding | v0.5.4 | indirect, MCP SDK JSON decoding |
| github.com/segmentio/asm | v1.1.3 | indirect, segmentio/encoding |
| github.com/yosida95/uritemplate/v3 | v3.0.2 | indirect, MCP SDK resource templates |
| golang.org/x/oauth2 | v0.35.0 | indirect, MCP SDK auth package (not used by the stdio server) |
| golang.org/x/sync | v0.20.0 | indirect |
| golang.org/x/sys | v0.41.0 | indirect |
| golang.org/x/time | v0.15.0 | indirect |

Nothing unexpected for a cobra + MCP SDK build.

## Not verified

- `govulncheck ./...` was not run in this review (no access to vuln.go.dev);
  run it in CI before release.
- Publication dates of the pinned versions were not re-checked.
- Windows and macOS were not tested; all tests ran on Linux.
- Fuzz targets were not re-run in this review.
- Behaviour of `git config --show-scope` for keys reached through `include.path`
  in every supported git version (the refusal relies on them being reported
  with the `local` scope).
