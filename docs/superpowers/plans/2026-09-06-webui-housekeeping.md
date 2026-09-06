# Web UI: безопасность и хозяйство (блок 4) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть последний блок работы над Web UI: привязать JWT к текущему паролю роутера, довести установщик до работающего Web UI без ручного шага, поднять CI до реальной проверки трёх стеков, свести документацию всех четырёх блоков и разобрать долг, накопленный ревью блоков 2 и 3.

**Architecture:** `ShadowAuth` учится считать отпечаток хеша пароля; `JWTService` кладёт его в claim `pwh`, а `authMiddleware` сверяет с текущим на каждом запросе — токен живёт ровно до смены пароля. Установщик получает `start_webui` и становится тестируемым через `--source-only`, как остальные скрипты репозитория. CI распадается на три параллельные джобы (Go, web, bats), для чего пакет `updater` сначала уводится с сети: боевой хост `raw.githubusercontent.com` становится инъектируемым, как уже инъектируемы API-хост, архитектура и интерпретатор. Фронт получает типизированный API-клиент, а состояние обновления переезжает из компонента в модуль, чтобы уход с вкладки Settings не убивал прогресс.

**Tech Stack:** Go 1.25 (`crypto/sha256`, `encoding/hex`, `errors.Is`, `regexp`, `httptest.NewTLSServer`), `github.com/golang-jwt/jwt/v5`, Bash 5 (`install.sh`, `lib/common.sh`), BusyBox ash (init-скрипты), Bats + bats-support/bats-assert, GitHub Actions, Vue 3 + TypeScript (axios, `vue-tsc`).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 7 «Блок 4. Безопасность и хозяйство». Секции 1–3 — контекст и принятые решения; §8 — таблица затрагиваемых файлов; §9 — риски. Секции 4–6 (блоки 1–3) выполнены и служат источником уже существующих API.

**Status:** Tasks 1–7 done (HEAD `6f97c8e`), tasks 8–16 and «Проверка блока» remain. План написан на `feature/webui-behavior` при HEAD `83ce86a`; код блока 3 закончен на `f0d30e9`. Ход выполнения, принятые решения и отложенные мелочи — в `.superpowers/sdd/2026-09-06-webui-housekeeping/progress.md` (каталог gitignored).

---

## Global Constraints

- Ветка: `feature/webui-behavior`. Все четыре блока идут в одной ветке и одном PR. Новых веток и worktree нет; `feature/webui-housekeeping` из таблицы §2 спека **не создаётся**.
- `docs/superpowers/` удаляется одним `git rm` **после** этого блока, перед единственным PR. В самом блоке не удалять.
- Сборка, тесты, `vet`, `vue-tsc` — только через `claude-forge:build` (build-runner), включая исполнителей-субагентов. `bats`, `jq`, `git` — напрямую. Go-тесты всегда с `-count=1` (иначе пакет вернётся как `(cached)`).
- Git: в `git add` только явные пути; никаких `-A`, `.`, `commit -a`, `stash`, `clean`. В рабочем дереве лежат файлы автора, которые не должны попасть ни в один коммит: изменённый `.claude/settings.local.json`; неотслеживаемые `.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `docs/superpowers/plans/*-prompt.md`. После каждого коммита проверять `git show --name-only HEAD`.
- Каждый коммит заканчивается вторым `-m` с трейлером `Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y`.
- Go-код gofmt-чистый в затронутых файлах (`gofmt -l <файлы>` пуст). **Не переформатировать файлы, которых задача не касается** — в модуле исторически есть не-gofmt файлы.
- Комментарии, идентификаторы, тексты коммитов, названия тестов — по-английски. Общение с пользователем — по-русски.
- Тестового фреймворка для Vue нет и в этом блоке не заводится (спек §2, «вне объёма»). Фронтовые задачи проверяются `npm run build` (он же `vue-tsc -b`) и ручным чек-листом в конце плана.
- `service.ConfigStore` не содержит `SaveVPNConfig`; `vpn-director.json` пишется только через `UpdateVPNConfig`. Блок 4 конфиг не пишет вообще.
- `updateflow` (`Check`, `Start`, `InProgress`, четыре сентинела, `GitHubError`) закончен и отревьюирован в блоке 3. Менять его — значит переоткрывать дизайн блока 3; ни одна задача этого не делает.
- `updatechecker` спек §6.3 оставил без изменений. Задача 7 трогает в нём ровно одну функцию (`notifyUsers`) ради дедупликации получателей — это осознанное решение, а не расширение объёма; кеш и тикер не трогаются.
- Ревьюеры пишут полный отчёт в `task-N-review.md` и отвечают кратким резюме: почтовый ящик обрезает длинные сообщения.
- Каждый фикс сопровождается mutation-доказательством: сломать свойство, показать красный тест, откатить.
- Тексты из спека дословно: claim `pwh`; отпечаток = первые 8 байт SHA-256 от хеша пароля в hex; `go-version-file: server/go.mod`; `test.yml` гоняет `go vet ./...` и `go test ./...` в `server/`, `npm ci` и `npm run build` в `web/`, `bats router/test/unit`.

---

## Отклонения от спека (согласованы с автором до написания плана)

1. **§7.1 не описывает путь ошибки; принято разделение 500 / 401.** `ShadowAuth.Fingerprint` отдаёт сентинел `ErrUserNotFound` для отсутствующего, заблокированного или беспарольного аккаунта и обычную ошибку для нечитаемого файла. `authMiddleware`: ошибка чтения файла → 500 «authentication error» (как `handleLogin` при ошибке `Verify`), `ErrUserNotFound` → 401, несовпадение или отсутствие claim → 401. Причина: единый 401 на нечитаемом `/etc/shadow` отправил бы SPA через интерцептор на страницу логина, где логин отвечает 500, и пользователь увидел бы цикл перезагрузок вместо диагностируемой ошибки.
2. **§7.3 требует `bats router/test/unit` в CI, но `router/test/test_helper.bash:4-5` грузит библиотеки по абсолютным путям `/usr/lib/bats/bats-{support,assert}/load.bash`.** Принято: `test.yml` ставит их ровно туда, шагом, дословно скопированным из уже работающего `.github/workflows/ipset-sources.yml`. `test_helper.bash` не трогается — от него зависят все 135 зелёных тестов.
3. **§7.3 `go test ./...` в CI ушёл бы в сеть.** `downloadScriptFile` строит URL из константы `repoRawURL`, а не из инъектируемого `baseURL` (тот покрывает только GitHub API), поэтому `TestDownloadRelease_CleansBeforeDownload` делает 19 реальных запросов к `raw.githubusercontent.com`. Принято: добавить `Service.rawBaseURL` по образцу трёх уже существующих тестовых швов (`baseURL`, `archSuffix`, `shell`) и увести тест на `httptest`. Заодно закрывает пункт триажа блока 3 и убирает ~15 с из каждого прогона.
4. **Проверка `https` ставится в `downloadBinaries`, а не в `downloadFile`.** Рекомендация ревью блока 3 говорит про URL ассета из ответа GitHub API. В `downloadFile` тот же чек убил бы все тесты со скриптами, которые ходят на `httptest.NewServer` по `http://`. Тесты ассетов переводятся на `httptest.NewTLSServer` + `server.Client()`, то есть начинают проверять настоящий https.
5. **`set -f` в `update_script.sh.tmpl` не добавляется; вместо него — тест на таблицу демонов.** Пункт триажа блока 3 «no `set -f`» выглядит однострочным, но им не является: шаги 4 и 5 скрипта копируют и раздают права через шесть `cp` и четыре `chmod` с глобами (`"$FILES_DIR/opt/vpn-director/"*.sh`, `/opt/vpn-director/*.sh`, …). Глобальный `set -f` сломал бы боевой путь копирования — самую опасную часть обновления — ради латентной проблемы. Принято: закрыть инвариант там, где он возникает, — тестом `TestDaemons_CarryNoShellMetacharacters` в Go, ноль риска для shell.
6. **`set -e`-переразбор таблицы демонов в семи местах (Task 3 M3 блока 3) не делается.** Триаж финального ревью блока 3 сам говорит «refactor when a fourth field is actually needed; doing it now is churn against a byte-exact golden». Четвёртого поля нет. Пункт остаётся открытым и явно назван в `.claude/rules/webui.md`, чтобы следующий, кто добавит поле, знал цену.
7. **`deadlineSlack` поднимается с 30 с до 60 с.** Спек §5.3 фиксирует «плюс 30 секунд», но в дереве это даёт отрицательный запас: `applyDeadline` 5 м 30 с против худшего случая 5 м 40 с (30 с ожидания флока + 5 мин команды + 10 с `WaitDelay`), и `logsDeadline` 70 с против 80 с. Один изменённый литерал закрывает Minor 4 финального ревью блока 2 и остаток фикс-волны разом.
8. **§7.4 в части README уже выполнен блоками 1–3 и работы не порождает.** Строка вкладки Logs (`README.md:117`, `README.ru.md:117`), раздел Web UI с `log_level` (:136) и раздел про обновление (:148-152) в дереве есть в обоих языках. Из README остаётся только то, чего в спеке нет: фраза о модели доверия обновления (рекомендация №6 финального ревью блока 3).

---

## Файловая структура

| Область | Файлы | Ответственность |
|---|---|---|
| Отпечаток пароля | `server/internal/auth/shadow.go`, `shadow_test.go` | `Fingerprint`, `ErrUserNotFound`, общий предикат «хеш непригоден» |
| Токен | `server/internal/auth/jwt.go`, `jwt_test.go` | claim `pwh` в `Create`/`Validate`, поле `Claims.PasswordHash` |
| Проверка на каждом запросе | `server/internal/webapi/middleware.go`, `router.go`, `handler_auth.go` (+ три `_test`), `test_helpers_test.go`, `router_test.go` | сверка `pwh` с текущим отпечатком, коды 401/500, тестовая теневая фикстура |
| Установщик | `install.sh`, `router/test/unit/install.bats` (новый), `router/test/mocks/nvram` | `start_webui`, next steps, `--source-only` для тестируемости, `log_level` в jq-секции |
| Обновлятор без сети | `server/internal/updater/downloader.go`, `updater.go`, `downloader_test.go` | `rawBaseURL`, `https`-only для ассетов |
| Обновлятор: пины | `server/internal/updater/downloader_test.go`, `updater_test.go`, `script_test.go` | боевая ветка `getArchSuffix`, дрейф `scriptFiles` ↔ `install.sh`, метасимволы в таблице демонов |
| CI | `.github/workflows/telegram-bot.yml`, `.github/workflows/test.yml` (новый) | версия Go из `go.mod`; три джобы на push и PR |
| Уведомления | `server/internal/startup/notify.go`, `notify_test.go`, `server/internal/updatechecker/checker.go`, `checker_test.go` | дедупликация получателей по `ChatID` |
| Дедлайны | `server/internal/webapi/deadline.go`, `deadline_test.go`, `handler_logs.go`, `handler_status.go` | `deadlineSlack` 60 с, `logsDeadline` из `deps.LogPaths`, продление `/api/ip` |
| Мелочи webapi | `server/internal/webapi/handler_update.go`, `handler_update_test.go`, `handler_servers.go` | общий 502-хелпер, негативный кейс `force`, снятие двойной обёртки |
| Мелочи Go | `server/internal/shell/shell.go`, `shell_test.go`, `server/internal/handler/update_test.go` | обёртка `ctx.Err()`, честные имена `TestExecContext_*`, счётчик сообщений |
| Shell роутера | `router/opt/vpn-director/lib/common.sh`, `router/test/unit/lock.bats`, `router/opt/vpn-director/vpn-director.json.template`, `router/opt/vpn-director/lib/tproxy.sh` | валидация `VPD_LOCK_WAIT`, граница теста, `log_level` в шаблоне, шапка Dependencies |
| Фронт: клиент | `web/src/api.ts`, `web/src/types.ts`, `web/src/components/LogsTab.vue` | типизированные ответы вместо `any` |
| Фронт: обновление | `web/src/updateState.ts` (новый), `web/src/components/SettingsTab.vue` | состояние переживает уход с вкладки; потребитель `updateStatus()` |
| Фронт: полировка | `web/src/components/SettingsTab.vue`, `web/src/App.vue`, `web/src/style.css` | кнопка при forced check, висячий `⬆ Update to `, клавиатурный путь баннера |
| Документация | `CLAUDE.md`, `.claude/rules/webui.md` (новый), `.claude/rules/telegram-bot.md`, `.claude/rules/xray-tproxy.md`, `README.md`, `README.ru.md` | сводная документация блоков 1–4 |
| Уборка | `server/.gitignore` | `/bot`, `/webui`; кандидаты на удаление — вопрос автору |

**Порядок задач обязателен:** 1 → 2 (отпечаток до middleware); 4 → 6 (updater уходит с сети до того, как CI начнёт гонять `go test`); 12 → 13 → 14 (типизированный клиент до логики резюме, логика до полировки — все три правят `SettingsTab.vue`); 15 — предпоследняя, документирует итоговое состояние; 16 — последняя.

---

### Task 1: `ShadowAuth.Fingerprint` и claim `pwh`

✅ Done — see commit(s): `0bfb7e0` (committed together with Task 2 — Task 1 alone leaves `go build` red)

---

### Task 2: middleware сверяет отпечаток на каждом запросе

✅ Done — see commit(s): `0bfb7e0`

---

### Task 3: установщик запускает Web UI

✅ Done — see commit(s): `ee168b7`, `cabb141` (fix round: the two start_webui assertions)

---

### Task 4: `updater` уходит с сети и требует https для ассетов

✅ Done — see commit(s): `e0688e6`, `618cc63` (fix round: the comment explaining the seam)

---

### Task 5: три пина вокруг таблицы демонов и списка файлов

✅ Done — see commit(s): `e33c335`

---

### Task 6: CI собирает то, что заявлено, и проверяет три стека

✅ Done — see commit(s): `e98b458`, `f38adb0` (fix round: the Go job could not compile a fresh checkout)

---

### Task 7: одно уведомление на чат, а не на имя пользователя

✅ Done — see commit(s): `c96f06a`, `6f97c8e` (fix round: dedupe leaked across ticks)

---

### Task 8: дедлайны с положительным запасом

**Files:**
- Modify: `server/internal/webapi/deadline.go`, `server/internal/webapi/handler_logs.go`, `server/internal/webapi/handler_status.go`
- Test: `server/internal/webapi/deadline_test.go`

**Interfaces:**
- Consumes: `service.ApplyTimeout` (5 м), `service.UpdateTimeout` (15 м), `service.StatusTimeout` (30 с), `service.TailTimeout` (10 с), `service.ExternalIPTimeout` (15 с), `extendWriteDeadline`, `Deps.LogPaths`.
- Produces: `deadlineSlack = 60 * time.Second`; `ipDeadline`; **`logsDeadline` перестаёт быть константой и становится функцией `logsDeadline(deps *Deps) time.Duration`**.

- [ ] **Step 1: Написать падающие тесты**

В `server/internal/webapi/deadline_test.go` дописать:

```go
// TestLogsDeadline_TracksTheNumberOfSources: /api/logs without a source tails
// every file in deps.LogPaths sequentially. A literal 4 in the deadline stops
// being true the moment a fifth source is wired in cmd/webui/main.go, and the
// symptom is a torn connection on the diagnostics page.
func TestLogsDeadline_TracksTheNumberOfSources(t *testing.T) {
	deps := newTestDeps(t)
	four := logsDeadline(deps)

	deps.LogPaths["extra"] = "/tmp/test-extra.log"
	five := logsDeadline(deps)

	if five-four != service.TailTimeout {
		t.Errorf("adding a log source moved the deadline by %s, want %s", five-four, service.TailTimeout)
	}
}

// TestDeadlines_CoverTheirWorstCase states the arithmetic the constants exist
// for. Each shell command can also hold its pipes for shell.waitDelay after
// the timeout fires, and a mutation waits for the config lock first.
func TestDeadlines_CoverTheirWorstCase(t *testing.T) {
	const waitDelay = 10 * time.Second      // shell.waitDelay
	const configLock = 30 * time.Second     // service.configLockTimeout

	deps := newTestDeps(t)
	cases := []struct {
		name      string
		deadline  time.Duration
		worstCase time.Duration
	}{
		{"apply", applyDeadline, configLock + service.ApplyTimeout + waitDelay},
		{"ipsets update", updateDeadline, configLock + service.UpdateTimeout + waitDelay},
		{"status", statusDeadline, service.StatusTimeout + waitDelay},
		{"logs", logsDeadline(deps), time.Duration(len(deps.LogPaths)) * (service.TailTimeout + waitDelay)},
		{"external ip", ipDeadline, service.ExternalIPTimeout + waitDelay},
	}
	for _, c := range cases {
		if c.deadline <= c.worstCase {
			t.Errorf("%s: deadline %s does not cover the worst case %s", c.name, c.deadline, c.worstCase)
		}
	}
}
```

В таблице `TestLongOpHandlers_ExtendWriteDeadline` заменить строку логов и добавить строку `/api/ip`:

```go
		{"status", "GET", "/api/status", "", handleStatus, statusDeadline},
		{"external ip", "GET", "/api/ip", "", handleIP, ipDeadline},
```

а для двух строк логов `want` вычислить внутри подтеста: заменить поле `want time.Duration` этих двух строк на `0` не получится, поэтому вынести их из таблицы в отдельный тест:

```go
func TestLogsHandler_ExtendsWriteDeadline(t *testing.T) {
	for _, path := range []string{"/api/logs", "/api/logs?source=vpn"} {
		t.Run(path, func(t *testing.T) {
			deps := newTestDeps(t)
			rec := newDeadlineRecorder()
			handleLogs(deps).ServeHTTP(rec, httptest.NewRequest("GET", path, nil))
			assertDeadlineAbout(t, rec, logsDeadline(deps))
		})
	}
}
```

и удалить из таблицы `TestLongOpHandlers_ExtendWriteDeadline` две строки `{"logs (all sources)", ...}` и `{"logs (single source)", ...}`.

Добавить в import-блок `"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"`.

- [ ] **Step 2: Убедиться, что тесты падают**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ -count=1`
Ожидается: `logsDeadline (variable of type time.Duration) is not a function`, `undefined: ipDeadline`.

- [ ] **Step 3: Поднять запас и сделать logsDeadline функцией**

`server/internal/webapi/deadline.go`:

```go
const (
	// deadlineSlack is the margin on top of a command's own timeout. Spec 5.3
	// says 30 s, which turned out to be negative margin in two places: a shell
	// command can hold its pipes for another shell.waitDelay (10 s) after the
	// timeout fires, and a mutation waits up to service.configLockTimeout
	// (30 s) for the config lock before the command even starts.
	deadlineSlack  = 60 * time.Second
	applyDeadline  = service.ApplyTimeout + deadlineSlack
	updateDeadline = service.UpdateTimeout + deadlineSlack
	// statusDeadline covers `vpn-director.sh status`, whose own StatusTimeout
	// equals WriteTimeout: without the extension the connection is torn down
	// exactly when the router is slow enough to need diagnostics.
	statusDeadline = service.StatusTimeout + deadlineSlack
	// ipDeadline covers `curl ifconfig.me`. It fits inside the 30 s
	// WriteTimeout today with five seconds to spare; the extension means
	// raising ExternalIPTimeout cannot silently put the route back over.
	ipDeadline = service.ExternalIPTimeout + deadlineSlack
	// importDeadline covers the 10-second subscription download plus one DNS
	// lookup per server; the import runs no shell command.
	importDeadline = 2 * time.Minute
	// githubDeadline covers the synchronous part of an update route: one
	// GitHub API call under updater.APITimeout. POST /api/update answers 202
	// as soon as the download goroutine is under way, so the script's own
	// minutes never sit on the connection.
	githubDeadline = updater.APITimeout + deadlineSlack
)

// logsDeadline covers the all-sources branch of /api/logs, which tails every
// file in deps.LogPaths sequentially under TailTimeout each. The count comes
// from the map rather than a literal, so wiring a fifth source in
// cmd/webui/main.go cannot leave the deadline behind.
func logsDeadline(deps *Deps) time.Duration {
	return time.Duration(len(deps.LogPaths))*service.TailTimeout + deadlineSlack
}
```

`server/internal/webapi/handler_logs.go`:

```go
		extendWriteDeadline(w, logsDeadline(deps))
```

`server/internal/webapi/handler_status.go`, первой строкой `handleIP`:

```go
	return func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, ipDeadline)

		ip, err := deps.Network.GetExternalIP()
```

- [ ] **Step 4: Прогнать пакет**

Через `claude-forge:build`: `cd server && go vet ./... && go test ./internal/webapi/ -count=1`

- [ ] **Step 5: Mutation-доказательство**

Временно вернуть `deadlineSlack = 30 * time.Second` — краснеет `TestDeadlines_CoverTheirWorstCase` на `apply` и `logs`. Вернуть 60. Временно заменить в `logsDeadline` `len(deps.LogPaths)` на `4` — краснеет `TestLogsDeadline_TracksTheNumberOfSources`. Вернуть. Временно убрать продление из `handleIP` — краснеет строка `external ip` таблицы. Вернуть.

- [ ] **Step 6: Коммит**

```bash
gofmt -l server/internal/webapi/deadline.go server/internal/webapi/deadline_test.go server/internal/webapi/handler_logs.go server/internal/webapi/handler_status.go
git add server/internal/webapi/deadline.go server/internal/webapi/deadline_test.go server/internal/webapi/handler_logs.go server/internal/webapi/handler_status.go
git commit -m "fix(webapi): give every response deadline a positive margin

Three residuals from block 2, all the same arithmetic: applyDeadline was
5m30s against a 5m40s worst case, logsDeadline 70s against 80s, and the
literal 4 in logsDeadline stopped being true as soon as a fifth log
source was wired. deadlineSlack goes to 60s, logsDeadline derives its
count from deps.LogPaths, and /api/ip extends too, so raising
ExternalIPTimeout cannot silently put it back over WriteTimeout." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 9: три мелочи webapi

**Files:**
- Modify: `server/internal/webapi/handler_update.go`, `server/internal/webapi/handler_servers.go`
- Test: `server/internal/webapi/handler_update_test.go`

**Interfaces:**
- Consumes: `updateflow.GitHubError`, `jsonError`, `syncXrayServers`.
- Produces: `writeGitHubError(w http.ResponseWriter, err error) bool` — возвращает `true`, если ошибка была `*updateflow.GitHubError` и ответ уже записан.

- [ ] **Step 1: Написать падающий негативный тест `force`**

В `server/internal/webapi/handler_update_test.go` заменить `TestHandleUpdateCheck_ForcePassedThrough` на табличный:

```go
func TestHandleUpdateCheck_ForcePassedThrough(t *testing.T) {
	// The negative case is the one that matters: a handler that hardcoded
	// force = true passes every other test in this file and would spend the
	// whole 60-per-hour GitHub budget on banner loads.
	tests := []struct {
		name string
		path string
		want bool
	}{
		{"force=1", "/api/update/check?force=1", true},
		{"no force", "/api/update/check", false},
		{"force=0", "/api/update/check?force=0", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			flow := deps.Update.(*mockUpdateFlow)

			rec := httptest.NewRecorder()
			handleUpdateCheck(deps)(rec, httptest.NewRequest("GET", tt.path, nil))

			if flow.checkForce != tt.want {
				t.Errorf("flow saw force=%v, want %v", flow.checkForce, tt.want)
			}
		})
	}
}
```

- [ ] **Step 2: Убедиться, что тест зелёный, и доказать, что он не вакуумный**

Через `claude-forge:build`: `cd server && go test ./internal/webapi/ -run TestHandleUpdateCheck_ForcePassedThrough -count=1`
Ожидается: PASS. Затем временно заменить в `handleUpdateCheck` аргумент на литерал `true` — подтесты `no force` и `force=0` краснеют. Вернуть. Выдержку приложить.

- [ ] **Step 3: Вынести общий 502**

В `server/internal/webapi/handler_update.go` добавить:

```go
// writeGitHubError answers 502 when err came from the GitHub API and reports
// whether it did. Both update routes surface the underlying message: a user
// looking at "Failed to check for updates" needs to know whether GitHub was
// down or the router has no DNS.
func writeGitHubError(w http.ResponseWriter, err error) bool {
	var ghErr *updateflow.GitHubError
	if !errors.As(err, &ghErr) {
		return false
	}
	jsonError(w, http.StatusBadGateway, "failed to check for updates: "+ghErr.Err.Error())
	return true
}
```

В `handleUpdateCheck` заменить блок `var ghErr ...` на:

```go
		if writeGitHubError(w, err) {
			return
		}
```

В `handleUpdateStart`, в ветке `default`:

```go
		default:
			if writeGitHubError(w, err) {
				return
			}
			slog.Warn("update start failed", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to start update: "+err.Error())
		}
```

- [ ] **Step 4: Снять двойную обёртку в импорте серверов**

В `server/internal/webapi/handler_servers.go`, в `syncXrayServers`:

```go
// syncXrayServers updates xray.servers with the IPs of all given servers under
// the config lock. The error is returned unwrapped: the only caller already
// prefixes it with "xray.servers sync failed", and wrapping here produced
// "servers saved, but xray.servers sync failed: sync xray.servers: ..." in the
// user's face.
func syncXrayServers(config service.ConfigStore, servers []vpnconfig.Server) error {
	return config.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.Xray.Servers = collectServerIPs(servers)
		return nil
	})
}
```

Если после этого в файле остался неиспользуемый импорт — убрать его; `fmt` в файле используется и в других местах, проверить перед удалением.

- [ ] **Step 5: Прогнать пакет**

Через `claude-forge:build`: `cd server && go vet ./... && go test ./internal/webapi/ -count=1`
Ожидается: PASS, включая `assertGitHubErrorBody`, который пинит дословный текст 502 — он не должен измениться.

- [ ] **Step 6: Коммит**

```bash
gofmt -l server/internal/webapi/handler_update.go server/internal/webapi/handler_update_test.go server/internal/webapi/handler_servers.go
git add server/internal/webapi/handler_update.go server/internal/webapi/handler_update_test.go server/internal/webapi/handler_servers.go
git commit -m "refactor(webapi): share the 502 branch and stop double-wrapping

Three block-3 leftovers: the GitHub 502 branch was copied into both
update handlers, TestHandleUpdateCheck_ForcePassedThrough asserted only
the positive case so a hardcoded force=true would have passed, and the
import error reached the user as \"servers saved, but xray.servers sync
failed: sync xray.servers: load config: ...\"." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 10: честные ошибки и честные имена тестов

**Files:**
- Modify: `server/internal/shell/shell.go`
- Test: `server/internal/shell/shell_test.go`, `server/internal/handler/update_test.go`

**Interfaces:**
- Consumes: `context.DeadlineExceeded`, `shell.ExecContext`.
- Produces: ошибка таймаута теперь оборачивает `ctx.Err()`; имена `TestExecContext_*` вместо `TestExec_*`.

- [ ] **Step 1: Написать падающий тест обёртки**

В `server/internal/shell/shell_test.go` дописать:

```go
// TestExecContext_TimeoutErrorUnwrapsToDeadlineExceeded: the message is pinned
// by spec 5.2 and stays exactly as it is, but a caller that asks
// errors.Is(err, context.DeadlineExceeded) has to get true - otherwise every
// future caller has to match on the string.
func TestExecContext_TimeoutErrorUnwrapsToDeadlineExceeded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := ExecContext(ctx, "sleep", "5")
	if err == nil {
		t.Fatal("expected a timeout error")
	}
	if !strings.HasPrefix(err.Error(), "command timed out after ") {
		t.Errorf("error = %q, want it to start with %q", err, "command timed out after ")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("errors.Is(err, context.DeadlineExceeded) = false for %v", err)
	}
}
```

Добавить в import-блок `"errors"`, `"strings"` при необходимости.

- [ ] **Step 2: Убедиться, что тест падает**

Через `claude-forge:build`: `cd server && go test ./internal/shell/ -run TestExecContext_TimeoutErrorUnwraps -count=1`
Ожидается: FAIL на `errors.Is(...) = false`.

- [ ] **Step 3: Обернуть `ctx.Err()`**

`server/internal/shell/shell.go`:

```go
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			d := time.Since(start)
			if deadline, ok := ctx.Deadline(); ok {
				d = deadline.Sub(start)
			}
			// The message is spec 5.2's wording; wrapping ctx.Err() keeps
			// errors.Is usable without anyone matching on the string.
			return result, fmt.Errorf("command timed out after %s: %w", d.Round(time.Millisecond), ctx.Err())
		}
```

Внимание: текст теперь имеет суффикс `: context deadline exceeded`. Проверить `grep -rn "command timed out after" server --include=*.go` и, если существующий тест сверяет строку целиком, поправить его на префиксную проверку, сохранив дословную часть.

- [ ] **Step 4: Переименовать тесты, которые ссылаются на несуществующий `Exec`**

В `server/internal/shell/shell_test.go` переименовать шесть тестов, названных по функции, которую блок 2 удалил: `TestExec_EchoHello` → `TestExecContext_EchoHello`, `TestExec_NonZeroExitCode` → `TestExecContext_NonZeroExitCode`, `TestExec_CommandWithOutput` → `TestExecContext_CommandWithOutput`, `TestExec_StderrCaptured` → `TestExecContext_StderrCaptured`, `TestExec_CommandNotFound` → `TestExecContext_CommandNotFound`, `TestExec_ExitCodeWithOutput` → `TestExecContext_ExitCodeWithOutput`.

- [ ] **Step 5: Добавить счётчик сообщений в тест бота**

В `server/internal/handler/update_test.go` добавить к `mockUpdateSender` счётчик рядом с `all()`:

```go
func (m *mockUpdateSender) count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.messages)
}
```

и в `TestUpdateHandler_MessagePerOutcome` заменить проверку

```go
			if got := sender.all(); !strings.Contains(got, tt.want) {
				t.Errorf("messages = %q, want to contain %q", got, tt.want)
			}
```

на

```go
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
```

- [ ] **Step 6: Прогнать пакеты**

Через `claude-forge:build`: `cd server && go test ./internal/shell/ ./internal/handler/ ./internal/service/ ./internal/webapi/ -count=1`
Ожидается: PASS. `service` и `webapi` в списке потому, что они — потребители текста ошибки таймаута.

- [ ] **Step 7: Mutation-доказательство**

Убрать `: %w` из обёртки — краснеет `TestExecContext_TimeoutErrorUnwrapsToDeadlineExceeded`. Вернуть. Временно добавить в ветку `errors.Is(err, updateflow.ErrInProgress)` функции `HandleUpdate` второй `h.send(chatID, "extra")` — краснеет подтест «in progress» на счётчике, а прежняя проверка `Contains` осталась бы зелёной. Убрать.

- [ ] **Step 8: Коммит**

```bash
gofmt -l server/internal/shell/shell.go server/internal/shell/shell_test.go server/internal/handler/update_test.go
git add server/internal/shell/shell.go server/internal/shell/shell_test.go server/internal/handler/update_test.go
git commit -m "fix(shell): wrap ctx.Err() in the timeout error

errors.Is(err, context.DeadlineExceeded) returned false, so any future
caller would have had to match on the message. The spec's wording is
kept as the prefix. The six TestExec_* tests are renamed after the
function they actually call: block 2 removed Exec." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 11: shell роутера — валидация, граница теста, шаблон, шапка

**Files:**
- Modify: `router/opt/vpn-director/lib/common.sh`, `router/opt/vpn-director/vpn-director.json.template`, `router/opt/vpn-director/lib/tproxy.sh`
- Test: `router/test/unit/lock.bats`

**Interfaces:**
- Consumes: `acquire_lock`, `VPD_LOCK_WAIT`, `log -l WARN`, хелперы `hold_lock`/`LOCK_NAME` из `lock.bats`.
- Produces: поведение — мусор в `VPD_LOCK_WAIT` даёт WARN и дефолт 120 вместо арифметической ошибки.

- [ ] **Step 1: Написать падающий bats-тест**

В `router/test/unit/lock.bats` дописать:

```bash
@test "acquire_lock: a non-numeric VPD_LOCK_WAIT is ignored, not an arithmetic error" {
    load_common
    # The lock is free, so this must simply succeed: without the guard the
    # arithmetic in `local limit=$((10#...))` aborts the script before flock is
    # ever tried, and an operator who exported the variable by hand loses every
    # apply on the router.
    VPD_LOCK_WAIT=abc run acquire_lock "$LOCK_NAME"

    assert_success
    assert_output --partial "Ignoring invalid VPD_LOCK_WAIT"
}

@test "acquire_lock: an empty-but-set VPD_LOCK_WAIT falls back to the default bound" {
    load_common
    VPD_LOCK_WAIT= run acquire_lock "$LOCK_NAME"

    assert_success
}
```

Заметка для исполнителя: `VPD_LOCK_WAIT=` (пустая строка) идёт по ветке `-z`, то есть по неблокирующей, и должна просто взять свободный лок — тест это и фиксирует.

- [ ] **Step 2: Убедиться, что первый тест падает**

Run: `bats router/test/unit/lock.bats`
Ожидается: FAIL с арифметической ошибкой bash на `10#abc`.

- [ ] **Step 3: Валидировать значение**

`router/opt/vpn-director/lib/common.sh`, в `acquire_lock`, перед `local limit=...`:

```bash
        # parse_option validates the value that comes from --wait, but
        # VPD_LOCK_WAIT can also be exported by hand: garbage there must not
        # abort the script with an arithmetic error while the lock is free.
        if [[ ! ${VPD_LOCK_WAIT} =~ ^[0-9]+$ ]]; then
            log -l WARN "Ignoring invalid VPD_LOCK_WAIT '${VPD_LOCK_WAIT}', using 120s"
            VPD_LOCK_WAIT=120
        fi
        # A leading zero makes (( )) read the value as octal, so 08/09 are not valid
        # numbers and the timeout check would never fire, leaving an unbounded wait.
        # Force base 10 once, then use that bound everywhere.
        local limit=$((10#${VPD_LOCK_WAIT}))
```

- [ ] **Step 4: Поднять верхнюю границу флакующего теста**

В `router/test/unit/lock.bats`, в тесте «VPD_LOCK_WAIT with a leading zero is a decimal bound, not octal», заменить

```bash
    [ "$elapsed" -lt 12 ]
```

на

```bash
    # The bug this test exists for is caught by the lower bound; four seconds
    # of headroom on a loaded CI runner is a flake, not a diagnostic.
    [ "$elapsed" -lt 20 ]
```

- [ ] **Step 5: Добавить `log_level` в шаблон конфига**

`router/opt/vpn-director/vpn-director.json.template`:

```json
  "webui": {
    "port": 8444,
    "cert_file": "/opt/vpn-director/certs/server.crt",
    "key_file": "/opt/vpn-director/certs/server.key",
    "jwt_secret": "",
    "log_level": "info"
  },
```

Проверить: `jq . router/opt/vpn-director/vpn-director.json.template >/dev/null && echo "template is valid JSON"`. Ключ теперь совпадает с тем, что пишет `setup_webui_config` (Task 3) и что документируют оба README.

- [ ] **Step 6: Дописать шапку tproxy.sh**

`router/opt/vpn-director/lib/tproxy.sh`, в блоке Dependencies:

```bash
#   - ipset.sh (_is_valid_country_code)
```

Строку поставить после `config.sh (XRAY_* variables)`. Порядок сорсинга в `vpn-director.sh` уже гарантирует наличие функции — правка документационная.

- [ ] **Step 7: Прогнать весь bats**

Run: `bats router/test/unit && bats router/test`
Ожидается: всё зелёное.

- [ ] **Step 8: Mutation-доказательство**

Убрать блок валидации — краснеет «a non-numeric VPD_LOCK_WAIT is ignored». Вернуть.

- [ ] **Step 9: Коммит**

```bash
git add router/opt/vpn-director/lib/common.sh router/test/unit/lock.bats router/opt/vpn-director/vpn-director.json.template router/opt/vpn-director/lib/tproxy.sh
git commit -m "fix(common): survive a hand-exported VPD_LOCK_WAIT

An operator who exports VPD_LOCK_WAIT=abc lost every apply to an
arithmetic error, even with the lock free: parse_option only validates
the value that arrives through --wait. Also raises the flaky upper bound
in lock.bats, adds the log_level key the README documents to the config
template, and names ipset.sh in tproxy.sh's Dependencies header." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 12: типизированный API-клиент

**Files:**
- Modify: `web/src/types.ts`, `web/src/api.ts`, `web/src/components/LogsTab.vue`, `web/src/components/SettingsTab.vue`

**Interfaces:**
- Consumes: ответы обработчиков `webapi` (`jsonOK`-полезные нагрузки, перечисленные ниже).
- Produces: типы `OkResponse`, `ImportResponse`, `IPResponse`, `ServersResponse`, `ClientsResponse`, `ExcludeSetsResponse`, `ExcludeIPsResponse`, `LogResponse`, `AllLogsResponse`, `UpdateStatusResponse`, `ConfigResponse`; методы `api.getLog(source, lines)` и `api.getAllLogs(lines)` вместо `api.getLogs(source?, lines?)`.

- [ ] **Step 1: Описать типы ответов**

В `web/src/types.ts` дописать (существующие интерфейсы не трогать):

```ts
/** Every mutation answers {"ok": true}. */
export interface OkResponse {
  ok: boolean
}

export interface ImportResponse {
  ok: boolean
  count: number
}

export interface IPResponse {
  ip: string
}

export interface ServersResponse {
  servers: Server[]
}

export interface ClientsResponse {
  clients: ClientInfo[]
}

export interface ExcludeSetsResponse {
  sets: string[]
}

export interface ExcludeIPsResponse {
  ips: string[]
}

/** GET /api/logs?source=... — one file. */
export interface LogResponse {
  output: string
  source: string
}

/** GET /api/logs — source name to contents, for every configured source. */
export type AllLogsResponse = Record<string, string>

export interface UpdateStatusResponse {
  in_progress: boolean
}

/** GET /api/config returns the whole vpn-director.json with jwt_secret blanked;
 *  the Settings tab only ever re-serialises it. */
export type ConfigResponse = Record<string, unknown>
```

- [ ] **Step 2: Типизировать клиент**

`web/src/api.ts` — заменить импорт и весь экспортируемый объект:

```ts
import axios from 'axios'
import type {
  AllLogsResponse,
  ClientsResponse,
  ConfigResponse,
  ExcludeIPsResponse,
  ExcludeSetsResponse,
  ImportResponse,
  IPResponse,
  LogResponse,
  OkResponse,
  ServersResponse,
  StatusResponse,
  UpdateCheckResponse,
  UpdateStartResponse,
  UpdateStatusResponse,
  VersionResponse,
} from './types'
```

```ts
export default {
  // Auth
  checkAuth: () =>
    api.get<VersionResponse>('/api/version', { skipAuthRedirect: true } as any),
  login: (username: string, password: string) =>
    api.post<OkResponse>('/api/login', { username, password }),
  logout: () =>
    api.post<OkResponse>('/api/logout'),

  // Status & Control
  getStatus: () =>
    api.get<StatusResponse>('/api/status'),
  apply: () =>
    api.post<OkResponse>('/api/apply'),
  restart: () =>
    api.post<OkResponse>('/api/restart'),
  stop: () =>
    api.post<OkResponse>('/api/stop'),
  updateIPsets: () =>
    api.post<OkResponse>('/api/ipsets/update'),

  // Info
  getIP: () =>
    api.get<IPResponse>('/api/ip'),
  getVersion: () =>
    api.get<VersionResponse>('/api/version'),

  // Servers
  getServers: () =>
    api.get<ServersResponse>('/api/servers'),
  selectServer: (index: number) =>
    api.post<OkResponse>('/api/servers/active', { index }),
  importServers: (url: string) =>
    api.post<ImportResponse>('/api/servers/import', { url }),

  // Clients
  getClients: () =>
    api.get<ClientsResponse>('/api/clients'),
  addClient: (ip: string, route: string) =>
    api.post<OkResponse>('/api/clients', { ip, route }),
  pauseClient: (ip: string) =>
    api.post<OkResponse>('/api/clients/pause', null, { params: { ip } }),
  resumeClient: (ip: string) =>
    api.post<OkResponse>('/api/clients/resume', null, { params: { ip } }),
  deleteClient: (ip: string) =>
    api.delete<OkResponse>('/api/clients', { params: { ip } }),

  // Exclusions
  getExcludeSets: () =>
    api.get<ExcludeSetsResponse>('/api/excludes/sets'),
  updateExcludeSets: (sets: string[]) =>
    api.post<OkResponse>('/api/excludes/sets', { sets }),
  getExcludeIPs: () =>
    api.get<ExcludeIPsResponse>('/api/excludes/ips'),
  addExcludeIP: (ip: string) =>
    api.post<OkResponse>('/api/excludes/ips', { ip }),
  deleteExcludeIP: (ip: string) =>
    api.delete<OkResponse>('/api/excludes/ips', { params: { ip } }),

  // Logs & Config
  // /api/logs answers with two different shapes, so it gets two methods: one
  // file with its name, or every configured source at once.
  getLog: (source: string, lines?: number) =>
    api.get<LogResponse>('/api/logs', { params: { source, ...(lines ? { lines } : {}) } }),
  getAllLogs: (lines?: number) =>
    api.get<AllLogsResponse>('/api/logs', { params: { ...(lines ? { lines } : {}) } }),
  getConfig: () =>
    api.get<ConfigResponse>('/api/config'),

  // Self-update
  checkUpdate: (force = false) =>
    api.get<UpdateCheckResponse>('/api/update/check', { params: force ? { force: 1 } : {} }),
  update: () =>
    api.post<UpdateStartResponse>('/api/update'),
  updateStatus: () =>
    api.get<UpdateStatusResponse>('/api/update/status'),
  // pollVersion is used while the server restarts: connection errors and a
  // brief 401 must not bounce the user to the login page.
  pollVersion: () =>
    api.get<VersionResponse>('/api/version', { skipAuthRedirect: true } as any),
}
```

- [ ] **Step 3: Развести две ветки в LogsTab**

`web/src/components/LogsTab.vue`:

```ts
    if (source.value) {
      const resp = await api.getLog(source.value, lines.value)
      logData.value = { [resp.data.source]: resp.data.output ?? '' }
    } else {
      const resp = await api.getAllLogs(lines.value)
      logData.value = resp.data
    }
```

- [ ] **Step 4: Убрать ставший лишним каст в SettingsTab**

`web/src/components/SettingsTab.vue`:

```ts
    const resp = await api.update()
    const data = resp.data
```

и удалить `UpdateStartResponse` из импорта типов, если он больше нигде в файле не используется.

- [ ] **Step 5: Собрать фронт**

Через `claude-forge:build`: `cd web && npm run build`
Ожидается: `vue-tsc` молчит, сборка проходит. Любая ошибка типов здесь — это найденное расхождение фронта с API; исправлять по факту, а не глушить `any`.

- [ ] **Step 6: Доказать, что типы работают**

Временно поменять в `types.ts` в `IPResponse` поле `ip` на `address` — `npm run build` должен упасть на `StatusTab.vue` (`Property 'ip' does not exist`). Вернуть. Выдержку приложить: до этой задачи такая ошибка не обнаруживалась вовсе.

- [ ] **Step 7: Коммит**

```bash
git add web/src/types.ts web/src/api.ts web/src/components/LogsTab.vue web/src/components/SettingsTab.vue
git commit -m "refactor(web): type the API client

Every method returned AxiosResponse<any>, so a renamed field on the Go
side reached the browser as undefined instead of a build failure.
/api/logs gets two methods because it answers with two shapes." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 13: обновление переживает уход с вкладки Settings

**Files:**
- Create: `web/src/updateState.ts`
- Modify: `web/src/components/SettingsTab.vue`

**Interfaces:**
- Consumes: `api.updateStatus()` (типизирован в Task 12, до сих пор без потребителя), `api.pollVersion()`, `api.update()`.
- Produces: модуль `updateState` с `updating`, `updateMessage`, `updateTarget`, `claimPolling()`, `releasePolling()`.

- [ ] **Step 1: Зафиксировать дефект вручную**

Собрать фронт (`cd web && npm run build`) и в dev-режиме (`cd server && go run ./cmd/webui --dev`, вход `admin`/`admin`) убедиться, что вкладка Settings рендерится за `v-if` (`App.vue:96`): уход на Status и возврат монтируют компонент заново и обнуляют любое состояние. Записать наблюдение в отчёт — автотеста для фронта в этом блоке нет.

- [ ] **Step 2: Создать модуль состояния**

`web/src/updateState.ts`:

```ts
import { ref } from 'vue'

// The Settings tab is rendered behind v-if in App.vue, so switching tabs
// unmounts it. An update takes tens of seconds and the user will switch tabs,
// so its progress lives here, outside the component: coming back must show the
// same screen rather than a fresh one that asks a restarting server for its
// version.
export const updating = ref(false)
export const updateMessage = ref('')
export const updateTarget = ref('')

// Only one poller may run. Without this a remount would start a second loop
// and the page would reload twice, the second time over a half-loaded first.
let polling = false

/** claimPolling returns false when a poll loop is already running. */
export function claimPolling(): boolean {
  if (polling) {
    return false
  }
  polling = true
  return true
}

export function releasePolling(): void {
  polling = false
}
```

- [ ] **Step 3: Переселить состояние из компонента**

`web/src/components/SettingsTab.vue` — в `<script setup>`:

Удалить локальные `const updating = ref(false)` и `const updateMessage = ref('')`, добавить импорт:

```ts
import {
  claimPolling,
  releasePolling,
  updateMessage,
  updateTarget,
  updating,
} from '../updateState'
```

- [ ] **Step 4: Сделать ожидание версии возобновляемым**

Заменить `waitForVersion` и `doUpdate` на:

```ts
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

// runPolling owns the waiting screen. It survives this component: if the user
// leaves the tab the loop keeps going against the module state, and a remount
// re-renders the same screen instead of starting a second loop.
async function runPolling() {
  if (!claimPolling()) return
  try {
    updateMessage.value = 'Updating, the server is restarting...'
    const arrived = await waitForVersion(updateTarget.value)
    if (arrived) {
      // Reload so the browser picks up the new bundle.
      window.location.reload()
      return
    }
    updateMessage.value = 'The new version did not come up within 5 minutes. Check the logs.'
  } finally {
    updating.value = false
    releasePolling()
  }
}

async function doUpdate() {
  const target = updateInfo.value?.latest
  if (!target) return
  if (!confirm(`Update VPN Director to ${target}?`)) return

  updating.value = true
  error.value = ''
  updateMessage.value = 'Starting update...'
  try {
    const resp = await api.update()
    if (resp.data.update_available === false) {
      await loadUpdate()
      updateMessage.value = 'Already running the latest version.'
      updating.value = false
      return
    }
    updateTarget.value = resp.data.to || target
  } catch (e: any) {
    updateMessage.value = 'Error: ' + (e.response?.data?.error || e.message)
    updating.value = false
    return
  }
  await runPolling()
}
```

- [ ] **Step 5: Дать `updateStatus()` потребителя**

Заменить `onMounted`:

```ts
// resumeFromServer covers the case the module state cannot: a page reload, or a
// second browser, meeting an update it never started. /api/update/status is the
// only way to tell "still updating" from "done".
async function resumeFromServer() {
  try {
    const resp = await api.updateStatus()
    if (!resp.data.in_progress) return
  } catch {
    // The server is unreachable; the normal error paths already say so.
    return
  }
  if (!updateTarget.value) {
    updateTarget.value = updateInfo.value?.latest || ''
  }
  if (!updateTarget.value) return
  updating.value = true
  void runPolling()
}

onMounted(async () => {
  if (updating.value) {
    // An update started before the tab was left. Rejoin its screen instead of
    // asking a restarting server for a version and a release list.
    void runPolling()
    return
  }
  await loadVersion()
  await loadUpdate()
  await resumeFromServer()
})
```

- [ ] **Step 6: Собрать и проверить вручную**

Через `claude-forge:build`: `cd web && npm run build`.
Затем в dev-режиме: нажать «Update to …» (в dev обновление отклоняется с 400 «dev mode», поэтому проверяется путь ошибки), уйти на Status и вернуться — сообщение об ошибке и состояние кнопок сохраняются, второго опроса не запускается. Полная проверка живого обновления — пункт ручного чек-листа в конце плана.

- [ ] **Step 7: Коммит**

```bash
git add web/src/updateState.ts web/src/components/SettingsTab.vue
git commit -m "fix(web): keep update progress across a tab switch

The Settings tab is rendered behind v-if, so leaving it mid-update
destroyed the progress state and the remount asked a restarting server
for its version. Progress moves into a module the tab reads, one poll
loop is enforced, and /api/update/status - wired but unused since block
3 - lets a reloaded page rejoin an update it did not start." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 14: три полировки интерфейса

**Files:**
- Modify: `web/src/components/SettingsTab.vue`, `web/src/App.vue`, `web/src/style.css`

**Interfaces:**
- Consumes: `checking`, `updating`, `updateInfo` из `SettingsTab.vue`; класс `.update-banner` из `style.css`.
- Produces: изменений в API нет.

- [ ] **Step 1: Заблокировать «Update» на время принудительной проверки**

`web/src/components/SettingsTab.vue`:

```ts
// checking is part of the guard: during a forced check updateInfo still holds
// the previous answer, so the button would offer to install a version the
// server is at that moment re-checking.
const canUpdate = computed(() => !!updateInfo.value?.update_available && !updating.value && !checking.value)
```

- [ ] **Step 2: Убрать висячий «⬆ Update to »**

В шаблоне:

```html
      <button class="btn btn-primary" :disabled="!canUpdate" @click="doUpdate">
        {{ updating ? 'Updating...' : (updateInfo?.latest ? `⬆ Update to ${updateInfo.latest}` : '⬆ Update') }}
      </button>
```

- [ ] **Step 3: Дать баннеру клавиатурный путь**

`web/src/App.vue`:

```html
        <button
          v-if="updateBanner"
          type="button"
          class="update-banner"
          @click="activeTab = 'settings'"
        >
          {{ updateBanner }}
        </button>
```

`web/src/style.css` — расширить существующее правило, чтобы кнопка не унаследовала оформление кнопки браузера:

```css
.update-banner {
  cursor: pointer;
  color: #f0a020;
  font-size: 0.85rem;
  background: none;
  border: none;
  padding: 0;
  font-family: inherit;
}
```

- [ ] **Step 4: Собрать и посмотреть**

Через `claude-forge:build`: `cd web && npm run build`.
В dev-режиме проверить: баннер фокусируется Tab и срабатывает по Enter и пробелу; кнопка обновления гаснет на время «⟳ Check for updates»; на dev-сборке подпись читается «⬆ Update», а не «⬆ Update to ».

- [ ] **Step 5: Коммит**

```bash
git add web/src/components/SettingsTab.vue web/src/App.vue web/src/style.css
git commit -m "fix(web): three interface leftovers from block 3

The Update button stayed live during a forced check and would have
offered a version the server was re-checking; a dev build rendered a
dangling \"⬆ Update to \"; and the update banner was a clickable span
with no keyboard path." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 15: сводная документация четырёх блоков

**Files:**
- Modify: `CLAUDE.md`, `.claude/rules/telegram-bot.md`, `.claude/rules/xray-tproxy.md`, `README.md`, `README.ru.md`
- Create: `.claude/rules/webui.md`

**Interfaces:**
- Consumes: итоговое состояние дерева после задач 1–14.
- Produces: документация; кода не касается.

- [ ] **Step 1: Проверить, что уже сделано, и не переписывать это заново**

Run:
```bash
grep -n 'Log viewer\|log_level\|Updates' README.md | head
grep -n 'future WebUI' CLAUDE.md
ls .claude/rules/webui.md 2>&1
grep -n 'webapi\|auth/\|cmd/webui\|version.go\|update_script' .claude/rules/telegram-bot.md
```
Ожидается: строка Logs и раздел про `log_level`/обновление в README уже есть (блоки 1–3) — их не трогаем; `CLAUDE.md:46` всё ещё «future WebUI»; `webui.md` нет; в дереве `telegram-bot.md` нет `webapi/`, `auth/`, `ssrf/`, `cmd/webui/`, а у `updater/` не перечислены `version.go` и `update_script.sh.tmpl`.

- [ ] **Step 2: Обновить `CLAUDE.md`**

В блок `## Commands`, после раздела «Import servers», добавить:

````markdown
```bash
# Build (from the repository root)
make build-webui         # Vue SPA -> go:embed -> webui binary
make build-all           # both daemons for arm64 and arm
make -C server test      # Go tests

# Web UI in development: plain HTTP, testdata/dev paths, mock shell, admin/admin
cd server && go run ./cmd/webui --dev
```
````

В таблице `## Architecture` заменить строку `| server/ | Go server: Telegram bot (+ future WebUI) for remote management |` на пять строк:

```markdown
| `server/cmd/bot/main.go` | Telegram bot daemon: DI, signal handling |
| `server/cmd/webui/main.go` | Web UI daemon: HTTPS server, DI, dev mode |
| `server/internal/webapi/` | HTTP API: router, JWT middleware, handlers, response deadlines |
| `server/internal/auth/` | Password check against `/etc/shadow`, JWT issue and validation |
| `web/` | Vue 3 SPA, embedded into the webui binary with `go:embed` |
```

В таблицу `## Config Files (after install)` добавить:

```markdown
| `/opt/vpn-director/certs/server.{crt,key}` | Self-signed TLS certificate for the Web UI |
```

Под таблицей, рядом с абзацем **Data storage**, добавить:

```markdown
**Web UI settings**: the `webui` section of `vpn-director.json` — `port` (8444), `cert_file`, `key_file`, `jwt_secret` (auto-generated when empty), `log_level` (`debug|info|warn|error`).
```

В список `## Modular Docs` добавить строку после `telegram-bot.md`:

```markdown
- `webui.md` — Web UI architecture, API table, authentication, dev mode, update flow
```

- [ ] **Step 3: Создать `.claude/rules/webui.md`**

```markdown
---
paths: "server/internal/webapi/**/*, server/internal/auth/**/*, server/cmd/webui/**/*, web/**/*"
---

# Web UI

HTTPS interface for VPN Director, served by the `webui` daemon. Shares the
`internal/service` layer with the Telegram bot; both write `vpn-director.json`
only through `ConfigService.UpdateVPNConfig`, which holds a `flock`.

## Architecture

```
server/cmd/webui/main.go     # Entry point: config, logging, DI, dev mode, TLS
server/internal/
├── webapi/
│   ├── server.go            # http.Server: TLS 1.2+, WriteTimeout 30s
│   ├── router.go            # Deps, route table, SPA fallback with cache headers
│   ├── middleware.go        # JWT + password-fingerprint check, login rate limit, request log
│   ├── deadline.go          # Per-handler write-deadline extensions
│   ├── apply.go             # Auto-apply shared by every mutation
│   ├── response.go          # jsonOK / jsonError / decodeJSON
│   ├── errline.go           # Last error line of a shell failure, for the client
│   └── handler_*.go         # status, servers, clients, excludes, logs, auth, update
└── auth/
    ├── shadow.go            # /etc/shadow verification, Fingerprint
    └── jwt.go               # HS256 issue and validation
web/                         # Vue 3 SPA (Vite), embedded via go:embed
```

## API

Every route below `/api/` except `POST /api/login` requires a valid token.

| Method | Path | Purpose |
|--------|------|---------|
| POST | `/api/login` | Password check against `/etc/shadow`, sets the cookie |
| POST | `/api/logout` | Clears the cookie |
| GET | `/api/status` | `vpn-director.sh status` |
| POST | `/api/apply` / `/api/restart` / `/api/stop` | VPN Director control |
| POST | `/api/ipsets/update` | `vpn-director.sh update` (`IPSET_FORCE_UPDATE=1`) |
| GET | `/api/ip` | External IP |
| GET | `/api/version` | Build version and commit |
| GET/POST | `/api/servers`, `/api/servers/active`, `/api/servers/import` | Xray servers |
| GET/POST/DELETE | `/api/clients`, `/api/clients/pause`, `/api/clients/resume` | LAN clients |
| GET/POST/DELETE | `/api/excludes/sets`, `/api/excludes/ips` | Exclusions |
| GET | `/api/logs` | One source (`?source=`) or every source at once |
| GET | `/api/config` | `vpn-director.json` with `jwt_secret` blanked |
| GET | `/api/update/check` | Latest release; `?force=1` pierces the 30-minute cache |
| POST | `/api/update` | Starts the unified update, answers 202 |
| GET | `/api/update/status` | Whether an update script is running |

Every mutation runs `Apply()` afterwards, as the bot does; the handlers
serialize on `Deps.OpMutex`.

## Authentication

- `POST /api/login` verifies the password against `/etc/shadow` (MD5, SHA-256
  and SHA-512 MCF hashes, pure Go), then issues an HS256 JWT valid for 24 hours
  in an `HttpOnly; Secure; SameSite=Strict` cookie.
- The token carries `pwh`: `ShadowAuth.Fingerprint`, the first 8 bytes of the
  SHA-256 of the stored password hash, in hex. `authMiddleware` recomputes it
  on **every** request — no cache — so changing the router password ends every
  session at once. A missing or mismatched claim is 401; an unreadable
  `/etc/shadow` is 500, because a 401 would send the SPA to a login page that
  fails the same way.
- `Authorization: Bearer <token>` is accepted wherever the cookie is, so a
  script can reuse a token. There is no endpoint that hands one out: obtain it
  from the login response's `Set-Cookie`.
- Failed logins are rate limited per IP: 5 attempts a minute, then a 30-second
  lockout.
- `jwt_secret` is generated on first start when empty and written back under
  the config lock. It is never rewritten, which is what lets a login session
  survive an update.

## Dev mode

```bash
cd server && go run ./cmd/webui --dev
```

Plain HTTP instead of TLS, `server/testdata/dev/` for config, shadow and logs,
`devmode.Executor` instead of real shell commands, and an `admin`/`admin`
shadow file created on first run. `testdata/dev/vpn-director.json` is
gitignored.

## Build with embed

```bash
make build-webui          # web -> web-embed -> make -C server build-webui
make build-webui-arm64    # the same, cross-compiled
```

`make web-embed` copies `web/dist` into `server/cmd/webui/web/dist`, which
`go:embed` picks up; that directory is gitignored. `spaHandler` serves hashed
bundles under `assets/` as immutable and revalidates `index.html`, so a new
binary's bundle names are picked up right after an update.

## Update flow

`internal/updateflow` is shared with the bot; the Web UI handlers are adapters
over it. See `telegram-bot.md` for the script and the daemon table. Points that
belong to the Web UI half:

- Both GitHub-facing routes extend the write deadline twice — once for the API
  call, once after it returns — because the flow serializes its callers on one
  mutex and a queued caller could otherwise consume the whole deadline.
- The front end polls `/api/version` every 3 seconds with `skipAuthRedirect`,
  for at most five minutes, then reloads. `/api/update/status` lets a reloaded
  page rejoin an update it did not start.
- The login session survives an update because nothing in the flow rewrites
  `jwt_secret`. The exception is the release that introduced the `pwh` claim:
  every token issued before it needs one repeat login.

## Trust model of self-update

Release metadata comes from `api.github.com`, binaries from the URLs that
response supplies and the shell scripts from `raw.githubusercontent.com` at the
release tag; all of it is written to the router and run as root. The **entire**
integrity guarantee is TLS to github.com plus GitHub account security — there
is no signature, no checksum and no pinning. That is the same model
`install.sh`'s `curl … | bash` establishes, and it is stated rather than
assumed. What the code does enforce: an asset URL must be `https`, every string
reaching the generated script passes a strict allow-list, downloads are capped
at 50 MB, and `files/` is wiped before every attempt so a partial download
cannot be executed later.

## Known limits

- The update script is a **restart** net, not a **rollback** net: its `EXIT`
  trap brings back the daemons that were running, but a failure part-way
  through the copy step leaves a mixed set of files.
- Three GitHub consumers share the unauthenticated 60-requests-per-hour budget:
  the bot's `Flow`, the Web UI's `Flow` (separate processes, separate caches)
  and `updatechecker`'s own ticker. Nothing coordinates them.
- The update script hand-parses its daemon table in seven places in two
  spellings (`${rest##*|}`, `${entry##*|}`). Adding a fourth field to
  `updater.Daemons` breaks all of them at once. What the table may contain is
  pinned by `TestDaemons_CarryNoShellMetacharacters`; what the script renders
  is pinned by the byte-exact golden.
```

- [ ] **Step 4: Дописать дерево в `.claude/rules/telegram-bot.md`**

В блоке Architecture заменить первую строку и дополнить список пакетов:

```
server/
├── cmd/bot/main.go           # Bot entry point, signal handling, DI setup
├── cmd/webui/main.go         # Web UI entry point — see webui.md
├── internal/
│   ├── auth/                 # /etc/shadow verification and JWT — see webui.md
│   ├── webapi/               # Web UI HTTP API — see webui.md
│   ├── ssrf/                 # Dial guard: refuse private and reserved addresses
```

и в разделе `updater/` дополнить список файлов:

```
│   ├── updater/              # Self-update logic
│   │   ├── updater.go        # Daemon table (asset names, binaries, init scripts), GitHub API, lock file
│   │   ├── github.go         # GitHub release fetching
│   │   ├── downloader.go     # Asset downloading
│   │   ├── script.go         # Update script generation
│   │   ├── version.go        # Semantic version comparison and validation
│   │   └── update_script.sh.tmpl # The script rendered from the daemon table
```

В разделе `## Build` добавить цели Web UI:

```bash
# Web UI (from the repository root: needs the SPA embedded first)
make build-webui
make build-webui-arm64
make build-webui-arm
```

- [ ] **Step 5: Поправить bullet в `.claude/rules/xray-tproxy.md`**

В разделе `## Fail-Safe` пункт про неизвестные коды стран описывает не отказ, а его отсутствие, и стоит в списке причин выхода. Вынести его из списка в примечание сразу под ним:

```markdown
## Fail-Safe

Script exits without changes if:
- Required exclusion ipsets not found
- xt_TPROXY module unavailable

> An unknown country code in `xray.exclude_sets` is **not** one of those
> reasons: `_tproxy_exclude_sets` drops it with a WARN before the check, so a
> typo cannot abort apply.
```

- [ ] **Step 6: Добавить в оба README фразу про модель доверия**

`README.md`, в раздел `### Updates`, последним абзацем перед цитатой про переход:

```markdown
Updates are authenticated by TLS to github.com and nothing else — there is no signature and no checksum on the binaries or the scripts, and they are installed and run as root. This is the same trust model as the `curl … | bash` install command above; anyone who can publish a release to this repository can run code on your router.
```

`README.ru.md`, в том же месте раздела про обновления:

```markdown
Целостность обновления обеспечивается только TLS-соединением с github.com: ни бинарники, ни скрипты не подписаны и не проверяются контрольной суммой, а устанавливаются и запускаются от root. Это та же модель доверия, что и у команды быстрой установки `curl … | bash` выше: тот, кто может опубликовать релиз в этом репозитории, может выполнить код на вашем роутере.
```

- [ ] **Step 7: Сверить документацию с кодом**

Run:
```bash
# every documented route exists in the router
grep -oE '`/api/[a-z/]+`' .claude/rules/webui.md | tr -d '`' | sort -u | while read -r r; do
  grep -q -- "$r\"" server/internal/webapi/router.go || echo "documented but not routed: $r"
done
# every route in the router is documented
grep -oE '"(GET|POST|DELETE) /api/[a-z/]+"' server/internal/webapi/router.go | awk '{print $2}' | tr -d '"' | sort -u | while read -r r; do
  grep -q -- "$r" .claude/rules/webui.md || echo "routed but not documented: $r"
done
grep -c 'future WebUI' CLAUDE.md
```
Ожидается: ни одной строки от обоих циклов; последний `grep -c` печатает `0`.

- [ ] **Step 8: Коммит**

```bash
git add CLAUDE.md .claude/rules/webui.md .claude/rules/telegram-bot.md .claude/rules/xray-tproxy.md README.md README.ru.md
git commit -m "docs: describe the Web UI the four blocks actually built

CLAUDE.md still called it a future component and named neither
cmd/webui, webapi, auth nor web/. The new .claude/rules/webui.md carries
the architecture, the route table, the fingerprint-bound authentication,
dev mode, the embed build and the update flow, including the three
/api/update* routes documented nowhere until now and the limits worth
knowing: the restart-not-rollback net, three GitHub consumers on one
budget and the hand-parsed daemon table. Both READMEs now state the
update trust model instead of leaving it to be assumed." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

### Task 16: уборка репозитория

**Files:**
- Modify: `server/.gitignore`

**Interfaces:**
- Consumes: список неотслеживаемых файлов автора из Global Constraints.
- Produces: игнор для локальных сборок `server/bot` и `server/webui`.

- [ ] **Step 1: Показать проблему**

Run: `git status --short server/`
Ожидается: `?? server/bot` и `?? server/webui` — локальные сборки, которые сейчас видны в каждом `git status` и рискуют попасть в коммит.

- [ ] **Step 2: Дополнить `server/.gitignore`**

```gitignore
# Build output
bin/

# Local builds at the module root (`go build ./cmd/bot`, `go build ./cmd/webui`)
/bot
/webui
```

- [ ] **Step 3: Проверить**

Run: `git status --short server/ && git check-ignore -v server/bot server/webui`
Ожидается: обе строки `??` исчезли; `check-ignore` называет правила `/bot` и `/webui`.

- [ ] **Step 4: Спросить автора про кандидатов на удаление**

Спек §7.5 называет три кандидата и оставляет решение за автором **в момент выполнения**. Показать факты и спросить — **ничего не удалять без явного ответа**:

```bash
ls -la review.diff test_exit.sh docs/superpowers/plans/2026-03-26-exclude-ips-and-multi-resolve-continuation-prompt.md
git log --oneline -1 -- review.diff test_exit.sh 2>/dev/null
```

Вопрос автору: удалять ли `review.diff`, `test_exit.sh` и `docs/superpowers/plans/2026-03-26-…-prompt.md`. Все три неотслеживаемые, то есть «удалить» здесь означает `rm`, а не `git rm`, и в PR они в любом случае не попадут. Ответ записать в отчёт задачи.

- [ ] **Step 5: Коммит**

```bash
git add server/.gitignore
git commit -m "chore: ignore the local daemon builds in server/

go build ./cmd/bot and ./cmd/webui drop their binaries at the module
root, where they showed up in every git status and were one careless
git add away from the index." -m "Claude-Session: https://claude.ai/code/session_011XQnWGeuYU2r2sPhgNoS8Y"
git show --name-only HEAD
```

---

## Проверка блока (после Task 16, без коммита кода)

- [ ] **Step 1: Go — сборка, vet, тесты**

Через `claude-forge:build`:
```bash
cd server && go build ./... && go vet ./... && go test ./... -count=1
```
Ожидается: все пакеты PASS, ни одной строки `(cached)`.

- [ ] **Step 2: Go — гонки в затронутых пакетах**

Через `claude-forge:build`:
```bash
cd server && go test -race ./internal/auth/ ./internal/webapi/ ./internal/updater/ ./internal/startup/ ./internal/updatechecker/ ./internal/shell/ -count=1
```

- [ ] **Step 3: Фронт**

Через `claude-forge:build`: `cd web && npm ci && npm run build`
Ожидается: `vue-tsc` молчит, сборка проходит.

- [ ] **Step 4: Bats**

Run: `bats router/test` и отдельно `bats router/test/unit`
Ожидается: всё зелёное; в `unit` на четыре теста больше, чем было (новый `install.bats`), плюс два новых в `lock.bats`.

- [ ] **Step 5: Форматирование и чистота дерева**

Run:
```bash
gofmt -l $(git diff --name-only 83ce86a..HEAD -- 'server/**/*.go')
bash -n install.sh
jq . router/opt/vpn-director/vpn-director.json.template >/dev/null && echo "template OK"
git status --short
```
Ожидается: `gofmt -l` пуст; `bash -n` молчит; `template OK`; в `git status` только материал автора (`.claude/settings.local.json`, `.mcp.json`, `docs/superpowers/plans/*-prompt.md` и то, что автор решил оставить в Task 16) — ни одного файла блока.

- [ ] **Step 6: Диапазон коммитов**

Run: `git log --oneline 83ce86a..HEAD && git diff --stat 83ce86a..HEAD`
Ожидается: по коммиту на задачу (Task 1 и 2 — один общий), в диффе нет ни одного файла из списка «материал автора».

- [ ] **Step 7: Ручной чек-лист блока 4 (выполняет человек, до PR)**

1. **Установка с нуля** на роутере: `install.sh` доходит до конца, печатает «Web UI started: https://<lan-ip>:8444», и по этому адресу открывается страница логина.
2. **Повторный запуск `install.sh`** поверх работающей установки: Web UI остаётся поднятым, второй экземпляр не появляется (`/opt/etc/init.d/S98vpn-director-webui check` — один процесс).
3. **Отзыв токена:** войти в Web UI, оставить вкладку открытой, сменить пароль администратора роутера в веб-интерфейсе прошивки, вернуться в Web UI и нажать любую кнопку — страница должна отправить на логин, а не выполнить действие.
4. **Один повторный логин после обновления:** войти в версию **до** этого блока, обновиться до сборки с ним, убедиться, что после перезапуска требуется один повторный вход, а после него сессия снова переживает обновление.
5. **Сломанный `/etc/shadow`:** временно сделать файл нечитаемым (`chmod 000 /etc/shadow`), обновить страницу — Web UI отвечает 500 «authentication error», а не уходит в цикл перезагрузок. Вернуть права.
6. **Обновление через уход с вкладки:** запустить обновление из Settings, сразу уйти на Status, вернуться на Settings — экран «Updating, the server is restarting...» на месте, страница перезагружается один раз, когда поднялась новая версия.
7. **Telegram:** после обновления из Web UI в чат приходит ровно одно сообщение «Update complete: … → …» на чат, даже если у пользователя менялся @-хэндл.
8. **CI:** после push ветки убедиться, что workflow `Test` зелёный во всех трёх джобах.

- [ ] **Step 8: Перед PR (отдельным коммитом, вне этого плана)**

`git rm -r docs/superpowers/` и коммит — плановые документы не должны попасть в диff PR. Тексты остаются доступны через `git show <hash>:docs/superpowers/...`.
