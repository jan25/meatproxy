// Package execx runs every external subprocess meatproxy spawns. It is the
// single place where timeouts, process-group kills, output caps, and logging
// are enforced, so no harness call can go rogue or hang a command.
package execx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"

	"github.com/jan25/meatproxy/internal/logx"
)

const outputCap = 1 << 20 // keep the last 1MB of any stream

// Result is the outcome of one subprocess run.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	ExitCode int
}

// Run executes argv[0] with args in dir, under the given timeout. On timeout
// the child's whole process group is killed. Output is capped and errors are
// logged, never dumped to the user's terminal.
func Run(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} // own process group
	cmd.Stdout = &cappedWriter{cap: outputCap}
	cmd.Stderr = &cappedWriter{cap: outputCap}
	cmd.Env = os.Environ()

	start := time.Now()
	err := cmd.Run()
	res := Result{
		Stdout:   cmd.Stdout.(*cappedWriter).Bytes(),
		Stderr:   cmd.Stderr.(*cappedWriter).Bytes(),
		ExitCode: cmd.ProcessState.ExitCode(),
	}
	dur := time.Since(start).Round(time.Millisecond)

	if err == nil {
		logx.Debugf("exec ok: %s (%s)", describe(name, args), dur)
		return res, nil
	}

	reason := err.Error()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if ctx.Err() == context.DeadlineExceeded {
			reason = fmt.Sprintf("timed out after %s (killed process group %d)", timeout, cmd.Process.Pid)
			err = fmt.Errorf("%s: timed out after %s", describe(name, args), timeout)
		} else {
			reason = fmt.Sprintf("exit %d", exitErr.ExitCode())
			err = fmt.Errorf("%s: exit %d: %s", describe(name, args), exitErr.ExitCode(), tail(res.Stderr, 500))
		}
	}
	logx.Errorf("exec failed: %s (%s): %s", describe(name, args), dur, reason)
	return res, err
}

// RunOK is Run with a friendlier signature for calls whose output is always
// needed and whose failure is fatal to the caller.
func RunOK(ctx context.Context, dir string, timeout time.Duration, name string, args ...string) ([]byte, error) {
	res, err := Run(ctx, dir, timeout, name, args...)
	if err != nil {
		return nil, err
	}
	return res.Stdout, nil
}

func describe(name string, args []string) string {
	// Never log user prompt content at info level; debug already has it via callers.
	return strings.TrimSpace(name + " " + strings.Join(args, " "))
}

func tail(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return "…" + s[len(s)-n:]
}

// cappedWriter keeps only the last cap bytes written to it.
type cappedWriter struct {
	buf bytes.Buffer
	cap int
}

func (w *cappedWriter) Write(p []byte) (int, error) {
	w.buf.Write(p)
	if w.buf.Len() > w.cap {
		kept := w.buf.Bytes()[w.buf.Len()-w.cap:]
		w.buf.Reset()
		w.buf.Write(kept)
	}
	return len(p), nil
}

func (w *cappedWriter) Bytes() []byte {
	b := w.buf.Bytes()
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
