# Changelog

Entries are added when pull requests merge, newest first. Format:

```md
## <PR number> — <short title> (YYYY-MM-DD)

- change
```

## #2 — Record merged PRs in changelog (2026-11-27)

- Add changelog workflow: every change merges via PR, and each merged PR gets an entry.

## #1 — Add install script and fix CLI UX issues (2026-11-27)

- Add `./install.sh` that builds onto PATH and honors `MEATPROXY_INSTALL_DIR`.
- Fix `--help` being parsed as a search keyword in `list` and `open`.
- Picker re-prompts on invalid input and renders cleanly with piped or interactive input.
- Rune-safe title truncation; changelog and PR workflow documented in `AGENTS.md`.

Prior history: the initial v0 (PRs #1–#3) shipped the CLI, SQLite/FTS5 session search, worktree-per-feature flow, docs, and the private→public repo change.
