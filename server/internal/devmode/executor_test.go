// internal/devmode/executor_test.go
package devmode

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
)

// mockExecutor records calls and returns configured results
type mockExecutor struct {
	result *shell.Result
	err    error
	calls  [][]string
	ctxs   []context.Context
}

func (m *mockExecutor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	m.calls = append(m.calls, append([]string{name}, args...))
	m.ctxs = append(m.ctxs, ctx)
	return m.result, m.err
}

// Compile-time interface check
var _ service.ShellExecutor = (*Executor)(nil)

func TestExecutor_SafeCommand_Curl(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "curl output", ExitCode: 0}}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "curl", "-s", "https://example.com")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Output != "curl output" {
		t.Errorf("expected 'curl output', got %q", result.Output)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call to real executor, got %d", len(mock.calls))
	}
	if mock.calls[0][0] != "curl" {
		t.Errorf("expected curl, got %q", mock.calls[0][0])
	}
}

func TestExecutor_SafeCommand_Tail(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "log lines", ExitCode: 0}}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "tail", "-n", "20", "/var/log/syslog")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.Output != "log lines" {
		t.Errorf("expected 'log lines', got %q", result.Output)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call to real executor, got %d", len(mock.calls))
	}
}

func TestExecutor_SafeCommand_FullPath(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "output", ExitCode: 0}}
	exec := NewExecutorWithReal(mock)

	// curl via full path should also work
	result, err := exec.Exec(context.Background(), "/usr/bin/curl", "-s", "https://example.com")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call to real executor, got %d", len(mock.calls))
	}
	if result.Output != "output" {
		t.Errorf("expected 'output', got %q", result.Output)
	}
}

func TestExecutor_MockCommand_VPNDirectorStatus(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "/opt/vpn-director/vpn-director.sh", "status")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "DEV MODE") {
		t.Errorf("expected mock output with 'DEV MODE', got %q", result.Output)
	}
	// Real executor should NOT be called
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 calls to real executor for mock command, got %d", len(mock.calls))
	}
}

func TestExecutor_MockCommand_VPNDirectorRestart(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "vpn-director.sh", "restart")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "DEV MODE") {
		t.Errorf("expected mock output with 'DEV MODE', got %q", result.Output)
	}
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 calls to real executor, got %d", len(mock.calls))
	}
}

func TestExecutor_MockCommand_VPNDirectorStop(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "vpn-director.sh", "stop")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "DEV MODE") {
		t.Errorf("expected mock output with 'DEV MODE', got %q", result.Output)
	}
}

func TestExecutor_MockCommand_VPNDirectorApply(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "vpn-director.sh", "apply")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "DEV MODE") {
		t.Errorf("expected mock output with 'DEV MODE', got %q", result.Output)
	}
}

func TestExecutor_MockCommand_VPNDirectorUpdate(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "vpn-director.sh", "update")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", result.ExitCode)
	}
	if !strings.Contains(result.Output, "DEV MODE") {
		t.Errorf("expected mock output with 'DEV MODE', got %q", result.Output)
	}
}

func TestExecutor_MockCommand_VPNDirectorUnknownCommand(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "vpn-director.sh", "unknown-command")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Unknown vpn-director.sh command should return exit code 1
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", result.ExitCode)
	}
}

func TestExecutor_UnknownCommand_Fails(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	result, err := exec.Exec(context.Background(), "rm", "-rf", "/")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	// Unknown command should fail with exit code 1
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 for unknown command, got %d", result.ExitCode)
	}
	// Real executor should NOT be called for unsafe commands
	if len(mock.calls) != 0 {
		t.Errorf("expected 0 calls to real executor for unknown command, got %d", len(mock.calls))
	}
}

func TestExecutor_UnknownCommand_NotSafe(t *testing.T) {
	mock := &mockExecutor{}
	exec := NewExecutorWithReal(mock)

	// wget is not in safe list
	result, err := exec.Exec(context.Background(), "wget", "https://example.com")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result.ExitCode != 1 {
		t.Errorf("expected exit code 1 for non-safe command, got %d", result.ExitCode)
	}
}

func TestNewExecutor_UsesDefaultExecutor(t *testing.T) {
	exec := NewExecutor()
	if exec.real == nil {
		t.Error("real executor should not be nil")
	}
}

func TestExecutor_MockCommand_SkipsLeadingOptions(t *testing.T) {
	exec := NewExecutorWithReal(&mockExecutor{})

	// VPNDirectorService passes --wait before the command; the mock must not
	// take the option for the command name.
	result, err := exec.Exec(context.Background(), "vpn-director.sh", "--wait", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Output, "applied") {
		t.Errorf("expected the apply mock response, got exit %d: %q", result.ExitCode, result.Output)
	}
}

func TestExecutor_SafeCommand_PassesContext(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "ok"}}
	exec := NewExecutorWithReal(mock)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := exec.Exec(ctx, "curl", "-s", "https://example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.ctxs) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.ctxs))
	}
	if _, ok := mock.ctxs[0].Deadline(); !ok {
		t.Error("the caller's context deadline did not reach the real executor")
	}
}
