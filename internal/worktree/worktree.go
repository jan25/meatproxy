// Package worktree creates and reuses one detached git worktree per feature,
// at <repo>/.worktrees/<feature>. No branch is created.
package worktree

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"meatproxy/internal/execx"
	"meatproxy/internal/logx"
)

const (
	gitTimeout    = 30 * time.Second
	worktreesName = ".worktrees"
)

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug normalizes a feature name to a filesystem-safe token.
func Slug(feature string) string {
	s := slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(feature)), "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return ""
	}
	if len(s) > 60 {
		s = s[:60]
	}
	return s
}

// RepoRoot returns the absolute git root containing dir, or "" if dir is not
// inside a work tree.
func RepoRoot(ctx context.Context, dir string) (string, error) {
	out, err := execx.RunOK(ctx, dir, gitTimeout, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return "", nil // not a repo (or git missing): caller decides
	}
	return strings.TrimSpace(string(out)), nil
}

// Ensure returns the worktree path for the feature, creating a detached
// worktree at the current HEAD if needed, and reusing it if it already exists.
func Ensure(ctx context.Context, repoRoot, slug string) (path string, created bool, err error) {
	path = filepath.Join(repoRoot, worktreesName, slug)

	if _, statErr := os.Stat(path); statErr == nil {
		return path, false, nil // reuse
	}

	if err := excludeWorktrees(repoRoot); err != nil {
		logx.Warnf("could not update .git/info/exclude: %v", err)
	}

	if _, err := execx.RunOK(ctx, repoRoot, gitTimeout,
		"git", "worktree", "add", "--detach", path); err != nil {
		return "", false, fmt.Errorf("create worktree %s: %w", path, err)
	}
	return path, true, nil
}

// excludeWorktrees adds .worktrees/ to .git/info/exclude so the worktrees
// directory never dirties the user's working tree or .gitignore.
func excludeWorktrees(repoRoot string) error {
	exclude := filepath.Join(repoRoot, ".git", "info", "exclude")
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	data, err := os.ReadFile(exclude)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	line := worktreesName + "/"
	if strings.Contains(string(data), line) {
		return nil
	}
	f, err := os.OpenFile(exclude, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(data) > 0 && data[len(data)-1] != '\n' {
		f.WriteString("\n")
	}
	_, err = f.WriteString("# meatproxy worktrees\n" + line + "\n")
	return err
}
