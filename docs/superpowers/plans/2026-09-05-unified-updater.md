# Web UI: единое самообновление (блок 3) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Заменить самообновление, знающее только про бота, на общий поток обновления, который выкладывает и перезапускает оба демона (`telegram-bot` и `webui`), запускается одинаково из Telegram и из Web UI и восстанавливает работавшие демоны при сбое.

**Architecture:** Пакет `updater` получает таблицу демонов (имя ассета = имя файла = имя процесса), качает по бинарнику на демона и рендерит шаблон скрипта из этой же таблицы; скрипт запоминает работавших демонов, останавливает их, копирует всё, пишет `notify.json` со `status`/`initiator` и восстанавливается через `trap ... EXIT` (у ash нет `ERR`). Новый пакет `updateflow` держит всю оркестрацию: `Check` с 30-минутным кешем и минутным порогом для `force`, `Start` с синхронным пре-флайтом и фоновой загрузкой, типизированные ошибки. Бот и Web UI становятся тонкими адаптерами над `Flow`: бот шлёт прогресс в чат, Web UI — в свой лог; `startup.CheckAndSendNotify` при `chat_id` 0 уведомляет все активные чаты.

**Tech Stack:** Go 1.25 (`text/template`, `context`, `sync.Mutex`, `errors.As`, `net/http` `ResponseController`), BusyBox ash на роутере (`#!/bin/sh`, `set -e`, `trap ... EXIT`, параметрические подстановки вместо массивов), Vue 3 + TypeScript (axios).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 6 «Блок 3. Единое самообновление». Секции 1–3 — контекст и принятые решения; секция 5 (блок 2) выполнена и служит источником уже существующих API (`extendWriteDeadline`, `UpdateVPNConfig`, `WebUILogPath`).

**Status:** Complete. All ten tasks are done on `feature/webui-behavior`, HEAD `f0d30e9`; «Проверка блока» Steps 1–5 are green and Step 6, the manual router checklist, is the author's and is still outstanding. The final whole-block review over `3830d7b..a493654` returned 0 Critical / 4 Important / 15 Minor and «merge with fixes»; the four Important findings were answered by one fix wave (`bec9a29`, `27cca41`, `f0d30e9`) and its re-review found them all addressed. Task bodies 1–7 were trimmed after execution; the full text stays in git history. The SDD ledger with every ruling, deferred minor and review verdict is `.superpowers/sdd/2026-09-05-unified-updater/progress.md`, and the deferred-minor triage that block 4 starts from is in `final-review.md` beside it.

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

✅ Done — see commit(s): `05b8774`, `da7d46b`

---

### Task 9: вкладка Settings и баннер о новой версии

✅ Done — see commit(s): `94911ff`, `28810a2`

---

### Task 10: документация блока

✅ Done — see commit(s): `2538087`, `894eb78`, `a493654`

---

## Проверка блока (после Task 10, без коммита кода)

- [x] **Steps 1–5** ✅ Done at `f0d30e9`: `go build`/`go vet`/`go test ./... -count=1` green over 20 packages with no `(cached)` line; `-race` clean over `updateflow`, `webapi`, `handler` and `updater`; `npm run build` with `vue-tsc` silent and `bats router/test/unit` 135/135; `gofmt -l` empty over all 26 changed Go files; `git status --short` free of any file of the author's.

- [ ] **Step 6: Ручной чек-лист на роутере (выполняет человек, до PR)**

1. Собрать и разложить бинарники вручную, запустить оба демона.
2. `POST /api/update/check` из браузера — показывает последний релиз.
3. Запустить обновление из Web UI: страница переходит в «Updating…», через несколько десятков секунд перезагружается с новой версией, повторный логин не требуется.
4. В Telegram приходит «Update complete: … → …» без запроса `/update`.
5. `/opt/etc/init.d/S98vpn-director-webui check` и `S98telegram-bot check` — оба running.
6. Сценарий отказа: временно переименовать `/opt/vpn-director/lib` перед обновлением, убедиться, что демоны вернулись и в Telegram пришло «Update failed: see /tmp/vpn-director-update/update.log», а сам лог на месте.
7. Проверить, что демон, остановленный до обновления, после обновления остался остановленным.

---

**Финальное ревью блока** (`3830d7b..a493654`): 0 Critical, 4 Important, 15 Minor, «merge with fixes». Четыре Important закрыты одной волной фиксов (`bec9a29`, `27cca41`, `f0d30e9`), ре-ревью подтвердило все четыре. Разбор отложенных Minor, с которого начинается блок 4, — в `.superpowers/sdd/2026-09-05-unified-updater/final-review.md`; все решения блока — в `progress.md` рядом.
