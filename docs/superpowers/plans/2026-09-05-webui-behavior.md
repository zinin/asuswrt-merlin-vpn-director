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
✅ Done — see commit(s): `ee1ad96`, `7c32436`, `2631aad`

---

### Task 2: Бот переходит на общий валидатор
✅ Done — see commit(s): `8c0cc65`

---

### Task 3: `VPNDirector.Update()` и честный Update IPsets
✅ Done — see commit(s): `3da6161`

---

### Task 4: Хелпер `saveAndApply`
✅ Done — see commit(s): `f4a1343`

---

### Task 5: Обработчики клиентов
✅ Done — see commit(s): `3c3a149`

---

### Task 6: Обработчики исключений
✅ Done — see commit(s): `5812178`

---

### Task 7: Ошибки разбора подписки и честная заглушка Update
✅ Done — see commit(s): `c3cc47e`

---

### Task 8: JSON 404 под `/api/` и заголовки кеша SPA
✅ Done — see commit(s): `bfd64c7`

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
