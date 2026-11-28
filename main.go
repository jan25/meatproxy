// meatproxy is a wrapper around coding harnesses (OpenCode in v0) that adds
// keyword search over past sessions and a worktree-per-feature workflow.
//
//	meatproxy new <feature>    # create/reuse worktree, launch opencode, link session
//	meatproxy list [keywords]  # search sessions (titles + indexed message text)
//	meatproxy open [keywords]  # search, pick, resume in opencode
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"meatproxy/internal/db"
	"meatproxy/internal/indexer"
	"meatproxy/internal/logx"
	"meatproxy/internal/oc"
	"meatproxy/internal/ui"
	"meatproxy/internal/worktree"
)

const searchLimit = 40

func main() {
	logx.Init("meatproxy")
	defer logx.Close()
	db.EnsureDir()

	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	ctx := context.Background()
	var err error
	switch os.Args[1] {
	case "new":
		err = cmdNew(ctx, os.Args[2:])
	case "list":
		err = cmdList(ctx, os.Args[2:])
	case "open":
		err = cmdOpen(ctx, os.Args[2:])
	case "__index-worker": // hidden: detached background content indexer
		logx.Close()
		logx.Init("index-worker")
		os.Exit(indexer.RunWorker())
	case "__worktree": // hidden: worktree ensure without launching opencode (testing)
		err = cmdWorktreeOnly(ctx, os.Args[2:])
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "%s unknown command %q\n\n", ui.Logo, os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, ui.Logo, "error:", err)
		logx.Errorf("command %s failed: %v", os.Args[1], err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(ui.Logo, ` meatproxy — session discovery + worktree-per-feature for OpenCode

usage:
  meatproxy new <feature>     create or reuse a detached worktree
                              (.worktrees/<feature>), launch opencode there,
                              and link the session; rerun to resume it
  meatproxy list [keywords…]  search past sessions (title + message text);
                              no keywords lists recent sessions
  meatproxy open [keywords…]  search, pick, and resume a session in opencode

flags:
  --deep                      block until all pending sessions are fully indexed
                              (without it, indexing continues in the background)
  --limit N                   max results for list (default `+fmt.Sprint(searchLimit)+`)

logging: ~/.meatproxy/logs (level via MEATPROXY_LOG=debug)
`)
}

// syncAndIndex performs the implicit sync: inline title sync (never stale
// results) plus a detached background worker for message text.
// OpenCode scopes session list per project directory, so we sync cwd, the git
// root, and its parent — enough to cover sessions started in the workspace.
func syncAndIndex(ctx context.Context, deep bool) (*sql.DB, error) {
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	root, _ := worktree.RepoRoot(ctx, cwd)
	scopes := []string{cwd}
	if root != "" && root != cwd {
		scopes = append(scopes, root)
	}
	if parent := filepath.Dir(root); root != "" && parent != "" && parent != "/" && parent != cwd && parent != root {
		if info, statErr := os.Stat(parent); statErr == nil && info.IsDir() {
			scopes = append(scopes, parent)
		}
	}
	var syncErr error
	for _, dir := range scopes {
		if err := indexer.SyncTitles(ctx, dir); err != nil {
			syncErr = err
			logx.Warnf("title sync for %s: %v", dir, err)
		}
	}
	handle, err := db.Open()
	if err != nil {
		return nil, err
	}
	// Feature directories are their own opencode projects; sync those too.
	seen := map[string]bool{}
	for _, scope := range scopes {
		features, err := db.FeaturesByRoot(handle, scope)
		if err != nil {
			handle.Close()
			return nil, err
		}
		for _, f := range features {
			if seen[f.WorktreePath] {
				continue
			}
			seen[f.WorktreePath] = true
			if _, statErr := os.Stat(f.WorktreePath); statErr != nil {
				continue
			}
			if err := indexer.SyncTitles(ctx, f.WorktreePath); err != nil {
				logx.Warnf("title sync for %s: %v", f.WorktreePath, err)
			}
		}
	}

	if deep {
		handle.Close()
		if err := indexer.DeepIndex(); err != nil {
			return nil, err
		}
		handle, err = db.Open()
		if err != nil {
			return nil, err
		}
	} else {
		indexer.EnsureContent() // detached; never blocks
	}
	var count int
	if err := handle.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&count); err != nil {
		handle.Close()
		return nil, err
	}
	if count == 0 && syncErr != nil {
		handle.Close()
		return nil, fmt.Errorf("title sync: %w", syncErr)
	}
	return handle, nil
}

// parseArgs accepts --deep / --limit anywhere in the argument list, plus -n
// as an alias for --limit, so flags-after-keywords works naturally.
func parseArgs(args []string, withLimit bool) (deep bool, limit int, keywords []string, err error) {
	limit = searchLimit
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "--deep", "-deep":
			deep = true
		case "--limit", "-limit", "-n", "--n":
			if !withLimit || i+1 >= len(args) {
				return false, 0, nil, fmt.Errorf("%s requires a number", a)
			}
			i++
			limit, err = strconv.Atoi(args[i])
			if err != nil || limit < 1 {
				return false, 0, nil, fmt.Errorf("%s requires a positive number", a)
			}
		default:
			keywords = append(keywords, a)
		}
	}
	return deep, limit, keywords, nil
}

type matchPick db.Match

func (m matchPick) Label() string {
	return fmt.Sprintf("%-7s %s  [%s]%s",
		ui.ShortRelTime(m.UpdatedAt), truncate(m.Title, 70),
		shortPath(m.ProjectDir), indexedMark(m.ContentIndexed))
}

func indexedMark(ok bool) string {
	if ok {
		return ""
	}
	return " (title-only)"
}

func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n-1]) + "…"
}

func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && strings.HasPrefix(p, home) {
		return "~" + p[len(home):]
	}
	return p
}

func printMatches(matches []db.Match) {
	for _, m := range matches {
		ui.Printf("%-7s %s  %s  %s%s\n",
			ui.ShortRelTime(m.UpdatedAt), truncate(m.Title, 70), shortPath(m.ProjectDir),
			m.OCID, indexedMark(m.ContentIndexed))
	}
}

// cmdList implements `meatproxy list [keywords…]`.
func cmdList(ctx context.Context, args []string) error {
	if wantsHelp(args) {
		usage()
		return nil
	}
	deep, limit, keywords, err := parseArgs(args, true)
	if err != nil {
		return err
	}
	handle, err := syncAndIndex(ctx, deep)
	if err != nil {
		return err
	}
	defer handle.Close()
	matches, err := db.Search(handle, keywords, limit)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		ui.Println("no matching sessions")
		return nil
	}
	printMatches(matches)
	return nil
}

// cmdOpen implements `meatproxy open [keywords…]`.
func cmdOpen(ctx context.Context, args []string) error {
	if wantsHelp(args) {
		usage()
		return nil
	}
	deep, _, keywords, err := parseArgs(args, false)
	if err != nil {
		return err
	}
	handle, err := syncAndIndex(ctx, deep)
	if err != nil {
		return err
	}
	defer handle.Close()
	matches, err := db.Search(handle, keywords, searchLimit)
	if err != nil {
		return err
	}
	if len(matches) == 0 {
		return fmt.Errorf("no matching sessions (try --deep to index message text first)")
	}

	var pick db.Match
	if len(matches) == 1 {
		pick = matches[0]
	} else {
		items := make([]ui.Pickable, len(matches))
		for i, m := range matches {
			items[i] = matchPick(m)
		}
		n := ui.Pick(items)
		if n < 0 {
			return nil
		}
		pick = matches[n]
	}

	ui.Printf("resuming %s in %s\n", truncate(pick.Title, 60), shortPath(pick.ProjectDir))
	return oc.Resume(pick.ProjectDir, pick.OCID)
}

// cmdNew implements `meatproxy new <feature>`.
func cmdNew(ctx context.Context, args []string) error {
	if wantsHelp(args) {
		usage()
		return nil
	}
	if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("usage: meatproxy new <feature>")
	}
	slug := worktree.Slug(args[0])
	if slug == "" {
		return fmt.Errorf("feature name must contain letters or digits")
	}

	// Outside a git repo there is no worktree to create: run a session in the
	// current directory and track the feature there instead.
	repoRoot, err := worktree.RepoRoot(ctx, ".")
	noRepo := err != nil || repoRoot == ""
	var path string
	var created bool
	if noRepo {
		path, err = os.Getwd()
		if err != nil {
			return err
		}
		repoRoot = path
	} else {
		path, created, err = worktree.Ensure(ctx, repoRoot, slug)
		if err != nil {
			return err
		}
	}

	handle, err := db.Open()
	if err != nil {
		return err
	}
	feature, existed, err := db.GetFeature(handle, slug, repoRoot)
	if err != nil {
		handle.Close()
		return err
	}
	feature.Slug, feature.RepoRoot, feature.WorktreePath = slug, repoRoot, path
	if err := db.UpsertFeature(handle, feature); err != nil {
		handle.Close()
		return err
	}
	handle.Close()

	switch {
	case noRepo:
		ui.Printf("no git repository; using current directory for feature %q (no worktree)\n", slug)
	case created:
		ui.Printf("worktree created: %s (detached HEAD)\n", path)
	case !existed:
		ui.Printf("worktree reused: %s\n", path)
	}

	// Resume the linked session when we have one and this is a rerun.
	if feature.OCSessionID != "" {
		if _, statErr := os.Stat(path); statErr == nil {
			ui.Printf("resuming session for feature %q\n", slug)
			return oc.Resume(path, feature.OCSessionID)
		}
	}

	// Fresh feature (or unlinkable earlier run): launch opencode, then link
	// whatever session the user started during the TUI run.
	if err := oc.Launch(path); err != nil {
		return err
	}
	linkSession(ctx, slug, repoRoot, path)
	return nil
}

// linkSession best-effort records the newest session created in the worktree.
func linkSession(ctx context.Context, slug, repoRoot, path string) {
	sessions, err := oc.ListSessions(ctx, path, 5)
	if err != nil || len(sessions) == 0 {
		logx.Warnf("could not link session for feature %q: %v", slug, err)
		return
	}
	handle, err := db.Open()
	if err != nil {
		return
	}
	defer handle.Close()
	feature, _, err := db.GetFeature(handle, slug, repoRoot)
	if err != nil {
		return
	}
	newest := sessions[0]
	if newest.ID == feature.OCSessionID {
		return // nothing new was created
	}
	if err := db.LinkFeatureSession(handle, slug, repoRoot, newest.ID); err != nil {
		logx.Warnf("link session: %v", err)
		return
	}
	ui.Printf("linked session %s to feature %q\n", newest.ID, slug)
}

// cmdWorktreeOnly is a hidden helper used in development to verify worktree
// creation without launching the TUI.
func cmdWorktreeOnly(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: meatproxy __worktree <feature>")
	}
	slug := worktree.Slug(args[0])
	repoRoot, err := worktree.RepoRoot(ctx, ".")
	if err != nil || repoRoot == "" {
		return fmt.Errorf("not inside a git repository")
	}
	path, created, err := worktree.Ensure(ctx, repoRoot, slug)
	if err != nil {
		return err
	}
	handle, err := db.Open()
	if err != nil {
		return err
	}
	defer handle.Close()
	feature, existed, err := db.GetFeature(handle, slug, repoRoot)
	if err != nil {
		return err
	}
	feature.Slug, feature.RepoRoot, feature.WorktreePath = slug, repoRoot, path
	if err := db.UpsertFeature(handle, feature); err != nil {
		return err
	}
	ui.Printf("slug=%s path=%s created=%v existed=%v\n", slug, filepath.Base(path), created, existed)
	return nil
}
