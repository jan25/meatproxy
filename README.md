# meatproxy

A thin wrapper around coding harnesses (OpenCode in v0) that adds two things:

1. **Keyword session discovery** — search past sessions by title and message text, backed by a local SQLite FTS5 index.
2. **Worktree per feature** — one git worktree and session per feature, automatically created or reused.

## Install

```sh
go install .          # installs $GOPATH/bin/meatproxy (usually ~/go/bin/meatproxy)
```

Requires Go 1.24+ and `opencode` on `PATH`. No cgo toolchain required (pure-Go SQLite).

## Usage

```sh
meatproxy new <feature>      # create or reuse a worktree, launch opencode, link the session
meatproxy list [keywords…]   # search sessions (no keywords = recent sessions)
meatproxy open [keywords…]   # search, pick, resume the session in opencode
```

Examples:

```sh
meatproxy new auth-refresh        # worktree .worktrees/auth-refresh, branch mp/auth-refresh
meatproxy new auth-refresh        # rerun → resumes that feature's session in its worktree
meatproxy list sqlite fts         # keyword search (prefix match, AND across words)
meatproxy open "rate limit"       # pick a session and resume it
meatproxy list --deep oldkeyword # block until every session's message text is indexed
```

`open` execs `opencode` in the session's project/worktree directory, so the TUI
appears in your terminal. Your shell stays in its original directory.

## How sync works (no `sync` command)

- **Titles:** `opencode session list --format json` runs inline at the start of `list`/`open` (~100ms). Results are never stale. Sync covers the current directory, the git root, its parent workspace, and all registered feature worktrees.
- **Message text:** a detached, flock-protected background process (`meatproxy __index-worker`) calls `opencode session export` and fills the FTS index via SQLite WAL. No command waits on it. Each worker has a 5-minute deadline and exits cleanly; a worker started while another runs exits immediately.
- **`--deep`** on `list`/`open` is the only blocking path: it finishes indexing before searching.
- Search matches titles immediately; message-text recall fills in over the first few runs. Newly created sessions are marked `(title-only)` until the worker indexes them.

## Storage and logs

All state lives under `~/.meatproxy/`:

| Path | Contents |
|---|---|
| `meatproxy.db` | sessions, FTS5 index, feature↔worktree↔session mapping |
| `logs/meatproxy-YYYYMMDD.log` | command log (subprocess calls, timings, errors) |
| `logs/index-worker-YYYYMMDD.log` | background indexer log |
| `index.lock` | prevents concurrent index workers |

Set `MEATPROXY_LOG=debug` for verbose logs. Logs older than 7 days, or oldest-first beyond 50MB total, are deleted on every start.

Every harness subprocess goes through `internal/execx`: hard timeouts (15s for session list, 60s per export, 30s for git ops), process-group kills on timeout, 1MB output caps, and full logging. No subprocess can hang a command or run indefinitely.

The interactive `opencode` TUI is the deliberate exception: it takes over the terminal with inherited stdio and no timeout.

## Worktrees

- Path: `<repo>/.worktrees/<feature>`, branch `mp/<feature>`.
- `.worktrees/` is added to `.git/info/exclude` (local-only; your `.gitignore` is untouched).
- If a worktree exists, `new` reuses it; if its session is linked, `new` resumes that session. Sessions created in a worktree are synced alongside the main repo's sessions.

## v0 limits

OpenCode only (provider seams exist for other harnesses); no config file; no worktree cleanup/merge commands; search is session-level (titles + message text), not token-level excerpts.
