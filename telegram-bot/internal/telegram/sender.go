package telegram

import (
	"log"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
)

// BotAPI is the interface for Telegram bot API operations
type BotAPI interface {
	Send(c tgbotapi.Chattable) (tgbotapi.Message, error)
	Request(c tgbotapi.Chattable) (*tgbotapi.APIResponse, error)
}

// MessageSender defines the interface for sending Telegram messages
type MessageSender interface {
	Send(chatID int64, text string) error
	SendPlain(chatID int64, text string) error
	SendLongPlain(chatID int64, text string) error
	SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error
	SendCodeBlock(chatID int64, header, content string) error
	EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error
	AckCallback(callbackID string) error
}

// Sender implements MessageSender using Telegram Bot API
type Sender struct {
	api BotAPI
}

// NewSender creates a new Sender
func NewSender(api BotAPI) *Sender {
	return &Sender{api: api}
}

// Send sends a MarkdownV2 formatted message
func (s *Sender) Send(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "MarkdownV2"
	_, err := s.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send message to %d: %v", chatID, err)
	}
	return err
}

// SendPlain sends a plain text message without formatting
func (s *Sender) SendPlain(chatID int64, text string) error {
	msg := tgbotapi.NewMessage(chatID, text)
	_, err := s.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send message to %d: %v", chatID, err)
	}
	return err
}

// SendLongPlain sends a long plain-text message, splitting into chunks if needed
// TODO: Consider adding a Markdown-safe variant (or parse-mode param) and UTF-8 safe chunking.
func (s *Sender) SendLongPlain(chatID int64, text string) error {
	for len(text) > 0 {
		chunk := text
		if len(chunk) > MaxMessageLength {
			chunk = text[:MaxMessageLength]
			// Try to break at newline
			if idx := lastIndex(chunk, '\n'); idx > MaxMessageLength/2 {
				chunk = text[:idx+1]
			}
		}
		if err := s.SendPlain(chatID, chunk); err != nil {
			return err
		}
		text = text[len(chunk):]
	}
	return nil
}

func lastIndex(s string, b byte) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == b {
			return i
		}
	}
	return -1
}

// SendWithKeyboard sends a message with inline keyboard
func (s *Sender) SendWithKeyboard(chatID int64, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.ParseMode = "MarkdownV2"
	msg.ReplyMarkup = keyboard
	_, err := s.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Failed to send message with keyboard to %d: %v", chatID, err)
	}
	return err
}

// SendCodeBlock sends a message with code block formatting
func (s *Sender) SendCodeBlock(chatID int64, header, content string) error {
	text := BuildCodeBlockMessage(header, content, MaxMessageLength)
	return s.Send(chatID, text)
}

// EditMessage edits an existing message
func (s *Sender) EditMessage(chatID int64, msgID int, text string, keyboard tgbotapi.InlineKeyboardMarkup) error {
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = "MarkdownV2"
	_, err := s.api.Send(edit)
	if err != nil {
		log.Printf("[ERROR] Failed to edit message %d: %v", msgID, err)
	}
	return err
}

// AckCallback acknowledges a callback query
func (s *Sender) AckCallback(callbackID string) error {
	_, err := s.api.Request(tgbotapi.NewCallback(callbackID, ""))
	if err != nil {
		log.Printf("[ERROR] Failed to acknowledge callback: %v", err)
	}
	return err
}
