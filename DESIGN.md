# DESIGN.md — meatproxy architecture

## Goals

1. Find old coding-harness sessions by keyword, fast.
2. Bind each feature to one git worktree and one session, creating or reusing both automatically.
3. Keep subprocesses and local state bounded: no rogue processes, no unbounded disk growth.

Non-goals for v0: other harnesses, config files, worktree cleanup/merge, token-level search excerpts.

## Shape

```
main.go ──► indexer ──► oc ──► execx ──► opencode CLI
    │           │                          └──► git
    ├──► db (SQLite + FTS5)
    ├──► worktree
    └──► ui / logx
```

`internal/oc` is the only harness-specific package. `internal/db` stores a `harness` discriminator, so adding Claude is a new provider package plus dispatch — no schema rewrite.

## Data model (`~/.meatproxy/meatproxy.db`)

- `sessions`: `oc_id`, `harness`, `title`, `project_dir`, created/updated (unix millis), `content_indexed`. Unique on `(harness, oc_id)`.
- `sessions_fts`: FTS5 virtual table (`title`, `body`), rowid = `sessions.id`. Body is the concatenation of user prompts and assistant text parts from `opencode session export` (reasoning parts skipped).
- `features`: `(slug, repo_root)` → worktree path, linked `oc_session_id`.
- `schema_migrations`: versioned, append-only migrations; applied on every open.

SQLite runs in WAL mode with a busy timeout so the index worker writes while commands read.

## Session discovery

`meatproxy list/open` runs `syncAndIndex`:
1. **Title sync (inline):** `opencode session list --format json` in cwd, git root, parent workspace, and every registered feature worktree. OpenCode scopes sessions per project directory, so multiple scopes are necessary. Upserts by `(harness, oc_id)`; still ~100ms total.
2. **Content indexing (detached):** `EnsureContent` spawns `meatproxy __index-worker` with its own process group and releases it immediately. The worker takes a non-blocking flock on `index.lock`, indexes up to 50 pending sessions per pass under a 5-minute deadline, then exits. A second worker exits immediately; WAL prevents read contention. Failures log and mark sessions indexed (no infinite retry).
3. **`--deep`:** runs the same worker loop inline and blocks — the only explicit wait.

Search: FTS5 prefix query, all keywords ANDed, newest-first ordering. No keywords lists recent sessions. Unindexed sessions are still discoverable by title and marked `(title-only)`.

## Worktree flow

`meatproxy new <feature>`:
- slugify name (`.worktrees/<slug>` path);
- ensure `.git/info/exclude` contains `.worktrees/` (never modify `.gitignore`);
- `git worktree add --detach` at the current HEAD — no branch is created;
- upsert feature mapping; if a session is linked, resume it; otherwise launch a fresh `opencode` TUI in the worktree and, after it exits, link the newest session found there (best effort).

`meatproxy open <keywords>` resumes in the session's recorded `project_dir` — for feature sessions that is the worktree path. The Go process execs the TUI; the calling shell's cwd is unchanged.

## Boundaries and safety

- `internal/execx` is the only place captured subprocesses are started: `context` timeouts (15s list, 60s export, 30s git), `Setpgid` + process-group kill on timeout, 1MB tail caps on stdout/stderr, and timed log entries. This is the defense against rogue processes.
- The TUI is deliberately outside `execx`: it owns the terminal with inherited stdio and no timeout.
- Logs: daily files in `~/.meatproxy/logs/`, `MEATPROXY_LOG` level control, pruned to 7 days / 50MB at startup.
- `~/.meatproxy` is user state; code and tests must never bulk-delete it.

## Known gaps / next work

- Claude provider (list/export/resume/launch) and generic provider interface.
- Cross-project discovery without multiple directory scopes (needs a harness CLI that lists all projects).
- Worse-case recall: FTS indexes full text but returns no snippets; results are session-level only.
- No config file; all defaults are constants in package code.
- `new` session linking is a heuristic (newest session in worktree after TUI exits); explicit session handoff would be more robust.
