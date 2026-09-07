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

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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
	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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

	err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "")
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

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir, ""); err != nil {
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

	filesDir := writeFilesDir(t, tmpDir)

	sender := &mockSender{}
	// No version to compare against: the failure is reported, as before.
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, ""); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
	}
	if _, err := os.Stat(filesDir); !os.IsNotExist(err) {
		t.Error("the payload of a failed attempt must not stay in tmpfs; a retry downloads it again")
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
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, ""); err != nil {
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
	if err := CheckAndSendNotify(sender, &mockStore{}, notifyFile, tmpDir, ""); err != nil {
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

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir, ""); err != nil {
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
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, ""); err == nil {
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

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir, ""); err == nil {
		t.Fatal("CheckAndSendNotify() must report a store failure")
	}
	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want none", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); err != nil {
		t.Error("the notification must survive a store failure and be retried")
	}
}

// TestCheckAndSendNotify_DeduplicatesByChatID: chatstore keys users by
// lowercased username, so a user who changed their Telegram handle appears
// twice with the same ChatID and used to get the same message twice.
func TestCheckAndSendNotify_DeduplicatesByChatID(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`)

	sender := &mockSender{}
	store := &mockStore{users: []chatstore.UserChat{
		{Username: "oldhandle", ChatID: 42},
		{Username: "newhandle", ChatID: 42},
		{Username: "someone", ChatID: 43},
	}}

	if err := CheckAndSendNotify(sender, store, notifyFile, tmpDir, ""); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 2 {
		t.Fatalf("sent %d messages, want 2 (one per chat): %+v", len(sender.sentMessages), sender.sentMessages)
	}
	perChat := map[int64]int{}
	for _, msg := range sender.sentMessages {
		perChat[msg.chatID]++
	}
	if perChat[42] != 1 || perChat[43] != 1 {
		t.Errorf("messages per chat = %v, want one each for 42 and 43", perChat)
	}
}

func writeNotify(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestCheckAndSendNotify_DropsAFailureTheRunningVersionOutlived(t *testing.T) {
	// A failed update leaves notify.json for the next bot start. When that
	// start is a different build - install.sh was re-run, or a later update
	// landed - the failure it describes has been overtaken, and reporting it
	// says something is broken when nothing is.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	logFile := filepath.Join(tmpDir, "update.log")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.0","new_version":"v0.11.1","status":"failed","initiator":"webui"}`)
	if err := os.WriteFile(logFile, []byte("ERROR: pgrep not found\n"), 0644); err != nil {
		t.Fatal(err)
	}
	filesDir := writeFilesDir(t, tmpDir)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.2"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 0 {
		t.Errorf("sent %d messages, want none: the failure was overtaken", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify.json must go, or the stale report returns on every restart")
	}
	if _, err := os.Stat(filesDir); !os.IsNotExist(err) {
		t.Error("the payload of the overtaken attempt must not stay in tmpfs")
	}
	if _, err := os.Stat(logFile); err != nil {
		t.Errorf("update.log must survive for diagnosis: %v", err)
	}
}

func TestCheckAndSendNotify_ReportsAFailureOfTheRunningVersion(t *testing.T) {
	// The daemon that failed is the one running now, so the report stands.
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.0","new_version":"v0.11.1","status":"failed","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.0"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1", len(sender.sentMessages))
	}
}

// writeFilesDir creates the download directory an update leaves behind.
func writeFilesDir(t *testing.T, updateDir string) string {
	t.Helper()
	filesDir := filepath.Join(updateDir, "files")
	if err := os.MkdirAll(filesDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(filesDir, "telegram-bot"), []byte("binary"), 0755); err != nil {
		t.Fatal(err)
	}
	return filesDir
}

// update_script.sh.tmpl copies both binaries into place before it starts
// anything (its step 4), so a daemon that fails to start in step 6 leaves this
// process running NewVersion while the Web UI is down. Differing from
// OldVersion does not make that failure old - it is precisely the failure of
// the build now running, and step 6 goes to the trouble of rewriting
// notify.json before starting the bot so that it gets reported.
func TestCheckAndSendNotify_ReportsAFailureThatLeftTheNewBinariesRunning(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.2","new_version":"v0.11.3","status":"failed","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.3"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1: the Web UI is down and this build is the one that broke it",
			len(sender.sentMessages))
	}
}

// updateflow.Start creates the lock before it downloads a byte, so a lock
// beside the payload can mean an attempt is filling files/ right now, and
// removing it would pull 16 MB out from under that attempt. It can equally be
// the failed script's own - that script starts the daemons back up before it
// drops its lock - and skipping then costs only the reclaimed space.
func TestCheckAndSendNotify_LeavesThePayloadOfAnAttemptThatHoldsTheLock(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.2","new_version":"v0.11.3","status":"failed","initiator":"webui"}`)
	filesDir := writeFilesDir(t, tmpDir)
	lockFile := filepath.Join(tmpDir, "lock")
	if err := os.WriteFile(lockFile, []byte("4242\n"), 0644); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.2"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1: the failure still has to be reported", len(sender.sentMessages))
	}
	if _, err := os.Stat(filesDir); err != nil {
		t.Errorf("files/ belongs to the attempt holding the lock, it must survive: %v", err)
	}
	// The lock is that attempt's, not ours to release: dropping it would let a
	// second update start beside the one already running.
	if data, err := os.ReadFile(lockFile); err != nil || string(data) != "4242\n" {
		t.Errorf("the attempt's lock reads %q (%v), want it untouched", data, err)
	}
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify.json is ours and must still go, or the report returns on every restart")
	}
}

// A claim, not a look. os.Stat cannot tell "no lock" from "a lock a moment
// from now": updateflow.Start creates its lock and starts filling files/ in
// the window between the look and os.RemoveAll, and the payload it has just
// downloaded goes with the one this notifier meant to clear - the retry the
// user started fails, and its error names a file that was there a moment ago.
// Creating the lock the way Start does closes the window; whoever loses that
// race is refused instead.
//
// A dangling symlink is the one state that tells a claim from a look apart
// without racing two goroutines: os.Stat follows it and reports nothing
// there, while link(2) refuses any name that already exists. What it stands in
// for is an attempt claiming the directory inside the window.
func TestCheckAndSendNotify_ClaimsTheDirectoryRatherThanLookingAtIt(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.2","new_version":"v0.11.3","status":"failed","initiator":"webui"}`)
	filesDir := writeFilesDir(t, tmpDir)
	if err := os.Symlink(filepath.Join(tmpDir, "no-such-target"), filepath.Join(tmpDir, "lock")); err != nil {
		t.Fatal(err)
	}

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.2"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1: the failure still has to be reported", len(sender.sentMessages))
	}
	if _, err := os.Stat(filesDir); err != nil {
		t.Errorf("the claim was refused, so files/ was not this notifier's to remove: %v", err)
	}
}

// The claim is held for the removal and no longer. Left behind it would cost
// more than the 16 MB it reclaims: the lock names this process, which is very
// much alive, so IsUpdateInProgress reads it as an update in flight and every
// attempt afterwards is refused with "already in progress" until the bot is
// restarted.
func TestCheckAndSendNotify_ReleasesTheClaimItTook(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.2","new_version":"v0.11.3","status":"failed","initiator":"webui"}`)
	filesDir := writeFilesDir(t, tmpDir)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.2"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if _, err := os.Stat(filesDir); !os.IsNotExist(err) {
		t.Errorf("files/ belongs to the failed attempt and must go: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "lock")); !os.IsNotExist(err) {
		t.Errorf("the notifier kept the claim it took: %v", err)
	}
}

// The two daemons need not be on one version. install.sh calls the Web UI
// optional and carries on when its download fails, and an update the Web UI
// starts writes its own version as old_version - so a bot on v0.11.3 can read
// a failed attempt from v0.11.2 to v0.11.4 and match neither end of it. That
// failure is the freshest thing on the router rather than an old one: the Web
// UI is down, and this report is the only word anyone gets.
func TestCheckAndSendNotify_ReportsAFailureOfAnAttemptFromAnotherDaemon(t *testing.T) {
	tmpDir := t.TempDir()
	notifyFile := filepath.Join(tmpDir, "notify.json")
	writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.2","new_version":"v0.11.4","status":"failed","initiator":"webui"}`)

	sender := &mockSender{}
	if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, "v0.11.3"); err != nil {
		t.Fatalf("CheckAndSendNotify() error = %v", err)
	}

	if len(sender.sentMessages) != 1 {
		t.Fatalf("sent %d messages, want 1: v0.11.3 does not outlive an attempt at v0.11.4", len(sender.sentMessages))
	}
	if _, err := os.Stat(notifyFile); !os.IsNotExist(err) {
		t.Error("notify.json must go once the report is out")
	}
}

// Overtaking is a comparison, and a version that does not parse compares to
// nothing. Neither a bot built outside a release nor a notify.json naming a tag
// this build cannot read may silence a failure: what cannot be dated is
// reported.
func TestCheckAndSendNotify_ReportsAFailureItCannotDate(t *testing.T) {
	cases := []struct {
		name       string
		running    string
		newVersion string
	}{
		{"the running build is not a release", "dev", "v0.11.1"},
		{"the attempt names something unparseable", "v0.99.0", "latest"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			notifyFile := filepath.Join(tmpDir, "notify.json")
			writeNotify(t, notifyFile, `{"chat_id":42,"old_version":"v0.11.0","new_version":"`+tc.newVersion+`","status":"failed","initiator":"webui"}`)

			sender := &mockSender{}
			if err := CheckAndSendNotify(sender, nil, notifyFile, tmpDir, tc.running); err != nil {
				t.Fatalf("CheckAndSendNotify() error = %v", err)
			}

			if len(sender.sentMessages) != 1 {
				t.Fatalf("sent %d messages, want 1: running %q against an attempt at %q dates nothing",
					len(sender.sentMessages), tc.running, tc.newVersion)
			}
		})
	}
}
