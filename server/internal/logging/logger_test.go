package logging

import (
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewSlogLogger_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	_, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}
	defer logger.Close()

	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Error("log file was not created")
	}
}

func TestNewSlogLogger_InvalidPath(t *testing.T) {
	_, _, err := NewSlogLogger("/nonexistent/dir/test.log")
	if err == nil {
		t.Error("expected error for invalid path, got nil")
	}
}

func TestNewSlogLogger_DefaultLevelInfo(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}
	defer logger.Close()

	// Default level is INFO
	if !slogger.Enabled(nil, slog.LevelInfo) {
		t.Error("INFO should be enabled by default")
	}
	if slogger.Enabled(nil, slog.LevelDebug) {
		t.Error("DEBUG should be filtered by default")
	}
}

func TestLogger_SetLevel(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}
	defer logger.Close()

	// Initially INFO - DEBUG filtered
	if slogger.Enabled(nil, slog.LevelDebug) {
		t.Error("DEBUG should be filtered initially")
	}

	// Change to DEBUG
	logger.SetLevel("debug")
	if !slogger.Enabled(nil, slog.LevelDebug) {
		t.Error("DEBUG should be enabled after SetLevel")
	}

	// Change to WARN
	logger.SetLevel("warn")
	if slogger.Enabled(nil, slog.LevelInfo) {
		t.Error("INFO should be filtered at WARN level")
	}
	if !slogger.Enabled(nil, slog.LevelWarn) {
		t.Error("WARN should be enabled at WARN level")
	}
}

func TestLogger_SetLevel_CaseInsensitive(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}
	defer logger.Close()

	logger.SetLevel("DEBUG")
	if !slogger.Enabled(nil, slog.LevelDebug) {
		t.Error("DEBUG (uppercase) should work")
	}

	logger.SetLevel("WaRn")
	if !slogger.Enabled(nil, slog.LevelWarn) {
		t.Error("WaRn (mixed case) should work")
	}
}

func TestLogger_SetLevel_InvalidDefaultsToInfo(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}
	defer logger.Close()

	// Set to debug first
	logger.SetLevel("debug")
	if !slogger.Enabled(nil, slog.LevelDebug) {
		t.Fatal("DEBUG should be enabled")
	}

	// Invalid level defaults to INFO (and logs warning)
	logger.SetLevel("invalid")
	if slogger.Enabled(nil, slog.LevelDebug) {
		t.Error("Invalid level should default to INFO, filtering DEBUG")
	}
	if !slogger.Enabled(nil, slog.LevelInfo) {
		t.Error("Invalid level should default to INFO")
	}

	// Empty string defaults to INFO (no warning)
	logger.SetLevel("")
	if !slogger.Enabled(nil, slog.LevelInfo) {
		t.Error("Empty string should default to INFO")
	}
}

func TestNewSlogLogger_RedirectsStandardLog(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	_, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}

	// Standard log should write to the same file
	log.Println("standard log message")
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "standard log message") {
		t.Errorf("log file should contain standard log output, got: %s", content)
	}
}

func TestLogger_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	_, logger, _ := NewSlogLogger(logPath)
	err := logger.Close()
	if err != nil {
		t.Errorf("Close() error: %v", err)
	}
}

func TestNewSlogLogger_WritesToFile(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}

	slogger.Info("test message", "key", "value")
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !strings.Contains(string(content), "test message") {
		t.Errorf("log file should contain 'test message', got: %s", content)
	}
	if !strings.Contains(string(content), "key=value") {
		t.Errorf("log file should contain 'key=value', got: %s", content)
	}
}

func TestNewSlogLogger_IncludesSource(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	slogger, logger, err := NewSlogLogger(logPath)
	if err != nil {
		t.Fatalf("NewSlogLogger() error: %v", err)
	}

	slogger.Info("test with source")
	logger.Close()

	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// Should contain source file info
	if !strings.Contains(string(content), "source=") {
		t.Errorf("log should contain source info, got: %s", content)
	}
}
