// internal/service/logs_test.go
package service

import (
	"errors"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/shell"
)

func TestLogService_Read(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "line1\nline2", ExitCode: 0}}
	svc := NewLogService(mock)

	out, err := svc.Read("/tmp/test.log", 20)
	if err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if out == "" {
		t.Error("expected output, got empty string")
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	if mock.calls[0][0] != "tail" || mock.calls[0][1] != "-n" || mock.calls[0][2] != "20" || mock.calls[0][3] != "/tmp/test.log" {
		t.Errorf("unexpected args: %v", mock.calls[0])
	}
}

func TestLogService_Read_Error(t *testing.T) {
	mock := &mockExecutor{err: errors.New("exec failed")}
	svc := NewLogService(mock)

	_, err := svc.Read("/tmp/test.log", 20)
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestLogService_NilExecutorUsesDefault(t *testing.T) {
	// Pass nil executor - should not panic and should use default
	svc := NewLogService(nil)
	if svc.executor == nil {
		t.Error("executor should not be nil after NewLogService with nil")
	}
}
