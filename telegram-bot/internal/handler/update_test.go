package handler

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/updater"
)

// mockUpdater implements updater.Updater for testing
type mockUpdater struct {
	mu               sync.Mutex
	latestRelease    *updater.Release
	latestReleaseErr error
	shouldUpdate     bool
	shouldUpdateErr  error
	updateInProgress bool
	createLockErr    error
	downloadErr      error
	runScriptErr     error

	// Track calls
	cleanFilesCalled  bool
	removeLockCalled  bool
	runScriptCalled   bool
	runScriptChatID   int64
	runScriptOldVer   string
	runScriptNewVer   string
	downloadCalled    bool
	downloadCtx       context.Context
	downloadRelease   *updater.Release
}

func (m *mockUpdater) GetLatestRelease(ctx context.Context) (*updater.Release, error) {
	if m.latestReleaseErr != nil {
		return nil, m.latestReleaseErr
	}
	return m.latestRelease, nil
}

func (m *mockUpdater) ShouldUpdate(currentVersion, latestTag string) (bool, error) {
	if m.shouldUpdateErr != nil {
		return false, m.shouldUpdateErr
	}
	return m.shouldUpdate, nil
}

func (m *mockUpdater) IsUpdateInProgress() bool {
	return m.updateInProgress
}

func (m *mockUpdater) CreateLock() error {
	return m.createLockErr
}

func (m *mockUpdater) RemoveLock() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLockCalled = true
}

func (m *mockUpdater) CleanFiles() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cleanFilesCalled = true
}

func (m *mockUpdater) DownloadRelease(ctx context.Context, release *updater.Release) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.downloadCalled = true
	m.downloadCtx = ctx
	m.downloadRelease = release
	return m.downloadErr
}

func (m *mockUpdater) RunUpdateScript(chatID int64, oldVersion, newVersion string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runScriptCalled = true
	m.runScriptChatID = chatID
	m.runScriptOldVer = oldVersion
	m.runScriptNewVer = newVersion
	return m.runScriptErr
}

// Thread-safe getters for verification in tests
func (m *mockUpdater) wasCleanFilesCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cleanFilesCalled
}

func (m *mockUpdater) wasRemoveLockCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.removeLockCalled
}

func (m *mockUpdater) wasRunScriptCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runScriptCalled
}

func (m *mockUpdater) getRunScriptArgs() (int64, string, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.runScriptChatID, m.runScriptOldVer, m.runScriptNewVer
}

// mockUpdateSender implements telegram.MessageSender for testing
type mockUpdateSender struct {
	mu        sync.Mutex
	messages  []string
	messageCh chan string // Channel for synchronization - receives each message
}

func newMockUpdateSender() *mockUpdateSender {
	return &mockUpdateSender{
		messageCh: make(chan string, 10), // Buffered to avoid blocking
	}
}

func (m *mockUpdateSender) Send(chatID int64, text string) error {
	m.mu.Lock()
	m.messages = append(m.messages, text)
	m.mu.Unlock()
	m.messageCh <- text
	return nil
}

func (m *mockUpdateSender) SendPlain(chatID int64, text string) error {
	m.mu.Lock()
	m.messages = append(m.messages, text)
	m.mu.Unlock()
	m.messageCh <- text
	return nil
}

func (m *mockUpdateSender) SendLongPlain(chatID int64, text string) error {
	return m.SendPlain(chatID, text)
}

func (m *mockUpdateSender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockUpdateSender) SendCodeBlock(chatID int64, header, content string) error {
	return nil
}

func (m *mockUpdateSender) EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockUpdateSender) AckCallback(callbackID string) error {
	return nil
}

func (m *mockUpdateSender) getMessages() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.messages))
	copy(result, m.messages)
	return result
}

// waitForMessages waits for n messages to be received, with timeout.
// Returns the messages received so far.
func (m *mockUpdateSender) waitForMessages(n int, timeout time.Duration) []string {
	deadline := time.After(timeout)
	for {
		m.mu.Lock()
		count := len(m.messages)
		m.mu.Unlock()
		if count >= n {
			return m.getMessages()
		}
		select {
		case <-m.messageCh:
			// Message received, check count again
		case <-deadline:
			return m.getMessages()
		}
	}
}

func TestUpdateHandler_DevMode(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{}
	h := NewUpdateHandler(sender, upd, true, "v1.0.0") // devMode=true

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Command /update is not available in dev mode" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_DevVersion(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{}
	h := NewUpdateHandler(sender, upd, false, "dev") // version="dev"

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Cannot check updates for dev build" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_UpdateInProgress(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		updateInProgress: true,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Update is already in progress, please wait..." {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_GitHubError(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestReleaseErr: errors.New("network error"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to check for updates: network error"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_AlreadyLatest(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.0.0"},
		shouldUpdate:  false,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Already running the latest version: v1.0.0" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_InvalidCurrentVersion(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
	}
	// Version with shell injection characters
	h := NewUpdateHandler(sender, upd, false, "v1.0.0;rm -rf /")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Invalid current version: v1.0.0;rm -rf /" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_InvalidReleaseVersion(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0$(whoami)"},
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Invalid release version: v1.1.0$(whoami)" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_VersionParseError(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease:   &updater.Release{TagName: "v1.1.0"},
		shouldUpdateErr: errors.New("invalid version format"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to parse version: invalid version format"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_CreateLockError(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		createLockErr: errors.New("lock failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to start update: lock failed"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_DownloadError(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		downloadErr:   errors.New("download failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	// Wait for 2 messages: "Starting update..." and "Download failed..."
	messages := sender.waitForMessages(2, time.Second)
	if len(messages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(messages))
	}
	// First message: "Starting update..."
	// Second message: "Download failed..."
	if messages[1] != "Download failed: download failed" {
		t.Errorf("Unexpected error message: %s", messages[1])
	}

	// Check cleanup was called (message sent after cleanup, so it's done)
	if !upd.wasCleanFilesCalled() {
		t.Error("CleanFiles should be called on download error")
	}
	if !upd.wasRemoveLockCalled() {
		t.Error("RemoveLock should be called on download error")
	}
}

func TestUpdateHandler_RunScriptError(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		runScriptErr:  errors.New("script failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	// Wait for 3 messages: "Starting...", "Files downloaded...", "Failed to run..."
	messages := sender.waitForMessages(3, time.Second)
	if len(messages) < 3 {
		t.Fatalf("Expected at least 3 messages, got %d", len(messages))
	}

	found := false
	for _, m := range messages {
		if m == "Failed to run update script: script failed" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected script error message, got: %v", messages)
	}

	// Check cleanup was called (message sent after cleanup, so it's done)
	if !upd.wasCleanFilesCalled() {
		t.Error("CleanFiles should be called on script error")
	}
	if !upd.wasRemoveLockCalled() {
		t.Error("RemoveLock should be called on script error")
	}
}

func TestUpdateHandler_Success(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}
	h.HandleUpdate(msg)

	// Wait for 3 messages
	messages := sender.waitForMessages(3, time.Second)
	// Expected messages:
	// 1. "Starting update v1.0.0 → v1.1.0..."
	// 2. "Files downloaded, starting update..."
	// 3. "Update script started, bot will restart in a few seconds..."
	if len(messages) != 3 {
		t.Fatalf("Expected 3 messages, got %d: %v", len(messages), messages)
	}

	if messages[0] != "Starting update v1.0.0 → v1.1.0..." {
		t.Errorf("Message 1: got %q", messages[0])
	}
	if messages[1] != "Files downloaded, starting update..." {
		t.Errorf("Message 2: got %q", messages[1])
	}
	if messages[2] != "Update script started, bot will restart in a few seconds..." {
		t.Errorf("Message 3: got %q", messages[2])
	}

	// Verify RunUpdateScript was called with correct args
	if !upd.wasRunScriptCalled() {
		t.Error("RunUpdateScript should be called")
	}
	chatID, oldVer, newVer := upd.getRunScriptArgs()
	if chatID != 42 {
		t.Errorf("RunUpdateScript chatID = %d, want 42", chatID)
	}
	if oldVer != "v1.0.0" {
		t.Errorf("RunUpdateScript oldVersion = %q, want v1.0.0", oldVer)
	}
	if newVer != "v1.1.0" {
		t.Errorf("RunUpdateScript newVersion = %q, want v1.1.0", newVer)
	}

	// Cleanup should NOT be called on success
	if upd.wasCleanFilesCalled() {
		t.Error("CleanFiles should NOT be called on success")
	}
	if upd.wasRemoveLockCalled() {
		t.Error("RemoveLock should NOT be called on success (script handles it)")
	}
}

func TestUpdateHandler_HandleCallback_UpdateRun(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.0.0"},
		shouldUpdate:  false, // No update available
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	cb := &tgbotapi.CallbackQuery{
		Data: "update:run",
		From: &tgbotapi.User{UserName: "testuser"},
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	// Should trigger HandleUpdate and send "Already running latest version"
	h.HandleCallback(cb)

	messages := sender.waitForMessages(1, time.Second)
	if len(messages) == 0 {
		t.Fatal("expected message to be sent")
	}
	if messages[0] != "Already running the latest version: v1.0.0" {
		t.Errorf("unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_HandleCallback_IgnoresOtherCallbacks(t *testing.T) {
	sender := newMockUpdateSender()
	upd := &mockUpdater{}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	cb := &tgbotapi.CallbackQuery{
		Data: "update:other",
		Message: &tgbotapi.Message{
			Chat: &tgbotapi.Chat{ID: 123},
		},
	}

	// Should not trigger HandleUpdate
	h.HandleCallback(cb)

	// Give a small window for any potential message
	messages := sender.getMessages()
	if len(messages) != 0 {
		t.Errorf("expected no messages for unknown callback, got: %v", messages)
	}
}
