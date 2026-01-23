// internal/service/network_test.go
package service

import (
	"errors"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
)

func TestNetworkService_GetExternalIP(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "1.2.3.4", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	ip, err := svc.GetExternalIP()
	if err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if ip != "1.2.3.4" {
		t.Errorf("expected 1.2.3.4, got %s", ip)
	}
}

func TestNetworkService_GetExternalIP_Error(t *testing.T) {
	mock := &mockExecutor{
		err: errors.New("network error"),
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestNetworkService_GetExternalIP_TrimSpace(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "  5.6.7.8\n", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	ip, err := svc.GetExternalIP()
	if err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if ip != "5.6.7.8" {
		t.Errorf("expected trimmed IP '5.6.7.8', got %q", ip)
	}
}

func TestNetworkService_GetExternalIP_NonZeroExitCode(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "", ExitCode: 1},
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error for non-zero exit code, got nil")
	}
}

func TestNetworkService_GetExternalIP_InvalidIPFormat(t *testing.T) {
	mock := &mockExecutor{
		result: &shell.Result{Output: "<html>error page</html>", ExitCode: 0},
	}
	svc := NewNetworkService(mock)

	_, err := svc.GetExternalIP()
	if err == nil {
		t.Error("expected error for invalid IP format, got nil")
	}
}
