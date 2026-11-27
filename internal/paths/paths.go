// Package paths resolves meatproxy's dedicated on-disk locations.
package paths

import (
	"os"
	"path/filepath"
)

// Dir returns ~/.meatproxy, creating it if needed.
func Dir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		panic("meatproxy: cannot resolve home directory: " + err.Error())
	}
	dir := filepath.Join(home, ".meatproxy")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic("meatproxy: cannot create " + dir + ": " + err.Error())
	}
	return dir
}

// DBPath returns the path of the SQLite database.
func DBPath() string { return filepath.Join(Dir(), "meatproxy.db") }

// LogsDir returns the log directory, creating it if needed.
func LogsDir() string {
	dir := filepath.Join(Dir(), "logs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		panic("meatproxy: cannot create " + dir + ": " + err.Error())
	}
	return dir
}

// LockPath returns the index-worker lock file path.
func LockPath() string { return filepath.Join(Dir(), "index.lock") }
