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

// mockUpdateSender implements telegram.MessageSender for testing
type mockUpdateSender struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockUpdateSender) Send(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockUpdateSender) SendPlain(chatID int64, text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
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

func TestUpdateHandler_DevMode(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{}
	h := NewUpdateHandler(sender, upd, true, "v1.0.0") // devMode=true

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Command /update is not available in dev mode" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_DevVersion(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{}
	h := NewUpdateHandler(sender, upd, false, "dev") // version="dev"

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Cannot check updates for dev build" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_UpdateInProgress(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		updateInProgress: true,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Update is already in progress, please wait..." {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_GitHubError(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestReleaseErr: errors.New("network error"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to check for updates: network error"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_AlreadyLatest(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.0.0"},
		shouldUpdate:  false,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Already running the latest version: v1.0.0" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_InvalidCurrentVersion(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
	}
	// Version with shell injection characters
	h := NewUpdateHandler(sender, upd, false, "v1.0.0;rm -rf /")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Invalid current version: v1.0.0;rm -rf /" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_InvalidReleaseVersion(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0$(whoami)"},
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	if messages[0] != "Invalid release version: v1.1.0$(whoami)" {
		t.Errorf("Unexpected message: %s", messages[0])
	}
}

func TestUpdateHandler_VersionParseError(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease:   &updater.Release{TagName: "v1.1.0"},
		shouldUpdateErr: errors.New("invalid version format"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to parse version: invalid version format"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_CreateLockError(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		createLockErr: errors.New("lock failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	messages := sender.getMessages()
	if len(messages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(messages))
	}
	expected := "Failed to start update: lock failed"
	if messages[0] != expected {
		t.Errorf("Got %q, want %q", messages[0], expected)
	}
}

func TestUpdateHandler_DownloadError(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		downloadErr:   errors.New("download failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	messages := sender.getMessages()
	if len(messages) < 2 {
		t.Fatalf("Expected at least 2 messages, got %d", len(messages))
	}
	// First message: "Starting update..."
	// Second message: "Download failed..."
	if messages[1] != "Download failed: download failed" {
		t.Errorf("Unexpected error message: %s", messages[1])
	}

	// Check cleanup was called
	if !upd.cleanFilesCalled {
		t.Error("CleanFiles should be called on download error")
	}
	if !upd.removeLockCalled {
		t.Error("RemoveLock should be called on download error")
	}
}

func TestUpdateHandler_RunScriptError(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
		runScriptErr:  errors.New("script failed"),
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleUpdate(msg)

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	messages := sender.getMessages()
	// Messages: "Starting...", "Files downloaded...", "Failed to run..."
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

	// Check cleanup was called
	if !upd.cleanFilesCalled {
		t.Error("CleanFiles should be called on script error")
	}
	if !upd.removeLockCalled {
		t.Error("RemoveLock should be called on script error")
	}
}

func TestUpdateHandler_Success(t *testing.T) {
	sender := &mockUpdateSender{}
	upd := &mockUpdater{
		latestRelease: &updater.Release{TagName: "v1.1.0"},
		shouldUpdate:  true,
	}
	h := NewUpdateHandler(sender, upd, false, "v1.0.0")

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}
	h.HandleUpdate(msg)

	// Wait for goroutine to complete
	time.Sleep(100 * time.Millisecond)

	messages := sender.getMessages()
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
	if !upd.runScriptCalled {
		t.Error("RunUpdateScript should be called")
	}
	if upd.runScriptChatID != 42 {
		t.Errorf("RunUpdateScript chatID = %d, want 42", upd.runScriptChatID)
	}
	if upd.runScriptOldVer != "v1.0.0" {
		t.Errorf("RunUpdateScript oldVersion = %q, want v1.0.0", upd.runScriptOldVer)
	}
	if upd.runScriptNewVer != "v1.1.0" {
		t.Errorf("RunUpdateScript newVersion = %q, want v1.1.0", upd.runScriptNewVer)
	}

	// Cleanup should NOT be called on success
	if upd.cleanFilesCalled {
		t.Error("CleanFiles should NOT be called on success")
	}
	if upd.removeLockCalled {
		t.Error("RemoveLock should NOT be called on success (script handles it)")
	}
}
