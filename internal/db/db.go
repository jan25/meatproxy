// Package db owns the SQLite database in ~/.meatproxy: schema migrations,
// session indexing, FTS search, and feature↔worktree↔session mapping.
package db

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jan25/meatproxy/internal/paths"

	_ "modernc.org/sqlite"
)

// Session is one harness session as stored for search.
type Session struct {
	OCID           string
	Harness        string
	Title          string
	ProjectDir     string
	CreatedAt      int64 // unix millis
	UpdatedAt      int64 // unix millis
	ContentIndexed bool
}

// Feature maps a feature slug in one repo to its worktree and opencode session.
type Feature struct {
	Slug         string
	RepoRoot     string
	WorktreePath string
	OCSessionID  string // empty until linked
	CreatedAt    int64
}

// Match is a search hit.
type Match struct {
	Session
	Rank float64
}

func Open() (*sql.DB, error) {
	handle, err := sql.Open("sqlite", paths.DBPath())
	if err != nil {
		return nil, err
	}
	// WAL lets the background index-worker write while commands read.
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := handle.Exec(pragma); err != nil {
			handle.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	if err := migrate(handle); err != nil {
		handle.Close()
		return nil, err
	}
	return handle, nil
}

// migrations are applied in order; version = index + 1.
var migrations = []string{
	// 1: sessions + FTS + features.
	`CREATE TABLE sessions (
		id INTEGER PRIMARY KEY,
		oc_id TEXT NOT NULL,
		harness TEXT NOT NULL DEFAULT 'opencode',
		title TEXT NOT NULL,
		project_dir TEXT NOT NULL,
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL,
		content_indexed INTEGER NOT NULL DEFAULT 0,
		UNIQUE (harness, oc_id)
	);
	CREATE INDEX idx_sessions_updated ON sessions (updated_at DESC);
	CREATE INDEX idx_sessions_content ON sessions (content_indexed) WHERE content_indexed = 0;
	CREATE VIRTUAL TABLE sessions_fts USING fts5(title, body);
	CREATE TABLE features (
		slug TEXT NOT NULL,
		repo_root TEXT NOT NULL,
		worktree_path TEXT NOT NULL,
		branch TEXT NOT NULL,
		oc_session_id TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		PRIMARY KEY (slug, repo_root)
	);`,
	// 2: drop the unused branch column — worktrees are detached.
	`CREATE TABLE features_v2 (
		slug TEXT NOT NULL,
		repo_root TEXT NOT NULL,
		worktree_path TEXT NOT NULL,
		oc_session_id TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL,
		PRIMARY KEY (slug, repo_root)
	);
	INSERT INTO features_v2 (slug, repo_root, worktree_path, oc_session_id, created_at)
		SELECT slug, repo_root, worktree_path, oc_session_id, created_at FROM features;
	DROP TABLE features;
	ALTER TABLE features_v2 RENAME TO features;`,
	// 3: keep the branch column for compatibility with older binaries that
	// still query it. New code never reads or writes it.
	`ALTER TABLE features ADD COLUMN branch TEXT NOT NULL DEFAULT '';`,
}

func migrate(handle *sql.DB) error {
	if _, err := handle.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY)`); err != nil {
		return err
	}
	var version int
	if err := handle.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM schema_migrations`).Scan(&version); err != nil {
		return err
	}
	for i := version; i < len(migrations); i++ {
		tx, err := handle.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(migrations[i]); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %d: %w", i+1, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations (version) VALUES (?)`, i+1); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// UpsertSession stores/refreshes one session; returns its row id.
func UpsertSession(handle *sql.DB, s Session) (int64, error) {
	res, err := handle.Exec(`INSERT INTO sessions (oc_id, harness, title, project_dir, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (harness, oc_id) DO UPDATE SET
			title = excluded.title,
			project_dir = excluded.project_dir,
			updated_at = excluded.updated_at`,
		s.OCID, s.Harness, s.Title, s.ProjectDir, s.CreatedAt, s.UpdatedAt)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	if id == 0 { // conflict path on some drivers: look it up
		err := handle.QueryRow(`SELECT id FROM sessions WHERE harness = ? AND oc_id = ?`, s.Harness, s.OCID).Scan(&id)
		if err != nil {
			return 0, err
		}
	}
	return id, nil
}

// SessionsWithoutContent returns up to limit session rows that still need
// message-text indexing, newest first.
func SessionsWithoutContent(handle *sql.DB, limit int) ([]Session, error) {
	rows, err := handle.Query(`SELECT oc_id, harness, title, project_dir, created_at, updated_at
		FROM sessions WHERE content_indexed = 0 ORDER BY updated_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Session
	for rows.Next() {
		var s Session
		if err := rows.Scan(&s.OCID, &s.Harness, &s.Title, &s.ProjectDir, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// IndexContent stores a session's message body in FTS and marks it indexed.
func IndexContent(handle *sql.DB, ocID, body string) error {
	tx, err := handle.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if err := tx.QueryRow(`SELECT id FROM sessions WHERE oc_id = ?`, ocID).Scan(&id); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM sessions_fts WHERE rowid = ?`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO sessions_fts (rowid, title, body)
		SELECT rowid, title, ? FROM sessions WHERE id = ?`, body, id); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE sessions SET content_indexed = 1 WHERE id = ?`, id); err != nil {
		return err
	}
	return tx.Commit()
}

// ftsQuery turns user keywords into a prefix-match FTS5 query.
func ftsQuery(keywords []string) string {
	parts := make([]string, len(keywords))
	for i, kw := range keywords {
		parts[i] = `"` + strings.ReplaceAll(kw, `"`, `""`) + `"*`
	}
	return strings.Join(parts, " ")
}

// Search finds sessions whose title or indexed body matches the keywords,
// newest first. Empty keywords returns recent sessions.
func Search(handle *sql.DB, keywords []string, limit int) ([]Match, error) {
	var rows *sql.Rows
	var err error
	if len(keywords) == 0 {
		rows, err = handle.Query(`SELECT s.oc_id, s.title, s.project_dir, s.created_at, s.updated_at, s.content_indexed, 0
			FROM sessions s ORDER BY s.updated_at DESC LIMIT ?`, limit)
	} else {
		rows, err = handle.Query(`SELECT s.oc_id, s.title, s.project_dir, s.created_at, s.updated_at, s.content_indexed,
				bm25(sessions_fts) AS rank
			FROM sessions_fts JOIN sessions s ON s.id = sessions_fts.rowid
			WHERE sessions_fts MATCH ? ORDER BY s.updated_at DESC LIMIT ?`, ftsQuery(keywords), limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Match
	for rows.Next() {
		var m Match
		if err := rows.Scan(&m.OCID, &m.Title, &m.ProjectDir, &m.CreatedAt, &m.UpdatedAt, &m.ContentIndexed, &m.Rank); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// UpsertFeature creates or updates a feature mapping; pass ocSessionID to link.
func UpsertFeature(handle *sql.DB, f Feature) error {
	_, err := handle.Exec(`INSERT INTO features (slug, repo_root, worktree_path, oc_session_id, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (slug, repo_root) DO UPDATE SET
			worktree_path = excluded.worktree_path,
			oc_session_id = CASE WHEN excluded.oc_session_id = '' THEN features.oc_session_id ELSE excluded.oc_session_id END`,
		f.Slug, f.RepoRoot, f.WorktreePath, f.OCSessionID, time.Now().UnixMilli())
	return err
}

// GetFeature looks up a feature by slug and repo root.
func GetFeature(handle *sql.DB, slug, repoRoot string) (Feature, bool, error) {
	var f Feature
	err := handle.QueryRow(`SELECT slug, repo_root, worktree_path, oc_session_id, created_at
		FROM features WHERE slug = ? AND repo_root = ?`, slug, repoRoot).
		Scan(&f.Slug, &f.RepoRoot, &f.WorktreePath, &f.OCSessionID, &f.CreatedAt)
	if err == sql.ErrNoRows {
		return Feature{}, false, nil
	}
	if err != nil {
		return Feature{}, false, err
	}
	return f, true, nil
}

// FeaturesByRoot returns every feature mapping recorded for a repo root.
func FeaturesByRoot(handle *sql.DB, repoRoot string) ([]Feature, error) {
	rows, err := handle.Query(`SELECT slug, repo_root, worktree_path, oc_session_id, created_at
		FROM features WHERE repo_root = ? ORDER BY created_at DESC`, repoRoot)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Feature
	for rows.Next() {
		var f Feature
		if err := rows.Scan(&f.Slug, &f.RepoRoot, &f.WorktreePath, &f.OCSessionID, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// LinkFeatureSession attaches an opencode session id to an existing feature.
func LinkFeatureSession(handle *sql.DB, slug, repoRoot, ocSessionID string) error {
	_, err := handle.Exec(`UPDATE features SET oc_session_id = ? WHERE slug = ? AND repo_root = ?`,
		ocSessionID, slug, repoRoot)
	return err
}

// EnsureDirExists is a small helper so callers outside the package don't
// import os for the data dir.
func EnsureDir() { _ = os.MkdirAll(paths.Dir(), 0o755) }
