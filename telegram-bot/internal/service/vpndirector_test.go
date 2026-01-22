// internal/service/vpndirector_test.go
package service

import (
	"errors"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
)

type mockExecutor struct {
	result *shell.Result
	err    error
	calls  [][]string
}

func (m *mockExecutor) Exec(name string, args ...string) (*shell.Result, error) {
	m.calls = append(m.calls, append([]string{name}, args...))
	return m.result, m.err
}

func TestVPNDirectorService_Status(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "running", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	output, err := svc.Status()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if output != "running" {
		t.Errorf("expected 'running', got %q", output)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0][0] != "/opt/vpn-director/vpn-director.sh" {
		t.Errorf("wrong script path: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_StatusError(t *testing.T) {
	mock := &mockExecutor{err: errors.New("exec failed")}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	_, err := svc.Status()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestVPNDirectorService_RestartXray(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "ok", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.RestartXray()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	// Should call: vpn-director.sh restart xray
	if mock.calls[0][1] != "restart" || mock.calls[0][2] != "xray" {
		t.Errorf("wrong args: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_Apply(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "applied", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Apply()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0][1] != "apply" {
		t.Errorf("wrong args: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_ApplyNonZeroExit(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "failed to apply", ExitCode: 1}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Apply()
	if err == nil {
		t.Error("expected error for non-zero exit, got nil")
	}
}

func TestVPNDirectorService_Restart(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "restarted", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Restart()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0][1] != "restart" {
		t.Errorf("wrong args: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_Stop(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "stopped", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Stop()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0][1] != "stop" {
		t.Errorf("wrong args: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_NilExecutorUsesDefault(t *testing.T) {
	// Pass nil executor - should not panic and should use default
	svc := NewVPNDirectorService("/opt/vpn-director", nil)
	if svc.executor == nil {
		t.Error("executor should not be nil after NewVPNDirectorService with nil")
	}
}
