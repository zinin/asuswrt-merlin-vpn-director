package telegram

import tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

// KeyboardBuilder provides a fluent interface for building inline keyboards
type KeyboardBuilder struct {
	rows       [][]tgbotapi.InlineKeyboardButton
	currentRow []tgbotapi.InlineKeyboardButton
}

// NewKeyboard creates a new keyboard builder
func NewKeyboard() *KeyboardBuilder {
	return &KeyboardBuilder{}
}

// Button adds a button to the current row
func (kb *KeyboardBuilder) Button(text, callbackData string) *KeyboardBuilder {
	kb.currentRow = append(kb.currentRow, tgbotapi.NewInlineKeyboardButtonData(text, callbackData))
	return kb
}

// Row finishes the current row and starts a new one
func (kb *KeyboardBuilder) Row() *KeyboardBuilder {
	if len(kb.currentRow) > 0 {
		kb.rows = append(kb.rows, kb.currentRow)
		kb.currentRow = nil
	}
	return kb
}

// Columns arranges all pending buttons into rows with n columns each
func (kb *KeyboardBuilder) Columns(n int) *KeyboardBuilder {
	if n <= 0 {
		n = 1
	}
	buttons := kb.currentRow
	kb.currentRow = nil

	for i := 0; i < len(buttons); i += n {
		end := i + n
		if end > len(buttons) {
			end = len(buttons)
		}
		kb.rows = append(kb.rows, buttons[i:end])
	}
	return kb
}

// Build returns the final InlineKeyboardMarkup
func (kb *KeyboardBuilder) Build() tgbotapi.InlineKeyboardMarkup {
	if len(kb.currentRow) > 0 {
		kb.rows = append(kb.rows, kb.currentRow)
	}
	if len(kb.rows) == 0 {
		return tgbotapi.InlineKeyboardMarkup{}
	}
	return tgbotapi.NewInlineKeyboardMarkup(kb.rows...)
}
