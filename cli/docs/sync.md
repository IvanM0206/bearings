# `bearings sync`

`bearings sync` keeps a notebook in a git repository and, if the current
branch has an upstream, in step with its remote.

```
bearings sync              commit, then pull --rebase and push
bearings sync --dry-run    show what would be committed; change nothing
bearings sync --verbose    also show which git is used
```

## Setting it up

The notebook directory must be the top level of its own git repository
(not a subdirectory of another one). Use a **private** remote: the notebook
holds your work notes.

```
git -C ~/bearings init
bearings sync                                   # first commit
git -C ~/bearings remote add origin <url of a private repository>
git -C ~/bearings push -u origin HEAD           # sets the upstream
```

From then on, `bearings sync` does everything. Without an upstream it only
commits, and says so.

## What it does

1. Checks that `git` (2.26 or later) is on `PATH`, that the notebook is the
   top level of a work tree, that no rebase or merge is in progress, and
   that the repository's own configuration is safe (see below).
2. Lists the changed notebook files: `bearings.yaml`, `sections/…` and
   `journal/…` paths that match the notebook layout exactly. Anything else
   (a stray `notes.txt`, an editor swap file, a misplaced journal file)
   is never staged and stays untracked.
3. Stages and commits exactly those paths, with the message
   `bearings sync <UTC timestamp>`. Changes you staged yourself in other
   files stay staged and are not committed.
4. With an upstream: `git pull --rebase`, then `git push` to the upstream.

`--dry-run` stops after step 2 and does not write to the repository (not
even git's index). `--json` prints
`{"dry_run", "changes": [{"path", "change"}], "commit", "upstream", "pulled", "pushed"}`
(`change` is `added`, `modified` or `deleted`).

## Conflicts

If your commits conflict with changes on the remote, the rebase stops and
`bearings sync` stops with it: nothing is pushed, and the repository is
left exactly as git left it, in the middle of the rebase. It prints the
commands to look at the conflict (`git status`, `git diff`), to continue
(`git add <file>`, `git rebase --continue`) or to give up
(`git rebase --abort`). It never resolves conflicts, resets, stashes or
force-pushes. While the rebase is unfinished, `bearings sync` refuses to
run, so a file with conflict markers is never committed by it.

A rejected push (someone pushed in the meantime) keeps your commits
locally; run `bearings sync` again.

## Security

A notebook directory may come from someone else, and so may its `.git`
directory (a copied folder, an unpacked tarball). git's configuration and
hooks in `.git` can run programs, so `bearings sync` treats them as
hostile.

**How git is run.** Always through `exec` with an explicit argument list,
never through a shell, with `-C <notebook>`. No notebook content is ever
an argument: the commit message is fixed, and the paths to commit (already
validated as notebook layout paths) go to git on standard input
(`--pathspec-from-file`). `--literal-pathspecs` turns off wildcard
matching. Environment variables that would point git at another repository
or change path matching (`GIT_DIR`, `GIT_WORK_TREE`, `GIT_INDEX_FILE`,
`GIT_*_PATHSPECS`, …) are removed. `bearings sync` never handles
credentials: git's own credential helpers, SSH agent and prompts work as
usual. Without a terminal, `GIT_TERMINAL_PROMPT=0` keeps git from waiting
for input.

**Settings overridden on every git command** (command-line `-c` options
win over every configuration file):

| Option | Why | Trade-off |
|---|---|---|
| `core.fsmonitor=false` | An fsmonitor setting is a program git runs on `status`, `add`, `commit`. | None for a notebook-sized repository. |
| `core.hooksPath=/dev/null` | Hooks (`.git/hooks/*`, or a directory named in the config) are programs. A path that cannot hold hooks disables all of them on every git version and platform; an empty value has meant different things in different git versions. | Your own hooks (a secret scanner, a formatter) do not run during `bearings sync`. They still run when you use git directly. |
| `protocol.ext.allow=never` | The `ext::` transport runs a command named in the remote URL. | None. |
| `protocol.file.allow=user` | Local paths work as remotes when you ask for them, but not for fetches git starts on its own (submodules). | None. |
| `submodule.recurse=false`, `--no-recurse-submodules` | Submodules could fetch from anywhere and run their own configuration. | Submodules in a notebook repository are not updated. |
| `push.default=upstream` | Push only the current branch, to its upstream. | A different push remote (`pushRemote`) is not supported. |

`GIT_EDITOR` and `GIT_SEQUENCE_EDITOR` are set to `:` (git's no-op editor)
and `GIT_PAGER` to `cat`, so a configured editor or pager never runs.

**Settings refused when the repository itself sets them.** Some settings
cannot be overridden without also overriding your own global values (your
SSH key setup, your password manager). Instead, `bearings sync` reads the
*names* of the settings in the repository's own configuration
(`.git/config`, the worktree configuration, and every file they include;
values are never read or printed) and refuses to run, naming the keys,
when any of these is set there:

- programs: `core.sshCommand`, `core.askPass`, `core.gitProxy`,
  `core.alternateRefsCommand`, `credential.helper` (also per URL),
  `gpg.program` (and `gpg.<format>.program`), `gpg.ssh.defaultKeyCommand`,
  `filter.<driver>.clean|smudge|process`, `diff.external`,
  `diff.<driver>.textconv|command`, `merge.<driver>.driver`,
  `remote.<name>.uploadpack|receivepack`;
- where data goes and how it is protected: `url.<base>.insteadOf` and
  `pushInsteadOf`, `remote.<name>.proxy`, `http.proxy`, `http.sslVerify`,
  `http.sslCAInfo`, `http.sslCAPath`, `http.sslCert`, `http.sslKey`,
  `http.cookieFile`, `http.extraHeader`, `http.curloptResolve` (also per
  URL), `protocol.allow` and `protocol.<name>.allow`;
- `core.worktree`.

If you set one of these yourself, move it to your global configuration
(`git config --global …`); there it is trusted and keeps working. A
repository that sets `core.fsmonitor` or `core.hooksPath` is not refused,
since those are overridden; `bearings sync` prints a warning instead.

This list covers the settings that make git run programs or send data
elsewhere during `add`, `commit`, `pull --rebase` and `push`; it cannot
make an untrusted repository trustworthy. If a notebook came with someone
else's `.git` directory, the safest course is to delete `.git` and run
`git init` again. Check where your notebook is pushed with
`git -C <notebook> remote -v`.

git's own protections stay on: it refuses repositories owned by another
user (`safe.directory`).
