package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds how long ExecContext waits, after cancelling a command,
// for its stdout/stderr pipes to close. vpn-director.sh spawns wget and
// sleep; a SIGTERM to the script can leave a child holding the pipe open, and
// without this bound CombinedOutput would block until that child exits. A
// variable rather than a constant so a test can shorten it.
var waitDelay = 10 * time.Second

type Result struct {
	Output   string
	ExitCode int
}

// Exec runs the command with no deadline. Kept for the callers that still
// take no context; they move to ExecContext in the next task.
func Exec(command string, args ...string) (*Result, error) {
	return ExecContext(context.Background(), command, args...)
}

// ExecContext runs the command and captures its combined stdout and stderr.
// A non-zero exit is reported through Result.ExitCode, not as an error.
// When ctx expires the process receives SIGTERM, so vpn-director.sh's EXIT
// trap can remove its temp files, and is killed after waitDelay if it has
// not exited; the returned error then reads "command timed out after <d>",
// where d is the context's timeout.
func ExecContext(ctx context.Context, command string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = waitDelay
	output, err := cmd.CombinedOutput()
	result := &Result{Output: string(output)}

	if err != nil && ctx.Err() != nil {
		result.ExitCode = -1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			d := time.Since(start)
			if deadline, ok := ctx.Deadline(); ok {
				d = deadline.Sub(start)
			}
			return result, fmt.Errorf("command timed out after %s", d.Round(time.Millisecond))
		}
		return result, fmt.Errorf("command cancelled: %w", ctx.Err())
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		err = nil // non-zero exit is not an error
	}
	return result, err
}
