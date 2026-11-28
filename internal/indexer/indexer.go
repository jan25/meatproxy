// Package indexer implements implicit, non-blocking synchronization between
// OpenCode and the local SQLite index.
//
// Title sync runs inline on every command (one fast `session list` call).
// Message-text indexing runs in a detached background worker process so no
// command ever waits on exports. `--deep` is the only blocking path.
package indexer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/jan25/meatproxy/internal/db"
	"github.com/jan25/meatproxy/internal/logx"
	"github.com/jan25/meatproxy/internal/oc"
	"github.com/jan25/meatproxy/internal/paths"
	"github.com/jan25/meatproxy/internal/ui"
)

const (
	// backlog is how many unindexed sessions the worker takes per run.
	workerBacklog = 50
	// workerDeadline caps a single worker run so it can never run indefinitely.
	workerDeadline = 5 * time.Minute
	// recentBacklog caps how far behind we try to catch up on each run.
	spawnBacklog = workerBacklog
)

// SyncTitles refreshes session metadata for the project at dir into the DB.
// Fast, and always run before list/open so results are never stale.
func SyncTitles(ctx context.Context, dir string) error {
	sessions, err := oc.ListSessions(ctx, dir, 500)
	if err != nil {
		return err
	}
	handle, err := db.Open()
	if err != nil {
		return err
	}
	defer handle.Close()
	for _, s := range sessions {
		if _, err := db.UpsertSession(handle, db.Session{
			OCID: s.ID, Harness: "opencode",
			Title: s.Title, ProjectDir: s.Directory,
			CreatedAt: s.Created, UpdatedAt: s.Updated,
		}); err != nil {
			return err
		}
	}
	logx.Debugf("title sync: %d sessions for %s", len(sessions), dir)
	return nil
}

// EnsureContent spawns a detached background worker to index message text for
// sessions that lack it, and returns immediately. Failures to spawn are logged
// and ignored — they never affect the command the user is running.
func EnsureContent() {
	pending, err := countPending()
	if err != nil {
		logx.Warnf("content backlog check: %v", err)
		return
	}
	if pending == 0 {
		return
	}
	logx.Debugf("content backlog: %d sessions; spawning index worker", pending)

	self, err := os.Executable()
	if err != nil {
		logx.Warnf("cannot spawn index worker: %v", err)
		return
	}
	cmd := execCommand(self, "__index-worker")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // detach from our group
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		logx.Warnf("spawn index worker: %v", err)
		return
	}
	// Deliberately not waited on: the worker outlives this command.
	_ = cmd.Process.Release()
}

// DeepIndex indexes everything missing, blocking until done or deadline.
func DeepIndex() error {
	pending, err := countPending()
	if err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	fmt.Fprintf(os.Stderr, "%s deep indexing %d sessions…\n", ui.Logo, pending)
	return runWorker(false)
}

func countPending() (int, error) {
	handle, err := db.Open()
	if err != nil {
		return 0, err
	}
	defer handle.Close()
	pending, err := db.SessionsWithoutContent(handle, spawnBacklog)
	if err != nil {
		return 0, err
	}
	return len(pending), nil
}

// RunWorker is the entry point of the hidden `__index-worker` mode.
func RunWorker() int {
	if err := runWorker(true); err != nil {
		logx.Errorf("index worker: %v", err)
		return 1
	}
	return 0
}

// runWorker indexes pending sessions under a lock, with a hard deadline.
func runWorker(alone bool) error {
	lock, err := acquireLock()
	if err != nil {
		if alone { // only the detached worker treats a busy lock as silent
			return nil
		}
		return err
	}
	defer lock.close()

	ctx, cancel := context.WithTimeout(context.Background(), workerDeadline)
	defer cancel()

	handle, err := db.Open()
	if err != nil {
		return err
	}
	defer handle.Close()

	for {
		pending, err := db.SessionsWithoutContent(handle, workerBacklog)
		if err != nil {
			return err
		}
		if len(pending) == 0 {
			return nil
		}
		for _, s := range pending {
			if ctx.Err() != nil {
				logx.Infof("index worker: deadline reached, %d+ sessions remain", len(pending))
				return nil
			}
			body, err := oc.ExportText(ctx, s.ProjectDir, s.OCID)
			if err != nil {
				logx.Warnf("export %s failed (%v); marking skipped", s.OCID, err)
				body = "" // index empty body so we don't retry forever
			}
			if err := db.IndexContent(handle, s.OCID, body); err != nil {
				return fmt.Errorf("index %s: %w", s.OCID, err)
			}
			logx.Debugf("indexed %s (%s)", s.OCID, s.Title)
		}
	}
}

// acquireLock takes an exclusive flock on ~/.meatproxy/index.lock.
type lockFile struct{ f *os.File }

func acquireLock() (*lockFile, error) {
	f, err := os.OpenFile(paths.LockPath(), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("another index worker is running: %w", err)
	}
	return &lockFile{f}, nil
}

func (l *lockFile) close() {
	_ = syscall.Flock(int(l.f.Fd()), syscall.LOCK_UN)
	_ = l.f.Close()
}

func execCommand(name string, args ...string) *exec.Cmd {
	return exec.Command(name, args...)
}
