# Web UI Behavior Fixes (Block 1) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Web UI перестаёт обманывать пользователя: изменения применяются сразу, клиенты валидируются как в боте, туннели наследуют исключения, Update IPsets действительно обновляет ipset-ы, логи Xray существуют, заглушка Update честная.

**Architecture:** Общая валидация адресов уезжает в `vpnconfig` и используется ботом и Web UI. В `webapi` появляется хелпер `saveAndApply`, который после сохранения `vpn-director.json` вызывает `vpn-director.sh apply`, как делает бот. Интерфейс `service.VPNDirector` получает `Update()`. Шаблон Xray получает секцию `log`, пути логов приходят в `webapi` из `paths` через `Deps.LogPaths`.

**Tech Stack:** Go 1.25 (`net/http`, стандартный mux с method-паттернами), Vue 3.5 + TypeScript + Vite, bash + jq, Bats.

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, раздел 4 «Блок 1. Поведение Web UI». Спек лежит в этой же ветке (`feature/webui-behavior`), коммит 91f4579.

## Global Constraints

- Go-модуль `server/`, `go 1.25.5` в `go.mod`. Новые зависимости не добавляются.
- Тексты ошибок API берутся из спека дословно: `invalid IPv4 address or CIDR`, `client already configured for <route>`, `client not found`, `invalid country code: <значение>`, `configuration saved, but apply failed: <строка>`, `failed to save configuration`, `failed to update ipsets: <строка>`, `no VLESS servers found in subscription: <e1>; <e2>; <e3>`, `self-update via Web UI is not supported yet`, `not found`.
- `<строка>` в ошибках это последняя непустая строка вывода команды, не длиннее 200 символов.
- Коды стран хранятся в нижнем регистре и проверяются по `^[a-z]{2}$`.
- Адреса клиентов и исключений: только IPv4 или IPv4-CIDR, `/32` срезается.
- Путь error-лога Xray: `/tmp/xray-error.log` (dev: `testdata/dev/xray-error.log`).
- Коммиты на английском в стиле репозитория: `feat(webui): ...`, `fix(webapi): ...`, `test(...)`, `docs: ...`. Каждая задача заканчивается коммитом.
- В этом проекте сборки и тесты запускаются через skill `claude-forge:build` (агент build-runner), а не напрямую. Команды ниже показывают, что именно запускать.
- Все Go-команды выполняются из каталога `server/`. Bats-тесты требуют установленного bats с `/usr/lib/bats/bats-support` и `/usr/lib/bats/bats-assert` (как в `.github/workflows/ipset-sources.yml`).
- Дизайн-документы (`docs/superpowers/`) удаляются из ветки перед созданием PR (Task 13).

---

## Структура файлов

| Файл | Ответственность |
|------|-----------------|
| `server/internal/vpnconfig/validate.go` (новый) | `NormalizeClientAddr`: единственный валидатор адресов клиентов и exclude-IP |
| `server/internal/webapi/errline.go` (новый) | `lastErrorLine`: последняя строка вывода команды для текста ошибки |
| `server/internal/webapi/apply.go` (новый) | `saveAndApply` и `writeSaveApplyResult`: сохранить, применить, ответить |
| `server/internal/webapi/handler_clients.go` | клиенты: нормализация, 409, наследование exclude, 404, авто-apply |
| `server/internal/webapi/handler_excludes.go` | исключения: коды стран, нормализация IP, авто-apply |
| `server/internal/webapi/handler_status.go` | `handleUpdateIPsets` через `VPN.Update()` |
| `server/internal/webapi/handler_servers.go` | ошибки разбора подписки в ответе импорта |
| `server/internal/webapi/handler_logs.go` | источники логов из `Deps.LogPaths`, заглушка Update 501 |
| `server/internal/webapi/router.go` | `Deps.LogPaths`, JSON 404 под `/api/`, заголовки кеша SPA |
| `server/internal/service/{interfaces,vpndirector}.go` | `VPNDirector.Update()` |
| `server/internal/paths/paths.go` | `XrayLogPath` |
| `server/internal/handler/clients.go`, `server/internal/wizard/exclude_ips.go` | бот переходит на `vpnconfig.NormalizeClientAddr` |
| `server/internal/handler/misc.go` | источник `xray` в `/logs` |
| `server/cmd/webui/main.go`, `server/cmd/bot/main.go` | карта логов для Web UI, ротация error-лога Xray в боте |
| `router/opt/etc/xray/config.json.template`, `server/testdata/dev/xray.template.json` | секция `log` |
| `web/src/components/{StatusTab,ClientsTab,ExclusionsTab}.vue` | независимая загрузка статуса, обработка `saved`, нижний регистр кодов |

---

### Task 1: Общий валидатор адресов `vpnconfig.NormalizeClientAddr`

**Files:**
- Create: `server/internal/vpnconfig/validate.go`
- Test: `server/internal/vpnconfig/validate_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: `func NormalizeClientAddr(s string) (string, error)` и `var ErrInvalidClientAddr = errors.New("invalid IPv4 address or CIDR")`. Все последующие задачи используют именно эти имена.

- [ ] **Step 1: Написать падающий тест**

Создать `server/internal/vpnconfig/validate_test.go`:

```go
package vpnconfig

import (
	"errors"
	"testing"
)

func TestNormalizeClientAddr(t *testing.T) {
	tests := []struct {
		input string
		want  string
		ok    bool
	}{
		{"192.168.50.10", "192.168.50.10", true},
		{"  192.168.50.10  ", "192.168.50.10", true},
		{"192.168.50.10/32", "192.168.50.10", true},
		{"192.168.50.0/24", "192.168.50.0/24", true},
		{"10.0.0.0/8", "10.0.0.0/8", true},
		{"", "", false},
		{"not-an-ip", "", false},
		{"256.1.1.1", "", false},
		{"1.2.3.4/33", "", false},
		{"1.2.3.4/", "", false},
		{"::1", "", false},
		{"::1/128", "", false},
		{"2001:db8::/32", "", false},
		{"::ffff:1.2.3.4", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := NormalizeClientAddr(tt.input)
			if !tt.ok {
				if err == nil {
					t.Fatalf("NormalizeClientAddr(%q) = %q, want error", tt.input, got)
				}
				if !errors.Is(err, ErrInvalidClientAddr) {
					t.Errorf("error = %v, want ErrInvalidClientAddr", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeClientAddr(%q) error = %v, want nil", tt.input, err)
			}
			if got != tt.want {
				t.Errorf("NormalizeClientAddr(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Убедиться, что тест падает**

Run: `cd server && go test ./internal/vpnconfig/ -run TestNormalizeClientAddr -v`
Expected: FAIL, `undefined: NormalizeClientAddr`.

- [ ] **Step 3: Реализация**

Создать `server/internal/vpnconfig/validate.go`:

```go
package vpnconfig

import (
	"errors"
	"net"
	"strings"
)

// ErrInvalidClientAddr is returned by NormalizeClientAddr for anything that
// is not an IPv4 address or an IPv4 CIDR.
var ErrInvalidClientAddr = errors.New("invalid IPv4 address or CIDR")

// NormalizeClientAddr trims s, accepts an IPv4 address or an IPv4 CIDR,
// strips a trailing /32 and returns the canonical string. It is the single
// validator for LAN client addresses and exclude IPs, shared by the bot and
// the Web UI so both persist the same form. Router ipsets are IPv4-only, so
// every IPv6 spelling (including IPv4-mapped) is rejected.
func NormalizeClientAddr(s string) (string, error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, ":") {
		return "", ErrInvalidClientAddr
	}
	if strings.Contains(s, "/") {
		ip, _, err := net.ParseCIDR(s)
		if err != nil || ip.To4() == nil {
			return "", ErrInvalidClientAddr
		}
		return strings.TrimSuffix(s, "/32"), nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return "", ErrInvalidClientAddr
	}
	return ip.To4().String(), nil
}
```

- [ ] **Step 4: Убедиться, что тест проходит**

Run: `cd server && go test ./internal/vpnconfig/ -v`
Expected: PASS, все подтесты `TestNormalizeClientAddr` зелёные, старые тесты пакета не тронуты.

- [ ] **Step 5: Commit**

```bash
git add server/internal/vpnconfig/validate.go server/internal/vpnconfig/validate_test.go
git commit -m "feat(vpnconfig): add NormalizeClientAddr shared IPv4/CIDR validator"
```

---

### Task 2: Бот переходит на общий валидатор

**Files:**
- Modify: `server/internal/handler/clients.go:296-330` (HandleTextInput), `:372-375` (handleAddRoute), `:404-419` (удалить локальные функции), импорты
- Modify: `server/internal/wizard/exclude_ips.go:120-131`, импорты
- Test: существующие `server/internal/handler/clients_test.go`, `server/internal/wizard/exclude_ips_test.go`

**Interfaces:**
- Consumes: `vpnconfig.NormalizeClientAddr` из Task 1.
- Produces: `wizard.IsValidIPOrCIDR(s string) bool` сохраняет имя и семантику, но делегирует общей функции.

- [ ] **Step 1: Запустить существующие тесты бота как базовую линию**

Run: `cd server && go test ./internal/handler/ ./internal/wizard/`
Expected: PASS (фиксируем, что до правок всё зелёное).

- [ ] **Step 2: Заменить валидацию в `handler/clients.go`**

В `HandleTextInput` заменить блок

```go
	if !isValidIPOrCIDR(input) {
		h.deps.Sender.SendPlain(chatID, "Invalid format. Enter IPv4 (192.168.50.10) or CIDR (192.168.50.0/24):")
		return
	}

	// Normalize for duplicate check
	normalized := normalizeIP(input)
	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	clients := vpnconfig.CollectClients(cfg)
	for _, c := range clients {
		if normalizeIP(c.IP) == normalized {
			h.deps.Sender.SendPlain(chatID, fmt.Sprintf("This IP is already configured for %s", c.Route))
			return
		}
	}

	// Save pending IP and show route selection
	h.mu.Lock()
	h.addState[chatID] = input
	h.mu.Unlock()

	h.showRouteSelection(chatID, input, cfg)
```

на

```go
	normalized, err := vpnconfig.NormalizeClientAddr(input)
	if err != nil {
		h.deps.Sender.SendPlain(chatID, "Invalid format. Enter IPv4 (192.168.50.10) or CIDR (192.168.50.0/24):")
		return
	}

	cfg, err := h.deps.Config.LoadVPNConfig()
	if err != nil {
		h.deps.Sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		return
	}

	// Compare normalized forms: older configs store both 1.2.3.4 and 1.2.3.4/32.
	clients := vpnconfig.CollectClients(cfg)
	for _, c := range clients {
		existing, err := vpnconfig.NormalizeClientAddr(c.IP)
		if err != nil {
			existing = c.IP
		}
		if existing == normalized {
			h.deps.Sender.SendPlain(chatID, fmt.Sprintf("This IP is already configured for %s", c.Route))
			return
		}
	}

	// Save pending IP (normalized) and show route selection
	h.mu.Lock()
	h.addState[chatID] = normalized
	h.mu.Unlock()

	h.showRouteSelection(chatID, normalized, cfg)
```

В `handleAddRoute` заменить

```go
	// Normalize IP: strip /32 for consistent storage
	ip = normalizeIP(ip)
```

на

```go
	// Normalize IP: strip /32 for consistent storage. The pending state already
	// holds the normalized form; this keeps older pending entries consistent.
	if n, err := vpnconfig.NormalizeClientAddr(ip); err == nil {
		ip = n
	}
```

Удалить целиком функции `isValidIPOrCIDR` и `normalizeIP` в конце файла (строки 404–419) и убрать `"net"` из импортов: после удаления пакет `net` в файле не используется.

- [ ] **Step 3: Заменить валидацию в `wizard/exclude_ips.go`**

Заменить функцию

```go
// IsValidIPOrCIDR validates input as IPv4 or IPv4 CIDR
func IsValidIPOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		ip, _, err := net.ParseCIDR(s)
		if err != nil {
			return false
		}
		return ip.To4() != nil
	}
	ip := net.ParseIP(s)
	return ip != nil && ip.To4() != nil
}
```

на

```go
// IsValidIPOrCIDR reports whether s is an IPv4 address or IPv4 CIDR.
// Validation lives in vpnconfig.NormalizeClientAddr so the bot and the
// Web UI agree on what they accept.
func IsValidIPOrCIDR(s string) bool {
	_, err := vpnconfig.NormalizeClientAddr(s)
	return err == nil
}
```

В блоке импортов заменить строку `"net"` на `"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"` (сейчас файл импортирует `fmt`, `net`, `strings`, `tgbotapi`, `telegram`; `strings` остаётся нужным для `HandleMessage`).

- [ ] **Step 4: Собрать и прогнать тесты**

Run: `cd server && go build ./... && go vet ./internal/handler/ ./internal/wizard/ && go test ./internal/handler/ ./internal/wizard/`
Expected: сборка без ошибок (в том числе без `imported and not used`), тесты PASS, включая `TestIsValidIPOrCIDR` и `TestClientsHandler_*`.

- [ ] **Step 5: Commit**

```bash
git add server/internal/handler/clients.go server/internal/wizard/exclude_ips.go
git commit -m "refactor(bot): use vpnconfig.NormalizeClientAddr for client and exclude IP validation"
```

---

### Task 3: `VPNDirector.Update()` и честный Update IPsets

**Files:**
- Modify: `server/internal/service/interfaces.go:32-39`
- Modify: `server/internal/service/vpndirector.go` (добавить метод)
- Test: `server/internal/service/vpndirector_test.go`
- Create: `server/internal/webapi/errline.go`
- Test: `server/internal/webapi/errline_test.go`
- Modify: `server/internal/webapi/test_helpers_test.go:12-22` (`mockVPN`)
- Modify: `server/internal/handler/status_test.go:20-24`, `server/internal/handler/xray_test.go:306-310`, `server/internal/handler/clients_test.go:73-77`, `server/internal/wizard/apply_test.go:19-29` (моки `VPNDirector`)
- Modify: `server/internal/webapi/handler_status.go:61-76`
- Test: `server/internal/webapi/handler_status_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: метод интерфейса `service.VPNDirector.Update() error`; `func lastErrorLine(err error) string` в пакете `webapi`; поля `applyCalls int` и `updateCalls int` у `mockVPN` в тестах `webapi`.

- [ ] **Step 1: Падающие тесты сервиса**

Добавить в `server/internal/service/vpndirector_test.go`:

```go
func TestVPNDirectorService_Update(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "updated", ExitCode: 0}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Update()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if len(mock.calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.calls))
	}
	// Should call: vpn-director.sh update (force-refreshes ipsets, unlike apply)
	if mock.calls[0][1] != "update" {
		t.Errorf("wrong args: %v", mock.calls[0])
	}
}

func TestVPNDirectorService_UpdateNonZeroExit(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "download failed", ExitCode: 1}}
	svc := NewVPNDirectorService("/opt/vpn-director", mock)

	err := svc.Update()
	if err == nil {
		t.Fatal("expected error for non-zero exit, got nil")
	}
	if !strings.Contains(err.Error(), "download failed") {
		t.Errorf("error should carry script output, got %q", err.Error())
	}
}
```

Добавить `"strings"` в импорты тестового файла.

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `cd server && go test ./internal/service/ -run TestVPNDirectorService_Update -v`
Expected: FAIL, `svc.Update undefined`.

- [ ] **Step 3: Интерфейс и реализация**

В `server/internal/service/interfaces.go` заменить

```go
// VPNDirector is the interface for VPN Director operations
type VPNDirector interface {
	Status() (string, error)
	Apply() error
	Restart() error
	RestartXray() error
	Stop() error
}
```

на

```go
// VPNDirector is the interface for VPN Director operations
type VPNDirector interface {
	Status() (string, error)
	Apply() error
	Restart() error
	RestartXray() error
	Stop() error
	// Update downloads fresh ipsets and reapplies the configuration
	// (vpn-director.sh update). Apply reuses cached ipsets instead.
	Update() error
}
```

В `server/internal/service/vpndirector.go` после `Stop` добавить:

```go
// Update downloads fresh ipsets and reapplies VPN Director configuration
func (s *VPNDirectorService) Update() error {
	result, err := s.executor.Exec(s.scriptPath(), "update")
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("update failed (exit %d): %s", result.ExitCode, result.Output)
	}
	return nil
}
```

- [ ] **Step 4: Обновить все моки `VPNDirector`**

`server/internal/webapi/test_helpers_test.go`, заменить определение `mockVPN`:

```go
// mockVPN implements service.VPNDirector for testing.
type mockVPN struct {
	statusOutput string
	err          error
	applyCalls   int // number of Apply() calls, to assert auto-apply
	updateCalls  int // number of Update() calls
}

func (m *mockVPN) Status() (string, error) { return m.statusOutput, m.err }
func (m *mockVPN) Apply() error            { m.applyCalls++; return m.err }
func (m *mockVPN) Restart() error          { return m.err }
func (m *mockVPN) RestartXray() error      { return m.err }
func (m *mockVPN) Stop() error             { return m.err }
func (m *mockVPN) Update() error           { m.updateCalls++; return m.err }
```

`server/internal/handler/status_test.go`, после `func (m *mockVPNDirector) Stop() error { return m.stopErr }` добавить:

```go
func (m *mockVPNDirector) Update() error            { return nil }
```

`server/internal/handler/xray_test.go`, после `Stop`:

```go
func (m *mockVPNDirectorWithXray) Update() error           { return nil }
```

`server/internal/handler/clients_test.go`, после `Stop`:

```go
func (m *mockVPNClients) Update() error           { return nil }
```

`server/internal/wizard/apply_test.go`, после `Stop`:

```go
func (m *mockVPNDirector) Update() error { return nil }
```

- [ ] **Step 5: Собрать всё**

Run: `cd server && go build ./... && go vet ./...`
Expected: без ошибок. Если `go vet` ругается на неиспользуемые импорты в тестах, поправить только их.

- [ ] **Step 6: Падающий тест `lastErrorLine`**

Создать `server/internal/webapi/errline_test.go`:

```go
package webapi

import (
	"errors"
	"strings"
	"testing"
)

func TestLastErrorLine(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, ""},
		{"single line", errors.New("apply failed (exit 1): boom"), "apply failed (exit 1): boom"},
		{"last non-empty line wins", errors.New("apply failed (exit 1): [INFO] step\n[ERROR] Invalid country code 'xx'\n\n"), "[ERROR] Invalid country code 'xx'"},
		{"trims spaces", errors.New("x\n   padded   \n"), "padded"},
		{"caps at 200 runes", errors.New(strings.Repeat("я", 250)), strings.Repeat("я", 200)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := lastErrorLine(tt.err); got != tt.want {
				t.Errorf("lastErrorLine() = %q, want %q", got, tt.want)
			}
		})
	}
}
```

- [ ] **Step 7: Реализация `lastErrorLine`**

Создать `server/internal/webapi/errline.go`:

```go
package webapi

import "strings"

// maxErrorLineRunes caps the shell output line echoed in API error messages.
const maxErrorLineRunes = 200

// lastErrorLine returns the last non-empty line of err's message, trimmed
// and capped at maxErrorLineRunes. Shell failures carry the whole script
// output; the last line is the ERROR line the user needs to see.
func lastErrorLine(err error) string {
	if err == nil {
		return ""
	}
	lines := strings.Split(err.Error(), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		if r := []rune(line); len(r) > maxErrorLineRunes {
			line = string(r[:maxErrorLineRunes])
		}
		return line
	}
	return ""
}
```

Run: `cd server && go test ./internal/webapi/ -run TestLastErrorLine -v`
Expected: PASS.

- [ ] **Step 8: Падающие тесты обработчика Update IPsets**

В `server/internal/webapi/handler_status_test.go` заменить `TestHandleUpdateIPsets_OK` на:

```go
func TestHandleUpdateIPsets_CallsUpdate(t *testing.T) {
	deps := newTestDeps(t)
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleUpdateIPsets(deps)

	req := httptest.NewRequest("POST", "/api/ipsets/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.updateCalls != 1 {
		t.Errorf("expected 1 Update() call, got %d", vpn.updateCalls)
	}
	if vpn.applyCalls != 0 {
		t.Errorf("expected no Apply() call, got %d", vpn.applyCalls)
	}

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestHandleUpdateIPsets_Error(t *testing.T) {
	deps := newTestDeps(t)
	deps.VPN = &mockVPN{err: errors.New("update failed (exit 1): [INFO] downloading\n[ERROR] no network\n")}

	handler := handleUpdateIPsets(deps)

	req := httptest.NewRequest("POST", "/api/ipsets/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to update ipsets: [ERROR] no network" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}
```

Run: `cd server && go test ./internal/webapi/ -run TestHandleUpdateIPsets -v`
Expected: FAIL, `TestHandleUpdateIPsets_CallsUpdate` видит `applyCalls == 1` и `updateCalls == 0`.

- [ ] **Step 9: Обработчик через `Update()`**

В `server/internal/webapi/handler_status.go` заменить `handleUpdateIPsets` вместе с комментарием-TODO на:

```go
// handleUpdateIPsets returns a handler that downloads fresh ipsets and
// reapplies the configuration via `vpn-director.sh update`.
func handleUpdateIPsets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		if err := deps.VPN.Update(); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to update ipsets: "+lastErrorLine(err))
			return
		}
		jsonOK(w, map[string]bool{"ok": true})
	}
}
```

Run: `cd server && go test ./internal/webapi/ ./internal/service/`
Expected: PASS.

- [ ] **Step 10: Commit**

```bash
git add server/internal/service/interfaces.go server/internal/service/vpndirector.go server/internal/service/vpndirector_test.go \
  server/internal/webapi/errline.go server/internal/webapi/errline_test.go \
  server/internal/webapi/test_helpers_test.go server/internal/webapi/handler_status.go server/internal/webapi/handler_status_test.go \
  server/internal/handler/status_test.go server/internal/handler/xray_test.go server/internal/handler/clients_test.go server/internal/wizard/apply_test.go
git commit -m "feat(webapi): run vpn-director.sh update behind POST /api/ipsets/update"
```

---

### Task 4: Хелпер `saveAndApply`

**Files:**
- Create: `server/internal/webapi/apply.go`
- Test: `server/internal/webapi/apply_test.go`

**Interfaces:**
- Consumes: `lastErrorLine` из Task 3, `mockVPN.applyCalls`, `mockConfig.saveVPNCfgErr` и `mockConfig.savedCfg` из тестовых хелперов.
- Produces: `func saveAndApply(deps *Deps, cfg *vpnconfig.VPNDirectorConfig) error`, `func writeSaveApplyResult(w http.ResponseWriter, err error)`, тип `*errSavedNotApplied`.

- [ ] **Step 1: Падающие тесты**

Создать `server/internal/webapi/apply_test.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

func TestSaveAndApply_OK(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, mc.cfg))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call after save, got %d", vpn.applyCalls)
	}
	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["ok"] {
		t.Error("expected ok: true")
	}
}

func TestSaveAndApply_SaveError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}, saveVPNCfgErr: errors.New("disk full")}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, &vpnconfig.VPNDirectorConfig{}))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run when save failed, got %d calls", vpn.applyCalls)
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "failed to save configuration" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if _, has := resp["saved"]; has {
		t.Error("saved must be absent when nothing was saved")
	}
}

func TestSaveAndApply_ApplyError(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [INFO] ensuring ipsets\n[ERROR] Invalid country code 'xx'\n")}

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, saveAndApply(deps, mc.cfg))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved before apply")
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	want := "configuration saved, but apply failed: [ERROR] Invalid country code 'xx'"
	if resp["error"] != want {
		t.Errorf("error = %v, want %q", resp["error"], want)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `cd server && go test ./internal/webapi/ -run TestSaveAndApply -v`
Expected: FAIL, `undefined: saveAndApply`, `undefined: writeSaveApplyResult`.

- [ ] **Step 3: Реализация**

Создать `server/internal/webapi/apply.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// errSavedNotApplied marks a saveAndApply failure where vpn-director.json was
// written but `vpn-director.sh apply` failed. The client must learn that the
// change is on disk and only the apply needs a retry.
type errSavedNotApplied struct{ cause error }

func (e *errSavedNotApplied) Error() string { return "saved but not applied: " + e.cause.Error() }
func (e *errSavedNotApplied) Unwrap() error { return e.cause }

// saveAndApply persists cfg and applies it, mirroring the bot which runs
// `vpn-director.sh apply` after every config mutation so the config on disk
// always matches the kernel state. A save failure is returned as-is; an
// apply failure is wrapped in *errSavedNotApplied.
func saveAndApply(deps *Deps, cfg *vpnconfig.VPNDirectorConfig) error {
	if err := deps.Config.SaveVPNConfig(cfg); err != nil {
		return err
	}
	if err := deps.VPN.Apply(); err != nil {
		return &errSavedNotApplied{cause: err}
	}
	return nil
}

// writeSaveApplyResult maps a saveAndApply result to the HTTP response:
// 200 {"ok":true}; 500 "failed to save configuration"; or 500 with
// "saved": true and the last line of the apply output when only the apply
// failed, so the UI can refresh the list and offer a retry.
func writeSaveApplyResult(w http.ResponseWriter, err error) {
	if err == nil {
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	var notApplied *errSavedNotApplied
	if errors.As(err, &notApplied) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("configuration saved, but apply failed: %s", lastErrorLine(notApplied.cause)),
			"saved": true,
		})
		return
	}
	jsonError(w, http.StatusInternalServerError, "failed to save configuration")
}
```

- [ ] **Step 4: Убедиться, что тесты проходят**

Run: `cd server && go test ./internal/webapi/ -run TestSaveAndApply -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add server/internal/webapi/apply.go server/internal/webapi/apply_test.go
git commit -m "feat(webapi): add saveAndApply helper that applies config after every save"
```

---

### Task 5: Обработчики клиентов

**Files:**
- Modify: `server/internal/webapi/handler_clients.go` (полная замена)
- Test: `server/internal/webapi/handler_clients_test.go`

**Interfaces:**
- Consumes: `vpnconfig.NormalizeClientAddr`, `saveAndApply`, `writeSaveApplyResult`, `mockVPN.applyCalls`.
- Produces: хелперы пакета `webapi`, которые Task 6 использует повторно: `func containsAddr(slice []string, addr string) bool`, `func removeAddr(slice []string, addr string) []string`. Также `func findClient(cfg *vpnconfig.VPNDirectorConfig, addr string) (clientMatch, bool)` и `func clientAddrFromQuery(w http.ResponseWriter, r *http.Request) (string, bool)`. Функции `contains` и `removeString` сохраняются.

- [ ] **Step 1: Обновить существующие тесты под новый контракт**

В `server/internal/webapi/handler_clients_test.go`:

Заменить `TestHandleAddClient_XrayDuplicate` на:

```go
func TestHandleAddClient_XrayDuplicate(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{
				Clients: []string{"192.168.50.10"},
			},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for xray" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved on conflict")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run on conflict, got %d calls", vpn.applyCalls)
	}
}
```

Заменить `TestHandleAddClient_TunnelRoute` на:

```go
func TestHandleAddClient_TunnelRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{ExcludeSets: []string{"ru", "us"}},
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.30", "route": "wgc1"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	tunnel, ok := mc.savedCfg.TunnelDirector.Tunnels["wgc1"]
	if !ok {
		t.Fatal("expected wgc1 tunnel to be created")
	}
	if len(tunnel.Clients) != 1 || tunnel.Clients[0] != "192.168.50.30" {
		t.Errorf("expected tunnel client [192.168.50.30], got %v", tunnel.Clients)
	}
	// A new tunnel inherits the Xray country exclusions, like the bot wizard.
	if strings.Join(tunnel.Exclude, ",") != "ru,us" {
		t.Errorf("expected tunnel exclude [ru us], got %v", tunnel.Exclude)
	}
	// The copy must not alias xray.exclude_sets.
	tunnel.Exclude[0] = "changed"
	if mc.savedCfg.Xray.ExcludeSets[0] != "ru" {
		t.Error("tunnel exclude must be a copy of xray.exclude_sets, not the same slice")
	}
}
```

В `TestHandleAddClient_TunnelRouteNilTunnels` после проверки `len(tunnel.Clients) != 1` добавить:

```go
	if tunnel.Exclude == nil {
		t.Error("expected non-nil exclude slice so it marshals to [] rather than null")
	}
```

Заменить `TestHandlePauseClient_OK` на:

```go
func TestHandlePauseClient_OK(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray:          vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},
			PausedClients: []string{},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}

	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved")
	}
	if len(mc.savedCfg.PausedClients) != 1 || mc.savedCfg.PausedClients[0] != "192.168.50.10" {
		t.Errorf("expected PausedClients=[192.168.50.10], got %v", mc.savedCfg.PausedClients)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}
```

В `TestHandlePauseClient_AlreadyPaused` в `cfg` добавить `Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}},` перед `PausedClients`.

В `TestHandleResumeClient_OK` в `cfg` добавить `Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10", "192.168.50.20"}},` перед `PausedClients`.

- [ ] **Step 2: Добавить новые тесты**

Добавить в конец `server/internal/webapi/handler_clients_test.go`:

```go
func TestHandleAddClient_ConflictOtherRoute(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.10/32"}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.10", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for wgc1" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleAddClient_StripsSlash32(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.40/32", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.Clients, ","); got != "192.168.50.40" {
		t.Errorf("expected /32 stripped, got %q", got)
	}
}

func TestHandleAddClient_RejectsIPv6(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleAddClient(deps)

	body := `{"ip": "::1", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid IPv4 address or CIDR" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleAddClient_AppliesAfterSave(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.50", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call after save, got %d", vpn.applyCalls)
	}
}

func TestHandleAddClient_SavedButApplyFailed(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [ERROR] iptables missing")}

	handler := handleAddClient(deps)

	body := `{"ip": "192.168.50.50", "route": "xray"}`
	req := httptest.NewRequest("POST", "/api/clients", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg == nil {
		t.Fatal("expected config to be saved even though apply failed")
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	if resp["error"] != "configuration saved, but apply failed: [ERROR] iptables missing" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
}

func TestHandlePauseClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.99", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved for an unknown client")
	}
}

func TestHandlePauseClient_InvalidIP(t *testing.T) {
	deps := newTestDeps(t)

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=::1", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandlePauseClient_KeepsStoredForm(t *testing.T) {
	// The shell subtracts paused_clients from the clients arrays by exact
	// string, so the paused entry must use the stored spelling (here with /32).
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}, Exclude: []string{"ru"}},
				},
			},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handlePauseClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/pause?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.PausedClients, ","); got != "192.168.50.20/32" {
		t.Errorf("expected paused entry in stored form 192.168.50.20/32, got %q", got)
	}
}

func TestHandleResumeClient_MatchesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32"}, Exclude: []string{"ru"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(mc.savedCfg.PausedClients) != 0 {
		t.Errorf("expected paused list empty, got %v", mc.savedCfg.PausedClients)
	}
}

func TestHandleResumeClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{PausedClients: []string{"192.168.50.10"}}}

	handler := handleResumeClient(deps)

	req := httptest.NewRequest("POST", "/api/clients/resume?ip=192.168.50.10", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a paused entry that is no longer a client, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteClient_NotFound(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients?ip=192.168.50.99", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleDeleteClient_RemovesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			TunnelDirector: vpnconfig.TunnelDirectorConfig{
				Tunnels: map[string]vpnconfig.TunnelConfig{
					"wgc1": {Clients: []string{"192.168.50.20/32", "192.168.50.30"}, Exclude: []string{"ru"}},
				},
			},
			PausedClients: []string{"192.168.50.20/32"},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleDeleteClient(deps)

	req := httptest.NewRequest("DELETE", "/api/clients?ip=192.168.50.20", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.TunnelDirector.Tunnels["wgc1"].Clients, ","); got != "192.168.50.30" {
		t.Errorf("expected wgc1 clients [192.168.50.30], got %q", got)
	}
	if len(mc.savedCfg.PausedClients) != 0 {
		t.Errorf("expected paused list cleared, got %v", mc.savedCfg.PausedClients)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestRemoveAddr(t *testing.T) {
	got := removeAddr([]string{"192.168.50.20/32", "192.168.50.30", "192.168.50.20", "garbage"}, "192.168.50.20")
	if strings.Join(got, ",") != "192.168.50.30,garbage" {
		t.Errorf("removeAddr = %v, want [192.168.50.30 garbage]", got)
	}
	if !containsAddr([]string{"1.2.3.4/32"}, "1.2.3.4") {
		t.Error("containsAddr must match the normalized form")
	}
	if containsAddr(nil, "1.2.3.4") {
		t.Error("containsAddr on nil must be false")
	}
}
```

- [ ] **Step 3: Убедиться, что тесты падают**

Run: `cd server && go test ./internal/webapi/ -run 'TestHandleAddClient|TestHandlePauseClient|TestHandleResumeClient|TestHandleDeleteClient|TestRemoveAddr' -v`
Expected: FAIL: `undefined: removeAddr`, `undefined: containsAddr`, а после их временного отсутствия остальные тесты падают по кодам ответов.

- [ ] **Step 4: Переписать `handler_clients.go`**

Заменить содержимое `server/internal/webapi/handler_clients.go` целиком:

```go
package webapi

import (
	"fmt"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// validRoutes is the set of allowed route names for client assignment.
var validRoutes = map[string]bool{
	"xray":   true,
	"wgc1":   true,
	"wgc2":   true,
	"wgc3":   true,
	"wgc4":   true,
	"wgc5":   true,
	"ovpnc1": true,
	"ovpnc2": true,
	"ovpnc3": true,
	"ovpnc4": true,
	"ovpnc5": true,
}

// handleListClients returns a handler that lists all VPN clients with their
// route assignment and pause status.
func handleListClients(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}
		clients := vpnconfig.CollectClients(cfg)
		jsonOK(w, map[string]interface{}{"clients": clients})
	}
}

// addClientRequest is the expected JSON body for POST /api/clients.
type addClientRequest struct {
	IP    string `json:"ip"`
	Route string `json:"route"`
}

// handleAddClient returns a handler that adds a client address to the
// specified route and applies the configuration. The address is normalized
// (IPv4 only, /32 stripped), must not already be configured in any route,
// and a newly created tunnel inherits xray.exclude_sets like the bot wizard.
func handleAddClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		var req addClientRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}
		ip, err := vpnconfig.NormalizeClientAddr(req.IP)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if req.Route == "" {
			jsonError(w, http.StatusBadRequest, "route is required")
			return
		}
		if !validRoutes[req.Route] {
			jsonError(w, http.StatusBadRequest, "invalid route: must be one of xray, wgc1-wgc5, ovpnc1-ovpnc5")
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		if existing, found := findClient(cfg, ip); found {
			jsonError(w, http.StatusConflict, fmt.Sprintf("client already configured for %s", existing.route))
			return
		}

		if req.Route == "xray" {
			cfg.Xray.Clients = append(cfg.Xray.Clients, ip)
		} else {
			if cfg.TunnelDirector.Tunnels == nil {
				cfg.TunnelDirector.Tunnels = make(map[string]vpnconfig.TunnelConfig)
			}
			tunnel, ok := cfg.TunnelDirector.Tunnels[req.Route]
			if !ok {
				// A new tunnel inherits the Xray country exclusions, like the
				// bot's configure wizard. An empty exclude would route the
				// client's local-country traffic through the tunnel as well.
				tunnel = vpnconfig.TunnelConfig{
					Clients: []string{},
					Exclude: append([]string{}, cfg.Xray.ExcludeSets...),
				}
			}
			tunnel.Clients = append(tunnel.Clients, ip)
			cfg.TunnelDirector.Tunnels[req.Route] = tunnel
		}

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handlePauseClient returns a handler that pauses a configured client.
// The paused entry keeps the stored spelling of the address because the
// shell subtracts paused_clients from the clients arrays by exact string.
func handlePauseClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		existing, found := findClient(cfg, ip)
		if !found {
			jsonError(w, http.StatusNotFound, "client not found")
			return
		}

		if !contains(cfg.PausedClients, existing.stored) {
			cfg.PausedClients = append(cfg.PausedClients, existing.stored)
		}

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handleResumeClient returns a handler that resumes a paused client.
func handleResumeClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		if _, found := findClient(cfg, ip); !found {
			jsonError(w, http.StatusNotFound, "client not found")
			return
		}

		cfg.PausedClients = removeAddr(cfg.PausedClients, ip)

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handleDeleteClient returns a handler that removes a client from all routes
// and from the paused list, matching every stored spelling of the address.
func handleDeleteClient(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		if _, found := findClient(cfg, ip); !found {
			jsonError(w, http.StatusNotFound, "client not found")
			return
		}

		cfg.Xray.Clients = removeAddr(cfg.Xray.Clients, ip)
		for name, tunnel := range cfg.TunnelDirector.Tunnels {
			tunnel.Clients = removeAddr(tunnel.Clients, ip)
			cfg.TunnelDirector.Tunnels[name] = tunnel
		}
		cfg.PausedClients = removeAddr(cfg.PausedClients, ip)

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// clientMatch describes a configured client found by normalized address.
type clientMatch struct {
	route  string // "xray" or the tunnel name
	stored string // the address exactly as stored in vpn-director.json
}

// findClient looks addr up across xray.clients and every tunnel, comparing
// normalized forms because older configs store both 1.2.3.4 and 1.2.3.4/32.
func findClient(cfg *vpnconfig.VPNDirectorConfig, addr string) (clientMatch, bool) {
	for _, c := range vpnconfig.CollectClients(cfg) {
		if sameAddr(c.IP, addr) {
			return clientMatch{route: c.Route, stored: c.IP}, true
		}
	}
	return clientMatch{}, false
}

// clientAddrFromQuery reads and normalizes the ip query parameter. It writes
// the error response itself and returns ok=false when the handler must stop.
func clientAddrFromQuery(w http.ResponseWriter, r *http.Request) (string, bool) {
	raw := r.URL.Query().Get("ip")
	if raw == "" {
		jsonError(w, http.StatusBadRequest, "ip query parameter is required")
		return "", false
	}
	ip, err := vpnconfig.NormalizeClientAddr(raw)
	if err != nil {
		jsonError(w, http.StatusBadRequest, err.Error())
		return "", false
	}
	return ip, true
}

// sameAddr reports whether stored denotes the same address as the normalized
// addr. Entries that fail normalization are compared verbatim.
func sameAddr(stored, addr string) bool {
	normalized, err := vpnconfig.NormalizeClientAddr(stored)
	if err != nil {
		normalized = stored
	}
	return normalized == addr
}

// containsAddr reports whether slice holds addr in any stored spelling.
func containsAddr(slice []string, addr string) bool {
	for _, s := range slice {
		if sameAddr(s, addr) {
			return true
		}
	}
	return false
}

// removeAddr returns a new slice without every entry that denotes addr,
// whatever its stored spelling.
func removeAddr(slice []string, addr string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if !sameAddr(s, addr) {
			result = append(result, s)
		}
	}
	return result
}

// contains returns true if the slice contains the item.
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// removeString returns a new slice with all exact matches of item removed.
func removeString(slice []string, item string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != item {
			result = append(result, s)
		}
	}
	return result
}
```

- [ ] **Step 5: Убедиться, что тесты проходят**

Run: `cd server && go vet ./internal/webapi/ && go test ./internal/webapi/`
Expected: PASS для всего пакета, включая обновлённые и новые тесты.

- [ ] **Step 6: Commit**

```bash
git add server/internal/webapi/handler_clients.go server/internal/webapi/handler_clients_test.go
git commit -m "fix(webapi): validate, de-duplicate and apply client changes like the bot"
```

---

### Task 6: Обработчики исключений

**Files:**
- Modify: `server/internal/webapi/handler_excludes.go` (полная замена)
- Test: `server/internal/webapi/handler_excludes_test.go`

**Interfaces:**
- Consumes: `vpnconfig.NormalizeClientAddr`, `saveAndApply`, `writeSaveApplyResult`, `containsAddr`, `removeAddr` из Task 5.
- Produces: `func normalizeExcludeSets(sets []string) ([]string, error)`.

- [ ] **Step 1: Новые и обновлённые тесты**

Добавить в конец `server/internal/webapi/handler_excludes_test.go`:

```go
func TestHandleUpdateExcludeSets_LowercasesAndDedupes(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["RU", " ua ", "ru"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeSets, ","); got != "ru,ua" {
		t.Errorf("expected [ru ua], got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestHandleUpdateExcludeSets_InvalidCode(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["ru", "xxx"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != `invalid country code: "xxx"` {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
	if mc.savedCfg != nil {
		t.Error("config must not be saved on validation error")
	}
}

func TestHandleUpdateExcludeSets_SavedButApplyFailed(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	deps.VPN = &mockVPN{err: errors.New("apply failed (exit 1): [ERROR] Invalid country code 'zz'")}

	handler := handleUpdateExcludeSets(deps)

	body := `{"sets": ["zz"]}`
	req := httptest.NewRequest("POST", "/api/excludes/sets", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["saved"] != true {
		t.Errorf("expected saved: true, got %v", resp["saved"])
	}
	if resp["error"] != "configuration saved, but apply failed: [ERROR] Invalid country code 'zz'" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
}

func TestHandleAddExcludeIP_NormalizesSlash32(t *testing.T) {
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "5.6.7.8/32"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeIPs, ","); got != "5.6.7.8" {
		t.Errorf("expected /32 stripped, got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestHandleAddExcludeIP_RejectsIPv6(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}

	handler := handleAddExcludeIP(deps)

	body := `{"ip": "2001:db8::/32"}`
	req := httptest.NewRequest("POST", "/api/excludes/ips", strings.NewReader(body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid IPv4 address or CIDR" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestHandleDeleteExcludeIP_MatchesStoredForm(t *testing.T) {
	mc := &mockConfig{
		cfg: &vpnconfig.VPNDirectorConfig{
			Xray: vpnconfig.XrayConfig{ExcludeIPs: []string{"1.2.3.4/32", "5.6.7.8"}},
		},
	}
	deps := newTestDeps(t)
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	handler := handleDeleteExcludeIP(deps)

	req := httptest.NewRequest("DELETE", "/api/excludes/ips?ip=1.2.3.4", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := strings.Join(mc.savedCfg.Xray.ExcludeIPs, ","); got != "5.6.7.8" {
		t.Errorf("expected [5.6.7.8], got %q", got)
	}
	if vpn.applyCalls != 1 {
		t.Errorf("expected 1 Apply() call, got %d", vpn.applyCalls)
	}
}

func TestNormalizeExcludeSets(t *testing.T) {
	got, err := normalizeExcludeSets([]string{"RU", " ua ", "ru", "By"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(got, ",") != "ru,ua,by" {
		t.Errorf("normalizeExcludeSets = %v, want [ru ua by]", got)
	}
	if got, err := normalizeExcludeSets(nil); err != nil || len(got) != 0 {
		t.Errorf("nil input: got %v, %v; want empty, nil", got, err)
	}
	for _, bad := range []string{"", "x", "xxx", "r1", "ru "+"\n"+"ua"} {
		if _, err := normalizeExcludeSets([]string{bad}); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}
```

- [ ] **Step 2: Убедиться, что тесты падают**

Run: `cd server && go test ./internal/webapi/ -run 'ExcludeSets|ExcludeIP' -v`
Expected: FAIL, `undefined: normalizeExcludeSets`, остальные по кодам ответов.

- [ ] **Step 3: Переписать `handler_excludes.go`**

Заменить содержимое `server/internal/webapi/handler_excludes.go` целиком:

```go
package webapi

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// countryCodeRe matches a two-letter ISO country code in lowercase, the form
// the bot, the config template and lib/ipset.sh use.
var countryCodeRe = regexp.MustCompile(`^[a-z]{2}$`)

// handleListExcludeSets returns a handler that lists configured exclusion sets.
func handleListExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}
		jsonOK(w, map[string]interface{}{"sets": cfg.Xray.ExcludeSets})
	}
}

// updateExcludeSetsRequest is the expected JSON body for POST /api/excludes/sets.
type updateExcludeSetsRequest struct {
	Sets *[]string `json:"sets"`
}

// normalizeExcludeSets trims, lowercases, validates and de-duplicates country
// codes, keeping first-occurrence order. Unknown codes (e.g. "xx") pass here
// and fail at apply time in lib/ipset.sh, which the auto-apply surfaces.
func normalizeExcludeSets(sets []string) ([]string, error) {
	out := make([]string, 0, len(sets))
	seen := make(map[string]bool, len(sets))
	for _, raw := range sets {
		code := strings.ToLower(strings.TrimSpace(raw))
		if !countryCodeRe.MatchString(code) {
			return nil, fmt.Errorf("invalid country code: %q", raw)
		}
		if seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out, nil
}

// handleUpdateExcludeSets returns a handler that replaces the exclusion sets
// list and applies the configuration.
func handleUpdateExcludeSets(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		var req updateExcludeSetsRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Sets == nil {
			jsonError(w, http.StatusBadRequest, "sets field is required")
			return
		}

		sets, err := normalizeExcludeSets(*req.Sets)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		cfg.Xray.ExcludeSets = sets

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handleListExcludeIPs returns a handler that lists configured exclusion IPs.
func handleListExcludeIPs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}
		jsonOK(w, map[string]interface{}{"ips": cfg.Xray.ExcludeIPs})
	}
}

// addExcludeIPRequest is the expected JSON body for POST /api/excludes/ips.
type addExcludeIPRequest struct {
	IP string `json:"ip"`
}

// handleAddExcludeIP returns a handler that adds an IPv4/CIDR to the exclusion
// list and applies the configuration.
func handleAddExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		var req addExcludeIPRequest
		if err := decodeJSON(r, &req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.IP == "" {
			jsonError(w, http.StatusBadRequest, "ip is required")
			return
		}
		ip, err := vpnconfig.NormalizeClientAddr(req.IP)
		if err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		if !containsAddr(cfg.Xray.ExcludeIPs, ip) {
			cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, ip)
		}

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}

// handleDeleteExcludeIP returns a handler that removes an IPv4/CIDR from the
// exclusion list (any stored spelling) and applies the configuration.
func handleDeleteExcludeIP(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()

		ip, ok := clientAddrFromQuery(w, r)
		if !ok {
			return
		}

		cfg, err := deps.Config.LoadVPNConfig()
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to load configuration")
			return
		}

		cfg.Xray.ExcludeIPs = removeAddr(cfg.Xray.ExcludeIPs, ip)

		writeSaveApplyResult(w, saveAndApply(deps, cfg))
	}
}
```

- [ ] **Step 4: Убедиться, что тесты проходят**

Run: `cd server && go vet ./internal/webapi/ && go test ./internal/webapi/`
Expected: PASS. Существующие `TestHandleUpdateExcludeSets_OK` (`ru,ua,by`), `_EmptyList`, `TestHandleAddExcludeIP_Duplicate` (200, одна запись) остаются зелёными.

- [ ] **Step 5: Commit**

```bash
git add server/internal/webapi/handler_excludes.go server/internal/webapi/handler_excludes_test.go
git commit -m "fix(webapi): validate country codes, normalize exclude IPs and apply after save"
```

---

### Task 7: Ошибки разбора подписки и честная заглушка Update

**Files:**
- Modify: `server/internal/webapi/handler_servers.go:157-162` и импорты, добавить `noServersMessage`
- Modify: `server/internal/webapi/handler_logs.go:86-92` (`handleUpdate`)
- Test: `server/internal/webapi/handler_servers_test.go`, `server/internal/webapi/handler_logs_test.go:236-262`

**Interfaces:**
- Consumes: `vless.DecodeSubscription(encoded string) ([]*vless.Server, []error)`.
- Produces: `func noServersMessage(errs []error) string`.

- [ ] **Step 1: Падающие тесты**

Добавить в `server/internal/webapi/handler_servers_test.go`:

```go
func TestNoServersMessage(t *testing.T) {
	if got := noServersMessage(nil); got != "no VLESS servers found in subscription" {
		t.Errorf("no errors: got %q", got)
	}
	two := []error{errors.New("line 1: bad scheme"), errors.New("line 2: missing uuid")}
	want := "no VLESS servers found in subscription: line 1: bad scheme; line 2: missing uuid"
	if got := noServersMessage(two); got != want {
		t.Errorf("two errors: got %q, want %q", got, want)
	}
	five := []error{errors.New("e1"), errors.New("e2"), errors.New("e3"), errors.New("e4"), errors.New("e5")}
	if got := noServersMessage(five); got != "no VLESS servers found in subscription: e1; e2; e3" {
		t.Errorf("five errors must be capped at three: got %q", got)
	}
}
```

В `server/internal/webapi/handler_logs_test.go` заменить `TestHandleUpdate_NotSupported` на:

```go
func TestHandleUpdate_NotImplemented(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleUpdate(deps)

	req := httptest.NewRequest("POST", "/api/update", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501, got %d: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "self-update via Web UI is not supported yet" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}
```

Run: `cd server && go test ./internal/webapi/ -run 'TestNoServersMessage|TestHandleUpdate_NotImplemented' -v`
Expected: FAIL, `undefined: noServersMessage`; после его появления заглушка отдаёт 200 вместо 501.

- [ ] **Step 2: Реализация**

В `server/internal/webapi/handler_servers.go` добавить `"strings"` в импорты, заменить

```go
		// Decode VLESS subscription.
		vlessServers, _ := vless.DecodeSubscription(string(body))
		if len(vlessServers) == 0 {
			jsonError(w, http.StatusBadRequest, "no VLESS servers found in subscription")
			return
		}
```

на

```go
		// Decode VLESS subscription. Parse errors travel back to the user so a
		// rejected link explains itself, as the bot's /import does.
		vlessServers, parseErrs := vless.DecodeSubscription(string(body))
		if len(vlessServers) == 0 {
			jsonError(w, http.StatusBadRequest, noServersMessage(parseErrs))
			return
		}
```

и добавить в конец файла:

```go
// noServersMessage explains an empty subscription. Up to three parse errors
// are appended so the user learns why the link was rejected.
func noServersMessage(errs []error) string {
	const msg = "no VLESS servers found in subscription"
	if len(errs) == 0 {
		return msg
	}
	parts := make([]string, 0, 3)
	for _, e := range errs {
		if len(parts) == 3 {
			break
		}
		parts = append(parts, e.Error())
	}
	return msg + ": " + strings.Join(parts, "; ")
}
```

В `server/internal/webapi/handler_logs.go` заменить `handleUpdate` на:

```go
// handleUpdate rejects self-update with 501 until the unified updater lands
// (spec block 3). A plain error status keeps the UI from reporting success.
func handleUpdate(_ *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, http.StatusNotImplemented, "self-update via Web UI is not supported yet")
	}
}
```

- [ ] **Step 3: Убедиться, что тесты проходят**

Run: `cd server && go vet ./internal/webapi/ && go test ./internal/webapi/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add server/internal/webapi/handler_servers.go server/internal/webapi/handler_servers_test.go server/internal/webapi/handler_logs.go server/internal/webapi/handler_logs_test.go
git commit -m "fix(webapi): surface subscription parse errors and return 501 from the update stub"
```

---

### Task 8: JSON 404 под `/api/` и заголовки кеша SPA

**Files:**
- Modify: `server/internal/webapi/router.go:53-98` (`registerProtectedRoutes`), `:100-127` (`spaHandler`), импорты
- Create: `server/internal/webapi/router_test.go`

**Interfaces:**
- Consumes: `newTestDeps`, `deps.JWT.Create(subject string) (string, error)`.
- Produces: ничего нового для других задач.

- [ ] **Step 1: Падающие тесты**

Создать `server/internal/webapi/router_test.go`:

```go
package webapi

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func testStaticFS() fstest.MapFS {
	return fstest.MapFS{
		"index.html":    {Data: []byte("<html>spa</html>")},
		"assets/app.js": {Data: []byte("console.log('app')")},
	}
}

func TestRouter_UnknownAPIPathIsJSON404(t *testing.T) {
	deps := newTestDeps(t)
	router := NewRouter(deps, testStaticFS())

	token, err := deps.JWT.Create("admin")
	if err != nil {
		t.Fatalf("create token: %v", err)
	}
	req := httptest.NewRequest("GET", "/api/nope", nil)
	req.AddCookie(&http.Cookie{Name: "token", Value: token})
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d: %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected JSON content type, got %q", ct)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "not found" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestRouter_UnknownAPIPathStillRequiresAuth(t *testing.T) {
	deps := newTestDeps(t)
	router := NewRouter(deps, testStaticFS())

	req := httptest.NewRequest("GET", "/api/nope", nil)
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 before the 404 fallback, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestSPAHandler_CacheHeaders(t *testing.T) {
	handler := spaHandler(testStaticFS())

	cases := []struct {
		path      string
		wantCache string
		wantBody  string
	}{
		{"/", "no-cache", "<html>spa</html>"},
		{"/clients", "no-cache", "<html>spa</html>"},
		{"/assets/app.js", "public, max-age=31536000, immutable", "console.log('app')"},
	}
	for _, c := range cases {
		t.Run(c.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", c.path, nil)
			rec := httptest.NewRecorder()

			handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if got := rec.Header().Get("Cache-Control"); got != c.wantCache {
				t.Errorf("Cache-Control = %q, want %q", got, c.wantCache)
			}
			body, _ := io.ReadAll(rec.Body)
			if string(body) != c.wantBody {
				t.Errorf("body = %q, want %q", string(body), c.wantBody)
			}
		})
	}
}
```

Run: `cd server && go test ./internal/webapi/ -run 'TestRouter_|TestSPAHandler_' -v`
Expected: FAIL: 404 приходит как `text/plain` с телом `404 page not found`; заголовков `Cache-Control` нет.

- [ ] **Step 2: Реализация**

В `server/internal/webapi/router.go` добавить `"strings"` в импорты. В конец `registerProtectedRoutes` (после `mux.HandleFunc("POST /api/update", handleUpdate(deps))`) добавить:

```go
	// Fallback for unknown API paths: JSON 404 instead of the mux's text/plain.
	// Auth still runs first because this mux sits behind authMiddleware.
	mux.HandleFunc("/api/", func(w http.ResponseWriter, _ *http.Request) {
		jsonError(w, http.StatusNotFound, "not found")
	})
```

Заменить `spaHandler` целиком:

```go
// spaHandler serves static files from the embedded filesystem. If a file is
// not found and the request path does not start with "/api/", it falls back to
// index.html so the Vue SPA router can handle the path. Hashed bundles under
// assets/ are immutable; index.html must be revalidated so a new binary's
// bundle names are picked up after an update.
func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}

		if f, err := staticFS.Open(path); err == nil {
			f.Close()
		} else {
			// File not found — serve index.html for SPA routing.
			path = "index.html"
			r.URL.Path = "/"
		}

		if strings.HasPrefix(path, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 3: Убедиться, что тесты проходят**

Run: `cd server && go vet ./internal/webapi/ && go test ./internal/webapi/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add server/internal/webapi/router.go server/internal/webapi/router_test.go
git commit -m "fix(webapi): JSON 404 for unknown API paths and cache headers for the SPA"
```

---

### Task 9: Секция `log` в шаблоне Xray

**Files:**
- Modify: `router/opt/etc/xray/config.json.template:1-2`
- Modify: `server/testdata/dev/xray.template.json:1-2`
- Test: `router/test/unit/xray_template.bats`, `router/test/unit/xrayconf.bats`, `server/internal/service/xray_test.go`

**Interfaces:**
- Consumes: ничего.
- Produces: файл `/tmp/xray-error.log`, который пишет Xray после следующей генерации `config.json`. Task 10 и Task 11 указывают на него.

- [ ] **Step 1: Падающие bats-тесты**

Добавить в конец `router/test/unit/xray_template.bats`:

```bash
@test "config.json.template writes Xray errors to /tmp/xray-error.log and disables the access log" {
    run jq -r '.log.error' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$status" -eq 0 ]
    [ "$output" = "/tmp/xray-error.log" ]
    run jq -r '.log.access' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$output" = "none" ]
    run jq -r '.log.loglevel' "$PROJECT_ROOT/opt/etc/xray/config.json.template"
    [ "$output" = "warning" ]
}
```

Добавить в конец `router/test/unit/xrayconf.bats`:

```bash
@test "generate: real template keeps the log section" {
    server='{"address":"1.2.3.4","port":443,"uuid":"u1","security":"reality","sni":"s","fingerprint":"firefox","public_key":"PBK","short_id":"sid"}'
    run xrayconf_generate "$PROJECT_ROOT/opt/etc/xray/config.json.template" <<< "$server"
    [ "$status" -eq 0 ]
    [ "$(printf '%s' "$output" | jq -r '.log.error')" = "/tmp/xray-error.log" ]
    [ "$(printf '%s' "$output" | jq -r '.outbounds | length')" = "1" ]
}
```

Run: `bats router/test/unit/xray_template.bats router/test/unit/xrayconf.bats`
Expected: два новых теста FAIL (`jq -r '.log.error'` печатает `null`).

- [ ] **Step 2: Падающий Go-тест генератора**

Добавить в `server/internal/service/xray_test.go`:

```go
func TestGenerateConfig_KeepsTemplateLogSection(t *testing.T) {
	tmpDir := t.TempDir()
	templatePath := filepath.Join(tmpDir, "config.json.template")
	outputPath := filepath.Join(tmpDir, "config.json")
	tmpl := `{"log":{"loglevel":"warning","access":"none","error":"/tmp/xray-error.log"},"inbounds":[],"outbounds":[],"routing":{}}`
	if err := os.WriteFile(templatePath, []byte(tmpl), 0644); err != nil {
		t.Fatal(err)
	}

	server := vpnconfig.Server{Address: "1.2.3.4", Port: 443, UUID: "u1", Security: "tls"}
	if err := NewXrayService(templatePath, outputPath).GenerateConfig(server); err != nil {
		t.Fatalf("GenerateConfig error: %v", err)
	}

	content, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var cfg map[string]interface{}
	if err := json.Unmarshal(content, &cfg); err != nil {
		t.Fatalf("output is not valid JSON: %v", err)
	}
	logSection, ok := cfg["log"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected log section to be preserved, got %v", cfg["log"])
	}
	if logSection["error"] != "/tmp/xray-error.log" || logSection["access"] != "none" {
		t.Errorf("log section altered: %v", logSection)
	}
}
```

Run: `cd server && go test ./internal/service/ -run TestGenerateConfig_KeepsTemplateLogSection -v`
Expected: PASS уже сейчас (генератор копирует шаблон и заменяет только `outbounds`); тест фиксирует это поведение как контракт.

- [ ] **Step 3: Добавить секцию в шаблоны**

В `router/opt/etc/xray/config.json.template` заменить первые строки

```json
{
  "inbounds": [
```

на

```json
{
  "log": {
    "loglevel": "warning",
    "access": "none",
    "error": "/tmp/xray-error.log"
  },
  "inbounds": [
```

В `server/testdata/dev/xray.template.json` сделать ту же вставку, но с `"error": "testdata/dev/xray-error.log"`.

- [ ] **Step 4: Убедиться, что всё проходит**

Run: `jq empty router/opt/etc/xray/config.json.template server/testdata/dev/xray.template.json && bats router/test/unit/xray_template.bats router/test/unit/xrayconf.bats && cd server && go test ./internal/service/`
Expected: оба JSON валидны, bats PASS, Go PASS.

- [ ] **Step 5: Commit**

```bash
git add router/opt/etc/xray/config.json.template server/testdata/dev/xray.template.json router/test/unit/xray_template.bats router/test/unit/xrayconf.bats server/internal/service/xray_test.go
git commit -m "feat(xray): write Xray error log to /tmp/xray-error.log via the config template"
```

---

### Task 10: `paths.XrayLogPath` и источники логов через `Deps.LogPaths`

**Files:**
- Modify: `server/internal/paths/paths.go`
- Test: `server/internal/paths/paths_test.go`
- Modify: `server/internal/webapi/router.go:13-26` (`Deps`)
- Modify: `server/internal/webapi/handler_logs.go:1-66`
- Modify: `server/internal/webapi/test_helpers_test.go` (`mockLogs`, `newTestDeps`)
- Test: `server/internal/webapi/handler_logs_test.go`
- Modify: `server/cmd/webui/main.go:120-131` (`deps`)

**Interfaces:**
- Consumes: ничего.
- Produces: `paths.Paths.XrayLogPath string`; поле `Deps.LogPaths map[string]string` (имя источника → путь); `func logSourceNames(logPaths map[string]string) []string`; `mockLogs.paths []string`.

- [ ] **Step 1: Падающие тесты**

В `server/internal/paths/paths_test.go` добавить строку в таблицу `TestDefault`:

```go
		{"XrayLogPath", p.XrayLogPath, "/tmp/", "xray-error.log"},
```

и в таблицу `TestDevPaths`:

```go
		{"XrayLogPath", p.XrayLogPath, "testdata/dev/", "xray-error.log"},
```

В `server/internal/webapi/test_helpers_test.go` заменить `mockLogs`:

```go
// mockLogs implements service.LogReader for testing and records the paths read.
type mockLogs struct {
	output string
	err    error
	paths  []string
}

func (m *mockLogs) Read(path string, _ int) (string, error) {
	m.paths = append(m.paths, path)
	return m.output, m.err
}
```

и в `newTestDeps` добавить поле в литерал `Deps`:

```go
		LogPaths: map[string]string{
			"bot":  "/tmp/test-telegram-bot.log",
			"vpn":  "/tmp/test-vpn-director.log",
			"xray": "/tmp/test-xray-error.log",
		},
```

В `server/internal/webapi/handler_logs_test.go` добавить:

```go
func TestHandleLogs_ReadsPathFromDeps(t *testing.T) {
	deps := newTestDeps(t)
	logs := &mockLogs{output: "xray warning"}
	deps.Logs = logs

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=xray", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(logs.paths) != 1 || logs.paths[0] != "/tmp/test-xray-error.log" {
		t.Errorf("expected read of /tmp/test-xray-error.log, got %v", logs.paths)
	}
}

func TestHandleLogs_InvalidSourceListsValidOnes(t *testing.T) {
	deps := newTestDeps(t)

	handler := handleLogs(deps)

	req := httptest.NewRequest("GET", "/api/logs?source=invalid", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "unknown source: valid values are bot, vpn, xray" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}
```

Run: `cd server && go test ./internal/paths/ ./internal/webapi/ -run 'TestDefault|TestDevPaths|TestHandleLogs' -v`
Expected: FAIL: `p.XrayLogPath undefined`, `unknown field LogPaths`.

- [ ] **Step 2: Реализация**

`server/internal/paths/paths.go`, полное содержимое:

```go
// Package paths provides centralized path configuration for the application
package paths

// Paths holds all configurable paths for the application
type Paths struct {
	ScriptsDir     string // /opt/vpn-director
	BotConfigPath  string // /opt/vpn-director/telegram-bot.json
	DefaultDataDir string // /opt/vpn-director/data
	XrayTemplate   string // /opt/etc/xray/config.json.template
	XrayConfig     string // /opt/etc/xray/config.json
	BotLogPath     string // /tmp/telegram-bot.log
	VPNLogPath     string // /tmp/vpn-director.log
	XrayLogPath    string // /tmp/xray-error.log (set by the log section of the Xray template)
}

// Default returns the default paths for production use
func Default() Paths {
	return Paths{
		ScriptsDir:     "/opt/vpn-director",
		BotConfigPath:  "/opt/vpn-director/telegram-bot.json",
		DefaultDataDir: "/opt/vpn-director/data",
		XrayTemplate:   "/opt/etc/xray/config.json.template",
		XrayConfig:     "/opt/etc/xray/config.json",
		BotLogPath:     "/tmp/telegram-bot.log",
		VPNLogPath:     "/tmp/vpn-director.log",
		XrayLogPath:    "/tmp/xray-error.log",
	}
}

// DevPaths returns paths for development mode using testdata/dev/
func DevPaths() Paths {
	return Paths{
		ScriptsDir:     "testdata/dev",
		BotConfigPath:  "testdata/dev/telegram-bot.json",
		DefaultDataDir: "testdata/dev/data",
		XrayTemplate:   "testdata/dev/xray.template.json",
		XrayConfig:     "testdata/dev/xray.json",
		BotLogPath:     "testdata/dev/bot.log",
		VPNLogPath:     "testdata/dev/vpn.log",
		XrayLogPath:    "testdata/dev/xray-error.log",
	}
}
```

В `server/internal/webapi/router.go` в структуру `Deps` после `Logs service.LogReader` добавить:

```go
	LogPaths     map[string]string // log source name -> file path, built by main from paths.Paths
```

В `server/internal/webapi/handler_logs.go` удалить глобальную карту `logPaths` и заменить `handleLogs` на:

```go
// handleLogs returns a handler that reads log files listed in deps.LogPaths.
// Query params: source (one of the map keys), lines (default 50, max 500).
// If source is specified, returns that single log. Otherwise returns all.
func handleLogs(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		source := r.URL.Query().Get("source")
		linesStr := r.URL.Query().Get("lines")

		lines := 50
		if linesStr != "" {
			n, err := strconv.Atoi(linesStr)
			if err != nil || n < 1 {
				jsonError(w, http.StatusBadRequest, "lines must be a positive integer")
				return
			}
			if n > 500 {
				n = 500
			}
			lines = n
		}

		if source != "" {
			path, ok := deps.LogPaths[source]
			if !ok {
				jsonError(w, http.StatusBadRequest,
					"unknown source: valid values are "+strings.Join(logSourceNames(deps.LogPaths), ", "))
				return
			}

			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to read log file")
				return
			}

			jsonOK(w, map[string]string{"output": output, "source": source})
			return
		}

		// No source specified: return all logs.
		result := make(map[string]string, len(deps.LogPaths))
		for name, path := range deps.LogPaths {
			output, err := deps.Logs.Read(path, lines)
			if err != nil {
				result[name] = "error: " + err.Error()
			} else {
				result[name] = output
			}
		}

		jsonOK(w, result)
	}
}

// logSourceNames returns the log source names in sorted order for messages.
func logSourceNames(logPaths map[string]string) []string {
	names := make([]string, 0, len(logPaths))
	for name := range logPaths {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
```

Импорты `handler_logs.go` становятся: `net/http`, `sort`, `strconv`, `strings`.

В `server/cmd/webui/main.go` в литерал `deps := &webapi.Deps{...}` после `Logs:    logSvc,` добавить:

```go
		LogPaths: map[string]string{
			"bot":  p.BotLogPath,
			"vpn":  p.VPNLogPath,
			"xray": p.XrayLogPath,
		},
```

- [ ] **Step 3: Убедиться, что тесты проходят**

Run: `cd server && go build ./... && go vet ./... && go test ./internal/paths/ ./internal/webapi/`
Expected: PASS, включая `TestHandleLogs_AllSources` (ключи `vpn`, `xray`, `bot` по-прежнему присутствуют).

- [ ] **Step 4: Commit**

```bash
git add server/internal/paths/paths.go server/internal/paths/paths_test.go server/internal/webapi/router.go server/internal/webapi/handler_logs.go server/internal/webapi/test_helpers_test.go server/internal/webapi/handler_logs_test.go server/cmd/webui/main.go
git commit -m "fix(webapi): take log sources from paths and point xray at the real error log"
```

---

### Task 11: Источник `xray` в `/logs` бота и ротация error-лога

**Files:**
- Modify: `server/internal/handler/misc.go:63-104`
- Test: `server/internal/handler/misc_test.go:141-175` и новый тест
- Modify: `server/cmd/bot/main.go:106`
- Modify: `README.md:116`

**Interfaces:**
- Consumes: `paths.Paths.XrayLogPath` из Task 10.
- Produces: ничего нового.

- [ ] **Step 1: Обновить и добавить тесты**

В `server/internal/handler/misc_test.go` в `TestMiscHandler_HandleLogs_DefaultArgs` заменить

```go
	// Default is "all" which reads both bot and vpn logs
	if len(logReader.calls) != 2 {
		t.Errorf("expected 2 log read calls, got %d", len(logReader.calls))
	}
```

на

```go
	// Default is "all" which reads bot, vpn and xray logs
	if len(logReader.calls) != 3 {
		t.Fatalf("expected 3 log read calls, got %d", len(logReader.calls))
	}
```

и в `testPaths` этого теста добавить `XrayLogPath: "/tmp/xray-error.log",`, а после проверки второго вызова добавить:

```go
	// Check third call (xray error log)
	if logReader.calls[2].path != "/tmp/xray-error.log" {
		t.Errorf("expected xray log path, got %q", logReader.calls[2].path)
	}
```

Добавить новый тест после `TestMiscHandler_HandleLogs_SourceVPN`:

```go
func TestMiscHandler_HandleLogs_SourceXray(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "xray warning"}
	testPaths := paths.Paths{
		BotLogPath:  "/tmp/bot.log",
		VPNLogPath:  "/tmp/vpn.log",
		XrayLogPath: "/tmp/xray-error.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs xray",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Fatalf("expected 1 log read call, got %d", len(logReader.calls))
	}
	if logReader.calls[0].path != "/tmp/xray-error.log" {
		t.Errorf("expected xray log path, got %q", logReader.calls[0].path)
	}
}
```

Run: `cd server && go test ./internal/handler/ -run 'TestMiscHandler_HandleLogs' -v`
Expected: FAIL: `DefaultArgs` видит 2 вызова, `SourceXray` получает usage-сообщение вместо чтения.

- [ ] **Step 2: Реализация**

В `server/internal/handler/misc.go` в `HandleLogs`:

заменить `case "bot", "vpn", "all":` на `case "bot", "vpn", "xray", "all":`;

заменить строку usage на `h.deps.Sender.Send(msg.Chat.ID, "Usage: `/logs [bot|vpn|xray|all] [lines]`")`;

после блока `if source == "vpn" || source == "all" { ... }` добавить:

```go
	if source == "xray" || source == "all" {
		h.sendLogFile(msg.Chat.ID, h.deps.Paths.XrayLogPath, "Xray", lines)
	}
```

В `server/cmd/bot/main.go` заменить

```go
	logger.StartRotation(ctx, []string{p.BotLogPath, p.VPNLogPath}, maxLogSize, time.Minute)
```

на

```go
	logger.StartRotation(ctx, []string{p.BotLogPath, p.VPNLogPath, p.XrayLogPath}, maxLogSize, time.Minute)
```

В `README.md` строку 116 заменить на:

```markdown
| **Logs** | Log viewer (bot, vpn, xray) |
```

- [ ] **Step 3: Убедиться, что тесты проходят**

Run: `cd server && go build ./... && go test ./internal/handler/`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git add server/internal/handler/misc.go server/internal/handler/misc_test.go server/cmd/bot/main.go README.md
git commit -m "feat(bot): add xray source to /logs and rotate the Xray error log"
```

---

### Task 12: Фронтенд: независимый статус, обработка `saved`, коды стран в нижнем регистре

**Files:**
- Modify: `web/src/components/StatusTab.vue`
- Modify: `web/src/components/ClientsTab.vue`
- Modify: `web/src/components/ExclusionsTab.vue`

**Interfaces:**
- Consumes: ответы API из Task 5 и Task 6 (`saved: true` при провале apply, 409 с текстом, 400 с текстом).
- Produces: ничего.

- [ ] **Step 1: StatusTab, независимая загрузка**

В `web/src/components/StatusTab.vue` заменить блок `<script setup>` на:

```vue
<script setup lang="ts">
import { ref, onMounted } from 'vue'
import api from '../api'

const status = ref('')
const ip = ref('')
const ipError = ref('')
const loading = ref(false)
const actionLoading = ref('')

function errorText(e: any): string {
  return e?.response?.data?.error || e?.message || 'unknown error'
}

async function loadStatus() {
  loading.value = true
  ipError.value = ''
  // Status and external IP load independently: a failing curl ifconfig.me
  // must not hide the VPN Director status.
  const [statusRes, ipRes] = await Promise.allSettled([api.getStatus(), api.getIP()])
  if (statusRes.status === 'fulfilled') {
    status.value = statusRes.value.data.output
  } else {
    status.value = 'Error: ' + errorText(statusRes.reason)
  }
  if (ipRes.status === 'fulfilled') {
    ip.value = ipRes.value.data.ip
  } else {
    ip.value = ''
    ipError.value = errorText(ipRes.reason)
  }
  loading.value = false
}

async function doAction(name: string, fn: () => Promise<any>) {
  actionLoading.value = name
  try {
    await fn()
    await loadStatus()
  } catch (e: any) {
    alert('Error: ' + errorText(e))
  } finally {
    actionLoading.value = ''
  }
}

onMounted(loadStatus)
</script>
```

В шаблоне заменить карточку External IP:

```vue
    <div class="card">
      <div class="card-title">External IP</div>
      <div v-if="ip" style="font-size: 20px; margin-top: 8px;">{{ ip }}</div>
      <div v-else-if="ipError" style="margin-top: 8px; color: #ff6b6b; font-size: 0.875rem;">
        unavailable: {{ ipError }}
      </div>
      <div v-else style="font-size: 20px; margin-top: 8px;">...</div>
    </div>
```

- [ ] **Step 2: ClientsTab, перечитывать список после «saved but apply failed»**

В `web/src/components/ClientsTab.vue` добавить после объявления `routeOptions`:

```ts
// Shows the server error; when the change was saved but apply failed
// (response carries saved: true) the list is refreshed so the saved
// change is visible and the Status tab's Apply can retry.
async function reportError(e: any) {
  alert('Error: ' + (e.response?.data?.error || e.message))
  if (e.response?.data?.saved) {
    await loadClients()
  }
}
```

и во всех четырёх функциях (`addClient`, `pauseClient`, `resumeClient`, `removeClient`) заменить строку

```ts
    alert('Error: ' + (e.response?.data?.error || e.message))
```

на

```ts
    await reportError(e)
```

- [ ] **Step 3: ExclusionsTab, нижний регистр и `saved`**

В `web/src/components/ExclusionsTab.vue`:

добавить после объявления `ipLoading`:

```ts
async function reportError(e: any) {
  alert('Error: ' + (e.response?.data?.error || e.message))
  if (e.response?.data?.saved) {
    await loadData()
  }
}
```

заменить начало `addCountry`:

```ts
async function addCountry() {
  const code = newCountry.value.trim().toLowerCase()
  if (!/^[a-z]{2}$/.test(code)) {
    alert('Please enter a valid 2-letter country code (e.g. us, de, jp)')
    return
  }
  if (countrySets.value.some((c) => c.toLowerCase() === code)) {
    alert('Country code already added')
    return
  }
```

во всех четырёх функциях (`addCountry`, `removeCountry`, `addIP`, `removeIP`) заменить `alert('Error: ' + (e.response?.data?.error || e.message))` на `await reportError(e)`;

в шаблоне у поля ввода кода страны заменить `placeholder="Country code (e.g. US)"` на `placeholder="Country code (e.g. us)"` и `text-transform: uppercase;` на `text-transform: lowercase;`.

- [ ] **Step 4: Сборка фронта**

Run: `cd web && npm ci && npm run build`
Expected: `vue-tsc -b` без ошибок типов, `vite build` создаёт `web/dist/`. `web/dist/` в gitignore, в коммит не попадает.

- [ ] **Step 5: Ручная проверка в dev-режиме**

Запустить бэкенд `cd server && go run ./cmd/webui --dev` и фронт `cd web && npm run dev`, открыть `http://localhost:5173`, логин `admin` / `admin`. Проверить:

- вкладка Status показывает статус из mock-исполнителя даже если внешний IP недоступен (отключить сеть или дождаться таймаута curl): карточка IP пишет `unavailable: ...`, статус на месте;
- Clients: добавление `192.168.50.10/32` сохраняет `192.168.50.10`; повторное добавление того же адреса в другой маршрут даёт `client already configured for ...`; добавление в `wgc1` создаёт туннель с `exclude` равным `xray.exclude_sets` (видно в Settings → Show Config);
- Exclusions: ввод `US` сохраняется как `us`; ввод `xxx` даёт `invalid country code`;
- Settings → Update показывает `self-update via Web UI is not supported yet`.

Остановить оба процесса.

- [ ] **Step 6: Commit**

```bash
git add web/src/components/StatusTab.vue web/src/components/ClientsTab.vue web/src/components/ExclusionsTab.vue
git commit -m "fix(web): load status and IP independently, refresh after failed apply, lowercase country codes"
```

---

### Task 13: Финальная проверка и подготовка ветки к PR

**Files:**
- Delete from branch: `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, `docs/superpowers/plans/2026-09-05-webui-behavior.md`

**Interfaces:**
- Consumes: всё выше.
- Produces: ветка `feature/webui-behavior`, готовая к PR.

- [ ] **Step 1: Полный прогон Go**

Run: `cd server && go build ./... && go vet ./... && go test ./...`
Expected: всё PASS, без предупреждений vet.

- [ ] **Step 2: Полный прогон bats**

Run: `bats router/test/unit/`
Expected: PASS, включая новые тесты из Task 9.

- [ ] **Step 3: Сборка Web UI бинарника с embed**

Run: `make build-webui`
Expected: `web/dist/` собран, скопирован в `server/cmd/webui/web/dist/`, бинарник `server/bin/webui` собран. Эти артефакты в gitignore.

- [ ] **Step 4: Убрать дизайн-документы из ветки**

Правило репозитория: документы `docs/superpowers/` не должны попасть в diff PR, они остаются в истории ветки.

```bash
git rm docs/superpowers/specs/2026-09-05-webui-hardening-design.md docs/superpowers/plans/2026-09-05-webui-behavior.md
git commit -m "chore: remove design docs from branch before PR"
```

Файл `docs/superpowers/plans/2026-03-26-exclude-ips-and-multi-resolve-continuation-prompt.md` не отслеживается git и не трогается.

- [ ] **Step 5: Проверить diff ветки против master**

Run: `git diff --stat master...HEAD`
Expected: в списке нет файлов из `docs/superpowers/`, нет `web/dist`, нет бинарников.

- [ ] **Step 6: Завершение ветки**

Использовать skill `superpowers:finishing-a-development-branch`. В описании PR перечислить:

- авто-apply после мутаций клиентов и исключений, контракт ответа `saved: true`;
- новые коды ответов: 409 для дубликата клиента, 404 для неизвестного клиента, 400 для IPv6 и неверных кодов стран, 501 для заглушки Update;
- наследование `exclude` новым туннелем;
- Update IPsets теперь `vpn-director.sh update`;
- error-лог Xray: секция `log` в шаблоне. Release note: на установленных роутерах `config.json` получит секцию после следующего выбора сервера в боте или Web UI, до этого источник `xray` в логах пуст;
- ссылка на спек в истории ветки: `git show 91f4579:docs/superpowers/specs/2026-09-05-webui-hardening-design.md`.
