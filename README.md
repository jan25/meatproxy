# meatproxy

Thin wrapper around coding harnesses (OpenCode in v0) for **keyword session discovery** and **one git worktree per feature**.

Requires Go 1.24+ and `opencode` on `PATH`.

```sh
./install.sh                             # builds and installs onto PATH
# or: go install .                        # installs to ~/go/bin
```

## Demo

```sh
$ cd ~/code/myapp

$ meatproxy new auth-refresh          # creates worktree + branch, launches opencode
worktree created: .worktrees/auth-refresh (branch mp/auth-refresh)
linked session ses_abc123 to feature "auth-refresh"

$ meatproxy new auth-refresh          # rerun: reuses worktree, resumes its session
resuming session for feature "auth-refresh"

$ meatproxy list rate limit           # keyword search (prefix match, AND across words)
2h      Fix rate limit headers  ~/code/myapp    ses_abc123

$ meatproxy open rate                  # unique match resumes directly; multiple → pick list
resuming Fix rate limit headers in .worktrees/auth-refresh

$ meatproxy list --deep "old bug"      # block until all message text is indexed
```

`list` with no keywords shows recent sessions. Flags (`--deep`, `--limit N` / `-n`) work before or after keywords. Logs: `~/.meatproxy/logs/` (`MEATPROXY_LOG=debug`).

## How it works

- Titles sync inline per command (~100ms); message text is indexed by a detached background worker using SQLite WAL. `--deep` is the only blocking path. Newly found sessions show `(title-only)` until indexed.
- Worktrees live at `<repo>/.worktrees/<feature>` on `mp/<feature>`; `.worktrees/` is excluded via `.git/info/exclude` (your `.gitignore` is untouched).
- State: `~/.meatproxy/meatproxy.db` (sessions, FTS5 index, worktree map). Logs older than 7 days or over 50MB are pruned automatically.
- All captured subprocesses use hardened timeouts, process-group kills, output caps, and logging; the interactive TUI is exempt.

Details for future contributors: [AGENTS.md](AGENTS.md), [DESIGN.md](DESIGN.md).

v0 limits: OpenCode only, no config file, no worktree cleanup/merge commands.
