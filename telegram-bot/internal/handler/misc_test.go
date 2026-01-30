// internal/handler/misc_test.go
package handler

import (
	"errors"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/paths"
	"github.com/zinin/asuswrt-merlin-vpn-director/telegram-bot/internal/telegram"
)

type mockSender struct {
	lastChatID      int64
	lastText        string
	lastCodeHeader  string
	lastCodeContent string
}

func (m *mockSender) Send(chatID int64, text string) error {
	m.lastChatID = chatID
	m.lastText = text
	return nil
}
func (m *mockSender) SendPlain(chatID int64, text string) error { return nil }
func (m *mockSender) SendLongPlain(chatID int64, text string) error  { return nil }
func (m *mockSender) SendWithKeyboard(chatID int64, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	return nil
}
func (m *mockSender) SendCodeBlock(chatID int64, header, content string) error {
	m.lastChatID = chatID
	m.lastCodeHeader = header
	m.lastCodeContent = content
	return nil
}
func (m *mockSender) EditMessage(chatID int64, msgID int, text string, kb tgbotapi.InlineKeyboardMarkup) error {
	return nil
}
func (m *mockSender) AckCallback(callbackID string) error { return nil }

type mockNetworkInfo struct {
	ip  string
	err error
}

func (m *mockNetworkInfo) GetExternalIP() (string, error) {
	return m.ip, m.err
}

type mockLogReader struct {
	output string
	err    error
	calls  []logReadCall
}

type logReadCall struct {
	path  string
	lines int
}

func (m *mockLogReader) Read(path string, lines int) (string, error) {
	m.calls = append(m.calls, logReadCall{path: path, lines: lines})
	return m.output, m.err
}

func TestMiscHandler_HandleVersion(t *testing.T) {
	sender := &mockSender{}
	deps := &Deps{
		Sender:      sender,
		Version:     "v1.0.0",
		VersionFull: "v1.0.0-5-gabc1234",
		Commit:      "abc1234",
		BuildDate:   "2026-01-30",
	}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 123}}
	h.HandleVersion(msg)

	if sender.lastChatID != 123 {
		t.Errorf("expected chatID 123, got %d", sender.lastChatID)
	}
	expected := telegram.EscapeMarkdownV2("v1.0.0-5-gabc1234 (abc1234, 2026-01-30)")
	if sender.lastText != expected {
		t.Errorf("expected %q, got %q", expected, sender.lastText)
	}
}

func TestMiscHandler_HandleStart(t *testing.T) {
	sender := &mockSender{}
	deps := &Deps{Sender: sender}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 456}}
	h.HandleStart(msg)

	if sender.lastChatID != 456 {
		t.Errorf("expected chatID 456, got %d", sender.lastChatID)
	}
	if sender.lastText == "" {
		t.Error("expected non-empty start message")
	}
}

func TestMiscHandler_HandleIP_Success(t *testing.T) {
	sender := &mockSender{}
	network := &mockNetworkInfo{ip: "1.2.3.4"}
	deps := &Deps{Sender: sender, Network: network}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 789}}
	h.HandleIP(msg)

	if sender.lastChatID != 789 {
		t.Errorf("expected chatID 789, got %d", sender.lastChatID)
	}
	expected := "\U0001F310 External IP: `1.2.3.4`"
	if sender.lastText != expected {
		t.Errorf("expected %q, got %q", expected, sender.lastText)
	}
}

func TestMiscHandler_HandleIP_Error(t *testing.T) {
	sender := &mockSender{}
	network := &mockNetworkInfo{err: errors.New("network error")}
	deps := &Deps{Sender: sender, Network: network}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 789}}
	h.HandleIP(msg)

	if sender.lastChatID != 789 {
		t.Errorf("expected chatID 789, got %d", sender.lastChatID)
	}
	expected := telegram.EscapeMarkdownV2("network error")
	if sender.lastText != expected {
		t.Errorf("expected %q, got %q", expected, sender.lastText)
	}
}

func TestMiscHandler_HandleLogs_DefaultArgs(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "log line 1\nlog line 2"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	// Default is "all" which reads both bot and vpn logs
	if len(logReader.calls) != 2 {
		t.Errorf("expected 2 log read calls, got %d", len(logReader.calls))
	}
	// Check first call (bot logs)
	if logReader.calls[0].path != "/tmp/bot.log" {
		t.Errorf("expected bot log path, got %q", logReader.calls[0].path)
	}
	if logReader.calls[0].lines != 20 {
		t.Errorf("expected 20 lines, got %d", logReader.calls[0].lines)
	}
	// Check second call (vpn logs)
	if logReader.calls[1].path != "/tmp/vpn.log" {
		t.Errorf("expected vpn log path, got %q", logReader.calls[1].path)
	}
}

func TestMiscHandler_HandleLogs_SourceBot(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "bot log"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs bot",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Errorf("expected 1 log read call, got %d", len(logReader.calls))
	}
	if logReader.calls[0].path != "/tmp/bot.log" {
		t.Errorf("expected bot log path, got %q", logReader.calls[0].path)
	}
}

func TestMiscHandler_HandleLogs_SourceVPN(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "vpn log"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs vpn",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Errorf("expected 1 log read call, got %d", len(logReader.calls))
	}
	if logReader.calls[0].path != "/tmp/vpn.log" {
		t.Errorf("expected vpn log path, got %q", logReader.calls[0].path)
	}
}

func TestMiscHandler_HandleLogs_WithLines(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "log"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs bot 50",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Errorf("expected 1 log read call, got %d", len(logReader.calls))
	}
	if logReader.calls[0].lines != 50 {
		t.Errorf("expected 50 lines, got %d", logReader.calls[0].lines)
	}
}

func TestMiscHandler_HandleLogs_LinesOnly(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "log"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs 30",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	// When only a number is given, source defaults to "all"
	if len(logReader.calls) != 2 {
		t.Errorf("expected 2 log read calls, got %d", len(logReader.calls))
	}
	if logReader.calls[0].lines != 30 {
		t.Errorf("expected 30 lines, got %d", logReader.calls[0].lines)
	}
}

func TestMiscHandler_HandleLogs_MaxLinesLimit(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "log"}
	testPaths := paths.Paths{
		BotLogPath: "/tmp/bot.log",
		VPNLogPath: "/tmp/vpn.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	// Request more than maxLogLines (500)
	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs bot 99999",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Fatalf("expected 1 log read call, got %d", len(logReader.calls))
	}
	// Should be capped at maxLogLines (500)
	if logReader.calls[0].lines != 500 {
		t.Errorf("expected 500 lines (maxLogLines), got %d", logReader.calls[0].lines)
	}
}
