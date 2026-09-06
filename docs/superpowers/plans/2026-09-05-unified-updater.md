# Web UI: единое самообновление (блок 3) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить самообновление, знающее только про бота, на общий поток обновления, который выкладывает и перезапускает оба демона (`telegram-bot` и `webui`), запускается одинаково из Telegram и из Web UI и восстанавливает работавшие демоны при сбое.

**Architecture:** Пакет `updater` получает таблицу демонов (имя ассета = имя файла = имя процесса), качает по бинарнику на демона и рендерит шаблон скрипта из этой же таблицы; скрипт запоминает работавших демонов, останавливает их, копирует всё, пишет `notify.json` со `status`/`initiator` и восстанавливается через `trap ... EXIT` (у ash нет `ERR`). Новый пакет `updateflow` держит всю оркестрацию: `Check` с 30-минутным кешем и минутным порогом для `force`, `Start` с синхронным пре-флайтом и фоновой загрузкой, типизированные ошибки. Бот и Web UI становятся тонкими адаптерами над `Flow`: бот шлёт прогресс в чат, Web UI — в свой лог; `startup.CheckAndSendNotify` при `chat_id` 0 уведомляет все активные чаты.

**Tech Stack:** Go 1.25 (`text/template`, `context`, `sync.Mutex`, `errors.As`, `net/http` `ResponseController`), BusyBox ash на роутере (`#!/bin/sh`, `set -e`, `trap ... EXIT`, параметрические подстановки вместо массивов), Vue 3 + TypeScript (axios).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 6 «Блок 3. Единое самообновление». Секции 1–3 — контекст и принятые решения; секция 5 (блок 2) выполнена и служит источником уже существующих API (`extendWriteDeadline`, `UpdateVPNConfig`, `WebUILogPath`).

**Status:** Tasks 1–7 done on `feature/webui-behavior`, HEAD `fcaf94e`; execution paused at the context threshold. Tasks 8–10 and «Проверка блока» remain. Task bodies above were trimmed after execution; the full text stays in git history. The SDD ledger with every ruling, deferred minor and review verdict is `.superpowers/sdd/2026-09-05-unified-updater/progress.md`.

---

## Global Constraints

- Ветка: `feature/webui-behavior`. Все четыре блока идут в одной ветке и одном PR, по порядку; блок 3 начинается с HEAD блока 2 (`ad03b7e` + docs-коммит `cbdb212`). Новых веток и worktree нет, `feature/unified-updater` из таблицы §2 спека **не создаётся**.
- `docs/superpowers/` в этом блоке **не удалять**: удаление одним `git rm` перед единственным PR после блока 4.
- Сборка, тесты, `vet`, `vue-tsc` — только через `claude-forge:build` (build-runner), включая исполнителей-субагентов. `bats`, `jq`, `git` — напрямую. Go-тесты всегда с `-count=1` (иначе пакет вернётся как `(cached)`).
- Git: в `git add` только явные пути; никаких `-A`, `.`, `commit -a`, `stash`, `clean`. В рабочем дереве лежат untracked-файлы автора (`.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `docs/superpowers/plans/*-prompt.md`) — они не должны попасть ни в один коммит.
- Каждый коммит заканчивается пустой строкой и `Claude-Session: <URL текущей сессии>` (используйте `-m "..." -m "Claude-Session: ..."`).
- Go-код gofmt-чистый в затронутых файлах (`gofmt -l <файлы>` пуст). **Не переформатировать файлы, которых задача не касается** — в модуле 11 исторически не-gofmt файлов.
- Выравнивание в литералах в тексте этого плана не нормализовано: после вставки кода прогоняйте `gofmt -w` по файлам своей задачи, прежде чем проверять `gofmt -l`.
- Комментарии, идентификаторы, тексты коммитов — по-английски, как в остальном репозитории. Общение с пользователем — по-русски.
- Тексты из спеки дословно: имена ассетов `telegram-bot-<arch>` и `webui-<arch>` для `arm64` и `arm`; init-скрипт `S98vpn-director-webui`; формат `notify.json` — `{"chat_id":0,"old_version":"v1.2.0","new_version":"v1.3.0","status":"ok","initiator":"webui"}`; сообщение о провале `Update failed: see /tmp/vpn-director-update/update.log`; баннер во фронте `Version vX available, see Settings`.
- Кеш проверки обновлений: 30 минут; `force` обходит кеш не чаще раза в минуту (GitHub без токена — 60 запросов в час).
- Опрос версии во фронте: каждые 3 секунды, `skipAuthRedirect`, потолок 5 минут, затем перезагрузка страницы.
- Скрипт обновления остаётся `#!/bin/sh` с `set -e`. Восстановление только через `trap ... EXIT` с проверкой кода выхода: ash не знает `ERR`.
- HTTP: глобальный `WriteTimeout` 30 с (блок 2). **Каждый новый маршрут, который синхронно ходит в GitHub, обязан продлевать дедлайн** через `extendWriteDeadline`.
- `service.ConfigStore` не содержит `SaveVPNConfig`; писать `vpn-director.json` можно только через `UpdateVPNConfig`. Блок 3 конфиг не пишет вообще: обновление **не должно** трогать `jwt_secret` — на этом держится обещание «cookie переживает обновление».
- `updatechecker` не меняется (спек §6.3).
- Первый переход на версию с этим блоком выполнит старый updater бота, который не трогает Web UI. Release notes этой версии просят один раз перезапустить `install.sh` — текст релиза не входит в объём плана, но упоминается в документации (Task 10).

---

## Отклонения от спека (согласованы с автором до написания плана)

1. **`Flow.Start` получает `initiator` явным параметром.** Спек §6.1 определяет `RunOptions.Initiator`, но сигнатура §6.3 `Start(ctx, chatID, progress)` не даёт его передать. Принято: `Start(ctx, initiator string, chatID int64, progress func(string))`.
2. **При `status: failed` каталог обновления не удаляется целиком.** Спек §6.6 велит и сослать пользователя на `/tmp/vpn-director-update/update.log`, и вычистить каталог после успешной отправки. Принято: при `failed` удаляется только `notify.json`, `update.log` остаётся; при `ok` — весь каталог, как в спеке.
3. **Три текста бота про валидацию версии сворачиваются в один.** Вместо «Invalid current version: X» / «Invalid release version: X» / «Failed to parse version: e» бот отвечает `Failed to start update: <ошибка>`. Путь достижим только при некорректном теге сборки. Остальные тексты сохранены дословно.
4. **Добавлено продление write-дедлайна на новых маршрутах** (`GET /api/update/check`, `POST /api/update`). В спеке шага нет; без него соединение рвётся на 30 с посреди запроса к GitHub API (у `updater` собственный таймаут 30 с).
5. **Текст «Update script started, bot will restart in a few seconds...» → «Update script started, the service will restart in a few seconds...»**: перезапускается уже не только бот.
6. **`GET /api/update/check` отвечает `{"update_available": false, "dev": true}` не только при `--dev`, но и для сборки с версией `dev`**: такая сборка не умеет сравнивать версии, и отдавать 500 на штатной ситуации незачем.

---

## Файловая структура

| Область | Файлы | Ответственность |
|---|---|---|
| Таблица демонов | `server/internal/updater/updater.go` | `Daemon`, `Daemons` — единственный источник имён ассетов, путей бинарников и init-скриптов |
| Загрузка | `server/internal/updater/downloader.go` (+`_test`) | `scriptFiles` с `S98vpn-director-webui`; `downloadBinaries` по одному бинарнику на демона; отсутствие ассета — ошибка |
| Запуск скрипта | `server/internal/updater/script.go` (+`_test`), `update_script.sh.tmpl`, `testdata/update_script.golden.sh` (новый) | `RunOptions`; рендер шаблона из таблицы демонов; golden-тест и `sh -n` |
| Интерфейс updater | `server/internal/updater/updater.go`, `server/internal/updater/github.go` | `RunUpdateScript(RunOptions)`; экспорт `APITimeout` |
| Оркестрация | `server/internal/updateflow/flow.go`, `start.go` (+`flow_test.go`, `start_test.go`) — новый пакет | `Check` (кеш 30 мин, порог `force`), `Start` (пре-флайт, лок, фоновая загрузка), `InProgress`, типизированные ошибки |
| Бот | `server/internal/handler/update.go` (+`_test`), `server/internal/handler/handler.go`, `server/internal/bot/bot.go` | `/update` как адаптер над `Flow`; удаление мёртвого `Deps.Updater` |
| Уведомление | `server/internal/startup/notify.go` (+`_test`), `server/internal/bot/bot.go` | `chat_id` 0 → все активные чаты; `status: failed`; правила очистки |
| Web API | `server/internal/webapi/handler_update.go` (новый, +`_test`), `router.go`, `deadline.go`, `handler_logs.go`, `handler_logs_test.go`, `test_helpers_test.go` | три маршрута, дедлайны, удаление заглушки 501 |
| Входная точка | `server/cmd/webui/main.go` | `updater.New()` + `updateflow.New` + `Deps.Update` |
| Фронт | `web/src/api.ts`, `web/src/types.ts`, `web/src/components/SettingsTab.vue`, `web/src/App.vue` | вкладка Settings, опрос версии, баннер в шапке |
| Документация | `README.md`, `README.ru.md`, `.claude/rules/telegram-bot.md` | поток обновления, оба демона, одноразовая миграция |

**Порядок задач обязателен:** 1 → 2 → 3 (таблица демонов до загрузки и до шаблона), 4 → 5 (Check до Start), 5 → 6 → 7 (Flow до адаптера бота, адаптер до уведомления), 5 → 8 → 9 (Flow до Web API, Web API до фронта). Задача 10 — последняя.

---

### Task 1: таблица демонов и загрузка обоих бинарников

✅ Done — see commit(s): `6b8f23a`

---

### Task 2: `RunOptions` вместо трёх позиционных аргументов

✅ Done — see commit(s): `0751e6c`

---

### Task 3: скрипт обновления работает со списком демонов

✅ Done — see commit(s): `b264824`, `25dd883`

---

### Task 4: пакет `updateflow` — проверка обновлений с кешем

✅ Done — see commit(s): `e35b050`, `686f301`

---

### Task 5: `updateflow.Start` — пре-флайт, лок, фоновое обновление

✅ Done — see commit(s): `eea9d98`, `d2c2df1`

---

### Task 6: `/update` бота становится адаптером над `Flow`

✅ Done — see commit(s): `76cf1f2`

---

### Task 7: уведомление после обновления для всех активных чатов

✅ Done — see commit(s): `fcaf94e`

---

### Task 8: маршруты `/api/update*` и подключение Web UI

**Files:**
- Create: `server/internal/webapi/handler_update.go`, `server/internal/webapi/handler_update_test.go`
- Modify: `server/internal/webapi/router.go` (поле `Deps.Update`, интерфейс `UpdateFlow`, три маршрута)
- Modify: `server/internal/webapi/deadline.go` (`githubDeadline`)
- Modify: `server/internal/webapi/handler_logs.go` (удалить заглушку `handleUpdate`)
- Modify: `server/internal/webapi/handler_logs_test.go` (удалить `TestHandleUpdate_NotImplemented`)
- Modify: `server/internal/webapi/test_helpers_test.go` (мок `UpdateFlow` + поле в `newTestDeps`)
- Modify: `server/internal/updater/github.go` (экспорт `APITimeout`)
- Modify: `server/cmd/webui/main.go` (создать `Flow`, положить в `Deps`)

**Interfaces:**
- Consumes: `updateflow.Flow`, `CheckResult`, `StartResult`, ошибки (Tasks 4–5); `extendWriteDeadline` (блок 2).
- Produces: `webapi.UpdateFlow` (интерфейс-потребитель); `Deps.Update UpdateFlow`; `updater.APITimeout`.

- [ ] **Step 1: Написать падающие тесты**

Создать `server/internal/webapi/handler_update_test.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

func TestHandleUpdateCheck_ReturnsTheResult(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.checkResult = updateflow.CheckResult{
		Current:         "v1.2.0",
		Latest:          "v1.3.0",
		UpdateAvailable: true,
		Changelog:       "what is new",
		CheckedAt:       time.Now(),
	}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["latest"] != "v1.3.0" || resp["update_available"] != true {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateCheck_ForcePassedThrough(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check?force=1", nil))

	if !flow.checkForce {
		t.Error("force=1 must reach the flow, otherwise the button does nothing")
	}
}

func TestHandleUpdateCheck_DevBuild(t *testing.T) {
	for _, devErr := range []error{updateflow.ErrDevMode, updateflow.ErrDevVersion} {
		deps := newTestDeps(t)
		deps.Update.(*mockUpdateFlow).checkErr = devErr

		rec := httptest.NewRecorder()
		handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %v, got %d", devErr, rec.Code)
		}
		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if resp["dev"] != true || resp["update_available"] != false {
			t.Errorf("unexpected dev body: %v", resp)
		}
	}
}

func TestHandleUpdateCheck_GitHubDown(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).checkErr = &updateflow.GitHubError{Err: errors.New("connection refused")}

	rec := httptest.NewRecorder()
	handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", "/api/update/check", nil))

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestHandleUpdateStart_Accepted(t *testing.T) {
	deps := newTestDeps(t)
	flow := deps.Update.(*mockUpdateFlow)
	flow.startResult = updateflow.StartResult{From: "v1.2.0", To: "v1.3.0"}

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rec.Code, rec.Body.String())
	}
	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["from"] != "v1.2.0" || resp["to"] != "v1.3.0" {
		t.Errorf("unexpected body: %v", resp)
	}
	if flow.startInitiator != "webui" || flow.startChatID != 0 {
		t.Errorf("Start(initiator=%q, chat=%d), want webui/0", flow.startInitiator, flow.startChatID)
	}
}

func TestHandleUpdateStart_StatusPerOutcome(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "up to date", err: updateflow.ErrUpToDate, want: http.StatusOK},
		{name: "dev mode", err: updateflow.ErrDevMode, want: http.StatusBadRequest},
		{name: "dev build", err: updateflow.ErrDevVersion, want: http.StatusBadRequest},
		{name: "in progress", err: updateflow.ErrInProgress, want: http.StatusConflict},
		{name: "github down", err: &updateflow.GitHubError{Err: errors.New("timeout")}, want: http.StatusBadGateway},
		{name: "anything else", err: errors.New("invalid release version"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Update.(*mockUpdateFlow).startErr = tt.err

			rec := httptest.NewRecorder()
			handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

			if rec.Code != tt.want {
				t.Fatalf("expected %d, got %d: %s", tt.want, rec.Code, rec.Body.String())
			}
		})
	}
}

func TestHandleUpdateStart_UpToDateBody(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).startErr = updateflow.ErrUpToDate

	rec := httptest.NewRecorder()
	handleUpdateStart(deps)(rec, httptest.NewRequest("POST", "/api/update", nil))

	var resp map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true || resp["update_available"] != false {
		t.Errorf("unexpected body: %v", resp)
	}
}

func TestHandleUpdateStatus(t *testing.T) {
	deps := newTestDeps(t)
	deps.Update.(*mockUpdateFlow).inProgress = true

	rec := httptest.NewRecorder()
	handleUpdateStatus(deps)(rec, httptest.NewRequest("GET", "/api/update/status", nil))

	var resp map[string]bool
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp["in_progress"] {
		t.Error("in_progress must follow the flow")
	}
}
```

Добавить мок в `server/internal/webapi/test_helpers_test.go`:

```go
// mockUpdateFlow implements UpdateFlow for testing.
type mockUpdateFlow struct {
	checkResult updateflow.CheckResult
	checkErr    error
	checkForce  bool

	startResult    updateflow.StartResult
	startErr       error
	startInitiator string
	startChatID    int64

	inProgress bool
}

func (m *mockUpdateFlow) Check(_ context.Context, force bool) (updateflow.CheckResult, error) {
	m.checkForce = force
	return m.checkResult, m.checkErr
}

func (m *mockUpdateFlow) Start(_ context.Context, initiator string, chatID int64, _ func(string)) (updateflow.StartResult, error) {
	m.startInitiator = initiator
	m.startChatID = chatID
	return m.startResult, m.startErr
}

func (m *mockUpdateFlow) InProgress() bool { return m.inProgress }
```

и в `newTestDeps` добавить в литерал `Deps`:

```go
		Update:  &mockUpdateFlow{},
```

Импорты теста дополнить `context` и `updateflow`.

Удалить `TestHandleUpdate_NotImplemented` из `server/internal/webapi/handler_logs_test.go`.

- [ ] **Step 2: Запустить и убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ -count=1`
Ожидание: FAIL — `Deps.Update` не существует, обработчиков нет.

- [ ] **Step 3: Экспортировать таймаут GitHub API**

В `server/internal/updater/github.go` переименовать константу и её единственное использование (строка 35):

```go
	// APITimeout bounds one GitHub API call. Handlers that make such a call
	// inside an HTTP request size their response deadline from it.
	APITimeout = 30 * time.Second
```

```go
	ctx, cancel := context.WithTimeout(ctx, APITimeout)
```

- [ ] **Step 4: Добавить дедлайн и обработчики**

В `server/internal/webapi/deadline.go` добавить в блок констант и импорт `updater`:

```go
	// githubDeadline covers the synchronous part of an update route: one
	// GitHub API call under updater.APITimeout. POST /api/update answers 202
	// as soon as the download goroutine is under way, so the script's own
	// minutes never sit on the connection.
	githubDeadline = updater.APITimeout + deadlineSlack
```

Создать `server/internal/webapi/handler_update.go`:

```go
package webapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/updateflow"
)

// handleUpdateCheck reports the latest release. force=1 bypasses the flow's
// 30-minute cache, at most once a minute. A dev build cannot compare versions,
// so it answers with the dev marker instead of an error the UI would have to
// explain.
func handleUpdateCheck(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Check(r.Context(), r.URL.Query().Get("force") == "1")
		if err == nil {
			jsonOK(w, res)
			return
		}
		if errors.Is(err, updateflow.ErrDevMode) || errors.Is(err, updateflow.ErrDevVersion) {
			jsonOK(w, map[string]interface{}{"update_available": false, "dev": true})
			return
		}
		var ghErr *updateflow.GitHubError
		if errors.As(err, &ghErr) {
			jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
			return
		}
		slog.Warn("update check failed", "error", err)
		jsonError(w, http.StatusInternalServerError, "failed to check for updates")
	}
}

// handleUpdateStart launches the unified self-update. It answers 202 as soon
// as the download starts: the update script stops this process a few seconds
// later, so the client learns the outcome by polling /api/version.
func handleUpdateStart(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		extendWriteDeadline(w, githubDeadline)

		res, err := deps.Update.Start(r.Context(), "webui", 0, func(line string) {
			slog.Info("self-update", "status", line)
		})
		if err == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"ok":   true,
				"from": res.From,
				"to":   res.To,
			})
			return
		}

		switch {
		case errors.Is(err, updateflow.ErrUpToDate):
			jsonOK(w, map[string]interface{}{"ok": true, "update_available": false})
		case errors.Is(err, updateflow.ErrDevMode), errors.Is(err, updateflow.ErrDevVersion):
			jsonError(w, http.StatusBadRequest, err.Error())
		case errors.Is(err, updateflow.ErrInProgress):
			jsonError(w, http.StatusConflict, err.Error())
		default:
			var ghErr *updateflow.GitHubError
			if errors.As(err, &ghErr) {
				jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
				return
			}
			slog.Warn("update start failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to start update: "+err.Error())
		}
	}
}

// handleUpdateStatus reports whether an update script is running, so a client
// that reconnected after a restart can tell "still updating" from "done".
func handleUpdateStatus(deps *Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		jsonOK(w, map[string]bool{"in_progress": deps.Update.InProgress()})
	}
}
```

Удалить `handleUpdate` из `server/internal/webapi/handler_logs.go` (строки 94–100).

- [ ] **Step 5: Подключить маршруты и зависимость**

В `server/internal/webapi/router.go` добавить импорты `context` и `updateflow`, интерфейс и поле:

```go
// UpdateFlow is the subset of updateflow.Flow the API handlers use. Declared
// here so the handler tests can drive every outcome without a fake GitHub.
type UpdateFlow interface {
	Check(ctx context.Context, force bool) (updateflow.CheckResult, error)
	Start(ctx context.Context, initiator string, chatID int64, progress func(string)) (updateflow.StartResult, error)
	InProgress() bool
}
```

В структуре `Deps` после `Logs`:

```go
	Update       UpdateFlow        // self-update orchestration, shared with the bot
```

Заменить в `registerProtectedRoutes` блок `// Self-update`:

```go
	// Self-update
	mux.HandleFunc("GET /api/update/check", handleUpdateCheck(deps))
	mux.HandleFunc("POST /api/update", handleUpdateStart(deps))
	mux.HandleFunc("GET /api/update/status", handleUpdateStatus(deps))
```

В `server/cmd/webui/main.go` добавить импорты `updateflow` и `updater`, создать поток перед `deps` и положить в литерал:

```go
	// The Web UI updates both daemons through the same flow as the bot; the
	// progress lines go to the Web UI log, since there is no chat to answer in.
	updateFlow := updateflow.New(updater.New(), Version, *devFlag)
```

```go
		Update:  updateFlow,
```

- [ ] **Step 6: Запустить тесты**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ ./internal/updater/ -count=1 -v`
Ожидание: PASS; ни один тест не ссылается на удалённую заглушку 501.

- [ ] **Step 7: Проверить сборку, vet и формат**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && gofmt -l internal/webapi/handler_update.go internal/webapi/handler_update_test.go internal/webapi/router.go internal/webapi/deadline.go internal/webapi/handler_logs.go internal/webapi/handler_logs_test.go internal/webapi/test_helpers_test.go internal/updater/github.go cmd/webui/main.go`
Ожидание: чисто.

- [ ] **Step 8: Проверить dev-режим руками**

```bash
cd server && go run ./cmd/webui --dev
```
В другом терминале: залогиниться (`admin`/`admin`) и запросить `POST /api/update` — ожидается 400 и текст `update is not available in dev mode`; `GET /api/update/check` — 200 с `{"update_available":false,"dev":true}`. Остановить сервер.

- [ ] **Step 9: Коммит**

```bash
git add server/internal/webapi/handler_update.go server/internal/webapi/handler_update_test.go server/internal/webapi/router.go server/internal/webapi/deadline.go server/internal/webapi/handler_logs.go server/internal/webapi/handler_logs_test.go server/internal/webapi/test_helpers_test.go server/internal/updater/github.go server/cmd/webui/main.go
git commit -m "feat(webapi): replace the update stub with the real check, start and status routes" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 9: вкладка Settings и баннер о новой версии

**Files:**
- Modify: `web/src/api.ts`, `web/src/types.ts`
- Modify: `web/src/components/SettingsTab.vue`
- Modify: `web/src/App.vue`

**Interfaces:**
- Consumes: `GET /api/update/check`, `POST /api/update`, `GET /api/version` (Task 8).
- Produces: ничего для Go-кода.

- [ ] **Step 1: Расширить клиент API и типы**

В `web/src/api.ts` заменить блок `// System`:

```ts
  // Self-update
  checkUpdate: (force = false) =>
    api.get('/api/update/check', { params: force ? { force: 1 } : {} }),
  update: () =>
    api.post('/api/update'),
  updateStatus: () =>
    api.get('/api/update/status'),
  // pollVersion is used while the server restarts: connection errors and a
  // brief 401 must not bounce the user to the login page.
  pollVersion: () =>
    api.get('/api/version', { skipAuthRedirect: true } as any),
```

В `web/src/types.ts` добавить:

```ts
export interface UpdateCheckResponse {
  current?: string
  latest?: string
  update_available: boolean
  changelog?: string
  checked_at?: string
  dev?: boolean
}

export interface UpdateStartResponse {
  ok: boolean
  from?: string
  to?: string
  update_available?: boolean
}
```

- [ ] **Step 2: Переписать вкладку Settings**

Заменить блок `<script setup>` и кнопку обновления в `web/src/components/SettingsTab.vue`:

```vue
<script setup lang="ts">
import { ref, computed, onMounted } from 'vue'
import api from '../api'
import type { VersionResponse, UpdateCheckResponse, UpdateStartResponse } from '../types'

const versionInfo = ref<VersionResponse | null>(null)
const updateInfo = ref<UpdateCheckResponse | null>(null)
const config = ref('')
const showConfig = ref(false)
const showChangelog = ref(false)
const loading = ref(false)
const checking = ref(false)
const updating = ref(false)
const updateMessage = ref('')
const error = ref('')

const canUpdate = computed(() => !!updateInfo.value?.update_available && !updating.value)

async function loadVersion() {
  try {
    const resp = await api.getVersion()
    versionInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  }
}

async function loadUpdate(force = false) {
  checking.value = true
  try {
    const resp = await api.checkUpdate(force)
    updateInfo.value = resp.data
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    checking.value = false
  }
}

async function loadConfig() {
  loading.value = true
  try {
    const resp = await api.getConfig()
    config.value = JSON.stringify(resp.data, null, 2)
  } catch (e: any) {
    error.value = e.response?.data?.error || e.message
  } finally {
    loading.value = false
  }
}

async function toggleConfig() {
  showConfig.value = !showConfig.value
  if (showConfig.value && !config.value) {
    await loadConfig()
  }
}

const sleep = (ms: number) => new Promise((resolve) => setTimeout(resolve, ms))

// waitForVersion polls /api/version until the new build answers or five
// minutes pass. Connection errors are expected: the server is restarting.
async function waitForVersion(target: string) {
  const deadline = Date.now() + 5 * 60 * 1000
  while (Date.now() < deadline) {
    await sleep(3000)
    try {
      const resp = await api.pollVersion()
      if (resp.data?.version === target) {
        return true
      }
    } catch {
      // server is down mid-restart, keep waiting
    }
  }
  return false
}

async function doUpdate() {
  const target = updateInfo.value?.latest
  if (!target) return
  if (!confirm(`Update VPN Director to ${target}?`)) return

  updating.value = true
  updateMessage.value = 'Starting update...'
  try {
    const resp = await api.update()
    const data = resp.data as UpdateStartResponse
    if (data.update_available === false) {
      updateMessage.value = 'Already running the latest version.'
      updating.value = false
      return
    }
    updateMessage.value = 'Updating, the server is restarting...'
    const arrived = await waitForVersion(data.to || target)
    if (arrived) {
      // Reload so the browser picks up the new bundle.
      window.location.reload()
      return
    }
    updateMessage.value = 'The new version did not come up within 5 minutes. Check the logs.'
  } catch (e: any) {
    updateMessage.value = 'Error: ' + (e.response?.data?.error || e.message)
  } finally {
    updating.value = false
  }
}

onMounted(async () => {
  await loadVersion()
  await loadUpdate()
})
</script>
```

Заменить блок карточки версии и кнопки в шаблоне:

```vue
  <div class="card">
    <div class="card-title">Version</div>
    <p v-if="error" class="error-msg">{{ error }}</p>
    <div v-if="versionInfo">
      <div class="kv">
        <span class="kv-label">Version</span>
        <span>{{ versionInfo.version }}</span>
      </div>
      <div class="kv">
        <span class="kv-label">Commit</span>
        <span style="font-family: monospace; font-size: 0.85rem;">{{ versionInfo.commit }}</span>
      </div>
      <div class="kv" v-if="updateInfo && !updateInfo.dev">
        <span class="kv-label">Latest</span>
        <span>{{ updateInfo.latest || '—' }}</span>
      </div>
      <div class="kv" v-if="updateInfo?.dev">
        <span class="kv-label">Latest</span>
        <span>dev build, updates disabled</span>
      </div>
    </div>
    <p v-else style="color: #999; font-size: 0.875rem;">Loading...</p>

    <div class="actions">
      <button class="btn" :disabled="checking || updating" @click="loadUpdate(true)">
        {{ checking ? '...' : '⟳ Check for updates' }}
      </button>
      <button class="btn btn-primary" :disabled="!canUpdate" @click="doUpdate">
        {{ updating ? 'Updating...' : `⬆ Update to ${updateInfo?.latest || ''}` }}
      </button>
      <button
        v-if="updateInfo?.changelog"
        class="btn btn-blue"
        @click="showChangelog = !showChangelog"
      >
        {{ showChangelog ? 'Hide changelog' : 'Changelog' }}
      </button>
    </div>
    <p v-if="updateMessage" style="font-size: 0.875rem;">{{ updateMessage }}</p>
    <pre
      v-if="showChangelog && updateInfo?.changelog"
      style="font-size: 12px; white-space: pre-wrap; line-height: 1.5; max-height: 300px; overflow-y: auto; background: #1a1a2e; padding: 0.75rem; border-radius: 4px; border: 1px solid #333;"
    >{{ updateInfo.changelog }}</pre>
  </div>
```

Удалить прежний отдельный блок `<div class="actions">` с кнопкой `⬆ Update` (он переехал внутрь карточки).

- [ ] **Step 3: Добавить баннер в шапку**

В `web/src/App.vue` в `<script setup>` добавить состояние и запрос после успешной аутентификации:

```ts
const updateBanner = ref('')

async function loadUpdateBanner() {
  try {
    const resp = await api.checkUpdate()
    if (resp.data?.update_available && resp.data?.latest) {
      updateBanner.value = `Version ${resp.data.latest} available, see Settings`
    }
  } catch {
    // a failed check must not get in the way of the UI
  }
}
```

и вызвать его из `checkAuth` после `authenticated.value = true`:

```ts
    authenticated.value = true
    loadUpdateBanner()
```

В шаблоне добавить строку в `topbar-info` перед версией:

```vue
        <span v-if="updateBanner" class="update-banner" @click="activeTab = 'settings'">
          {{ updateBanner }}
        </span>
```

и стиль в `web/src/style.css`:

```css
.update-banner {
  cursor: pointer;
  color: #f0a020;
  font-size: 0.85rem;
}
```

- [ ] **Step 4: Собрать фронт**

Через `claude-forge:build`: `cd web && npm run build`
Ожидание: `vue-tsc` без ошибок, сборка успешна.

- [ ] **Step 5: Проверить в dev-режиме**

```bash
cd server && go run ./cmd/webui --dev
```
и в другом терминале `cd web && npm run dev`. Проверить по чек-листу:
1. Вкладка Settings показывает версию и «dev build, updates disabled»;
2. Кнопка «Update to …» неактивна;
3. «Check for updates» не роняет страницу;
4. Баннер в шапке не появляется (dev).
Остановить оба процесса.

- [ ] **Step 6: Коммит**

```bash
git add web/src/api.ts web/src/types.ts web/src/components/SettingsTab.vue web/src/App.vue web/src/style.css
git commit -m "feat(webui): self-update from the Settings tab with a version banner" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 10: документация блока

**Files:**
- Modify: `README.md`, `README.ru.md`
- Modify: `.claude/rules/telegram-bot.md`

**Interfaces:**
- Consumes: поведение из Tasks 1–9.
- Produces: ничего.

- [ ] **Step 1: Обновить `.claude/rules/telegram-bot.md`**

Заменить секцию `## Self-Update` целиком (внешний блок ниже — на четырёх обратных кавычках, потому что внутри есть вложенный блок кода):

````markdown
## Self-Update (`/update`)

Both daemons — `telegram-bot` and `webui` — are updated together, from the bot
or from the Web UI. The orchestration lives in `internal/updateflow`; the bot
command and the Web UI handlers are adapters over it.

1. `Flow.Check` asks the GitHub API for the latest release (result cached for
   30 minutes; a forced check pierces the cache at most once a minute)
2. `Flow.Start` creates the lock file (`/tmp/vpn-director-update/lock`)
3. Release assets go to `/tmp/vpn-director-update/files/`: every script from
   `scriptFiles` plus one binary per daemon (`telegram-bot-<arch>`,
   `webui-<arch>`). A release missing either binary is a download error.
4. `update.sh` is generated from the daemon table and run detached
5. The script remembers which daemons were running, stops them, copies
   everything, writes `notify.json` and starts back exactly those daemons
6. On failure an `EXIT` trap restarts the daemons that were running and writes
   `notify.json` with `"status": "failed"`

`notify.json`:

```json
{"chat_id": 0, "old_version": "v1.2.0", "new_version": "v1.3.0",
 "status": "ok", "initiator": "webui"}
```

`chat_id` 0 marks an update started from the Web UI: on its next start the bot
notifies every active chat. A successful notification clears
`/tmp/vpn-director-update`; a failed one keeps `update.log`, because the
message points at it.

**Dev mode**: `/update` is disabled with `--dev` and for a `dev` build.
````

- [ ] **Step 2: Обновить `README.md`**

В таблицу вкладок Web UI (строка про **Settings**) заменить описание:

```markdown
| **Settings** | Version, self-update, configuration |
```

Добавить в раздел `## Web UI` после `### Service Management` новый подраздел:

```markdown
### Updates

The **Settings** tab shows the running version, the latest GitHub release and
its changelog. «Update to vX» downloads the release and restarts both the Web
UI and the Telegram bot; the page polls for the new version and reloads itself
when it comes up. The login session survives the update.

An update started from the Web UI is announced in Telegram to every chat the
bot has talked to. `/update` in the bot does the same thing from the other
side — both paths update both daemons.

> Upgrading **to** the first release with the unified updater is still done by
> the old bot-only updater, which does not know about the Web UI. Run
> `install.sh` once after that upgrade; every later update handles both.
```

В таблице команд бота строка `/update` остаётся как есть.

- [ ] **Step 3: Обновить `README.ru.md`**

Заменить строку 118 таблицы возможностей:

```markdown
| **Settings** | Версия, самообновление, конфигурация |
```

Добавить после подраздела `### Управление сервисом` (строка 138 и далее) новый подраздел:

```markdown
### Обновления

Вкладка **Settings** показывает текущую версию, последний релиз на GitHub и его
changelog. Кнопка «Update to vX» скачивает релиз и перезапускает и Web UI, и
Telegram-бота; страница опрашивает версию и сама перезагружается, когда новая
сборка поднялась. Сессия входа переживает обновление.

Об обновлении, запущенном из Web UI, бот сообщает в Telegram во все чаты, с
которыми он общался. Команда `/update` в боте делает то же самое с другой
стороны — оба пути обновляют оба демона.

> Переход **на** первый релиз с единым обновлением выполняет прежний
> обновлятор бота, который не знает про Web UI. После этого перехода один раз
> запустите `install.sh`; все последующие обновления охватывают оба демона.
```

- [ ] **Step 4: Проверить, что документация не расходится с кодом**

```bash
grep -n 'S98vpn-director-webui' server/internal/updater/downloader.go
grep -n 'webui' server/internal/updater/updater.go
grep -rn 'update/check\|update/status' server/internal/webapi/router.go
```
Ожидание: init-скрипт в списке загрузки, демон `webui` в таблице, оба новых маршрута зарегистрированы — ровно то, что обещает документация.

- [ ] **Step 5: Коммит**

```bash
git add README.md README.ru.md .claude/rules/telegram-bot.md
git commit -m "docs: describe the unified self-update of both daemons" -m "Claude-Session: <URL текущей сессии>"
```

---

## Проверка блока (после Task 10, без коммита кода)

- [ ] **Step 1: Полный прогон Go**

Через `claude-forge:build`: `cd server && go build ./... && go vet ./... && go test ./... -count=1`
Ожидание: PASS без строк `(cached)`.

- [ ] **Step 2: Гонки в новом пакете**

Через `claude-forge:build`: `cd server && go test ./internal/updateflow/ ./internal/webapi/ ./internal/handler/ -count=1 -race`
Ожидание: PASS.

- [ ] **Step 3: Фронт и bats**

Через `claude-forge:build`: `cd web && npm run build`
Напрямую: `bats router/test/unit`
Ожидание: обе проверки зелёные (блок 3 не трогает shell роутера, bats — регрессия).

- [ ] **Step 4: Формат**

Через `claude-forge:build`: `cd server && gofmt -l $(git diff --name-only cbdb212..HEAD -- '*.go' | sed 's|^server/||')`
Ожидание: пусто. Файлы вне диффа не проверять и не форматировать.

- [ ] **Step 5: Чистота дерева**

```bash
git status --short
```
Ожидание: в изменённых/добавленных нет ни одного файла автора (`.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `.claude/settings.local.json`, `docs/superpowers/plans/*-prompt.md`).

- [ ] **Step 6: Ручной чек-лист на роутере (выполняет человек, до PR)**

1. Собрать и разложить бинарники вручную, запустить оба демона.
2. `POST /api/update/check` из браузера — показывает последний релиз.
3. Запустить обновление из Web UI: страница переходит в «Updating…», через несколько десятков секунд перезагружается с новой версией, повторный логин не требуется.
4. В Telegram приходит «Update complete: … → …» без запроса `/update`.
5. `/opt/etc/init.d/S98vpn-director-webui check` и `S98telegram-bot check` — оба running.
6. Сценарий отказа: временно переименовать `/opt/vpn-director/lib` перед обновлением, убедиться, что демоны вернулись и в Telegram пришло «Update failed: see /tmp/vpn-director-update/update.log», а сам лог на месте.
7. Проверить, что демон, остановленный до обновления, после обновления остался остановленным.
