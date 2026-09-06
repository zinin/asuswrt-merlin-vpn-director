package shell

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestExecContext_EchoHello(t *testing.T) {
	result, err := ExecContext(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	expected := "hello\n"
	if result.Output != expected {
		t.Errorf("expected output %q, got %q", expected, result.Output)
	}
}

func TestExecContext_NonZeroExitCode(t *testing.T) {
	result, err := ExecContext(context.Background(), "sh", "-c", "exit 42")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", result.ExitCode)
	}
}

func TestExecContext_CommandWithOutput(t *testing.T) {
	result, err := ExecContext(context.Background(), "sh", "-c", "echo first; echo second")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "first") || !strings.Contains(result.Output, "second") {
		t.Errorf("expected output to contain 'first' and 'second', got %q", result.Output)
	}
}

func TestExecContext_StderrCaptured(t *testing.T) {
	result, err := ExecContext(context.Background(), "sh", "-c", "echo error >&2")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "error") {
		t.Errorf("expected stderr to be captured, got %q", result.Output)
	}
}

func TestExecContext_CommandNotFound(t *testing.T) {
	_, err := ExecContext(context.Background(), "/nonexistent/command/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for non-existent command")
	}
}

func TestExecContext_ExitCodeWithOutput(t *testing.T) {
	result, err := ExecContext(context.Background(), "sh", "-c", "echo output; exit 5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ExitCode != 5 {
		t.Errorf("expected exit code 5, got %d", result.ExitCode)
	}

	if !strings.Contains(result.Output, "output") {
		t.Errorf("expected output to contain 'output', got %q", result.Output)
	}
}

func TestExecContext_TimeoutTerminatesProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// exec replaces the shell with sleep, so SIGTERM reaches sleep itself and
	// the output pipe closes as soon as it dies.
	_, err := ExecContext(ctx, "sh", "-c", "exec sleep 5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "command timed out after ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "command timed out after ")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ExecContext took %s; the process was not terminated on timeout", elapsed)
	}
}

// A command that exits by itself while something it started keeps the output
// pipe open ends in exec.ErrWaitDelay. The command succeeded, so ExecContext
// must not turn that into an error: the Web UI would report "apply failed"
// for an apply that finished.
func TestExecContext_ChildHoldingThePipeIsNotAFailure(t *testing.T) {
	old := waitDelay
	waitDelay = 100 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })

	// The shell exits at once; the background sleep inherits its stdout.
	result, err := ExecContext(context.Background(), "sh", "-c", "sleep 1 & echo done")
	if err != nil {
		t.Fatalf("ExecContext() error = %v, want nil", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", result.ExitCode)
	}
	if !strings.Contains(result.Output, "done") {
		t.Errorf("Output = %q, want what the command printed", result.Output)
	}
}

func TestExecContext_TimeoutKillsChildThatIgnoresTerm(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// The shell ignores SIGTERM and its child inherits that; only the
	// WaitDelay kill and pipe close can end the call.
	_, err := ExecContext(ctx, "sh", "-c", "trap '' TERM; sleep 5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ExecContext took %s; WaitDelay did not kill the process", elapsed)
	}
}

// TestExecContext_TimeoutErrorUnwrapsToDeadlineExceeded: spec 5.2's wording is
// pinned as the error's prefix, which is all this test asserts about the text,
// but a caller that asks errors.Is(err, context.DeadlineExceeded) has to get
// true - otherwise every future caller has to match on the string.
func TestExecContext_TimeoutErrorUnwrapsToDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := ExecContext(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.HasPrefix(err.Error(), "command timed out after ") {
		t.Errorf("error = %q, want it to start with %q", err, "command timed out after ")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false for %v", err)
	}
}
