package chatstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestStore_RecordInteraction_NewUser(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)

	err := store.RecordInteraction("john_doe", 123456789)
	if err != nil {
		t.Fatalf("RecordInteraction failed: %v", err)
	}

	users, err := store.GetActiveUsers()
	if err != nil {
		t.Fatalf("GetActiveUsers failed: %v", err)
	}

	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}

	if users[0].Username != "john_doe" {
		t.Errorf("expected username john_doe, got %s", users[0].Username)
	}
	if users[0].ChatID != 123456789 {
		t.Errorf("expected chatID 123456789, got %d", users[0].ChatID)
	}
}

func TestStore_RecordInteraction_UpdatesLastSeen(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)

	_ = store.RecordInteraction("john_doe", 123)
	time.Sleep(10 * time.Millisecond)
	_ = store.RecordInteraction("john_doe", 123)

	users, _ := store.GetActiveUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
}

func TestStore_SetInactive(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)
	_ = store.RecordInteraction("john_doe", 123)

	err := store.SetInactive("john_doe")
	if err != nil {
		t.Fatalf("SetInactive failed: %v", err)
	}

	users, _ := store.GetActiveUsers()
	if len(users) != 0 {
		t.Errorf("expected 0 active users after SetInactive, got %d", len(users))
	}
}

func TestStore_RecordInteraction_ReactivatesUser(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)
	_ = store.RecordInteraction("john_doe", 123)
	_ = store.SetInactive("john_doe")
	_ = store.RecordInteraction("john_doe", 123)

	users, _ := store.GetActiveUsers()
	if len(users) != 1 {
		t.Errorf("expected 1 active user after reactivation, got %d", len(users))
	}
}

func TestStore_MarkNotified(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)
	_ = store.RecordInteraction("john_doe", 123)

	if store.IsNotified("john_doe", "v1.0.0") {
		t.Error("expected IsNotified to return false before marking")
	}

	err := store.MarkNotified("john_doe", "v1.0.0")
	if err != nil {
		t.Fatalf("MarkNotified failed: %v", err)
	}

	if !store.IsNotified("john_doe", "v1.0.0") {
		t.Error("expected IsNotified to return true after marking")
	}

	if store.IsNotified("john_doe", "v1.1.0") {
		t.Error("expected IsNotified to return false for different version")
	}
}

func TestStore_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store1 := New(path)
	_ = store1.RecordInteraction("john_doe", 123)
	_ = store1.MarkNotified("john_doe", "v1.0.0")

	// Create new store instance to test persistence
	store2 := New(path)
	users, _ := store2.GetActiveUsers()

	if len(users) != 1 {
		t.Fatalf("expected 1 user after reload, got %d", len(users))
	}

	if !store2.IsNotified("john_doe", "v1.0.0") {
		t.Error("expected notified version to persist")
	}
}

func TestStore_NonExistentFile(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "nonexistent", "chats.json")

	store := New(path)

	// Should not fail, just return empty
	users, err := store.GetActiveUsers()
	if err != nil {
		t.Fatalf("GetActiveUsers on non-existent file failed: %v", err)
	}
	if len(users) != 0 {
		t.Errorf("expected 0 users, got %d", len(users))
	}
}

func TestStore_ConcurrentAccess(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)

	done := make(chan bool)
	for i := 0; i < 10; i++ {
		go func(n int) {
			_ = store.RecordInteraction("user", int64(n))
			_, _ = store.GetActiveUsers()
			done <- true
		}(i)
	}

	for i := 0; i < 10; i++ {
		<-done
	}
}

func TestStore_UsernameNormalization(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "chats.json")

	store := New(path)
	_ = store.RecordInteraction("John_Doe", 123)

	// Should find with lowercase
	users, _ := store.GetActiveUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 user, got %d", len(users))
	}
	if users[0].Username != "john_doe" {
		t.Errorf("expected normalized username john_doe, got %s", users[0].Username)
	}

	// Should work with different case
	if !store.IsNotified("JOHN_DOE", "v1.0.0") == false {
		// Just checking it doesn't panic
	}
	_ = store.MarkNotified("JOHN_DOE", "v1.0.0")
	if !store.IsNotified("john_doe", "v1.0.0") {
		t.Error("expected case-insensitive notified check")
	}
}
