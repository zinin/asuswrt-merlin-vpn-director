// internal/service/vpndirector_test.go
package service

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
)

type mockExecutor struct {
	result   *shell.Result
	err      error
	calls    [][]string
	timeouts []time.Duration // time left to each call's context deadline
}

func (m *mockExecutor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	m.calls = append(m.calls, append([]string{name}, args...))
	if deadline, ok := ctx.Deadline(); ok {
		m.timeouts = append(m.timeouts, time.Until(deadline))
	}
	return m.result, m.err
}

// assertCall checks the recorded call's arguments after the script path.
func assertCall(t *testing.T, call []string, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(call[1:], want) {
		t.Errorf("args = %v, want %v", call[1:], want)
	}
}

// assertTimeout checks that the executor saw a context deadline about want away.
func assertTimeout(t *testing.T, got, want time.Duration) {
	t.Helper()
	if got > want || got < want-time.Second {
		t.Errorf("context timeout = %s, want about %s", got, want)
	}
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
	assertCall(t, mock.calls[0], "status")
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
	// Should call: vpn-director.sh --wait restart xray
	assertCall(t, mock.calls[0], "--wait", "restart", "xray")
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
	assertCall(t, mock.calls[0], "--wait", "apply")
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
	assertCall(t, mock.calls[0], "--wait", "restart")
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
	assertCall(t, mock.calls[0], "--wait", "stop")
}

func TestVPNDirectorService_Update(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "updated", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Update()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	// Should call: vpn-director.sh --wait update (force-refreshes ipsets, unlike apply)
	assertCall(t, mock.calls[0], "--wait", "update")
}

func TestVPNDirectorService_UpdateNonZeroExit(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "download failed", ExitCode: 1}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Update()
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("error should carry script output, got %q", err.Error())
	}
}

func TestVPNDirectorService_Timeouts(t *testing.T) {
	tests := []struct {
		name string
		call func(*VPNDirectorService) error
		want time.Duration
	}{
		{"status", func(s *VPNDirectorService) error { _, err := s.Status(); return err }, StatusTimeout},
		{"apply", (*VPNDirectorService).Apply, ApplyTimeout},
		{"restart", (*VPNDirectorService).Restart, ApplyTimeout},
		{"restart xray", (*VPNDirectorService).RestartXray, ApplyTimeout},
		{"stop", (*VPNDirectorService).Stop, ApplyTimeout},
		{"update", (*VPNDirectorService).Update, UpdateTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExecutor{result: &shell.Result{Output: "ok"}}
			svc := NewVPNDirectorService("/opt/vpn-director", mock)
			if err := tt.call(svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(mock.timeouts) != 1 {
				t.Fatalf("expected the executor to see one context deadline, got %d", len(mock.timeouts))
			}
			assertTimeout(t, mock.timeouts[0], tt.want)
		})
	}
}

func TestVPNDirectorService_NilExecutorUsesDefault(t *testing.T) {
	// Pass nil executor - should not panic and should use default
	svc := NewVPNDirectorService("/opt/vpn-director", nil)
	if svc.executor == nil {
		t.Error("executor should not be nil after NewVPNDirectorService with nil")
	}
}
