// Package oc wraps the OpenCode CLI: listing sessions, exporting transcript
// text, and launching/resuming the TUI.
package oc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"meatproxy/internal/execx"
)

const (
	listTimeout   = 15 * time.Second
	exportTimeout = 60 * time.Second
)

// Session is the subset of `opencode session list --format json` meatproxy uses.
type Session struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Created   int64  `json:"created"` // unix millis
	Updated   int64  `json:"updated"` // unix millis
	Directory string `json:"directory"`
}

// ListSessions returns up to max recent sessions for the project at dir.
func ListSessions(ctx context.Context, dir string, max int) ([]Session, error) {
	out, err := execx.RunOK(ctx, dir, listTimeout, "opencode", "session", "list", "--format", "json", "-n", fmt.Sprint(max))
	if err != nil {
		return nil, fmt.Errorf("opencode session list: %w", err)
	}
	if len(out) == 0 {
		return nil, nil
	}
	var sessions []Session
	if err := json.Unmarshal(out, &sessions); err != nil {
		return nil, fmt.Errorf("parse opencode session list: %w", err)
	}
	return sessions, nil
}

// exportShape mirrors `opencode session export` JSON.
type exportShape struct {
	Info struct {
		ID    string `json:"id"`
		Title string `json:"title"`
	} `json:"info"`
	Messages []struct {
		Type string `json:"type"`
		Text string `json:"text"`
		Body []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"messages"`
}

// ExportText returns the searchable text of a session: every user prompt and
// every non-reasoning assistant text part, joined by newlines.
func ExportText(ctx context.Context, dir, sessionID string) (string, error) {
	out, err := execx.RunOK(ctx, dir, exportTimeout, "opencode", "session", "export", sessionID)
	if err != nil {
		return "", fmt.Errorf("opencode session export %s: %w", sessionID, err)
	}
	var ex exportShape
	if err := json.Unmarshal(out, &ex); err != nil {
		return "", fmt.Errorf("parse opencode session export: %w", err)
	}
	var b []byte
	for _, m := range ex.Messages {
		switch m.Type {
		case "user":
			if m.Text != "" {
				b = append(b, m.Text...)
				b = append(b, '\n')
			}
		case "assistant":
			for _, part := range m.Body {
				if part.Type == "text" && part.Text != "" {
					b = append(b, part.Text...)
					b = append(b, '\n')
				}
			}
		}
	}
	return string(b), nil
}

// Launch starts a fresh interactive opencode TUI in dir (foreground).
func Launch(dir string) error {
	return runInteractive(dir, []string{"opencode", dir})
}

// Resume continues an existing session in dir (foreground).
func Resume(dir, sessionID string) error {
	return runInteractive(dir, []string{"opencode", dir, "--session", sessionID})
}

// runInteractive hands the terminal to opencode: inherited stdio, no timeout,
// no output capture — the TUI owns the screen until the user exits.
// (Deliberately outside execx: the hardened runner is for captured runs.)
func runInteractive(dir string, argv []string) error {
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
