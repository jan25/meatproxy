# AGENTS.md — working on meatproxy

## What this is

Go CLI wrapper around OpenCode: keyword session search (SQLite FTS5) + worktree-per-feature. v0 targets OpenCode only. Read `DESIGN.md` before changing behavior; keep `README.md` user-facing and short.

## workflow

All changes go through a pull request — never commit directly to `main`. After a PR merges, add a `## <PR number> — <title> (YYYY-MM-DD)` entry to `CHANGELOG.md` via a follow-up PR. Keep entries short and factual.

## commands

```sh
go build ./...          # build
go vet ./...            # vet
gofmt -w main.go internal
go install .            # binary → ~/go/bin/meatproxy
```

Hidden commands used in development: `meatproxy __worktree <feature>` (ensure worktree without launching TUI), `meatproxy __index-worker` (one-shot indexer run). Do not remove: they are the only non-interactive test seams.

## layout

| Path | Responsibility |
|---|---|
| `main.go` | CLI dispatch, argument parsing, command flows, output formatting |
| `internal/db` | schema migrations, session upserts, FTS search, feature map |
| `internal/oc` | OpenCode CLI provider (list / export / resume / launch) |
| `internal/indexer` | inline title sync, detached worker, flock lock |
| `internal/execx` | **all** captured subprocesses: timeouts, kills, caps, logging |
| `internal/worktree` | detached git worktree create/reuse, `.git/info/exclude` |
| `internal/ui`, `internal/logx`, `internal/paths` | pick list, rotating logs, `~/.meatproxy` paths |

## invariants — do not break

1. **No subprocess may hang or run unbounded.** Captured calls go through `execx.Run`. Interactive `opencode` TUI is the sole exception and must stay outside `execx` (inherited stdio, no timeout).
2. **No command blocks on message-text indexing.** Titles sync inline; exports run in the detached `__index-worker` process under a 5-minute deadline and flock. `--deep` is the only blocking path.
3. **SQLite WAL is required** — the background worker writes while commands read. Keep `busy_timeout` and migrations in `internal/db`.
4. **Schema changes require a new migration**: append a step to the `migrations` slice in `internal/db/db.go`; never edit an applied step. Existing users' DBs must upgrade on open.
5. **OpenCode is the source of truth for sessions.** The DB is a cache; never treat it as authoritative enough to overwrite harness state.
6. **`~/.meatproxy` is user data.** Never delete the DB, logs, or worktrees as a side effect of tests or cleanup. Test in a scratch git repo and scrub only test rows you inserted.
7. Keyword search uses FTS5 prefix matching with AND; no keywords = recent sessions, newest first.
8. Worktree paths are `<repo>/.worktrees/<slug>` with detached HEAD — never create a branch; never touch the user's `.gitignore` (use `.git/info/exclude`).

## conventions

- Standard library first; only extra dependency is `modernc.org/sqlite` (pure Go, no cgo). Do not add cgo SQLite.
- Errors to stderr with context; never print raw subprocess output to the user except the TUI itself.
- Log through `internal/logx`; use `MEATPROXY_LOG=debug` when diagnosing.
- Keep hidden commands hidden and stable; scripts/tests may rely on them.
- Test harness flows with a stub `opencode` earlier on `PATH` (stub `session list --format json`, `session export`, and no-arg launch) — no real TUI in non-interactive tests.

## extension points

- New harness (Claude): add a provider package alongside `internal/oc` with list/export/resume/launch, add a `harness` discriminator in DB writes and dispatch in `main.go`. Do not hardcode OpenCode in `internal/db` search paths.
- Config file: none in v0; add a loader in `internal/paths` and thread options through `main.go`, not through individual packages.
