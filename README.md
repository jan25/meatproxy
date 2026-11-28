# meatproxy

Sessions from coding harnesses accumulate fast, and finding the one where you solved a specific problem usually means scrolling through names. Switching between parallel features has the same problem: manually creating worktrees and remembering which session belongs to which one.

meatproxy fixes both: it makes past sessions searchable by keyword and bundles each feature into its own detached worktree with a linked session.

## How it works, at a glance

- `opencode` remains the source of truth for sessions. meatproxy reads its session list and transcripts, keeps a local SQLite full-text index, and searches that index by keyword. Indexing happens in the background, so commands stay fast.
- `new <feature>` creates a worktree at `.worktrees/<feature>` (detached HEAD — no branch), launches `opencode` there, and remembers the session. Rerunning it resumes the same worktree and session.
- `list` finds sessions by title or message text; `open` resumes the chosen session in its original directory.

Requires Go 1.24+ and `opencode` on `PATH`. `./install.sh` installs the CLI on PATH and adds an `mp` alias if you don't already have one.

## Demo

```sh
$ mp new auth-refresh                  # start a feature in its own worktree
worktree created: .worktrees/auth-refresh (detached HEAD)
linked session ses_abc123 to feature "auth-refresh"

$ mp new auth-refresh                  # come back to the same feature
resuming session for feature "auth-refresh"

$ mp list rate limit                   # find a past session by keyword
2h      Fix rate limit headers  ~/code/myapp    ses_abc123

$ mp open rate                         # one match resumes it directly
resuming Fix rate limit headers in .worktrees/auth-refresh

$ mp open auth                         # several matches → numbered pick list
  1. 2h      Fix rate limit headers  [~/code/myapp]
  2. 3d      Auth refresh work       [~/code/myapp/.worktrees/auth]
number (enter to cancel): 2

$ mp list --deep "old bug"             # wait for full-text indexing before searching
```

`list` with no keywords shows recent sessions. Flags (`--deep`, `--limit N` / `-n`) work before or after keywords.

Implementation details — storage, sync mechanics, subprocess safety, schema, and extension points — are in [DESIGN.md](DESIGN.md). Contributor rules are in [AGENTS.md](AGENTS.md).
