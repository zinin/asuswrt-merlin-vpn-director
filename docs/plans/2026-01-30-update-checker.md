# Update Checker Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Автоматическая проверка обновлений с уведомлением пользователей через Telegram.

**Architecture:** Фоновая горутина проверяет GitHub API с настраиваемым интервалом. При обнаружении новой версии отправляет уведомление всем активным пользователям. Chat ID хранятся в отдельном файле `chats.json`.

**Tech Stack:** Go 1.21+, telegram-bot-api/v5, стандартная библиотека (sync, time, encoding/json)

**Design doc:** `docs/plans/2026-01-30-update-checker-design.md`

---

## Task 1: Добавить UpdateCheckInterval в config

**Files:**
- Modify: `telegram-bot/internal/config/config.go`
- Modify: `telegram-bot/internal/config/config_test.go`

**Step 1: Write the failing test**

В `telegram-bot/internal/config/config_test.go` добавить:

```go
func TestLoad_WithUpdateCheckInterval(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": ["user1"],
		"update_check_interval": "1h"
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.UpdateCheckInterval != time.Hour {
		t.Errorf("expected update_check_interval 1h, got %v", cfg.UpdateCheckInterval)
	}
}

func TestLoad_UpdateCheckIntervalZero(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": []
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.UpdateCheckInterval != 0 {
		t.Errorf("expected zero update_check_interval for missing field, got %v", cfg.UpdateCheckInterval)
	}
}

func TestLoad_UpdateCheckIntervalExplicitZero(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	jsonContent := `{
		"bot_token": "test-token",
		"allowed_users": [],
		"update_check_interval": "0"
	}`

	err := os.WriteFile(configPath, []byte(jsonContent), 0644)
	if err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.UpdateCheckInterval != 0 {
		t.Errorf("expected zero update_check_interval for '0', got %v", cfg.UpdateCheckInterval)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/config/... -run TestLoad_WithUpdateCheckInterval`
Expected: FAIL — `cfg.UpdateCheckInterval undefined`

**Step 3: Write minimal implementation**

В `telegram-bot/internal/config/config.go` заменить всё содержимое:

```go
package config

import (
	"encoding/json"
	"os"
	"time"
)

type Config struct {
	BotToken            string        `json:"bot_token"`
	AllowedUsers        []string      `json:"allowed_users"`
	LogLevel            string        `json:"log_level"`
	UpdateCheckInterval time.Duration `json:"-"` // Parsed from string
}

// rawConfig is used for JSON unmarshaling with string duration
type rawConfig struct {
	BotToken            string   `json:"bot_token"`
	AllowedUsers        []string `json:"allowed_users"`
	LogLevel            string   `json:"log_level"`
	UpdateCheckInterval string   `json:"update_check_interval"`
}

func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var raw rawConfig
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, err
	}

	cfg := &Config{
		BotToken:     raw.BotToken,
		AllowedUsers: raw.AllowedUsers,
		LogLevel:     raw.LogLevel,
	}

	// Parse duration if provided and not "0"
	if raw.UpdateCheckInterval != "" && raw.UpdateCheckInterval != "0" {
		d, err := time.ParseDuration(raw.UpdateCheckInterval)
		if err != nil {
			return nil, err
		}
		cfg.UpdateCheckInterval = d
	}

	return cfg, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -v ./internal/config/...`
Expected: All PASS

**Step 5: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/config/config.go telegram-bot/internal/config/config_test.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add update_check_interval config field

Adds optional field to telegram-bot.json for configuring automatic
update check interval. If omitted or set to "0", checking is disabled.
EOF
)"
```

---

## Task 2: Создать ChatStore для хранения chat_id

**Files:**
- Create: `telegram-bot/internal/chatstore/store.go`
- Create: `telegram-bot/internal/chatstore/store_test.go`

**Step 1: Write the failing test**

Создать `telegram-bot/internal/chatstore/store_test.go`:

```go
package chatstore

import (
	"os"
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
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/chatstore/...`
Expected: FAIL — package not found

**Step 3: Write minimal implementation**

Создать `telegram-bot/internal/chatstore/store.go`:

```go
package chatstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// UserChat represents a user with their chat ID.
type UserChat struct {
	Username string
	ChatID   int64
}

// userRecord is the stored data for each user.
type userRecord struct {
	ChatID           int64     `json:"chat_id"`
	FirstSeen        time.Time `json:"first_seen"`
	LastSeen         time.Time `json:"last_seen"`
	Active           bool      `json:"active"`
	NotifiedVersions []string  `json:"notified_versions"`
}

// Store provides thread-safe persistent storage for chat IDs.
type Store struct {
	mu    sync.RWMutex
	path  string
	users map[string]*userRecord
}

// New creates a new Store. If file exists, it loads existing data.
func New(path string) *Store {
	s := &Store{
		path:  path,
		users: make(map[string]*userRecord),
	}
	s.load()
	return s
}

// load reads data from file. Errors are ignored (empty store on failure).
func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.users)
}

// save writes data to file atomically.
func (s *Store) save() error {
	data, err := json.MarshalIndent(s.users, "", "  ")
	if err != nil {
		return err
	}

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}

	return os.Rename(tmpPath, s.path)
}

// RecordInteraction updates user's last_seen and sets active=true.
// Creates new user record if not exists.
func (s *Store) RecordInteraction(username string, chatID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	if record, ok := s.users[username]; ok {
		record.ChatID = chatID
		record.LastSeen = now
		record.Active = true
	} else {
		s.users[username] = &userRecord{
			ChatID:           chatID,
			FirstSeen:        now,
			LastSeen:         now,
			Active:           true,
			NotifiedVersions: []string{},
		}
	}

	return s.save()
}

// GetActiveUsers returns all users with active=true.
func (s *Store) GetActiveUsers() ([]UserChat, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var users []UserChat
	for username, record := range s.users {
		if record.Active {
			users = append(users, UserChat{
				Username: username,
				ChatID:   record.ChatID,
			})
		}
	}
	return users, nil
}

// MarkNotified adds version to user's notified_versions list.
func (s *Store) MarkNotified(username string, version string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.users[username]
	if !ok {
		return nil
	}

	// Check if already notified
	for _, v := range record.NotifiedVersions {
		if v == version {
			return nil
		}
	}

	record.NotifiedVersions = append(record.NotifiedVersions, version)
	return s.save()
}

// IsNotified checks if user was notified about version.
func (s *Store) IsNotified(username string, version string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	record, ok := s.users[username]
	if !ok {
		return false
	}

	for _, v := range record.NotifiedVersions {
		if v == version {
			return true
		}
	}
	return false
}

// SetInactive marks user as inactive (e.g., bot blocked).
func (s *Store) SetInactive(username string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, ok := s.users[username]
	if !ok {
		return nil
	}

	record.Active = false
	return s.save()
}
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -v ./internal/chatstore/...`
Expected: All PASS

**Step 5: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/chatstore/
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add ChatStore for persistent chat_id storage

Thread-safe storage mapping username to chat_id with metadata.
Tracks first_seen, last_seen, active status, and notified versions.
Used for proactive update notifications.
EOF
)"
```

---

## Task 3: Добавить GetReleaseBody в updater для changelog

**Files:**
- Modify: `telegram-bot/internal/updater/github.go`
- Modify: `telegram-bot/internal/updater/updater.go`
- Modify: `telegram-bot/internal/updater/github_test.go`

**Step 1: Write the failing test**

В `telegram-bot/internal/updater/github_test.go` добавить тест (найти существующий `TestGetLatestRelease` и добавить проверку Body):

```go
func TestGetLatestRelease_IncludesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"tag_name": "v1.0.0",
			"body": "## Changelog\n- Fix bug\n- Add feature",
			"assets": []
		}`
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(response))
	}))
	defer server.Close()

	svc := NewWithBaseURL(server.URL)
	release, err := svc.GetLatestRelease(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expected := "## Changelog\n- Fix bug\n- Add feature"
	if release.Body != expected {
		t.Errorf("expected body %q, got %q", expected, release.Body)
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/updater/... -run TestGetLatestRelease_IncludesBody`
Expected: FAIL — `release.Body undefined`

**Step 3: Write minimal implementation**

В `telegram-bot/internal/updater/updater.go` добавить поле Body в Release (строка ~25):

```go
// Release represents a GitHub release.
type Release struct {
	TagName string
	Body    string // Release notes / changelog
	Assets  []Asset
}
```

В `telegram-bot/internal/updater/github.go` добавить Body в githubRelease и парсинг:

Изменить структуру githubRelease (~строка 21):
```go
// githubRelease represents the GitHub API response for releases/latest.
type githubRelease struct {
	TagName string        `json:"tag_name"`
	Body    string        `json:"body"`
	Assets  []githubAsset `json:"assets"`
}
```

В функции GetLatestRelease (~строка 65) добавить присваивание Body:
```go
	release := &Release{
		TagName: ghRelease.TagName,
		Body:    ghRelease.Body,
		Assets:  make([]Asset, len(ghRelease.Assets)),
	}
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -v ./internal/updater/...`
Expected: All PASS

**Step 5: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/updater/github.go telegram-bot/internal/updater/updater.go telegram-bot/internal/updater/github_test.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): include release body in GitHub API response

Adds Body field to Release struct for changelog display in update
notifications.
EOF
)"
```

---

## Task 4: Создать UpdateChecker сервис

**Files:**
- Create: `telegram-bot/internal/updatechecker/checker.go`
- Create: `telegram-bot/internal/updatechecker/checker_test.go`

**Step 1: Write the failing test**

Создать `telegram-bot/internal/updatechecker/checker_test.go`:

```go
package updatechecker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// mockUpdater implements updater.Updater for testing
type mockUpdater struct {
	release      *updater.Release
	releaseErr   error
	shouldUpdate bool
	shouldErr    error
}

func (m *mockUpdater) GetLatestRelease(ctx context.Context) (*updater.Release, error) {
	return m.release, m.releaseErr
}

func (m *mockUpdater) ShouldUpdate(current, latest string) (bool, error) {
	return m.shouldUpdate, m.shouldErr
}

func (m *mockUpdater) IsUpdateInProgress() bool            { return false }
func (m *mockUpdater) CreateLock() error                   { return nil }
func (m *mockUpdater) RemoveLock()                         {}
func (m *mockUpdater) CleanFiles()                         {}
func (m *mockUpdater) DownloadRelease(context.Context, *updater.Release) error { return nil }
func (m *mockUpdater) RunUpdateScript(int64, string, string) error             { return nil }

// mockSender captures sent messages
type mockSender struct {
	mu       sync.Mutex
	messages []sentMessage
}

type sentMessage struct {
	chatID int64
	text   string
}

func (m *mockSender) Send(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID, text})
	return nil
}

func (m *mockSender) getMessages() []sentMessage {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]sentMessage{}, m.messages...)
}

// mockAuth always authorizes
type mockAuth struct{}

func (m *mockAuth) IsAuthorized(username string) bool { return true }

func TestChecker_SendsNotificationOnNewVersion(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		release: &updater.Release{
			TagName: "v2.0.0",
			Body:    "New features",
		},
		shouldUpdate: true,
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// Run single check
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected notification to be sent")
	}

	if msgs[0].chatID != 111 {
		t.Errorf("expected chatID 111, got %d", msgs[0].chatID)
	}
}

func TestChecker_SkipsAlreadyNotified(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)
	_ = store.MarkNotified("user1", "v2.0.0")

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v2.0.0", Body: "New"},
		shouldUpdate: true,
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for already notified user, got %d", len(msgs))
	}
}

func TestChecker_SkipsWhenNoUpdate(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v1.0.0"},
		shouldUpdate: false,
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages when no update, got %d", len(msgs))
	}
}

func TestChecker_HandlesGitHubError(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		releaseErr: errors.New("network error"),
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx) // Should not panic

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages on error, got %d", len(msgs))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test -v ./internal/updatechecker/...`
Expected: FAIL — package not found

**Step 3: Write minimal implementation**

Создать `telegram-bot/internal/updatechecker/checker.go`:

```go
package updatechecker

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

const maxChangelogLength = 500

// Sender is the interface for sending messages.
type Sender interface {
	Send(chatID int64, text string) error
}

// Authorizer checks if a user is authorized.
type Authorizer interface {
	IsAuthorized(username string) bool
}

// ChatStore is the interface for chat storage.
type ChatStore interface {
	GetActiveUsers() ([]chatstore.UserChat, error)
	IsNotified(username string, version string) bool
	MarkNotified(username string, version string) error
	SetInactive(username string) error
}

// Checker periodically checks for updates and notifies users.
type Checker struct {
	updater        updater.Updater
	store          ChatStore
	sender         Sender
	auth           Authorizer
	currentVersion string
}

// New creates a new Checker.
func New(
	upd updater.Updater,
	store ChatStore,
	sender Sender,
	auth Authorizer,
	currentVersion string,
) *Checker {
	return &Checker{
		updater:        upd,
		store:          store,
		sender:         sender,
		auth:           auth,
		currentVersion: currentVersion,
	}
}

// Run starts the checker loop. Blocks until ctx is cancelled.
func (c *Checker) Run(ctx context.Context, interval time.Duration) {
	slog.Info("Update checker started", "interval", interval)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial check
	c.checkOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Update checker stopped")
			return
		case <-ticker.C:
			c.checkOnce(ctx)
		}
	}
}

// checkOnce performs a single update check.
func (c *Checker) checkOnce(ctx context.Context) {
	// Skip if current version is dev
	if c.currentVersion == "dev" {
		return
	}

	release, err := c.updater.GetLatestRelease(ctx)
	if err != nil {
		slog.Warn("Failed to check for updates", "error", err)
		return
	}

	shouldUpdate, err := c.updater.ShouldUpdate(c.currentVersion, release.TagName)
	if err != nil {
		slog.Warn("Failed to compare versions", "error", err)
		return
	}

	if !shouldUpdate {
		slog.Debug("No update available", "current", c.currentVersion, "latest", release.TagName)
		return
	}

	slog.Info("New version available", "current", c.currentVersion, "latest", release.TagName)

	c.notifyUsers(release)
}

// notifyUsers sends update notification to all active authorized users.
func (c *Checker) notifyUsers(release *updater.Release) {
	users, err := c.store.GetActiveUsers()
	if err != nil {
		slog.Warn("Failed to get active users", "error", err)
		return
	}

	for _, user := range users {
		// Check if user is still authorized
		if !c.auth.IsAuthorized(user.Username) {
			continue
		}

		// Check if already notified
		if c.store.IsNotified(user.Username, release.TagName) {
			continue
		}

		// Send notification
		msg := c.formatNotification(release)
		err := c.sender.Send(user.ChatID, msg)

		if err != nil {
			if isBlockedError(err) {
				slog.Info("User blocked bot, marking inactive", "username", user.Username)
				_ = c.store.SetInactive(user.Username)
			} else {
				slog.Warn("Failed to send notification", "username", user.Username, "error", err)
			}
			continue
		}

		// Mark as notified
		_ = c.store.MarkNotified(user.Username, release.TagName)
		slog.Info("Sent update notification", "username", user.Username, "version", release.TagName)
	}
}

// formatNotification creates the notification message.
func (c *Checker) formatNotification(release *updater.Release) string {
	changelog := release.Body
	if len(changelog) > maxChangelogLength {
		changelog = changelog[:maxChangelogLength] + "..."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🆕 Доступна новая версия %s\n\n", release.TagName))
	sb.WriteString(fmt.Sprintf("Текущая версия: %s\n\n", c.currentVersion))

	if changelog != "" {
		sb.WriteString("📋 Что нового:\n")
		sb.WriteString(changelog)
		sb.WriteString("\n\n")
	}

	sb.WriteString("Используйте /update для обновления")

	return sb.String()
}

// isBlockedError checks if error indicates bot was blocked.
func isBlockedError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "bot was blocked") ||
		strings.Contains(errStr, "chat not found") ||
		strings.Contains(errStr, "user is deactivated")
}
```

**Step 4: Run tests to verify they pass**

Run: `cd telegram-bot && go test -v ./internal/updatechecker/...`
Expected: All PASS

**Step 5: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/updatechecker/
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add UpdateChecker for automatic update notifications

Periodically checks GitHub for new releases and notifies active users.
Tracks notified versions per-user to avoid duplicate notifications.
Marks users inactive if bot is blocked.
EOF
)"
```

---

## Task 5: Интегрировать ChatStore в Bot

**Files:**
- Modify: `telegram-bot/internal/bot/bot.go`
- Modify: `telegram-bot/internal/bot/bot_test.go` (если есть релевантные тесты)

**Step 1: Modify bot.go to accept ChatStore**

В `telegram-bot/internal/bot/bot.go`:

Добавить импорт:
```go
import (
	// ... existing imports
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
)
```

Добавить поле в Bot struct (~строка 20):
```go
type Bot struct {
	api       *tgbotapi.BotAPI
	auth      *Auth
	router    *Router
	sender    telegram.MessageSender
	devMode   bool
	executor  service.ShellExecutor
	updater   updater.Updater
	chatStore *chatstore.Store
}
```

Добавить option функцию после WithUpdater (~строка 46):
```go
// WithChatStore sets the chat store for recording user interactions.
func WithChatStore(store *chatstore.Store) Option {
	return func(b *Bot) {
		b.chatStore = store
	}
}
```

В функции Run, после проверки авторизации для сообщений (~строка 171), добавить:
```go
			if !b.auth.IsAuthorized(username) {
				slog.Warn("Unauthorized access attempt", "username", username)
				b.sender.SendPlain(msg.Chat.ID, "Access denied")
				continue
			}
			// Record interaction for update notifications
			if b.chatStore != nil {
				_ = b.chatStore.RecordInteraction(username, msg.Chat.ID)
			}
```

Аналогично для callback queries (~строка 188):
```go
			if !b.auth.IsAuthorized(username) {
				slog.Warn("Unauthorized callback", "username", username)
				continue
			}
			// Record interaction for update notifications
			if b.chatStore != nil {
				_ = b.chatStore.RecordInteraction(username, cb.Message.Chat.ID)
			}
```

**Step 2: Add Auth() getter for UpdateChecker**

В конец `telegram-bot/internal/bot/bot.go` добавить:
```go
// Auth returns the authorization handler (for update checker).
func (b *Bot) Auth() *Auth {
	return b.auth
}
```

**Step 3: Run existing tests**

Run: `cd telegram-bot && go test -v ./internal/bot/...`
Expected: All PASS (chatStore is optional, nil-safe)

**Step 4: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/bot/bot.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): integrate ChatStore for recording user interactions

Records chat_id on each message/callback from authorized users.
Adds WithChatStore option and Auth() getter for update checker.
EOF
)"
```

---

## Task 6: Интегрировать UpdateChecker в main.go

**Files:**
- Modify: `telegram-bot/cmd/bot/main.go`

**Step 1: Update main.go**

В `telegram-bot/cmd/bot/main.go`:

Добавить импорты:
```go
import (
	// ... existing imports
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/chatstore"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updatechecker"
)
```

После загрузки конфига (~строка 86) и перед созданием бота, добавить создание ChatStore:
```go
	// Create chat store for update notifications
	var chatStore *chatstore.Store
	if !*devFlag {
		chatStore = chatstore.New(p.DefaultDataDir + "/chats.json")
		opts = append(opts, bot.WithChatStore(chatStore))
	}
```

После создания бота и регистрации команд (~строка 112), добавить запуск checker:
```go
	if err := b.RegisterCommands(); err != nil {
		slog.Warn("Failed to register commands", "error", err)
	}

	// Start update checker if configured
	if cfg.UpdateCheckInterval > 0 && !*devFlag && Version != "dev" {
		checker := updatechecker.New(
			updater.New(),
			chatStore,
			telegram.NewSender(nil), // Will create proper sender below
			b.Auth(),
			Version,
		)
		go checker.Run(ctx, cfg.UpdateCheckInterval)
	}
```

Но подождите — нам нужен sender с реальным API. Нужно реструктурировать. Вот правильный подход:

**Step 1 (revised): Update main.go properly**

Заменить секцию создания бота и запуска на:

```go
	// Create chat store for update notifications
	var chatStore *chatstore.Store
	if !*devFlag {
		chatStore = chatstore.New(p.DefaultDataDir + "/chats.json")
		opts = append(opts, bot.WithChatStore(chatStore))
	}

	b, err := bot.New(cfg, p, Version, VersionFull, Commit, BuildDate, opts...)
	if err != nil {
		slog.Error("Failed to create bot", "error", err)
		os.Exit(1)
	}

	if err := b.RegisterCommands(); err != nil {
		slog.Warn("Failed to register commands", "error", err)
	}

	// Start update checker if configured (not in dev mode, not dev version)
	if cfg.UpdateCheckInterval > 0 && !*devFlag && Version != "dev" {
		checker := updatechecker.New(
			updater.New(),
			chatStore,
			b.Sender(),
			b.Auth(),
			Version,
		)
		go checker.Run(ctx, cfg.UpdateCheckInterval)
	}

	slog.Info("Telegram Bot started", "version", versionString())
	b.Run(ctx)
	slog.Info("Bot stopped")
```

**Step 2: Add Sender() getter to Bot**

В `telegram-bot/internal/bot/bot.go` добавить:
```go
// Sender returns the message sender (for update checker).
func (b *Bot) Sender() telegram.MessageSender {
	return b.sender
}
```

**Step 3: Build and verify**

Run: `cd telegram-bot && go build ./cmd/bot`
Expected: Build succeeds

**Step 4: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/cmd/bot/main.go telegram-bot/internal/bot/bot.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): start UpdateChecker on bot startup

Launches background goroutine to check for updates at configured
interval. Only enabled when update_check_interval > 0 and not in
dev mode.
EOF
)"
```

---

## Task 7: Добавить inline-кнопку обновления в уведомление

**Files:**
- Modify: `telegram-bot/internal/updatechecker/checker.go`
- Modify: `telegram-bot/internal/updatechecker/checker_test.go`
- Modify: `telegram-bot/internal/telegram/sender.go` (добавить SendWithKeyboard в интерфейс)

**Step 1: Verify SendWithKeyboard exists in telegram.MessageSender**

Проверить `telegram-bot/internal/telegram/sender.go`. Интерфейс `MessageSender` уже содержит `SendWithKeyboard`. Если нет — добавить.

**Step 2: Update Sender interface in updatechecker**

В `telegram-bot/internal/updatechecker/checker.go` изменить интерфейс Sender:

```go
import (
	// ... add this import
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// Sender is the interface for sending messages.
type Sender interface {
	SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error
}
```

**Step 3: Update formatNotification to return keyboard**

Изменить метод formatNotification (обрезка по рунам для UTF-8):

```go
// formatNotification creates the notification message and keyboard.
func (c *Checker) formatNotification(release *updater.Release) (string, tgbotapi.InlineKeyboardMarkup) {
	changelog := release.Body
	// Truncate by runes to preserve UTF-8 validity
	runes := []rune(changelog)
	if len(runes) > maxChangelogLength {
		changelog = string(runes[:maxChangelogLength]) + "..."
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("🆕 Доступна новая версия %s\n\n", escape(release.TagName)))
	sb.WriteString(fmt.Sprintf("Текущая версия: %s\n\n", escape(c.currentVersion)))

	if changelog != "" {
		sb.WriteString("📋 Что нового:\n")
		sb.WriteString(escape(changelog))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔄 Обновить", "update:run"),
		),
	)

	return sb.String(), keyboard
}

// escape escapes special characters for MarkdownV2.
func escape(s string) string {
	replacer := strings.NewReplacer(
		"_", "\\_",
		"*", "\\*",
		"[", "\\[",
		"]", "\\]",
		"(", "\\(",
		")", "\\)",
		"~", "\\~",
		"`", "\\`",
		">", "\\>",
		"#", "\\#",
		"+", "\\+",
		"-", "\\-",
		"=", "\\=",
		"|", "\\|",
		"{", "\\{",
		"}", "\\}",
		".", "\\.",
		"!", "\\!",
	)
	return replacer.Replace(s)
}
```

**Step 4: Update notifyUsers to use keyboard**

```go
// notifyUsers sends update notification to all active authorized users.
func (c *Checker) notifyUsers(release *updater.Release) {
	users, err := c.store.GetActiveUsers()
	if err != nil {
		slog.Warn("Failed to get active users", "error", err)
		return
	}

	for _, user := range users {
		// Check if user is still authorized
		if !c.auth.IsAuthorized(user.Username) {
			continue
		}

		// Check if already notified
		if c.store.IsNotified(user.Username, release.TagName) {
			continue
		}

		// Send notification with keyboard (MarkdownV2 via SendWithKeyboard)
		msg, keyboard := c.formatNotification(release)
		err := c.sender.SendWithKeyboard(user.ChatID, msg, keyboard)

		if err != nil {
			if isBlockedError(err) {
				slog.Info("User blocked bot, marking inactive", "username", user.Username)
				_ = c.store.SetInactive(user.Username)
			} else {
				slog.Warn("Failed to send notification", "username", user.Username, "error", err)
			}
			continue
		}

		// Mark as notified
		_ = c.store.MarkNotified(user.Username, release.TagName)
		slog.Info("Sent update notification", "username", user.Username, "version", release.TagName)
	}
}
```

**Step 5: Update tests**

В `telegram-bot/internal/updatechecker/checker_test.go` обновить mockSender:

```go
import (
	// ... add
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockSender captures sent messages
type mockSender struct {
	mu       sync.Mutex
	messages []sentMessage
}

type sentMessage struct {
	chatID   int64
	text     string
	keyboard *tgbotapi.InlineKeyboardMarkup
}

func (m *mockSender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, sentMessage{chatID: chatID, text: text, keyboard: &keyboard})
	return nil
}
```

**Step 6: Run tests**

Run: `cd telegram-bot && go test -v ./internal/updatechecker/...`
Expected: All PASS

**Step 7: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/updatechecker/
git commit -m "$(cat <<'EOF'
feat(telegram-bot): add inline update button to notifications

Update notifications now include an inline keyboard with "🔄 Обновить"
button that triggers the update flow. Uses MarkdownV2 with proper
escaping and UTF-8 safe truncation.
EOF
)"
```

---

## Task 8: Добавить обработку callback "update:run" в router

**Files:**
- Modify: `telegram-bot/internal/bot/router.go`
- Modify: `telegram-bot/internal/handler/update.go`

**Step 1: Update UpdateRouterHandler interface**

В `telegram-bot/internal/bot/router.go` расширить интерфейс:

```go
// UpdateRouterHandler defines methods for update command
type UpdateRouterHandler interface {
	HandleUpdate(msg *tgbotapi.Message)
	HandleCallback(cb *tgbotapi.CallbackQuery)
}
```

**Step 2: Add callback routing for update:**

В `telegram-bot/internal/bot/router.go` в функции `RouteCallback`, добавить обработку:

```go
func (r *Router) RouteCallback(cb *tgbotapi.CallbackQuery) {
	if strings.HasPrefix(cb.Data, "servers:") {
		r.servers.HandleCallback(cb)
		return
	}
	if strings.HasPrefix(cb.Data, "update:") {
		r.update.HandleCallback(cb)
		return
	}
	r.wizard.HandleCallback(cb)
}
```

**Step 3: Add HandleCallback to UpdateHandler**

В `telegram-bot/internal/handler/update.go` добавить:

```go
// HandleCallback handles update callbacks from inline buttons.
func (h *UpdateHandler) HandleCallback(cb *tgbotapi.CallbackQuery) {
	if cb.Data != "update:run" {
		return
	}
	// Create a fake message with the callback's chat
	msg := &tgbotapi.Message{
		Chat: cb.Message.Chat,
		From: cb.From,
	}
	h.HandleUpdate(msg)
}
```

**Step 4: Run tests**

Run: `cd telegram-bot && go test -v ./...`
Expected: All PASS

**Step 5: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add telegram-bot/internal/bot/router.go telegram-bot/internal/handler/update.go
git commit -m "$(cat <<'EOF'
feat(telegram-bot): handle update:run callback from notification button

Allows users to trigger update directly from the notification message
inline keyboard.
EOF
)"
```

---

## Task 9: Обновить документацию

**Files:**
- Modify: `.claude/rules/telegram-bot.md`
- Modify: `telegram-bot/testdata/dev/telegram-bot.json.example` (если есть)

**Step 1: Update telegram-bot.md**

В секции "Config File" добавить новое поле:

```markdown
## Config File

`/opt/vpn-director/telegram-bot.json`:
```json
{
  "bot_token": "123456:ABC...",
  "allowed_users": ["username1", "username2"],
  "log_level": "info",
  "update_check_interval": "1h"
}
```

**Fields:**
- `bot_token` - Telegram Bot API token (required)
- `allowed_users` - Array of Telegram usernames
- `log_level` - Optional: `debug`, `info`, `warn`, `error` (default: `info`)
- `update_check_interval` - Optional: Go duration (e.g., `1h`, `30m`). If omitted or `"0"`, automatic update checking is disabled.
```

Добавить новую секцию "Automatic Update Notifications":

```markdown
## Automatic Update Notifications

When `update_check_interval` is set, the bot periodically checks GitHub for new releases and notifies users.

**How it works:**
1. Bot checks GitHub API at configured interval
2. If new version found, sends notification to all active users
3. Notification includes changelog and "🔄 Обновить" button
4. Each user is notified only once per version

**Data storage:**
- `/opt/vpn-director/data/chats.json` - stores chat IDs and notification history

**User tracking:**
- Chat ID recorded on first message from authorized user
- Users marked inactive if bot is blocked
- Reactivated automatically when user messages bot again
```

**Step 2: Update example config**

В `telegram-bot/testdata/dev/telegram-bot.json.example` добавить:

```json
{
  "bot_token": "YOUR_BOT_TOKEN",
  "allowed_users": ["your_username"],
  "log_level": "debug",
  "update_check_interval": "1m"
}
```

**Step 3: Commit**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git add .claude/rules/telegram-bot.md telegram-bot/testdata/dev/telegram-bot.json.example
git commit -m "$(cat <<'EOF'
docs: add update_check_interval configuration documentation

Documents automatic update notification feature and chats.json storage.
EOF
)"
```

---

## Task 10: Финальное тестирование

**Step 1: Run all tests**

Run: `cd telegram-bot && go test -v ./...`
Expected: All PASS

**Step 2: Build for all architectures**

Run: `cd telegram-bot && make build && make build-arm64 && make build-arm`
Expected: All builds succeed

**Step 3: Verify with go vet**

Run: `cd telegram-bot && go vet ./...`
Expected: No issues

**Step 4: Final commit (if any fixes needed)**

```bash
cd /opt/github/zinin/asuswrt-merlin-vpn-director
git status
# If clean, skip. Otherwise:
git add -A
git commit -m "fix: address issues found during final testing"
```

---

## Summary

| Task | Description | Files |
|------|-------------|-------|
| 1 | Add UpdateCheckInterval to config | config.go, config_test.go |
| 2 | Create ChatStore | chatstore/store.go, store_test.go |
| 3 | Add Release.Body for changelog | updater/github.go, updater.go |
| 4 | Create UpdateChecker service | updatechecker/checker.go, checker_test.go |
| 5 | Integrate ChatStore in Bot | bot/bot.go |
| 6 | Integrate UpdateChecker in main | cmd/bot/main.go |
| 7 | Add inline keyboard to notifications | updatechecker/checker.go |
| 8 | Handle update:start callback | bot/router.go, handler/update.go |
| 9 | Update documentation | telegram-bot.md, example config |
| 10 | Final testing | - |
