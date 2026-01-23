package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogger_New(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, err := New(logPath)
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer logger.Close()

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

func TestLogger_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	logger, _ := New(logPath)
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestLogger_New_InvalidPath(t *testing.T) {
	_, err := New("/nonexistent/dir/test.log")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}
