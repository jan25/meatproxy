# Changelog

Entries are added when pull requests merge, newest first. Format:

```md
## <PR number> — <short title> (YYYY-MM-DD)

- change
```

## #7 — README scoped to purpose and usage (2026-11-27)

- README now covers only what the tool solves and how to use it; implementation details live in DESIGN.md.

## #6 — Session-only fallback outside git repos (2026-11-27)

- `new` no longer errors outside a repo: it runs a session in the current directory and tracks the feature there for reruns.
- Feature directories are synced for all scopes, including directory-scoped features outside repos.

## #5 — Detached worktrees, system install, README restructure (2026-11-27)

- Worktrees are created with detached HEAD; no branch is created. Schema v2 drops the branch column; v3 restores it as empty so older binaries keep working.
- `install.sh` installs to a default system PATH directory, refreshes `~/go/bin/meatproxy`, and adds an `mp` alias only if none exists.
- README now covers the problem, high-level workflow, and real usage; implementation notes moved to `DESIGN.md`.

## #4 — Empty merge (2026-11-27)

- No code changed; a push from the wrong worktree merged an empty squash commit. Changes landed in #5.

## #3 — Fix changelog prior-history note (2026-11-27)

- Replace outdated PR range in the prior-history note with a plain description.

## #2 — Record merged PRs in changelog (2026-11-27)

- Add changelog workflow: every change merges via PR, and each merged PR gets an entry.

## #1 — Add install script and fix CLI UX issues (2026-11-27)

- Add `./install.sh` that builds onto PATH and honors `MEATPROXY_INSTALL_DIR`.
- Fix `--help` being parsed as a search keyword in `list` and `open`.
- Picker re-prompts on invalid input and renders cleanly with piped or interactive input.
- Rune-safe title truncation; changelog and PR workflow documented in `AGENTS.md`.

Prior history: the initial v0 was committed before this changelog existed — CLI, SQLite/FTS5 session search, worktree-per-feature flow, docs, and the private→public repo change.
