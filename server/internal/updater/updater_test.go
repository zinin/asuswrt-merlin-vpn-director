package updater

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestIsUpdateInProgress_NoLockFile(t *testing.T) {
	tempDir := t.TempDir()
	s := &Service{lockFile: filepath.Join(tempDir, "lock")}

	if s.IsUpdateInProgress() {
		t.Error("IsUpdateInProgress() = true, want false (no lock file)")
	}
}

func TestIsUpdateInProgress_ValidLock(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")
	s := &Service{lockFile: lockFile}

	// Create lock with current PID (which is alive)
	pid := os.Getpid()
	if err := os.WriteFile(lockFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	if !s.IsUpdateInProgress() {
		t.Error("IsUpdateInProgress() = false, want true (valid lock with alive process)")
	}
}

func TestIsUpdateInProgress_StaleLock_InvalidPID(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")
	s := &Service{lockFile: lockFile}

	// Create lock with invalid PID content
	if err := os.WriteFile(lockFile, []byte("not-a-number"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	if s.IsUpdateInProgress() {
		t.Error("IsUpdateInProgress() = true, want false (invalid PID)")
	}

	// Lock file should be removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Stale lock file was not removed")
	}
}

func TestIsUpdateInProgress_StaleLock_DeadProcess(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")
	s := &Service{lockFile: lockFile}

	// Create lock with a PID that almost certainly doesn't exist
	// Use a very high PID that's unlikely to be in use
	if err := os.WriteFile(lockFile, []byte("999999999"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	if s.IsUpdateInProgress() {
		t.Error("IsUpdateInProgress() = true, want false (dead process)")
	}

	// Lock file should be removed
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("Stale lock file was not removed")
	}
}

func TestCreateLock_Success(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "subdir", "lock")
	s := &Service{lockFile: lockFile}

	if err := s.CreateLock(); err != nil {
		t.Fatalf("CreateLock() error = %v", err)
	}

	// Verify lock file exists with correct PID
	data, err := os.ReadFile(lockFile)
	if err != nil {
		t.Fatalf("Failed to read lock file: %v", err)
	}

	pid, err := strconv.Atoi(string(data))
	if err != nil {
		t.Fatalf("Lock file contains invalid PID: %v", err)
	}

	if pid != os.Getpid() {
		t.Errorf("Lock file PID = %d, want %d", pid, os.Getpid())
	}
}

func TestCreateLock_AlreadyExists(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")
	s := &Service{lockFile: lockFile}

	// Create lock file first
	if err := os.WriteFile(lockFile, []byte("12345"), 0644); err != nil {
		t.Fatalf("Failed to create existing lock file: %v", err)
	}

	// Try to create lock - should fail
	err := s.CreateLock()
	if err == nil {
		t.Error("CreateLock() should fail when lock already exists")
	}

	// Original content should be preserved
	data, _ := os.ReadFile(lockFile)
	if string(data) != "12345" {
		t.Errorf("Original lock content was modified: got %q, want %q", string(data), "12345")
	}
}

func TestCreateLock_AtomicRaceCondition(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")

	// Simulate race: two services try to create lock
	s1 := &Service{lockFile: lockFile}
	s2 := &Service{lockFile: lockFile}

	// First one succeeds
	if err := s1.CreateLock(); err != nil {
		t.Fatalf("First CreateLock() error = %v", err)
	}

	// Second one must fail
	err := s2.CreateLock()
	if err == nil {
		t.Error("Second CreateLock() should fail (race condition protection)")
	}
}

func TestRemoveLock(t *testing.T) {
	tempDir := t.TempDir()
	lockFile := filepath.Join(tempDir, "lock")
	s := &Service{lockFile: lockFile}

	// Create lock file
	if err := os.WriteFile(lockFile, []byte("12345"), 0644); err != nil {
		t.Fatalf("Failed to create lock file: %v", err)
	}

	// Remove it
	s.RemoveLock()

	// Verify it's gone
	if _, err := os.Stat(lockFile); !os.IsNotExist(err) {
		t.Error("RemoveLock() did not remove the lock file")
	}
}

func TestRemoveLock_NoFile(t *testing.T) {
	tempDir := t.TempDir()
	s := &Service{lockFile: filepath.Join(tempDir, "nonexistent")}

	// Should not panic when file doesn't exist
	s.RemoveLock()
}

func TestCleanFiles(t *testing.T) {
	tempDir := t.TempDir()
	s := &Service{updateDir: tempDir}

	// Create files directory with some content
	filesDir := filepath.Join(tempDir, "files")
	if err := os.MkdirAll(filepath.Join(filesDir, "subdir"), 0755); err != nil {
		t.Fatalf("Failed to create files directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "test.txt"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Clean it
	s.CleanFiles()

	// Verify it's gone
	if _, err := os.Stat(filesDir); !os.IsNotExist(err) {
		t.Error("CleanFiles() did not remove the files directory")
	}
}

func TestCleanFiles_NoDirectory(t *testing.T) {
	tempDir := t.TempDir()
	s := &Service{updateDir: filepath.Join(tempDir, "nonexistent")}

	// Should not panic when directory doesn't exist
	s.CleanFiles()
}

func TestGetters_Defaults(t *testing.T) {
	s := &Service{}

	if got := s.getLockFile(); got != LockFile {
		t.Errorf("getLockFile() = %q, want %q", got, LockFile)
	}
	if got := s.getUpdateDir(); got != UpdateDir {
		t.Errorf("getUpdateDir() = %q, want %q", got, UpdateDir)
	}
	if got := s.getFilesDir(); got != FilesDir {
		t.Errorf("getFilesDir() = %q, want %q", got, FilesDir)
	}
	if got := s.getScriptFile(); got != ScriptFile {
		t.Errorf("getScriptFile() = %q, want %q", got, ScriptFile)
	}
}

func TestGetters_Custom(t *testing.T) {
	s := &Service{
		lockFile:   "/custom/lock",
		updateDir:  "/custom/update",
		scriptFile: "/custom/script.sh",
	}

	if got := s.getLockFile(); got != "/custom/lock" {
		t.Errorf("getLockFile() = %q, want %q", got, "/custom/lock")
	}
	if got := s.getUpdateDir(); got != "/custom/update" {
		t.Errorf("getUpdateDir() = %q, want %q", got, "/custom/update")
	}
	if got := s.getFilesDir(); got != "/custom/update/files" {
		t.Errorf("getFilesDir() = %q, want %q", got, "/custom/update/files")
	}
	if got := s.getScriptFile(); got != "/custom/script.sh" {
		t.Errorf("getScriptFile() = %q, want %q", got, "/custom/script.sh")
	}
}
