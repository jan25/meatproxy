// Package logx provides file-based debug logging with automatic retention.
//
// Logs go to ~/.meatproxy/logs/<name>-YYYYMMDD.log. Prune deletes old or
// oversized logs on every start so disk usage stays bounded.
package logx

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"meatproxy/internal/paths"
)

type Level int

const (
	Debug Level = iota
	Info
	Warn
	Error
)

func (l Level) String() string {
	switch l {
	case Debug:
		return "DEBUG"
	case Info:
		return "INFO"
	case Warn:
		return "WARN"
	default:
		return "ERROR"
	}
}

// Retention settings.
const (
	keepDays    = 7
	maxDirBytes = 50 << 20 // 50MB
)

var (
	mu      sync.Mutex
	min     = Info
	outfile io.WriteCloser
)

// Init opens (or reuses) the daily log file for the given component
// ("meatproxy" or "index-worker"). Log level comes from MEATPROXY_LOG
// (debug|info|warn|error), defaulting to info.
func Init(component string) {
	if v := os.Getenv("MEATPROXY_LOG"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			min = Debug
		case "info":
			min = Info
		case "warn":
			min = Warn
		case "error":
			min = Error
		}
	}
	name := fmt.Sprintf("%s-%s.log", component, time.Now().Format("20060102"))
	f, err := os.OpenFile(filepath.Join(paths.LogsDir(), name), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// Logging is best-effort; fall back to discarding output.
		f = nil
	}
	outfile = f
	Prune()
}

// Close flushes and closes the log file.
func Close() {
	mu.Lock()
	defer mu.Unlock()
	if outfile != nil {
		_ = outfile.Close()
		outfile = nil
	}
}

func Log(level Level, format string, args ...any) {
	if level < min {
		return
	}
	line := fmt.Sprintf("%s %s %s\n",
		time.Now().Format("2006-01-02T15:04:05.000"), level, fmt.Sprintf(format, args...))
	mu.Lock()
	defer mu.Unlock()
	if outfile != nil {
		_, _ = io.WriteString(outfile, line)
	}
}

func Debugf(format string, args ...any) { Log(Debug, format, args...) }
func Infof(format string, args ...any)  { Log(Info, format, args...) }
func Warnf(format string, args ...any)  { Log(Warn, format, args...) }
func Errorf(format string, args ...any) { Log(Error, format, args...) }

// Prune deletes log files older than keepDays, then trims the directory to
// maxDirBytes oldest-first. Best effort: errors are ignored.
func Prune() {
	dir := paths.LogsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	type entry struct {
		path  string
		size  int64
		stamp time.Time
	}
	var entriesSorted []entry
	var total int64
	cutoff := time.Now().AddDate(0, 0, -keepDays)
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := e.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		if info.ModTime().Before(cutoff) {
			_ = os.Remove(path)
			total -= info.Size()
			continue
		}
		entriesSorted = append(entriesSorted, entry{path, info.Size(), info.ModTime()})
	}
	if total <= maxDirBytes {
		return
	}
	sort.Slice(entriesSorted, func(i, j int) bool { return entriesSorted[i].stamp.Before(entriesSorted[j].stamp) })
	for _, en := range entriesSorted {
		if total <= maxDirBytes {
			break
		}
		if err := os.Remove(en.path); err == nil {
			total -= en.size
		}
	}
}
