package logging

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTruncateIfNeeded(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create file with 100 bytes
	data := make([]byte, 100)
	if err := os.WriteFile(logPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Should not truncate (100 < 200)
	TruncateIfNeeded(logPath, 200)
	info, _ := os.Stat(logPath)
	if info.Size() != 100 {
		t.Errorf("should not truncate, size = %d", info.Size())
	}

	// Should truncate (100 > 50)
	TruncateIfNeeded(logPath, 50)
	info, _ = os.Stat(logPath)
	if info.Size() != 0 {
		t.Errorf("should truncate to 0, size = %d", info.Size())
	}
}

func TestTruncateIfNeeded_FileNotExist(t *testing.T) {
	// Should not panic on non-existent file
	TruncateIfNeeded("/nonexistent/path/file.log", 100)
}

func TestTruncateIfNeeded_ExactSize(t *testing.T) {
	tmpDir := t.TempDir()
	logPath := filepath.Join(tmpDir, "test.log")

	// Create file with exactly maxSize bytes
	data := make([]byte, 100)
	if err := os.WriteFile(logPath, data, 0644); err != nil {
		t.Fatal(err)
	}

	// Should not truncate (100 == 100, not greater)
	TruncateIfNeeded(logPath, 100)
	info, _ := os.Stat(logPath)
	if info.Size() != 100 {
		t.Errorf("should not truncate at exact size, size = %d", info.Size())
	}
}
