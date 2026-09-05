# Web UI: надёжность (блок 2) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать связку Web UI + Telegram-бот + `vpn-director.sh` устойчивой к долгим и зависшим shell-командам, к параллельной записи одного `vpn-director.json` двумя демонами и к молчаливому пропуску apply при занятом локе; дать Web UI собственный лог-файл; перестать валить apply из-за одного несуществующего кода страны.

**Architecture:** Shell-исполнитель получает контекст с таймаутом (SIGTERM → WaitDelay → SIGKILL), сервисы задают лимит на команду и передают `vpn-director.sh --wait`, а скрипт ждёт лок вместо выхода с кодом 0. HTTP-обработчики, запускающие команды, отодвигают дедлайн ответа через `http.ResponseController`, глобальный `WriteTimeout` возвращается к 30 с. Запись конфига становится атомарной (temp + fsync + chmod 0600 + rename) и идёт через `ConfigStore.UpdateVPNConfig` под межпроцессным `flock`; метод `SaveVPNConfig` покидает интерфейс, чтобы обойти лок было невозможно на уровне компилятора. Web UI поднимает `logging.NewSlogLogger`, оба демона ротируют один список логов, источник `webui` появляется в API, боте и вкладке Logs.

**Tech Stack:** Go 1.25 (`os/exec` `CommandContext`/`Cancel`/`WaitDelay`, `net/http` `ResponseController`, `syscall.Flock`), bash + BusyBox `flock` на роутере, Bats (bats-support/bats-assert), Vue 3 (одна строка).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 5 «Блок 2. Надёжность» (в истории ветки: `git show 91f4579:docs/superpowers/specs/2026-09-05-webui-hardening-design.md`). Задача 12 закрывает риск, найденный финальным ревью блока 1 и добавленный в объём блока 2 решением автора (не из спеки).

**Status:** Block 2 complete at `ad03b7e` on `feature/webui-behavior`. Task bodies trimmed after execution; full text remains in git history.

## Global Constraints

- Ветка: `feature/webui-behavior`. Все четыре блока идут в одной ветке и одном PR, блоки выполняются по порядку; блок 2 начинается с HEAD блока 1 (`7becf18` → коммит плана). Новых веток и worktree нет.
- `docs/superpowers/` в этом блоке **не удалять**: удаление одним коммитом перед единственным PR после блока 4.
- Сборка, тесты, `vet`, `vue-tsc` — только через `claude-forge:build` (build-runner), включая исполнителей-субагентов. `bats`, `jq`, `git` — напрямую. Go-тесты всегда с `-count=1` (иначе каждый пакет может вернуться как `(cached)`).
- Git: в `git add` только явные пути; никаких `-A`, `.`, `commit -a`, `stash`, `clean`. В рабочем дереве лежат untracked-файлы автора (`.mcp.json`, `review.diff`, `test_exit.sh`, `server/bot`, `server/webui`, `docs/superpowers/*-prompt.md`) — они не должны попасть ни в один коммит.
- Каждый коммит заканчивается пустой строкой и `Claude-Session: <URL текущей сессии>` (используйте `-m "..." -m "Claude-Session: ..."`).
- Тексты из спеки дословно: ошибка таймаута `command timed out after <d>`; ошибка лока `config lock timeout`; сообщение shell `Timed out waiting for lock`; лок-файл `.vpn-director.json.lock` в каталоге конфига; опция `--wait[=SEC]`, по умолчанию 120 с; переменная `VPD_LOCK_WAIT`.
- Лимиты команд (спека §5.2): `status` 30 s; `apply`, `restart`, `stop`, `restart xray` 5 min; `update` 15 min; `curl` 15 s; `tail` 10 s. HTTP-дедлайн = лимит команды + 30 s; импорт 2 min; глобальный `WriteTimeout` = 30 s.
- Лок конфига: `syscall.Flock(LOCK_EX|LOCK_NB)`, опрос каждые 50 ms, до 30 s. Лок shell-скрипта `/var/lock/vpn-director.lock` Go не трогает.
- Права файлов конфига: 0600 (`vpn-director.json`, `servers.json`).
- Ротация: константа `logging.DefaultMaxSize = 200 * 1024`; список файлов один на оба демона.
- Go-код gofmt-чистый в затронутых файлах (`gofmt -l <файлы>` пуст). Не переформатировать файлы, которых задача не касается (в модуле есть 11 исторически не-gofmt файлов).
- Комментарии, идентификаторы, тексты коммитов — по-английски, как в остальном репозитории.
- Порядок задач обязателен: 1 → 2 → 3 (shell понимает `--wait` до того, как Go начнёт его передавать), 6 → 7 → 8 → 9 (интерфейс до миграций, удаление `SaveVPNConfig` после них), 10 → 11 (лог-файл до источника `webui`). Задачи 4, 5, 12 независимы и могут идти в любом месте после задачи 3.

---

## Файловая структура

| Область | Файлы | Ответственность |
|---|---|---|
| Исполнитель | `server/internal/shell/shell.go` (+`_test`) | `ExecContext`: контекст, SIGTERM, `WaitDelay`, текст таймаута |
| Сервисы | `server/internal/service/interfaces.go`, `vpndirector.go`, `network.go`, `logs.go`, `config.go` (+тесты) | `ShellExecutor` с ctx; константы лимитов; `--wait`; `UpdateVPNConfig` + `flock`; sentinel-ошибки |
| Dev-режим | `server/internal/devmode/executor.go` (+`_test`) | ctx в сигнатуре; пропуск `--`-опций перед командой |
| HTTP | `server/internal/webapi/deadline.go` (новый, +`_test`), `server.go`, `middleware.go`, `apply.go` (+`_test`), `handler_status.go`, `handler_servers.go`, `handler_clients.go`, `handler_excludes.go`, `handler_logs_test.go`, `test_helpers_test.go` | дедлайны ответа; `updateAndApply` + `httpError`; миграция писателей; источник `webui` |
| Конфиг | `server/internal/vpnconfig/vpnconfig.go` (+`_test`) | атомарная запись 0600; `WebUIConfig.LogLevel` |
| Бот | `server/internal/handler/{handler,clients,exclude,import,misc}.go` (+тесты), `server/internal/wizard/apply.go` (+тесты) | миграция писателей на `UpdateVPNConfig`; источник `webui` в `/logs` |
| Входные точки | `server/cmd/webui/main.go`, `server/cmd/bot/main.go` | лог-файл Web UI, уровень, ротация; `jwt_secret` через `UpdateVPNConfig`; `LogPaths["webui"]` |
| Пути/лог | `server/internal/paths/paths.go` (+`_test`), `server/internal/logging/rotation.go` (+`_test`) | `WebUILogPath`, `RotatedLogs()`, `DefaultMaxSize` |
| Shell | `router/opt/vpn-director/vpn-director.sh`, `router/opt/vpn-director/lib/common.sh`, `router/opt/vpn-director/lib/tproxy.sh` | `--wait`, ожидание лока; фильтр невалидных кодов стран |
| Bats | `router/test/unit/lock.bats` (новый), `router/test/integration/vpn_director.bats`, `router/test/unit/tproxy.bats` | `acquire_lock` в обоих режимах; парсинг `--wait`; фильтр кодов |
| Фронт | `web/src/components/LogsTab.vue` | источник `webui` в списке |
| Документация | `README.md`, `README.ru.md`, `CLAUDE.md`, `.claude/rules/shell-conventions.md`, `.claude/rules/telegram-bot.md`, `server/.gitignore` | `--wait`, `log_level`, `/logs [...|webui|all]`, вкладка Logs, лок-файл в gitignore |

---

### Task 1: `shell.ExecContext` — контекст, SIGTERM, WaitDelay

✅ Done — see commit(s): `5810626`

### Task 2: `vpn-director.sh --wait` и ожидание лока в `acquire_lock`

✅ Done — see commit(s): `4d03f38`, `1ae26dd`

### Task 3: `ShellExecutor` с контекстом, лимиты команд, `--wait`, dev-режим, моки

✅ Done — see commit(s): `6ad2bc3`

### Task 4: HTTP-дедлайны: `extendWriteDeadline`, `lockLongOp`, `statusWriter.Unwrap`, `WriteTimeout` 30 s

✅ Done — see commit(s): `176e415`, `ad03b7e` (whole-block fix wave: status and logs also extend the deadline)

### Task 5: Атомарная запись 0600 в `vpnconfig`

✅ Done — see commit(s): `7fc77fe`

### Task 6: `ConfigStore.UpdateVPNConfig` — flock, load, fn, save; пять моков

✅ Done — see commit(s): `3faaf09`

### Task 7: Писатели Web UI → `updateAndApply`

✅ Done — see commit(s): `a09b7e6`

### Task 8: Писатели бота → `UpdateVPNConfig`

✅ Done — see commit(s): `0589b08`, `b07b6fe`

### Task 9: `jwt_secret` через `UpdateVPNConfig`; `SaveVPNConfig` покидает интерфейс

✅ Done — see commit(s): `ce313de`, `e4123c4`

### Task 10: Лог-файл Web UI, `log_level`, `DefaultMaxSize`, ротация в обоих демонах

✅ Done — see commit(s): `b3d4fa0`

### Task 11: Источник `webui` в `/api/logs`, `/logs` бота и вкладке Logs

✅ Done — see commit(s): `963e790`

### Task 12: Невалидный код страны в `xray.exclude_sets` не валит apply

✅ Done — see commit(s): `71ff912`

### Task 13: Финальная проверка блока 2

✅ Done — no code commit (gates green at `71ff912`); whole-block review + one fix wave landed in `ad03b7e`

---

## Вне объёма блока 2 (follow-ups)

- Shell-писатели конфига (`configure.sh`, `install.sh`) не берут `.vpn-director.json.lock` и оставляют 0644 — решение брейнсторма («configure.sh интерактивный»).
- `servers.json` не под локом (спека защищает только `vpn-director.json`); запись атомарна.
- `vpnconfig.CollectClients` и pause-путь бота сравнивают `paused_clients` по точной строке — поведение, не надёжность.
- Bats-харнесс на фиксированных `/tmp`-путях (`test_helper.bash:27,39-47`) — причина одноразового `Executed 126 instead of expected 127`; переезд на `BATS_TEST_TMPDIR` — отдельная работа.
- `npm audit` (7 advisories) и 11 исторически не-gofmt файлов — унаследованы.
- Дедлайн импорта 2 минуты не ограничивает DNS-резолв на сервер (`ResolveIPs` без контекста) — при подписках на десятки хостов с мёртвым DNS импорт может упереться в дедлайн; спека принимает.

## Заметки для описания единого PR (часть блока 2)

- **Release note.** Конфиг `vpn-director.json` и `servers.json` после первого сохранения из бота или Web UI получают права 0600; рядом с конфигом появляется файл `.vpn-director.json.lock`. На роутерах, где бот и Web UI работают вместе, ошибка `config lock timeout` означает, что другой процесс держал лок дольше 30 с — практически недостижимо (лок держится на время load–save).
- **Release note.** Web UI пишет `/tmp/vpn-director-webui.log`; уровень — `webui.log_level` в `vpn-director.json`. Оба демона ротируют четыре лога при 200 КБ.
- **Поведение.** `vpn-director.sh --wait[=SEC]`: Go-демоны ждут занятый лок до 120 с; хуки (`firewall-start`, `wan-event`, `S99vpn-director`) без опции ведут себя по-старому. Таймауты команд: 30 с / 5 мин / 15 мин; истечение даёт `command timed out after …` и SIGTERM скрипту.
- **Поведение.** Глобальный `WriteTimeout` вернулся к 30 с; долгие хендлеры продлевают дедлайн ответа сами (лимит команды + 30 с, импорт 2 мин). После whole-block ревью то же продление получили `/api/status` и `/api/logs`.
- **Поведение (задача 12).** Неизвестный двухбуквенный код страны в `xray.exclude_sets` больше не обрывает apply и не срывает бут: он отбрасывается с `WARN Ignoring invalid country code` в `/tmp/vpn-director.log`, API отвечает 200. Это заменяет премису спеки §4.4 про `ipset_ensure`.
- **API.** Проверки 409/404 в мутациях клиентов выполняются под локом конфига; тексты и коды не менялись. Ошибка чтения конфига в мутации — 500 `failed to load configuration`; таймаут лока — 500 `failed to save configuration` с причиной в логе Web UI.
- **Интерфейсы Go.** `service.ShellExecutor.Exec` принимает `context.Context`; `service.ConfigStore` теряет `SaveVPNConfig` и получает `UpdateVPNConfig`.
