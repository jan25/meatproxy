# 🥩 meatproxy

Sessions from coding harnesses pile up, and finding the one where you solved a specific problem usually means scrolling through names. Working on parallel features has the same friction: creating worktrees by hand and remembering which session belongs to which.

meatproxy makes past sessions searchable by keyword and pairs each feature with its own worktree and session.

Requires Go 1.24+ and `opencode` on `PATH`. Install with the script (it also adds an `mp` alias):

```sh
curl -fsSL https://raw.githubusercontent.com/jan25/meatproxy/main/install.sh | sh
```

From a local clone, run `./install.sh` instead.

## Usage

```sh
$ mp new auth-refresh                  # start a feature in its own worktree
🥩 worktree created: .worktrees/auth-refresh (detached HEAD)
🥩 linked session ses_abc123 to feature "auth-refresh"

$ mp new auth-refresh                  # come back to the same feature
🥩 resuming session for feature "auth-refresh"

$ mp list rate limit                   # find a past session by keyword
🥩 2h      Fix rate limit headers  ~/code/myapp    ses_abc123

$ mp open rate                         # one match resumes it directly
🥩 resuming Fix rate limit headers in .worktrees/auth-refresh

$ mp open auth                         # several matches → numbered pick list
🥩   1. 2h      Fix rate limit headers  [~/code/myapp]
🥩   2. 3d      Auth refresh work       [~/code/myapp/.worktrees/auth]
🥩 number (enter to cancel): 2

$ mp list --deep "old bug"             # wait for full-text indexing before searching

$ mp update                            # refresh the CLI from the published install script
🥩 updated to the latest version
```

`list` with no keywords shows recent sessions. Flags (`--deep`, `--limit N` / `-n`) work before or after keywords.

Details: [DESIGN.md](DESIGN.md). Contributor rules: [AGENTS.md](AGENTS.md).
