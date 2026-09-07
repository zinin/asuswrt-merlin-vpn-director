package shell

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// ExecContext runs the command and captures its combined stdout and stderr.
// A non-zero exit is reported through Result.ExitCode, not as an error.
// When ctx expires the process receives SIGTERM, so vpn-director.sh's EXIT
// trap can remove its temp files, and is killed after waitDelay if it has
// not exited; the returned error then starts with "command timed out after
// <d>", where d is the context's timeout, and unwraps to
// context.DeadlineExceeded.
func ExecContext(ctx context.Context, command string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, command, args...)
	// Own process group. vpn-director.sh spawns wget and sleep, and they
	// inherit FD 200 with /var/lock/vpn-director.lock: signalling the shell
	// alone leaves a child holding that lock until it ends on its own, and
	// every later run then skips silently or waits out its --wait.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM); err != nil {
			if errors.Is(err, syscall.ESRCH) {
				return os.ErrProcessDone
			}
			return err
		}
		return nil
	}
	cmd.WaitDelay = waitDelay
	output, err := cmd.CombinedOutput()
	// WaitDelay ends the shell only, so a child that ignored SIGTERM is still
	// running. Kill the group: its id stays reserved while the group has
	// members, which keeps this signal off an unrelated process.
	if ctx.Err() != nil && cmd.Process != nil {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	result := &Result{Output: string(output)}

	if err != nil && ctx.Err() != nil {
		result.ExitCode = -1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			d := time.Since(start)
			if deadline, ok := ctx.Deadline(); ok {
				d = deadline.Sub(start)
			}
			// The message is spec 5.2's wording; wrapping ctx.Err() keeps
			// errors.Is usable without anyone matching on the string.
			return result, fmt.Errorf("command timed out after %s: %w", d.Round(time.Millisecond), ctx.Err())
		}
		return result, fmt.Errorf("command cancelled: %w", ctx.Err())
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		err = nil // non-zero exit is not an error
	}

	// The command exited by itself, but something it left behind still held
	// the output pipe when waitDelay ran out. exec reports ErrWaitDelay for
	// that even though the exit status was fine, and Output carries whatever
	// was read up to then. An apply that completed must not reach the user as
	// "apply failed".
	if errors.Is(err, exec.ErrWaitDelay) {
		err = nil
	}
	return result, err
}
