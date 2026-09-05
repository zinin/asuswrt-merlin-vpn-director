package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/chatstore"
)

// mockSender implements telegram.MessageSender for testing
type mockSender struct {
	sentMessages []sentMessage
	sendErr      error
	failFor      map[int64]bool
}

type sentMessage struct {
	chatID int64
	text   string
	plain  bool
}

func (m *mockSender) Send(chatID int64, text string) error {
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, false})
	return m.sendErr
}

func (m *mockSender) SendPlain(chatID int64, text string) error {
	if m.failFor[chatID] {
		return errors.New("bot was blocked by the user")
	}
	if m.sendErr != nil {
		return m.sendErr
	}
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, true})
	return nil
}

func (m *mockSender) SendLongPlain(chatID int64, text string) error {
	return m.SendPlain(chatID, text)
}

func (m *mockSender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockSender) SendCodeBlock(chatID int64, header, content string) error {
	return nil
}

func (m *mockSender) EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	return nil
}

func (m *mockSender) AckCallback(callbackID string) error {
	return nil
}

// mockStore implements ChatStore for testing.
type mockStore struct {
	users []chatstore.UserChat
	err   error
}

func (m *mockStore) GetActiveUsers() ([]chatstore.UserChat, error) { return m.users, m.err }

func TestCheckAndSendNotify_Success(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create notify file
	jsonData := `{"chat_id":123456789,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// Check message was sent as plain text
	if len(sender.sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.sentMessages))
	}

	msg := sender.sentMessages[0]
	if msg.chatID != 123456789 {
		t.Errorf("chatID = %d, want 123456789", msg.chatID)
	}
	if msg.text != "Update complete: v1.0.0 → v1.1.0" {
		t.Errorf("text = %q, want 'Update complete: v1.0.0 → v1.1.0'", msg.text)
	}
	if !msg.plain {
		t.Error("Message should be sent as plain text")
	}

	// Check update dir was deleted
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("update dir should be deleted after successful send")
	}
}

func TestCheckAndSendNotify_NoFile(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	// Don't create the file

	sender := &mockSender{}

	// Should not error when file doesn't exist
	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v, want nil for missing file", err)
	}

	// No messages sent
	if len(sender.sentMessages) != 0 {
		t.Errorf("Expected 0 messages, got %d", len(sender.sentMessages))
	}
}

func TestCheckAndSendNotify_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create invalid JSON
	if err := os.WriteFile(notifyFile, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if len(sender.sentMessages) != 0 {
		t.Error("Should not send message on parse error")
	}
}

func TestCheckAndSendNotify_SendError(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create valid notify file
	jsonData := `{"chat_id":123,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{
		sendErr: errors.New("telegram API error"),
	}

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err == nil {
		t.Fatal("Expected error when send fails")
	}

	// Update dir should NOT be deleted on send error
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Error("update dir should NOT be deleted on send failure (for retry)")
	}
}

func TestCheckAndSendNotify_CleanupAfterSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	updateLogFile := filepath.Join(tmpDir, "update.log")

	// Create notify file and update.log
	jsonData := `{"chat_id":123,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updateLogFile, []byte("log content"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// Both files should be deleted (entire directory)
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify file should be deleted")
	}
	if _, err := os.Stat(updateLogFile); !os.IsNotExist(err) {
		t.Error("update.log should be deleted")
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("update dir should be deleted")
	}
}

func TestCheckAndSendNotify_EmptyVersions(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create notify file with empty versions (valid but unusual)
	jsonData := `{"chat_id":123,"old_version":"","new_version":""}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir)
	if err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	// Should still send message with empty versions
	if len(sender.sentMessages) != 1 {
		t.Fatalf("Expected 1 message, got %d", len(sender.sentMessages))
	}
	if sender.sentMessages[0].text != "Update complete:  → " {
		t.Errorf("Unexpected message: %s", sender.sentMessages[0].text)
	}
}

func TestCheckAndSendNotify_ZeroChatIDGoesToEveryActiveUser(t *testing.T) {
	// A Web UI update has no chat of its own; the bot tells everyone it talks to.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	store := &mockStore{users: []chatstore.UserChat{
		{Username: "alice", ChatID: 111},
		{Username: "bob", ChatID: 222},
	}}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 2 {
		t.Fatalf("sent %d messages, want 2", len(sender.sentMessages))
	}
	for _, msg := range sender.sentMessages {
		if msg.text != "Update complete: v1.2.0 → v1.3.0" {
			t.Errorf("text = %q", msg.text)
		}
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("the update directory must be cleaned after a successful send")
	}
}

func TestCheckAndSendNotify_FailedStatusKeepsTheLog(t *testing.T) {
	// The message points at update.log, so the log has to survive the cleanup.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	logFile := filepath.Join(tmpDir, "update.log")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v1.2.0","new_version":"v1.3.0","status":"failed","initiator":"bot"}`)
	if err := os.WriteFile(logFile, []byte("cp: cannot stat\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
	}
	want := "Update failed: see " + logFile
	if sender.sentMessages[0].text != want {
		t.Errorf("text = %q, want %q", sender.sentMessages[0].text, want)
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("update.log must survive a failed update: %v", err)
	}
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify.json must go, or the message repeats on every restart")
	}
}

func TestCheckAndSendNotify_ZeroChatIDWithoutStoreIsSkipped(t *testing.T) {
	// Dev mode has no chat store; there is nobody to notify and nothing to fix.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want none", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("without a store the notification stays for the next start")
	}
}

func TestCheckAndSendNotify_ZeroChatIDWithNoActiveUsersIsCleanedUp(t *testing.T) {
	// Nothing can ever be delivered, so leaving the file would keep the
	// directory around forever.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, &mockStore{}, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("an undeliverable notification must not linger")
	}
}

func TestCheckAndSendNotify_PartialFailureKeepsNothingBack(t *testing.T) {
	// One blocked user must not hold the notification for everyone else.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{failFor: map[int64]bool{111: true}}
	store := &mockStore{users: []chatstore.UserChat{
		{Username: "alice", ChatID: 111},
		{Username: "bob", ChatID: 222},
	}}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}
	if _, err := os.Stat(tmpDir); !os.IsNotExist(err) {
		t.Error("one successful send is enough to clean up")
	}
}

func TestCheckAndSendNotify_AllSendsFailKeepTheFile(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"bot"}`)

	sender := &mockSender{sendErr: errors.New("network down")}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir); err == nil {
		t.Fatal("CheckAndSendNotify() must report a total delivery failure")
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("an undelivered notification must be retried on the next start")
	}
}

func TestCheckAndSendNotify_StoreErrorKeepsTheFile(t *testing.T) {
	// The recipients are unknown, not absent: dropping the file here would lose
	// the notification for good.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	store := &mockStore{err: errors.New("chats.json is unreadable")}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir); err == nil {
		t.Fatal("CheckAndSendNotify() must report a store failure")
	}
	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want none", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("the notification must survive a store failure and be retried")
	}
}

func writeNotify(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}
