package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// mockSender implements telegram.MessageSender for testing
type mockSender struct {
	sentMessages []sentMessage
	sendErr      error
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
	m.sentMessages = append(m.sentMessages, sentMessage{chatID, text, true})
	return m.sendErr
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

func TestCheckAndSendNotify_Success(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create notify file
	jsonData := `{"chat_id":123456789,"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
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
	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
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

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
	if err == nil {
		t.Fatal("Expected error for invalid JSON")
	}
	if len(sender.sentMessages) != 0 {
		t.Error("Should not send message on parse error")
	}
}

func TestCheckAndSendNotify_MissingChatID(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")

	// Create JSON without chat_id (defaults to 0)
	jsonData := `{"old_version":"v1.0.0","new_version":"v1.1.0"}`
	if err := os.WriteFile(notifyFile, []byte(jsonData), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
	if err == nil {
		t.Fatal("Expected error for missing chat_id")
	}
	if len(sender.sentMessages) != 0 {
		t.Error("Should not send message without chat_id")
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

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
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

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
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

	err := CheckAndSendNotify(sender, notifyFile, tmpDir)
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
