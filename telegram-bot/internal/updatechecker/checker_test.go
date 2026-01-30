package updatechecker

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
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

func (m *mockUpdater) IsUpdateInProgress() bool                                       { return false }
func (m *mockUpdater) CreateLock() error                                              { return nil }
func (m *mockUpdater) RemoveLock()                                                    {}
func (m *mockUpdater) CleanFiles()                                                    {}
func (m *mockUpdater) DownloadRelease(context.Context, *updater.Release) error        { return nil }
func (m *mockUpdater) RunUpdateScript(int64, string, string) error                    { return nil }

// mockSender captures sent messages (implements Sender interface with SendWithKeyboard)
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

func TestChecker_SetsInactiveOnBlock(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v2.0.0", Body: "New"},
		shouldUpdate: true,
	}

	// Sender that returns "bot was blocked" error
	sender := &mockSenderWithError{
		err: errors.New("Forbidden: bot was blocked by the user"),
	}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx)

	// User should be marked inactive
	users, _ := store.GetActiveUsers()
	if len(users) != 0 {
		t.Errorf("expected user to be marked inactive, got %d active users", len(users))
	}
}

func TestChecker_SkipsDevVersion(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v2.0.0", Body: "New"},
		shouldUpdate: true,
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	// Current version is "dev"
	checker := New(upd, store, sender, auth, "dev")

	ctx := context.Background()
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for dev version, got %d", len(msgs))
	}
}

func TestChecker_SkipsUnauthorizedUser(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v2.0.0", Body: "New"},
		shouldUpdate: true,
	}

	sender := &mockSender{}
	// Auth that rejects all users
	auth := &mockAuthReject{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) != 0 {
		t.Errorf("expected no messages for unauthorized user, got %d", len(msgs))
	}
}

func TestChecker_NotifiesMultipleUsers(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)
	_ = store.RecordInteraction("user2", 222)

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
	if len(msgs) != 2 {
		t.Errorf("expected 2 messages for 2 users, got %d", len(msgs))
	}
}

func TestChecker_TruncatesLongChangelog(t *testing.T) {
	tmpDir := t.TempDir()
	store := chatstore.New(tmpDir + "/chats.json")
	_ = store.RecordInteraction("user1", 111)

	// Create a very long changelog
	longBody := ""
	for i := 0; i < 600; i++ {
		longBody += "x"
	}

	upd := &mockUpdater{
		release:      &updater.Release{TagName: "v2.0.0", Body: longBody},
		shouldUpdate: true,
	}

	sender := &mockSender{}
	auth := &mockAuth{}

	checker := New(upd, store, sender, auth, "v1.0.0")

	ctx := context.Background()
	checker.checkOnce(ctx)

	msgs := sender.getMessages()
	if len(msgs) == 0 {
		t.Fatal("expected notification to be sent")
	}

	// Check that message contains escaped "..." indicating truncation
	// EscapeMarkdownV2 escapes dots: "..." -> "\.\.\."
	if !contains(msgs[0].text, "\\.\\.\\.") {
		t.Error("expected message to contain truncation indicator (escaped '...')")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// mockSenderWithError returns error on SendWithKeyboard
type mockSenderWithError struct {
	err error
}

func (m *mockSenderWithError) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return m.err
}

// mockAuthReject rejects all users
type mockAuthReject struct{}

func (m *mockAuthReject) IsAuthorized(username string) bool { return false }
