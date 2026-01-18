# Wizard UI Improvements Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Improve Telegram bot wizard UI to match console `configure.sh` — extended country list, emoji checkmarks, server numbering, dynamic grid.

**Architecture:** Modify `wizard.go` to add country data, update `wizard_handlers.go` to use new button formatting and grid logic.

**Tech Stack:** Go, telegram-bot-api v5

---

## Task 1: Add CountryNames map to wizard.go

**Files:**
- Modify: `telegram-bot/internal/wizard/wizard.go:1-9`
- Test: `telegram-bot/internal/wizard/wizard_test.go` (create)

**Step 1: Write the failing test**

Create file `telegram-bot/internal/wizard/wizard_test.go`:

```go
package wizard

import "testing"

func TestDefaultExclusions_Contains10Countries(t *testing.T) {
	expected := []string{"ru", "ua", "by", "kz", "de", "fr", "nl", "pl", "tr", "il"}
	if len(DefaultExclusions) != 10 {
		t.Errorf("expected 10 countries, got %d", len(DefaultExclusions))
	}
	for _, code := range expected {
		found := false
		for _, ex := range DefaultExclusions {
			if ex == code {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("missing country code: %s", code)
		}
	}
}

func TestCountryNames_HasAllCodes(t *testing.T) {
	for _, code := range DefaultExclusions {
		if _, ok := CountryNames[code]; !ok {
			t.Errorf("CountryNames missing entry for: %s", code)
		}
	}
}

func TestCountryNames_Values(t *testing.T) {
	tests := map[string]string{
		"ru": "Russia",
		"ua": "Ukraine",
		"de": "Germany",
		"il": "Israel",
	}
	for code, expected := range tests {
		if CountryNames[code] != expected {
			t.Errorf("CountryNames[%s] = %s, want %s", code, CountryNames[code], expected)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test ./internal/wizard/ -v -run "TestDefaultExclusions|TestCountryNames"`

Expected: FAIL — `DefaultExclusions` has only 4 items, `CountryNames` undefined

**Step 3: Write implementation**

Replace content of `telegram-bot/internal/wizard/wizard.go`:

```go
package wizard

// DefaultExclusions lists country codes available for exclusion from proxy
var DefaultExclusions = []string{
	"ru", "ua", "by", "kz", "de",
	"fr", "nl", "pl", "tr", "il",
}

// CountryNames maps country codes to human-readable names
var CountryNames = map[string]string{
	"ru": "Russia",
	"ua": "Ukraine",
	"by": "Belarus",
	"kz": "Kazakhstan",
	"de": "Germany",
	"fr": "France",
	"nl": "Netherlands",
	"pl": "Poland",
	"tr": "Turkey",
	"il": "Israel",
}

var RouteOptions = []string{
	"xray",
	"ovpnc1", "ovpnc2", "ovpnc3", "ovpnc4", "ovpnc5",
	"wgc1", "wgc2", "wgc3", "wgc4", "wgc5",
}
```

**Step 4: Run test to verify it passes**

Run: `cd telegram-bot && go test ./internal/wizard/ -v -run "TestDefaultExclusions|TestCountryNames"`

Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/wizard/wizard.go telegram-bot/internal/wizard/wizard_test.go
git commit -m "feat(telegram-bot): extend exclusions to 10 countries with names"
```

---

## Task 2: Add helper functions for UI formatting

**Files:**
- Modify: `telegram-bot/internal/bot/wizard_handlers.go` (add helper functions at end)
- Test: `telegram-bot/internal/bot/wizard_helpers_test.go` (create)

**Step 1: Write the failing test**

Create file `telegram-bot/internal/bot/wizard_helpers_test.go`:

```go
package bot

import "testing"

func TestGetServerGridColumns(t *testing.T) {
	tests := []struct {
		count    int
		expected int
	}{
		{1, 1},
		{5, 1},
		{6, 2},
		{10, 2},
		{11, 3},
		{57, 3},
		{100, 3},
	}
	for _, tt := range tests {
		got := getServerGridColumns(tt.count)
		if got != tt.expected {
			t.Errorf("getServerGridColumns(%d) = %d, want %d", tt.count, got, tt.expected)
		}
	}
}

func TestTruncateServerName(t *testing.T) {
	tests := []struct {
		name     string
		maxLen   int
		expected string
	}{
		{"Short", 20, "Short"},
		{"This is a very long server name", 15, "This is a ve..."},
		{"Exact15CharLen", 14, "Exact15CharLen"},
		{"Exact", 5, "Exact"},
		{"TooLong", 5, "To..."},
	}
	for _, tt := range tests {
		got := truncateServerName(tt.name, tt.maxLen)
		if got != tt.expected {
			t.Errorf("truncateServerName(%q, %d) = %q, want %q", tt.name, tt.maxLen, got, tt.expected)
		}
	}
}

func TestFormatExclusionButton(t *testing.T) {
	tests := []struct {
		code     string
		selected bool
		expected string
	}{
		{"ru", true, "✅ ru Russia"},
		{"ru", false, "🔲 ru Russia"},
		{"de", true, "✅ de Germany"},
		{"unknown", false, "🔲 unknown unknown"},
	}
	for _, tt := range tests {
		got := formatExclusionButton(tt.code, tt.selected)
		if got != tt.expected {
			t.Errorf("formatExclusionButton(%q, %v) = %q, want %q", tt.code, tt.selected, got, tt.expected)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `cd telegram-bot && go test ./internal/bot/ -v -run "TestGetServerGridColumns|TestTruncateServerName|TestFormatExclusionButton"`

Expected: FAIL — functions undefined

**Step 3: Write implementation**

Add to end of `telegram-bot/internal/bot/wizard_handlers.go` (before closing brace if any, or at end of file):

```go
// getServerGridColumns returns number of columns based on server count
func getServerGridColumns(count int) int {
	switch {
	case count <= 5:
		return 1
	case count <= 10:
		return 2
	default:
		return 3
	}
}

// truncateServerName truncates name to maxLen, adding "..." if truncated
func truncateServerName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	if maxLen <= 3 {
		return name[:maxLen]
	}
	return name[:maxLen-3] + "..."
}

// formatExclusionButton formats exclusion button text with emoji and country name
func formatExclusionButton(code string, selected bool) string {
	mark := "🔲"
	if selected {
		mark = "✅"
	}
	name := code
	if n, ok := wizard.CountryNames[code]; ok {
		name = n
	}
	return fmt.Sprintf("%s %s %s", mark, code, name)
}
```

**Step 4: Run test to verify it passes**

Run: `cd telegram-bot && go test ./internal/bot/ -v -run "TestGetServerGridColumns|TestTruncateServerName|TestFormatExclusionButton"`

Expected: PASS

**Step 5: Commit**

```bash
git add telegram-bot/internal/bot/wizard_handlers.go telegram-bot/internal/bot/wizard_helpers_test.go
git commit -m "feat(telegram-bot): add helper functions for wizard UI formatting"
```

---

## Task 3: Update sendServerSelection with numbering and dynamic grid

**Files:**
- Modify: `telegram-bot/internal/bot/wizard_handlers.go:125-160`

**Step 1: Update sendServerSelection function**

Replace the `sendServerSelection` function (lines 125-160) with:

```go
// Step 1: Server selection
func (b *Bot) sendServerSelection(chatID int64, _ *wizard.State) {
	vpnCfg, err := vpnconfig.LoadVPNDirectorConfig(scriptsDir + "/vpn-director.json")
	if err != nil {
		b.sendMessage(chatID, "Config load error")
		b.wizard.Clear(chatID)
		return
	}

	dataDir := vpnCfg.DataDir
	if dataDir == "" {
		dataDir = scriptsDir + "/data"
	}

	servers, err := vpnconfig.LoadServers(dataDir + "/servers.json")
	if err != nil || len(servers) == 0 {
		b.sendMessage(chatID, "No servers found. Use /import")
		b.wizard.Clear(chatID)
		return
	}

	cols := getServerGridColumns(len(servers))
	// Max button text length depends on columns (Telegram limits)
	maxNameLen := 30
	if cols == 2 {
		maxNameLen = 20
	} else if cols == 3 {
		maxNameLen = 14
	}

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for i, s := range servers {
		btnText := fmt.Sprintf("%d. %s", i+1, truncateServerName(s.Name, maxNameLen))
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, fmt.Sprintf("server:%d", i))
		row = append(row, btn)
		if len(row) == cols {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	msg := tgbotapi.NewMessage(chatID, fmt.Sprintf("Step 1/4: Select Xray server (%d available)", len(servers)))
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send server selection: %v", err)
	}
}
```

**Step 2: Run existing tests**

Run: `cd telegram-bot && go test ./internal/bot/ -v`

Expected: PASS (no functional change to test contracts)

**Step 3: Commit**

```bash
git add telegram-bot/internal/bot/wizard_handlers.go
git commit -m "feat(telegram-bot): add server numbering and dynamic grid to wizard"
```

---

## Task 4: Update sendExclusionsSelection with emoji and 5x2 grid

**Files:**
- Modify: `telegram-bot/internal/bot/wizard_handlers.go:162-206`

**Step 1: Update sendExclusionsSelection function**

Replace the `sendExclusionsSelection` function (lines 162-206) with:

```go
// Step 2: Exclusions selection
func (b *Bot) sendExclusionsSelection(chatID int64, state *wizard.State) {
	defaultExclusions := wizard.DefaultExclusions
	stateExclusions := state.GetExclusions()

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, ex := range defaultExclusions {
		btnText := formatExclusionButton(ex, stateExclusions[ex])
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, "excl:"+ex)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}

	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Done", "excl:done"),
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	var selected []string
	for k, v := range stateExclusions {
		if v {
			selected = append(selected, k)
		}
	}
	sort.Strings(selected)
	text := "Step 2/4: Exclude from proxy\n"
	if len(selected) > 0 {
		text += fmt.Sprintf("Selected: %s", strings.Join(selected, ", "))
	} else {
		text += "Selected: (none)"
	}

	msg := tgbotapi.NewMessage(chatID, text)
	msg.ReplyMarkup = tgbotapi.NewInlineKeyboardMarkup(rows...)
	if _, err := b.api.Send(msg); err != nil {
		log.Printf("[ERROR] Failed to send exclusions selection: %v", err)
	}
}
```

**Step 2: Run tests**

Run: `cd telegram-bot && go test ./internal/bot/ -v`

Expected: PASS

**Step 3: Commit**

```bash
git add telegram-bot/internal/bot/wizard_handlers.go
git commit -m "feat(telegram-bot): update exclusions UI with emoji checkmarks and 5x2 grid"
```

---

## Task 5: Update updateExclusionsMessage to match

**Files:**
- Modify: `telegram-bot/internal/bot/wizard_handlers.go:208-252`

**Step 1: Update updateExclusionsMessage function**

Replace the `updateExclusionsMessage` function (lines 208-252) with:

```go
func (b *Bot) updateExclusionsMessage(msg *tgbotapi.Message, state *wizard.State) {
	defaultExclusions := wizard.DefaultExclusions
	stateExclusions := state.GetExclusions()

	var rows [][]tgbotapi.InlineKeyboardButton
	var row []tgbotapi.InlineKeyboardButton
	for _, ex := range defaultExclusions {
		btnText := formatExclusionButton(ex, stateExclusions[ex])
		btn := tgbotapi.NewInlineKeyboardButtonData(btnText, "excl:"+ex)
		row = append(row, btn)
		if len(row) == 2 {
			rows = append(rows, row)
			row = nil
		}
	}
	if len(row) > 0 {
		rows = append(rows, row)
	}
	rows = append(rows, tgbotapi.NewInlineKeyboardRow(
		tgbotapi.NewInlineKeyboardButtonData("Done", "excl:done"),
		tgbotapi.NewInlineKeyboardButtonData("Cancel", "cancel"),
	))

	var selected []string
	for k, v := range stateExclusions {
		if v {
			selected = append(selected, k)
		}
	}
	sort.Strings(selected)
	text := "Step 2/4: Exclude from proxy\n"
	if len(selected) > 0 {
		text += fmt.Sprintf("Selected: %s", strings.Join(selected, ", "))
	} else {
		text += "Selected: (none)"
	}

	edit := tgbotapi.NewEditMessageTextAndMarkup(
		msg.Chat.ID, msg.MessageID, text,
		tgbotapi.NewInlineKeyboardMarkup(rows...),
	)
	if _, err := b.api.Send(edit); err != nil {
		log.Printf("[ERROR] Failed to update exclusions message: %v", err)
	}
}
```

**Step 2: Run tests**

Run: `cd telegram-bot && go test ./internal/bot/ -v`

Expected: PASS

**Step 3: Commit**

```bash
git add telegram-bot/internal/bot/wizard_handlers.go
git commit -m "feat(telegram-bot): update exclusions message update to match new UI"
```

---

## Task 6: Build and verify

**Step 1: Run all tests**

Run: `cd telegram-bot && go test ./... -v`

Expected: All PASS

**Step 2: Build binary**

Run: `cd telegram-bot && make build`

Expected: Binary created at `bin/telegram-bot`

**Step 3: Commit final state (if any uncommitted)**

```bash
git status
# If clean, skip. Otherwise:
git add -A && git commit -m "chore: cleanup after wizard UI improvements"
```

---

## Summary

| Task | Description |
|------|-------------|
| 1 | Add `CountryNames` map and extend `DefaultExclusions` to 10 countries |
| 2 | Add helper functions: `getServerGridColumns`, `truncateServerName`, `formatExclusionButton` |
| 3 | Update `sendServerSelection` with numbering and dynamic grid |
| 4 | Update `sendExclusionsSelection` with emoji checkmarks and 5x2 grid |
| 5 | Update `updateExclusionsMessage` to match new UI |
| 6 | Build and verify all tests pass |
