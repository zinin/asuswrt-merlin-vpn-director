package handler

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// mockFlow implements UpdateFlow for testing.
type mockFlow struct {
	mu sync.Mutex

	res       updateflow.StartResult
	err       error
	progress  []string
	started   bool
	initiator string
	chatID    int64
}

func (m *mockFlow) Start(_ context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error) {
	m.mu.Lock()
	m.started = true
	m.initiator = initiator
	m.chatID = chatID
	lines := m.progress
	res, err := m.res, m.err
	m.mu.Unlock()

	for _, line := range lines {
		progress(line)
	}
	return res, err
}

// mockUpdateSender implements telegram.MessageSender for testing.
type mockUpdateSender struct {
	mu       sync.Mutex
	messages []string
}

func (m *mockUpdateSender) Send(_ int64, text string) error      { return m.record(text) }
func (m *mockUpdateSender) SendPlain(_ int64, text string) error { return m.record(text) }
func (m *mockUpdateSender) SendLongPlain(_ int64, text string) error {
	return m.record(text)
}

func (m *mockUpdateSender) SendWithKeyboard(_ int64, text string, _ tgbotapi.InlineKeyboardMarkup) error {
	return m.record(text)
}
func (m *mockUpdateSender) SendCodeBlock(_ int64, _, content string) error { return m.record(content) }
func (m *mockUpdateSender) EditMessage(_ int64, _ int, text string, _ tgbotapi.InlineKeyboardMarkup) error {
	return m.record(text)
}
func (m *mockUpdateSender) AckCallback(_ string) error { return nil }

func (m *mockUpdateSender) record(text string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, text)
	return nil
}

func (m *mockUpdateSender) all() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return strings.Join(m.messages, "\n")
}

func (m *mockUpdateSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}

func testMessage() *tgbotapi.Message {
	return &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}}
}

func TestUpdateHandler_MessagePerOutcome(t *testing.T) {
	tests := []struct {
		name string
		res  updateflow.StartResult
		err  error
		want string
	}{
		{
			name: "started",
			res:  updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"},
			want: "Starting update v1.2.0 → v1.3.0...",
		},
		{
			name: "dev mode",
			err:  updateflow.ErrDevMode,
			want: "Command /update is not available in dev mode",
		},
		{
			name: "dev build",
			err:  updateflow.ErrDevVersion,
			want: "Cannot check updates for dev build",
		},
		{
			name: "in progress",
			err:  updateflow.ErrInProgress,
			want: "Update is already in progress, please wait...",
		},
		{
			name: "up to date",
			res:  updateflow.StartResult{From: "v1.3.0", To: "v1.3.0"},
			err:  updateflow.ErrUpToDate,
			want: "Already running the latest version: v1.3.0",
		},
		{
			name: "github down",
			err:  &updateflow.GitHubError{Err: errors.New("connection refused")},
			want: "Failed to check for updates: connection refused",
		},
		{
			name: "anything else",
			err:  errors.New("invalid release version: vX"),
			want: "Failed to start update: invalid release version: vX",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &mockUpdateSender{}
			flow := &mockFlow{res: tt.res, err: tt.err}
			h := NewUpdateHandler(sender, flow, "v1.2.0")

			h.HandleUpdate(testMessage())

			// Contains alone lets a spurious extra message through. HandleUpdate
			// takes exactly one branch of its switch and no row of this table
			// feeds mockFlow any progress lines, so every outcome is one
			// message - which is what the pre-block tests asserted with exact
			// equality before the table replaced them.
			if n := sender.count(); n != 1 {
				t.Fatalf("sent %d messages for this outcome, want exactly 1: %q", n, sender.all())
			}
			if got := sender.all(); !strings.Contains(got, tt.want) {
				t.Errorf("messages = %q, want to contain %q", got, tt.want)
			}
		})
	}
}

func TestUpdateHandler_ForwardsProgressToTheChat(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{
		res:      updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"},
		progress: []string{"Files downloaded, starting update..."},
	}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleUpdate(testMessage())

	if got := sender.all(); !strings.Contains(got, "Files downloaded, starting update...") {
		t.Errorf("progress not forwarded: %q", got)
	}
}

// Start returns once the download goroutine is scheduled, and that goroutine
// reports through the same callback. A chat that reads "Files downloaded..."
// before "Starting update v1.2.0 → v1.3.0..." looks like a second update.
func TestUpdateHandler_AnnouncesTheStartBeforeProgress(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{
		res:      updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"},
		progress: []string{"Files downloaded, starting update..."},
	}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleUpdate(testMessage())

	all := sender.all()
	start := strings.Index(all, "Starting update v1.2.0 → v1.3.0...")
	progress := strings.Index(all, "Files downloaded, starting update...")
	switch {
	case start < 0:
		t.Fatalf("no start announcement in %q", all)
	case progress < 0:
		t.Fatalf("progress never reached the chat: %q", all)
	case start > progress:
		t.Errorf("progress came first:\n%s", all)
	}
}

func TestUpdateHandler_StartsAsBotWithTheChatID(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{res: updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleUpdate(testMessage())

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.initiator != "bot" || flow.chatID != 42 {
		t.Errorf("Start(initiator=%q, chatID=%d), want bot/42", flow.initiator, flow.chatID)
	}
}

func TestUpdateHandler_HandleCallback_UpdateRun(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{res: updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{
		Data:    "update:run",
		Message: &tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 42}},
		From:    &tgbotapi.User{UserName: "admin"},
	})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if !flow.started {
		t.Error("the update:run button must start an update")
	}
}

func TestUpdateHandler_HandleCallback_IgnoresOtherCallbacks(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{Data: "clients:add"})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.started {
		t.Error("an unrelated callback must not start an update")
	}
}

func TestUpdateHandler_HandleCallback_WithoutMessage(t *testing.T) {
	sender := &mockUpdateSender{}
	flow := &mockFlow{}
	h := NewUpdateHandler(sender, flow, "v1.2.0")

	h.HandleCallback(&tgbotapi.CallbackQuery{Data: "update:run"})

	flow.mu.Lock()
	defer flow.mu.Unlock()
	if flow.started {
		t.Error("an inline callback without a chat has nowhere to report to")
	}
}
