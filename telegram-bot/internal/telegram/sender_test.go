package telegram

import (
	"strings"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// MockBotAPI implements the minimal interface needed for testing
type MockBotAPI struct {
	SentMessages []tgbotapi.Chattable
	LastError    error
}

func (m *MockBotAPI) Send(c tgbotapi.Chattable) (tgbotapi.Message, error) {
	m.SentMessages = append(m.SentMessages, c)
	return tgbotapi.Message{}, m.LastError
}

func (m *MockBotAPI) Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error) {
	m.SentMessages = append(m.SentMessages, c)
	return &tgbotapi.APIResponse{Ok: true}, m.LastError
}

func TestSender_Send(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	err := sender.Send(123, "Hello \\*world\\*")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(mock.SentMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.SentMessages))
	}

	msg, ok := mock.SentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", mock.SentMessages[0])
	}
	if msg.ChatID != 123 {
		t.Errorf("expected chatID 123, got %d", msg.ChatID)
	}
	if msg.ParseMode != "MarkdownV2" {
		t.Errorf("expected MarkdownV2, got %s", msg.ParseMode)
	}
}

func TestSender_SendPlain(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	err := sender.SendPlain(123, "Plain text")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	msg, ok := mock.SentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", mock.SentMessages[0])
	}
	if msg.ParseMode != "" {
		t.Errorf("expected empty parse mode for plain, got %s", msg.ParseMode)
	}
}

func TestSender_SendCodeBlock(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	err := sender.SendCodeBlock(456, "Status:", "running\nok")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	msg, ok := mock.SentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", mock.SentMessages[0])
	}
	if msg.ChatID != 456 {
		t.Errorf("expected chatID 456, got %d", msg.ChatID)
	}
	// Should contain code block markers
	if !strings.Contains(msg.Text, "```") {
		t.Errorf("expected code block, got %s", msg.Text)
	}
}

func TestSender_SendWithKeyboard(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	kb := NewKeyboard().Button("Test", "test").Row().Build()
	err := sender.SendWithKeyboard(456, "Pick one", kb)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	msg, ok := mock.SentMessages[0].(tgbotapi.MessageConfig)
	if !ok {
		t.Fatalf("expected MessageConfig, got %T", mock.SentMessages[0])
	}
	if msg.ReplyMarkup == nil {
		t.Error("expected keyboard, got nil")
	}
}

func TestSender_SendLongPlain(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	// Create a message longer than MaxMessageLength
	longText := strings.Repeat("a", MaxMessageLength+100)

	err := sender.SendLongPlain(123, longText)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should be split into 2 messages
	if len(mock.SentMessages) != 2 {
		t.Errorf("expected 2 messages for long text, got %d", len(mock.SentMessages))
	}
}

func TestSender_SendLongPlain_BreaksAtNewline(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	// Create text with newline in the middle
	part1 := strings.Repeat("a", MaxMessageLength-100)
	part2 := strings.Repeat("b", 200)
	longText := part1 + "\n" + part2

	err := sender.SendLongPlain(123, longText)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Should be split into 2 messages, breaking at newline
	if len(mock.SentMessages) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(mock.SentMessages))
	}
}

func TestSender_EditMessage(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	kb := NewKeyboard().Button("OK", "ok").Build()
	err := sender.EditMessage(123, 456, "Updated", kb)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(mock.SentMessages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mock.SentMessages))
	}
}

func TestSender_AckCallback(t *testing.T) {
	mock := &MockBotAPI{}
	sender := NewSender(mock)

	err := sender.AckCallback("callback123")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(mock.SentMessages) != 1 {
		t.Fatalf("expected 1 request, got %d", len(mock.SentMessages))
	}
}
