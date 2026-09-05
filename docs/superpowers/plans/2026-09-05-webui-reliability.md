# Web UI: надёжность (блок 2) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Сделать связку Web UI + Telegram-бот + `vpn-director.sh` устойчивой к долгим и зависшим shell-командам, к параллельной записи одного `vpn-director.json` двумя демонами и к молчаливому пропуску apply при занятом локе; дать Web UI собственный лог-файл; перестать валить apply из-за одного несуществующего кода страны.

**Architecture:** Shell-исполнитель получает контекст с таймаутом (SIGTERM → WaitDelay → SIGKILL), сервисы задают лимит на команду и передают `vpn-director.sh --wait`, а скрипт ждёт лок вместо выхода с кодом 0. HTTP-обработчики, запускающие команды, отодвигают дедлайн ответа через `http.ResponseController`, глобальный `WriteTimeout` возвращается к 30 с. Запись конфига становится атомарной (temp + fsync + chmod 0600 + rename) и идёт через `ConfigStore.UpdateVPNConfig` под межпроцессным `flock`; метод `SaveVPNConfig` покидает интерфейс, чтобы обойти лок было невозможно на уровне компилятора. Web UI поднимает `logging.NewSlogLogger`, оба демона ротируют один список логов, источник `webui` появляется в API, боте и вкладке Logs.

**Tech Stack:** Go 1.25 (`os/exec` `CommandContext`/`Cancel`/`WaitDelay`, `net/http` `ResponseController`, `syscall.Flock`), bash + BusyBox `flock` на роутере, Bats (bats-support/bats-assert), Vue 3 (одна строка).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 5 «Блок 2. Надёжность» (в истории ветки: `git show 91f4579:docs/superpowers/specs/2026-09-05-webui-hardening-design.md`). Задача 12 закрывает риск, найденный финальным ревью блока 1 и добавленный в объём блока 2 решением автора (не из спеки).

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

**Files:**
- Modify: `server/internal/shell/shell.go` (весь файл, 24 строки)
- Modify: `server/internal/shell/shell_test.go` (6 существующих тестов + 2 новых)

**Interfaces:**
- Consumes: ничего нового.
- Produces: `func ExecContext(ctx context.Context, command string, args ...string) (*Result, error)`; ошибка таймаута с префиксом `command timed out after `; пакетная переменная `waitDelay time.Duration` (10 s) для тестов. `Exec(command, args...)` временно остаётся тонкой обёрткой — её удалит задача 3.

- [ ] **Step 1: Переписать существующие тесты на `ExecContext` и добавить два падающих**

В `shell_test.go` заменить каждый вызов `Exec(` на `ExecContext(context.Background(), ` (6 мест), добавить `"context"` и `"time"` в импорты и дописать в конец файла:

```go
func TestExecContext_TimeoutTerminatesProcess(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// exec replaces the shell with sleep, so SIGTERM reaches sleep itself and
	// the output pipe closes as soon as it dies.
	_, err := ExecContext(ctx, "sh", "-c", "exec sleep 5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if !strings.HasPrefix(err.Error(), "command timed out after ") {
		t.Errorf("error = %q, want prefix %q", err.Error(), "command timed out after ")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ExecContext took %s; the process was not terminated on timeout", elapsed)
	}
}

func TestExecContext_TimeoutKillsChildThatIgnoresTerm(t *testing.T) {
	old := waitDelay
	waitDelay = 300 * time.Millisecond
	t.Cleanup(func() { waitDelay = old })

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	start := time.Now()
	// The shell ignores SIGTERM and its child inherits that; only the
	// WaitDelay kill and pipe close can end the call.
	_, err := ExecContext(ctx, "sh", "-c", "trap '' TERM; sleep 5")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected a timeout error, got nil")
	}
	if elapsed > 2*time.Second {
		t.Errorf("ExecContext took %s; WaitDelay did not kill the process", elapsed)
	}
}
```

- [ ] **Step 2: Убедиться, что тесты не компилируются**

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/shell/`
Expected: FAIL, `undefined: ExecContext` (и `undefined: waitDelay`).

- [ ] **Step 3: Реализовать `ExecContext`**

Новое содержимое `server/internal/shell/shell.go`:

```go
package shell

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
)

// waitDelay bounds how long ExecContext waits, after cancelling a command,
// for its stdout/stderr pipes to close. vpn-director.sh spawns wget and
// sleep; a SIGTERM to the script can leave a child holding the pipe open, and
// without this bound CombinedOutput would block until that child exits. A
// variable rather than a constant so a test can shorten it.
var waitDelay = 10 * time.Second

type Result struct {
	Output   string
	ExitCode int
}

// Exec runs the command with no deadline. Kept for the callers that still
// take no context; they move to ExecContext in the next task.
func Exec(command string, args ...string) (*Result, error) {
	return ExecContext(context.Background(), command, args...)
}

// ExecContext runs the command and captures its combined stdout and stderr.
// A non-zero exit is reported through Result.ExitCode, not as an error.
// When ctx expires the process receives SIGTERM, so vpn-director.sh's EXIT
// trap can remove its temp files, and is killed after waitDelay if it has
// not exited; the returned error then reads "command timed out after <d>",
// where d is the context's timeout.
func ExecContext(ctx context.Context, command string, args ...string) (*Result, error) {
	start := time.Now()
	cmd := exec.CommandContext(ctx, command, args...)
	cmd.Cancel = func() error { return cmd.Process.Signal(syscall.SIGTERM) }
	cmd.WaitDelay = waitDelay
	output, err := cmd.CombinedOutput()
	result := &Result{Output: string(output)}

	if err != nil && ctx.Err() != nil {
		result.ExitCode = -1
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			d := time.Since(start)
			if deadline, ok := ctx.Deadline(); ok {
				d = deadline.Sub(start)
			}
			return result, fmt.Errorf("command timed out after %s", d.Round(time.Millisecond))
		}
		return result, fmt.Errorf("command cancelled: %w", ctx.Err())
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.ExitCode = exitErr.ExitCode()
		err = nil // non-zero exit is not an error
	}
	return result, err
}
```

- [ ] **Step 4: Прогнать тесты пакета**

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/shell/ -v`
Expected: PASS, 8 тестов; оба новых укладываются в < 2 s каждый.

- [ ] **Step 5: gofmt и коммит**

```bash
gofmt -l server/internal/shell/shell.go server/internal/shell/shell_test.go   # должно быть пусто
git add server/internal/shell/shell.go server/internal/shell/shell_test.go
git commit -m "feat(shell): add ExecContext with SIGTERM cancel and a WaitDelay kill" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 2: `vpn-director.sh --wait` и ожидание лока в `acquire_lock`

**Files:**
- Modify: `router/opt/vpn-director/vpn-director.sh:13-18` (шапка), `:41-50` (`parse_option`), `:92-97` (`show_help`)
- Modify: `router/opt/vpn-director/lib/common.sh:27-29` (шапка API), `:419-445` (`acquire_lock`)
- Create: `router/test/unit/lock.bats`
- Modify: `router/test/integration/vpn_director.bats` (добавить секцию тестов `--wait` после теста `--source-only`, строка ~201)
- Modify: `.claude/rules/shell-conventions.md:16`, `CLAUDE.md:12-20` (блок Commands), `README.md:92`, `README.ru.md:92`

**Interfaces:**
- Consumes: `flock -n` (BusyBox на роутере, util-linux на dev-машине), `log` из `common.sh`.
- Produces: опция CLI `--wait[=SEC]` → `export VPD_LOCK_WAIT=<sec>` (по умолчанию 120); `acquire_lock` при заданной `VPD_LOCK_WAIT` ждёт с шагом 1 s и выходит с кодом 1 и `log -l ERROR "Timed out waiting for lock ..."`; без переменной — прежнее поведение (`flock -n`, `exit 0`). Задача 3 передаёт `--wait` из Go.

Почему сначала shell, потом Go: если Go начнёт передавать `--wait` раньше, чем скрипт его понимает, каждый apply упадёт с `Unknown option: --wait`.

- [ ] **Step 1: Написать падающие bats-тесты для `acquire_lock`**

Создать `router/test/unit/lock.bats`:

```bash
#!/usr/bin/env bats

load '../test_helper'

# acquire_lock opens /var/lock/<name>.lock on FD 200. The tests hold the same
# file on FD 201 from the test shell: `run` forks a subshell that inherits the
# open file description, and flock locks belong to descriptions, so the fresh
# open on FD 200 inside acquire_lock really contends with it.

LOCK_NAME="bats_lock_$$"
LOCK_FILE="/var/lock/${LOCK_NAME}.lock"

teardown() {
    rm -f "$LOCK_FILE"
    rm -rf /tmp/bats_test_*
    rm -rf /tmp/tunnel_director/*
}

hold_lock() {
    exec 201>"$LOCK_FILE"
    flock -n 201
}

@test "acquire_lock: takes a free lock and records the PID" {
    load_common
    run acquire_lock "$LOCK_NAME"
    assert_success
    [ -f "$LOCK_FILE" ]
    run cat "$LOCK_FILE"
    assert_output --regexp '^[0-9]+$'
}

@test "acquire_lock: busy lock without VPD_LOCK_WAIT exits 0 without waiting" {
    load_common
    hold_lock
    unset VPD_LOCK_WAIT
    local start=$SECONDS
    run acquire_lock "$LOCK_NAME"
    assert_success
    assert_output --partial "Another instance is already running"
    [ $((SECONDS - start)) -lt 2 ]
}

@test "acquire_lock: busy lock with VPD_LOCK_WAIT times out with ERROR and exit 1" {
    load_common
    hold_lock
    local start=$SECONDS
    VPD_LOCK_WAIT=1 run acquire_lock "$LOCK_NAME"
    assert_failure 1
    assert_output --partial "ERROR"
    assert_output --partial "Timed out waiting for lock"
    [ $((SECONDS - start)) -ge 1 ]
}

@test "acquire_lock: with VPD_LOCK_WAIT waits for the holder and then takes the lock" {
    load_common
    hold_lock
    # Release the shared description after ~1 s from a background subshell;
    # flock -u acts on the description, so the waiter's next retry succeeds.
    ( sleep 1; flock -u 201 ) &
    VPD_LOCK_WAIT=10 run acquire_lock "$LOCK_NAME"
    assert_success
    assert_output --partial "waiting up to 10s"
    wait
}
```

- [ ] **Step 2: Прогнать и увидеть падение**

Run: `bats router/test/unit/lock.bats`
Expected: тесты 1–2 проходят (текущее поведение), тесты 3–4 падают: третий — `assert_failure 1` не выполняется (сейчас `exit 0`), четвёртый — нет строки `waiting up to 10s`.

- [ ] **Step 3: Реализовать ожидание в `acquire_lock`**

Заменить блок `router/opt/vpn-director/lib/common.sh:419-445` (комментарий-шапка + функция) на:

```bash
###################################################################################################
# acquire_lock - acquire an exclusive lock for the running script
# -------------------------------------------------------------------------------------------------
# Usage:
#   acquire_lock             # uses get_script_name -n for lock name
#   acquire_lock foo         # uses /var/lock/foo.lock
#
# Behavior:
#   * Creates /var/lock/<name>.lock and acquires exclusive lock
#     on file descriptor 200.
#   * VPD_LOCK_WAIT unset: non-blocking. If the lock is already held, logs the
#     fact and exits with code 0 (firewall-start, wan-event and S99vpn-director
#     rely on this: a second apply during boot is simply redundant).
#   * VPD_LOCK_WAIT=<sec> (set by `vpn-director.sh --wait[=SEC]`): retries
#     `flock -n` once a second for up to <sec> seconds, because BusyBox flock
#     has no -w, then logs an ERROR and exits with code 1 so the caller learns
#     that nothing was applied.
#   * The lock persists until the script exits, automatically releasing it.
###################################################################################################
acquire_lock() {
    local name="${1:-$(get_script_name -n)}"
    local file="/var/lock/${name}.lock"
    local waited=0

    # Ensure /var/lock exists (tmpfs on most routers)
    [ -d /var/lock ] || mkdir -p /var/lock 2>/dev/null

    exec 200>"$file"           # FD 200 -> /var/lock/foo.lock
    if [[ -z ${VPD_LOCK_WAIT:-} ]]; then
        if ! flock -n 200; then
            log "Another instance is already running (lock: $file) - exiting"
            exit 0
        fi
    else
        until flock -n 200; do
            if (( waited >= VPD_LOCK_WAIT )); then
                log -l ERROR "Timed out waiting for lock (lock: $file, waited ${waited}s)"
                exit 1
            fi
            (( waited == 0 )) && log "Another instance is running (lock: $file) - waiting up to ${VPD_LOCK_WAIT}s"
            sleep 1
            waited=$((waited + 1))
        done
    fi
    printf '%s\n' "$$" 1>&200  # store our PID for clarity
}
```

Обновить строки 27–29 шапки `common.sh`:

```bash
#   acquire_lock [<name>]
#       Acquires an exclusive lock via /var/lock/<name>.lock. Without VPD_LOCK_WAIT it exits 0 at
#       once when another instance holds it; with VPD_LOCK_WAIT=<sec> it waits up to <sec> seconds
#       and exits 1 on timeout.
```

- [ ] **Step 4: Прогнать `lock.bats` и остальные shell-тесты**

Run: `bats router/test/unit/lock.bats && bats router/test/common.bats`
Expected: 4/4 и все тесты `common.bats` зелёные.

- [ ] **Step 5: Написать падающие интеграционные тесты для парсинга `--wait`**

Добавить в конец `router/test/integration/vpn_director.bats`:

```bash
# ============================================================================
# --wait option
# ============================================================================

@test "vpn-director: --wait exports VPD_LOCK_WAIT=120 by default" {
    run bash -c 'source "$1" --source-only --wait apply && echo "wait=$VPD_LOCK_WAIT cmd=$COMMAND"' -- "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output "wait=120 cmd=apply"
}

@test "vpn-director: --wait=SEC exports the given number of seconds" {
    run bash -c 'source "$1" --source-only --wait=30 apply && echo "wait=$VPD_LOCK_WAIT"' -- "$SCRIPTS_DIR/vpn-director.sh"
    assert_success
    assert_output "wait=30"
}

@test "vpn-director: --wait with a non-numeric value fails" {
    run "$SCRIPTS_DIR/vpn-director.sh" --wait=abc apply
    assert_failure
    assert_output --partial "Invalid --wait value"
}

@test "vpn-director: --help documents --wait" {
    run "$SCRIPTS_DIR/vpn-director.sh" --help
    assert_success
    assert_output --partial "--wait"
}
```

Run: `bats router/test/integration/vpn_director.bats`
Expected: 4 новых теста падают (`Unknown option: --wait`).

- [ ] **Step 6: Добавить опцию в `vpn-director.sh`**

`parse_option` (строки 41–50) целиком:

```bash
parse_option() {
    case $1 in
        -f|--force)   FORCE=1; return 0 ;;
        -q|--quiet)   QUIET=1; return 0 ;;
        -v|--verbose) VERBOSE=1; export DEBUG=1; return 0 ;;
        --dry-run)    DRY_RUN=1; return 0 ;;
        --wait)       export VPD_LOCK_WAIT=120; return 0 ;;
        --wait=*)
            local secs="${1#--wait=}"
            [[ $secs =~ ^[0-9]+$ ]] || { echo "Invalid --wait value: $secs (expected seconds)" >&2; exit 1; }
            export VPD_LOCK_WAIT="$secs"; return 0 ;;
        -h|--help)    COMMAND="help"; return 0 ;;
        -*)           echo "Unknown option: $1" >&2; exit 1 ;;
        *)            return 1 ;;
    esac
}
```

В шапке файла (после строки 17 `#   -h, --help     Show this help`) и в `show_help` (после строки `  -h, --help     Show this help`) добавить одинаковую строку:

```
  --wait[=SEC]   Wait up to SEC seconds (default 120) for a running instance instead of exiting
```

(в шапке — с префиксом `#   `).

- [ ] **Step 7: Прогнать интеграционные тесты**

Run: `bats router/test/integration/vpn_director.bats`
Expected: все зелёные, включая 4 новых.

- [ ] **Step 8: Документация**

`.claude/rules/shell-conventions.md:16` →
```
- Locking: `acquire_lock [name]` prevents concurrent script execution; with `VPD_LOCK_WAIT=<sec>` (set by `vpn-director.sh --wait[=SEC]`) it waits for the lock instead of exiting 0
```

`CLAUDE.md` — после строки 16 (`... update              # Update ipsets + reapply`) добавить:
```
/opt/vpn-director/vpn-director.sh --wait apply        # Queue for a running instance (120 s) instead of skipping
```

`README.md:92` и `README.ru.md:92` — после строки с `--dry-run apply` добавить:
```
/opt/vpn-director/vpn-director.sh --wait apply        # Wait up to 120 s for a running instance instead of skipping
```
(в `README.ru.md` комментарий: `# Ждать занятый лок до 120 с вместо пропуска`).

- [ ] **Step 9: Коммит**

```bash
git add router/opt/vpn-director/vpn-director.sh router/opt/vpn-director/lib/common.sh \
        router/test/unit/lock.bats router/test/integration/vpn_director.bats \
        .claude/rules/shell-conventions.md CLAUDE.md README.md README.ru.md
git commit -m "feat(vpn-director): add --wait so callers queue for the lock instead of skipping apply" \
  -m "acquire_lock exits 0 when the lock is busy, which is right for the boot hooks and wrong for the Go daemons: their apply became a silent no-op reported as success. With VPD_LOCK_WAIT the function retries flock -n once a second (BusyBox flock has no -w) and exits 1 with an ERROR on timeout. Hooks keep the old behaviour because they do not pass the option." \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 3: `ShellExecutor` с контекстом, лимиты команд, `--wait`, dev-режим, моки

**Files:**
- Modify: `server/internal/service/interfaces.go:16-19`, `:57-64`
- Modify: `server/internal/service/vpndirector.go` (весь файл)
- Modify: `server/internal/service/network.go:26-28`, `server/internal/service/logs.go:22-24`
- Modify: `server/internal/devmode/executor.go:43-63`, `:70-80`
- Modify: `server/internal/shell/shell.go` (удалить `Exec`)
- Test: `server/internal/service/vpndirector_test.go`, `logs_test.go`, `network_test.go`, `server/internal/devmode/executor_test.go`

**Interfaces:**
- Consumes: `shell.ExecContext` (задача 1); `vpn-director.sh --wait` (задача 2).
- Produces: `service.ShellExecutor.Exec(ctx context.Context, name string, args ...string) (*shell.Result, error)`; экспортированные константы `service.StatusTimeout = 30*time.Second`, `service.ApplyTimeout = 5*time.Minute`, `service.UpdateTimeout = 15*time.Minute`, `service.ExternalIPTimeout = 15*time.Second`, `service.TailTimeout = 10*time.Second`. Задача 4 строит из них HTTP-дедлайны. Интерфейс `service.VPNDirector` не меняется — контекст строится внутри сервисов от `context.Background()`.

- [ ] **Step 1: Обновить моки и написать падающие тесты**

`server/internal/service/vpndirector_test.go`: заменить `mockExecutor` и добавить хелперы:

```go
type mockExecutor struct {
	result   *shell.Result
	err      error
	calls    [][]string
	timeouts []time.Duration // time left to each call's context deadline
}

func (m *mockExecutor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	m.calls = append(m.calls, append([]string{name}, args...))
	if deadline, ok := ctx.Deadline(); ok {
		m.timeouts = append(m.timeouts, time.Until(deadline))
	}
	return m.result, m.err
}

// assertCall checks the recorded call's arguments after the script path.
func assertCall(t *testing.T, call []string, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(call[1:], want) {
		t.Errorf("args = %v, want %v", call[1:], want)
	}
}

// assertTimeout checks that the executor saw a context deadline about want away.
func assertTimeout(t *testing.T, got, want time.Duration) {
	t.Helper()
	if got > want || got < want-time.Second {
		t.Errorf("context timeout = %s, want about %s", got, want)
	}
}
```

Заменить проверки аргументов в существующих тестах: `Status` → `assertCall(t, mock.calls[0], "status")`; `RestartXray` → `assertCall(t, mock.calls[0], "--wait", "restart", "xray")`; `Apply` → `"--wait", "apply"`; `Restart` → `"--wait", "restart"`; `Stop` → `"--wait", "stop"`; `Update` → `"--wait", "update"`. Добавить:

```go
func TestVPNDirectorService_Timeouts(t *testing.T) {
	tests := []struct {
		name string
		call func(*VPNDirectorService) error
		want time.Duration
	}{
		{"status", func(s *VPNDirectorService) error { _, err := s.Status(); return err }, StatusTimeout},
		{"apply", (*VPNDirectorService).Apply, ApplyTimeout},
		{"restart", (*VPNDirectorService).Restart, ApplyTimeout},
		{"restart xray", (*VPNDirectorService).RestartXray, ApplyTimeout},
		{"stop", (*VPNDirectorService).Stop, ApplyTimeout},
		{"update", (*VPNDirectorService).Update, UpdateTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := &mockExecutor{result: &shell.Result{Output: "ok"}}
			svc := NewVPNDirectorService("/opt/vpn-director", mock)
			if err := tt.call(svc); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(mock.timeouts) != 1 {
				t.Fatalf("expected the executor to see one context deadline, got %d", len(mock.timeouts))
			}
			assertTimeout(t, mock.timeouts[0], tt.want)
		})
	}
}
```

`logs_test.go` — добавить:

```go
func TestLogService_Read_UsesTailTimeout(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "line"}}
	svc := NewLogService(mock)
	if _, err := svc.Read("/tmp/test.log", 5); err != nil {
		t.Fatalf("Read error: %v", err)
	}
	if len(mock.timeouts) != 1 {
		t.Fatalf("expected one context deadline, got %d", len(mock.timeouts))
	}
	assertTimeout(t, mock.timeouts[0], TailTimeout)
}
```

`network_test.go` — добавить:

```go
func TestNetworkService_GetExternalIP_UsesTimeout(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "1.2.3.4"}}
	svc := NewNetworkService(mock)
	if _, err := svc.GetExternalIP(); err != nil {
		t.Fatalf("GetExternalIP error: %v", err)
	}
	if len(mock.timeouts) != 1 {
		t.Fatalf("expected one context deadline, got %d", len(mock.timeouts))
	}
	assertTimeout(t, mock.timeouts[0], ExternalIPTimeout)
}
```

`server/internal/devmode/executor_test.go`: сигнатура мока `Exec(ctx context.Context, name string, args ...string)` с полем `ctxs []context.Context` (append каждого ctx); все вызовы `exec.Exec(` → `exec.Exec(context.Background(), `; добавить:

```go
func TestExecutor_MockCommand_SkipsLeadingOptions(t *testing.T) {
	exec := NewExecutorWithReal(&mockExecutor{})

	// VPNDirectorService passes --wait before the command; the mock must not
	// take the option for the command name.
	result, err := exec.Exec(context.Background(), "vpn-director.sh", "--wait", "apply")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ExitCode != 0 || !strings.Contains(result.Output, "applied") {
		t.Errorf("expected the apply mock response, got exit %d: %q", result.ExitCode, result.Output)
	}
}

func TestExecutor_SafeCommand_PassesContext(t *testing.T) {
	mock := &mockExecutor{result: &shell.Result{Output: "ok"}}
	exec := NewExecutorWithReal(mock)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	if _, err := exec.Exec(ctx, "curl", "-s", "https://example.com"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.ctxs) != 1 {
		t.Fatalf("expected 1 call, got %d", len(mock.ctxs))
	}
	if _, ok := mock.ctxs[0].Deadline(); !ok {
		t.Error("the caller's context deadline did not reach the real executor")
	}
}
```

- [ ] **Step 2: Убедиться, что модуль не компилируется**

Run (через `claude-forge:build`): `cd server && go vet ./internal/service/ ./internal/devmode/`
Expected: ошибки компиляции (mockExecutor не реализует старый `ShellExecutor`, `assertTimeout` использует несуществующие константы).

- [ ] **Step 3: Изменить интерфейс и исполнитель по умолчанию**

`server/internal/service/interfaces.go` — импорт `"context"`; строки 16–19:

```go
// ShellExecutor is the interface for executing shell commands. ctx bounds the
// command: services build it from context.Background() with their own
// per-command timeout, never from an HTTP request, so a closed browser tab
// cannot kill an apply half-way through its iptables changes.
type ShellExecutor interface {
	Exec(ctx context.Context, name string, args ...string) (*shell.Result, error)
}
```

строки 57–64:

```go
// defaultExecutor wraps shell.ExecContext
type defaultExecutor struct{}

func (e *defaultExecutor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	return shell.ExecContext(ctx, name, args...)
}
```

Удалить `Exec` из `server/internal/shell/shell.go` (функцию и её комментарий; `context` остаётся в импортах для `ExecContext`).

- [ ] **Step 4: Переписать `vpndirector.go`**

Новое содержимое `server/internal/service/vpndirector.go`:

```go
// internal/service/vpndirector.go
package service

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/shell"
)

// Per-command limits for vpn-director.sh. A limit is a safety net against a
// hung script, not an expected duration: apply and update download country
// ipsets and legitimately run for minutes on a throttled link. Each limit
// includes up to two minutes the script may spend in --wait for its lock.
const (
	// StatusTimeout bounds `status`, which only reads kernel state.
	StatusTimeout = 30 * time.Second
	// ApplyTimeout bounds apply, restart, stop and restart xray.
	ApplyTimeout = 5 * time.Minute
	// UpdateTimeout bounds `update`, which re-downloads every configured
	// country set from up to three sources.
	UpdateTimeout = 15 * time.Minute
)

// Compile-time interface check
var _ VPNDirector = (*VPNDirectorService)(nil)

// VPNDirectorService handles VPN Director shell operations
type VPNDirectorService struct {
	scriptsDir string
	executor   ShellExecutor
}

// NewVPNDirectorService creates a new VPNDirectorService
func NewVPNDirectorService(scriptsDir string, executor ShellExecutor) *VPNDirectorService {
	if executor == nil {
		executor = DefaultExecutor()
	}
	return &VPNDirectorService{
		scriptsDir: scriptsDir,
		executor:   executor,
	}
}

func (s *VPNDirectorService) scriptPath() string {
	return filepath.Join(s.scriptsDir, "vpn-director.sh")
}

// run executes vpn-director.sh with args under timeout.
func (s *VPNDirectorService) run(timeout time.Duration, args ...string) (*shell.Result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return s.executor.Exec(ctx, s.scriptPath(), args...)
}

// runChecked runs a mutating command and turns a non-zero exit into an error
// carrying the script output, so callers can show its last line. --wait makes
// the script queue for its own lock instead of exiting 0 when another
// instance is running, which used to turn a concurrent apply into a silent
// no-op reported as success.
func (s *VPNDirectorService) runChecked(timeout time.Duration, what string, args ...string) error {
	result, err := s.run(timeout, append([]string{"--wait"}, args...)...)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("%s failed (exit %d): %s", what, result.ExitCode, result.Output)
	}
	return nil
}

// Status returns VPN Director status
func (s *VPNDirectorService) Status() (string, error) {
	result, err := s.run(StatusTimeout, "status")
	if err != nil {
		return "", err
	}
	return result.Output, nil
}

// Apply applies VPN Director configuration
func (s *VPNDirectorService) Apply() error { return s.runChecked(ApplyTimeout, "apply", "apply") }

// Restart restarts VPN Director
func (s *VPNDirectorService) Restart() error { return s.runChecked(ApplyTimeout, "restart", "restart") }

// RestartXray restarts only Xray
func (s *VPNDirectorService) RestartXray() error {
	return s.runChecked(ApplyTimeout, "restart xray", "restart", "xray")
}

// Stop stops VPN Director
func (s *VPNDirectorService) Stop() error { return s.runChecked(ApplyTimeout, "stop", "stop") }

// Update downloads fresh ipsets and reapplies VPN Director configuration
func (s *VPNDirectorService) Update() error { return s.runChecked(UpdateTimeout, "update", "update") }
```

- [ ] **Step 5: Таймауты в `network.go` и `logs.go`**

`network.go` — импорты `"context"`, `"time"`; перед `NetworkService`:

```go
// ExternalIPTimeout bounds the curl call behind /ip and the Status tab. curl
// has its own --max-time 10; this is the outer safety net.
const ExternalIPTimeout = 15 * time.Second
```

и в `GetExternalIP`:

```go
	ctx, cancel := context.WithTimeout(context.Background(), ExternalIPTimeout)
	defer cancel()
	result, err := s.executor.Exec(ctx, "curl", "-s", "--connect-timeout", "5", "--max-time", "10", "ifconfig.me")
```

`logs.go` — импорты `"context"`, `"fmt"`, `"time"`; перед `LogService`:

```go
// TailTimeout bounds `tail -n` on a log file; reading a local file should
// never take long, so a hit means a hung filesystem, not a slow log.
const TailTimeout = 10 * time.Second
```

и в `Read`:

```go
	ctx, cancel := context.WithTimeout(context.Background(), TailTimeout)
	defer cancel()
	result, err := s.executor.Exec(ctx, "tail", "-n", fmt.Sprintf("%d", lines), path)
```

- [ ] **Step 6: Dev-исполнитель**

`server/internal/devmode/executor.go` — импорты `"context"`, `"strings"`; сигнатура и проброс:

```go
func (e *Executor) Exec(ctx context.Context, name string, args ...string) (*shell.Result, error) {
	baseName := filepath.Base(name)

	// Check if it's a safe command
	if e.isSafe(baseName) {
		slog.Info("DEV: executing safe command", "command", baseName, "args", args)
		return e.real.Exec(ctx, name, args...)
	}
	...
```

В `mockVPNDirector` перед `if len(args) == 0`:

```go
	// Options such as --wait precede the command; the mock ignores them.
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		args = args[1:]
	}
```

- [ ] **Step 7: Собрать и прогнать весь модуль**

Run (через `claude-forge:build`): `cd server && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: сборка и vet чистые; все пакеты `ok`, ни одного `(cached)`; новые тесты `TestVPNDirectorService_Timeouts` (6 подтестов), `TestLogService_Read_UsesTailTimeout`, `TestNetworkService_GetExternalIP_UsesTimeout`, `TestExecutor_MockCommand_SkipsLeadingOptions`, `TestExecutor_SafeCommand_PassesContext` проходят.

- [ ] **Step 8: gofmt и коммит**

```bash
files="server/internal/shell/shell.go server/internal/service/interfaces.go server/internal/service/vpndirector.go \
  server/internal/service/network.go server/internal/service/logs.go server/internal/devmode/executor.go \
  server/internal/service/vpndirector_test.go server/internal/service/logs_test.go server/internal/service/network_test.go \
  server/internal/devmode/executor_test.go"
gofmt -l $files   # должно быть пусто
git add $files
git commit -m "feat(service): bound every shell command with a context timeout and pass --wait to vpn-director.sh" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 4: HTTP-дедлайны: `extendWriteDeadline`, `lockLongOp`, `statusWriter.Unwrap`, `WriteTimeout` 30 s

**Files:**
- Create: `server/internal/webapi/deadline.go`, `server/internal/webapi/deadline_test.go`
- Modify: `server/internal/webapi/middleware.go:196-210` (statusWriter), `server/internal/webapi/server.go:12-23`, `:45`
- Modify: `server/internal/webapi/handler_status.go` (apply, restart, stop, update ipsets), `handler_servers.go` (select, import), `handler_clients.go` (add, pause, resume, delete), `handler_excludes.go` (sets, add ip, delete ip)

**Interfaces:**
- Consumes: `service.ApplyTimeout`, `service.UpdateTimeout` (задача 3).
- Produces: `extendWriteDeadline(w http.ResponseWriter, d time.Duration)`; `lockLongOp(w http.ResponseWriter, deps *Deps, d time.Duration) func()`; константы `applyDeadline`, `updateDeadline`, `importDeadline`; `(*statusWriter).Unwrap() http.ResponseWriter`. Задача 7 сохраняет вызовы `lockLongOp` в хендлерах при миграции.

Ловушка, ради которой существует `Unwrap`: `loggingMiddleware` оборачивает ответ в `statusWriter`; без `Unwrap` `http.NewResponseController(w).SetWriteDeadline` возвращает `ErrNotSupported`, хелпер молча его игнорирует, и 30-секундный `WriteTimeout` продолжает рвать соединение — ровно то, что блок 1 закрыл десятиминутной константой.

- [ ] **Step 1: Написать падающие тесты**

Создать `server/internal/webapi/deadline_test.go`:

```go
package webapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// deadlineRecorder is a ResponseRecorder that also accepts write deadlines,
// standing in for the *http.response the server hands to handlers.
type deadlineRecorder struct {
	*httptest.ResponseRecorder
	deadlines []time.Time
}

func (d *deadlineRecorder) SetWriteDeadline(t time.Time) error {
	d.deadlines = append(d.deadlines, t)
	return nil
}

func newDeadlineRecorder() *deadlineRecorder {
	return &deadlineRecorder{ResponseRecorder: httptest.NewRecorder()}
}

// assertDeadlineAbout checks that the most recent deadline is now+want, within 2 s.
func assertDeadlineAbout(t *testing.T, rec *deadlineRecorder, want time.Duration) {
	t.Helper()
	if len(rec.deadlines) == 0 {
		t.Fatal("no write deadline was set")
	}
	got := time.Until(rec.deadlines[len(rec.deadlines)-1])
	if got > want || got < want-2*time.Second {
		t.Errorf("deadline in %s, want about %s", got, want)
	}
}

func TestExtendWriteDeadline_IgnoresUnsupportedWriter(t *testing.T) {
	rec := httptest.NewRecorder() // has no SetWriteDeadline: must neither panic nor fail
	extendWriteDeadline(rec, time.Minute)
}

func TestExtendWriteDeadline_ReachesWriterThroughStatusWriter(t *testing.T) {
	rec := newDeadlineRecorder()
	sw := &statusWriter{ResponseWriter: rec, status: http.StatusOK}
	extendWriteDeadline(sw, applyDeadline)
	assertDeadlineAbout(t, rec, applyDeadline)
}

func TestLockLongOp_ExtendsBeforeAndAfterTheWait(t *testing.T) {
	deps := newTestDeps(t)
	rec := newDeadlineRecorder()

	unlock := lockLongOp(rec, deps, updateDeadline)
	unlock()

	if len(rec.deadlines) != 2 {
		t.Fatalf("expected 2 deadline extensions (before and after the lock), got %d", len(rec.deadlines))
	}
	assertDeadlineAbout(t, rec, updateDeadline)

	// The mutex must be released: a second Lock must not block.
	locked := make(chan struct{})
	go func() { deps.OpMutex.Lock(); deps.OpMutex.Unlock(); close(locked) }()
	select {
	case <-locked:
	case <-time.After(time.Second):
		t.Fatal("OpMutex still held after unlock()")
	}
}

func TestExtendWriteDeadline_OutlivesServerWriteTimeout(t *testing.T) {
	// The real chain: net/http response -> loggingMiddleware's statusWriter ->
	// handler. WriteTimeout is 200 ms and the handler answers after 600 ms;
	// only a working extension lets the body reach the client.
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		extendWriteDeadline(w, 5*time.Second)
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte("late but delivered"))
	}))
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err != nil {
		t.Fatalf("request failed, so the write deadline was not extended: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if string(body) != "late but delivered" {
		t.Errorf("body = %q, want %q", body, "late but delivered")
	}
}

func TestExtendWriteDeadline_ControlWithoutExtensionFails(t *testing.T) {
	// Proves the previous test is not vacuous: the same server without the
	// extension tears the connection down.
	handler := loggingMiddleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(600 * time.Millisecond)
		_, _ = w.Write([]byte("too late"))
	}))
	srv := httptest.NewUnstartedServer(handler)
	srv.Config.WriteTimeout = 200 * time.Millisecond
	srv.Start()
	defer srv.Close()

	resp, err := srv.Client().Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected the 200 ms WriteTimeout to tear the connection, but the request succeeded")
	}
}

func TestLongOpHandlers_ExtendWriteDeadline(t *testing.T) {
	tests := []struct {
		name    string
		method  string
		path    string
		body    string
		handler func(*Deps) http.HandlerFunc
		want    time.Duration
	}{
		{"apply", "POST", "/api/apply", "", handleApply, applyDeadline},
		{"restart", "POST", "/api/restart", "", handleRestart, applyDeadline},
		{"stop", "POST", "/api/stop", "", handleStop, applyDeadline},
		{"update ipsets", "POST", "/api/ipsets/update", "", handleUpdateIPsets, updateDeadline},
		{"add client", "POST", "/api/clients", `{"ip":"192.168.50.10","route":"xray"}`, handleAddClient, applyDeadline},
		{"pause client", "POST", "/api/clients/pause?ip=192.168.50.10", "", handlePauseClient, applyDeadline},
		{"resume client", "POST", "/api/clients/resume?ip=192.168.50.10", "", handleResumeClient, applyDeadline},
		{"delete client", "DELETE", "/api/clients?ip=192.168.50.10", "", handleDeleteClient, applyDeadline},
		{"update exclude sets", "POST", "/api/excludes/sets", `{"sets":["ru"]}`, handleUpdateExcludeSets, applyDeadline},
		{"add exclude ip", "POST", "/api/excludes/ips", `{"ip":"1.2.3.4"}`, handleAddExcludeIP, applyDeadline},
		{"delete exclude ip", "DELETE", "/api/excludes/ips?ip=1.2.3.4", "", handleDeleteExcludeIP, applyDeadline},
		{"select server", "POST", "/api/servers/active", `{"index":0}`, handleSelectServer, applyDeadline},
		{"import (rejected before download)", "POST", "/api/servers/import", `{"url":"http://insecure.example"}`, handleImportServers, importDeadline},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := newTestDeps(t)
			deps.Config = &mockConfig{
				cfg: &vpnconfig.VPNDirectorConfig{
					TunnelDirector: vpnconfig.TunnelDirectorConfig{Tunnels: map[string]vpnconfig.TunnelConfig{}},
				},
				servers: []vpnconfig.Server{{Address: "srv", Port: 443, UUID: "u", IPs: []string{"1.1.1.1"}}},
			}
			rec := newDeadlineRecorder()
			req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
			tt.handler(deps).ServeHTTP(rec, req)
			assertDeadlineAbout(t, rec, tt.want)
		})
	}
}
```

- [ ] **Step 2: Убедиться, что не компилируется**

Run (через `claude-forge:build`): `cd server && go vet ./internal/webapi/`
Expected: `undefined: extendWriteDeadline`, `undefined: lockLongOp`, `undefined: applyDeadline` и т. д.

- [ ] **Step 3: Создать `deadline.go`**

```go
package webapi

import (
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
)

// Response deadlines for handlers that run shell commands. Each is the
// command's own timeout plus deadlineSlack, so the handler can still write
// its response after the service layer has cut the command off. The
// server-wide WriteTimeout (30 s, server.go) stays in force for every other
// route.
const (
	deadlineSlack  = 30 * time.Second
	applyDeadline  = service.ApplyTimeout + deadlineSlack
	updateDeadline = service.UpdateTimeout + deadlineSlack
	// importDeadline covers the 10-second subscription download plus one DNS
	// lookup per server; the import runs no shell command.
	importDeadline = 2 * time.Minute
)

// extendWriteDeadline pushes the response deadline past the server-wide
// WriteTimeout for handlers that run long shell commands. Writers that do
// not support deadlines (httptest.ResponseRecorder) are ignored; any other
// failure is logged, because a silently lost deadline reproduces the torn
// connection this helper exists to prevent.
func extendWriteDeadline(w http.ResponseWriter, d time.Duration) {
	err := http.NewResponseController(w).SetWriteDeadline(time.Now().Add(d))
	if err != nil && !errors.Is(err, http.ErrNotSupported) {
		slog.Warn("failed to extend write deadline", "error", err)
	}
}

// lockLongOp serializes a shell-running handler on deps.OpMutex and extends
// the write deadline both before and after the wait, so the deadline covers
// the command itself and not the time spent queued behind another one. The
// returned func releases the mutex.
func lockLongOp(w http.ResponseWriter, deps *Deps, d time.Duration) func() {
	extendWriteDeadline(w, d)
	deps.OpMutex.Lock()
	extendWriteDeadline(w, d)
	return deps.OpMutex.Unlock
}
```

- [ ] **Step 4: `Unwrap` у `statusWriter` и `WriteTimeout` 30 s**

В `middleware.go` после `WriteHeader`:

```go
// Unwrap exposes the underlying writer so http.ResponseController can reach
// its SetWriteDeadline. Without it every extendWriteDeadline call would fail
// with ErrNotSupported and the 30-second WriteTimeout would still apply.
func (sw *statusWriter) Unwrap() http.ResponseWriter { return sw.ResponseWriter }
```

В `server.go` удалить константу `writeTimeout` вместе с её комментарием (строки 12–23) и заменить строку `WriteTimeout: writeTimeout,` на:

```go
		// Handlers that run vpn-director.sh extend this per request, see
		// extendWriteDeadline; every other route answers within seconds.
		WriteTimeout: 30 * time.Second,
```

- [ ] **Step 5: Подключить хелперы в тринадцати хендлерах**

В `handler_status.go` в `handleApply`, `handleRestart`, `handleStop` заменить

```go
		deps.OpMutex.Lock()
		defer deps.OpMutex.Unlock()
```
на
```go
		unlock := lockLongOp(w, deps, applyDeadline)
		defer unlock()
```
в `handleUpdateIPsets` — то же с `updateDeadline`.

В `handler_clients.go` (`handleAddClient`, `handlePauseClient`, `handleResumeClient`, `handleDeleteClient`) и `handler_excludes.go` (`handleUpdateExcludeSets`, `handleAddExcludeIP`, `handleDeleteExcludeIP`) — та же замена с `applyDeadline`.

В `handler_servers.go`: в `handleSelectServer` заменить пару `Lock/defer Unlock` на `unlock := lockLongOp(w, deps, applyDeadline); defer unlock()`. В `handleImportServers` первой строкой тела хендлера добавить `extendWriteDeadline(w, importDeadline)` (загрузка подписки и DNS идут до мьютекса), а пару `Lock/defer Unlock` перед `SaveServers` заменить на `unlock := lockLongOp(w, deps, importDeadline); defer unlock()`.

- [ ] **Step 6: Прогнать пакет**

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/webapi/ -run 'Deadline|LongOp' -v && go test -count=1 ./internal/webapi/`
Expected: 7 новых тестов (включая 13 подтестов таблицы) PASS; весь пакет зелёный.

- [ ] **Step 7: gofmt и коммит**

```bash
files="server/internal/webapi/deadline.go server/internal/webapi/deadline_test.go server/internal/webapi/middleware.go \
  server/internal/webapi/server.go server/internal/webapi/handler_status.go server/internal/webapi/handler_servers.go \
  server/internal/webapi/handler_clients.go server/internal/webapi/handler_excludes.go"
gofmt -l $files   # должно быть пусто
git add $files
git commit -m "fix(webapi): extend the write deadline per long-running handler and restore the 30-second WriteTimeout" \
  -m "Block 1 raised WriteTimeout to ten minutes so an apply could finish; that also covered every read-only route and the SPA. Each shell-running handler now pushes its own deadline through http.ResponseController (command timeout plus 30 s), before and after waiting for OpMutex. statusWriter gains Unwrap, without which the controller cannot reach the connection and the extension is silently lost." \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 5: Атомарная запись 0600 в `vpnconfig`

**Files:**
- Modify: `server/internal/vpnconfig/vpnconfig.go:114-137`
- Test: `server/internal/vpnconfig/vpnconfig_test.go` (добавить в конец)

**Interfaces:**
- Consumes: ничего нового.
- Produces: `SaveServers` и `SaveVPNDirectorConfig` пишут через временный файл в том же каталоге, `Sync`, `Chmod(0600)`, `Rename`; неэкспортированный `writeFileAtomic(path string, data []byte) error`. Задача 6 опирается на атомарность: читатели остаются без лока.

- [ ] **Step 1: Падающие тесты**

Добавить в `vpnconfig_test.go`:

```go
// assertNoTempFiles fails if writeFileAtomic left a temp file behind.
func assertNoTempFiles(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp-") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
}

func TestSaveVPNDirectorConfig_AtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vpn-director.json")
	// A pre-existing world-readable file must come back as 0600.
	if err := os.WriteFile(path, []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := SaveVPNDirectorConfig(path, &VPNDirectorConfig{DataDir: "/data"}); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
	loaded, err := LoadVPNDirectorConfig(path)
	if err != nil || loaded.DataDir != "/data" {
		t.Fatalf("round trip failed: %v, %+v", err, loaded)
	}
	assertNoTempFiles(t, dir)
}

func TestSaveServers_AtomicAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "servers.json")

	if err := SaveServers(path, []Server{{Address: "a", Port: 443, UUID: "u"}}); err != nil {
		t.Fatalf("save: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("mode = %o, want 0600", perm)
	}
	assertNoTempFiles(t, dir)
}

func TestSaveVPNDirectorConfig_FailedRenameLeavesNoTemp(t *testing.T) {
	dir := t.TempDir()
	// A directory in place of the target makes the final rename fail after
	// the temp file has already been written and synced.
	target := filepath.Join(dir, "vpn-director.json")
	if err := os.MkdirAll(filepath.Join(target, "occupied"), 0755); err != nil {
		t.Fatal(err)
	}

	if err := SaveVPNDirectorConfig(target, &VPNDirectorConfig{}); err == nil {
		t.Fatal("expected an error when the target is a directory")
	}
	assertNoTempFiles(t, dir)
}
```

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/vpnconfig/ -run 'AtomicAndPrivate|FailedRename' -v`
Expected: FAIL — `mode = 644, want 0600` в двух тестах; третий проходит случайно (сейчас `os.WriteFile` на каталог тоже падает) — это нормально, он пришпиливает уборку temp-файла на будущее.

- [ ] **Step 2: Реализация**

В `vpnconfig.go` импорт `"path/filepath"`; заменить тела `SaveServers` и `SaveVPNDirectorConfig` и добавить хелпер:

```go
func SaveServers(path string, servers []Server) error {
	data, err := json.MarshalIndent(servers, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}
```

```go
func SaveVPNDirectorConfig(path string, cfg *VPNDirectorConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(path, append(data, '\n'))
}

// writeFileAtomic writes data to path through a temp file in the same
// directory, fsyncs it, sets 0600 and renames it over path. A reader (the
// shell scripts, the other daemon) therefore never sees a partial file, a
// crash leaves either the old or the new content, and jwt_secret and server
// UUIDs stop being world-readable.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op once the rename succeeds

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(0600); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
```

- [ ] **Step 3: Прогнать пакет**

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/vpnconfig/`
Expected: PASS, включая старые `TestSaveServers_RoundTrip`, `TestSaveVPNDirectorConfig_RoundTrip`, `_FormattedJSON`, `_PausedClients_OmitEmpty`.

- [ ] **Step 4: gofmt и коммит**

```bash
gofmt -l server/internal/vpnconfig/vpnconfig.go server/internal/vpnconfig/vpnconfig_test.go
git add server/internal/vpnconfig/vpnconfig.go server/internal/vpnconfig/vpnconfig_test.go
git commit -m "fix(vpnconfig): write vpn-director.json and servers.json atomically with 0600" -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 6: `ConfigStore.UpdateVPNConfig` — flock, load, fn, save; пять моков

**Files:**
- Modify: `server/internal/service/interfaces.go:21-30`
- Modify: `server/internal/service/config.go` (структура, конструктор, новые методы)
- Modify: `server/.gitignore`
- Test: `server/internal/service/config_test.go`; моки: `server/internal/webapi/test_helpers_test.go:47-75`, `server/internal/handler/status_test.go:27-39`, `server/internal/handler/clients_test.go:49-66`, `server/internal/wizard/server_test.go:61-95`, `server/internal/wizard/apply_test.go:45-83` (`mockConfigStoreForImport` в `handler/import_test.go` наследует метод через встраивание)

**Interfaces:**
- Consumes: атомарный `SaveVPNDirectorConfig` (задача 5).
- Produces: метод интерфейса `UpdateVPNConfig(fn func(cfg *vpnconfig.VPNDirectorConfig) error) error`; sentinel-ошибки `service.ErrConfigLoad` (обёртка ошибки чтения, `fn` не вызывался) и `service.ErrConfigLockTimeout` (текст `config lock timeout`); `(*ConfigService).LockPath() string`. Контракт моков: ошибка загрузки → `fmt.Errorf("%w: %w", service.ErrConfigLoad, err)`; `fn` на собственном `cfg`; ошибка `fn` → возврат без сохранения; успех → `savedCfg`/`savedConfig` = тот же `cfg`, затем ошибка сохранения, если задана. Задачи 7–9 переводят писателей на этот метод.

- [ ] **Step 1: Падающие тесты сервиса**

Добавить в `server/internal/service/config_test.go` (импорты `errors`, `sync`, `syscall`, `time`, `vpnconfig`):

```go
func writeTestConfig(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "vpn-director.json"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestConfigService_UpdateVPNConfig_LoadMutateSave(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{"data_dir": "/d", "xray": {"clients": ["1.1.1.1"]}}`)
	svc := NewConfigService(dir, filepath.Join(dir, "data"))

	err := svc.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.Xray.Clients = append(cfg.Xray.Clients, "2.2.2.2")
		return nil
	})
	if err != nil {
		t.Fatalf("UpdateVPNConfig: %v", err)
	}

	cfg, err := svc.LoadVPNConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Xray.Clients) != 2 || cfg.Xray.Clients[1] != "2.2.2.2" {
		t.Errorf("clients = %v, want the appended entry saved", cfg.Xray.Clients)
	}
	if _, err := os.Stat(svc.LockPath()); err != nil {
		t.Errorf("lock file %s not created: %v", svc.LockPath(), err)
	}
}

func TestConfigService_UpdateVPNConfig_FnErrorSkipsSaveAndReleasesLock(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{"data_dir": "/d"}`)
	svc := NewConfigService(dir, filepath.Join(dir, "data"))
	svc.lockTimeout = 200 * time.Millisecond

	sentinel := errors.New("nope")
	err := svc.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.DataDir = "/changed"
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want the fn error unchanged", err)
	}
	cfg, _ := svc.LoadVPNConfig()
	if cfg.DataDir != "/d" {
		t.Error("a change made by a failing fn must not be saved")
	}
	// The lock must be released: a second update succeeds within the timeout.
	if err := svc.UpdateVPNConfig(func(*vpnconfig.VPNDirectorConfig) error { return nil }); err != nil {
		t.Fatalf("lock not released after fn error: %v", err)
	}
}

func TestConfigService_UpdateVPNConfig_MissingConfigIsErrConfigLoad(t *testing.T) {
	dir := t.TempDir()
	svc := NewConfigService(dir, filepath.Join(dir, "data"))

	called := false
	err := svc.UpdateVPNConfig(func(*vpnconfig.VPNDirectorConfig) error { called = true; return nil })
	if !errors.Is(err, ErrConfigLoad) {
		t.Fatalf("err = %v, want ErrConfigLoad", err)
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the cause was lost: %v", err)
	}
	if called {
		t.Error("fn must not run when the config cannot be loaded")
	}
}

func TestConfigService_UpdateVPNConfig_TimesOutWhenLockHeld(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{"data_dir": "/d"}`)
	svc := NewConfigService(dir, filepath.Join(dir, "data"))
	svc.lockTimeout = 200 * time.Millisecond
	svc.lockPoll = 20 * time.Millisecond

	holder, err := os.OpenFile(svc.LockPath(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	if err := syscall.Flock(int(holder.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}

	called := false
	start := time.Now()
	err = svc.UpdateVPNConfig(func(*vpnconfig.VPNDirectorConfig) error { called = true; return nil })
	if !errors.Is(err, ErrConfigLockTimeout) {
		t.Fatalf("err = %v, want ErrConfigLockTimeout", err)
	}
	if err.Error() != "config lock timeout" {
		t.Errorf("error text = %q, want %q", err.Error(), "config lock timeout")
	}
	if called {
		t.Error("fn must not run without the lock")
	}
	if time.Since(start) < 200*time.Millisecond {
		t.Error("returned before the lock timeout elapsed")
	}
}

func TestConfigService_UpdateVPNConfig_SerializesConcurrentWriters(t *testing.T) {
	dir := t.TempDir()
	writeTestConfig(t, dir, `{"xray": {"clients": []}}`)
	// Two services, two lock descriptors, like the bot and the Web UI.
	a := NewConfigService(dir, filepath.Join(dir, "data"))
	b := NewConfigService(dir, filepath.Join(dir, "data"))

	const n = 25
	var wg sync.WaitGroup
	for _, svc := range []*ConfigService{a, b} {
		wg.Add(1)
		go func(svc *ConfigService) {
			defer wg.Done()
			for i := 0; i < n; i++ {
				err := svc.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
					cfg.Xray.Clients = append(cfg.Xray.Clients, "x")
					return nil
				})
				if err != nil {
					t.Error(err)
					return
				}
			}
		}(svc)
	}
	wg.Wait()

	cfg, err := a.LoadVPNConfig()
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Xray.Clients) != 2*n {
		t.Errorf("lost updates: %d clients saved, want %d", len(cfg.Xray.Clients), 2*n)
	}
}
```

Run (через `claude-forge:build`): `cd server && go vet ./internal/service/`
Expected: `svc.UpdateVPNConfig undefined`, `undefined: ErrConfigLoad`, `svc.lockTimeout undefined`.

- [ ] **Step 2: Интерфейс и sentinel-ошибки**

`interfaces.go` — импорт `"errors"`; перед `ConfigStore`:

```go
// ErrConfigLoad marks an UpdateVPNConfig failure that happened before fn ran:
// vpn-director.json could not be read and nothing was changed. The cause is
// wrapped as well, so errors.Is(err, os.ErrNotExist) still works.
var ErrConfigLoad = errors.New("load config")

// ErrConfigLockTimeout is returned when another process held the config lock
// for the whole wait; nothing was read or written.
var ErrConfigLockTimeout = errors.New("config lock timeout")
```

В `ConfigStore` после `SaveServers`:

```go
	// UpdateVPNConfig runs fn under an exclusive cross-process lock:
	// lock, load, fn, save, unlock. Readers stay lock-free because Save is
	// atomic. A load failure comes back wrapped in ErrConfigLoad, an error
	// from fn is returned as is and skips the save, and a lock held by
	// another process for the whole wait yields ErrConfigLockTimeout.
	UpdateVPNConfig(fn func(cfg *vpnconfig.VPNDirectorConfig) error) error
```

- [ ] **Step 3: Реализация в `ConfigService`**

`config.go` — импорты `"errors"`, `"fmt"`, `"syscall"`, `"time"`; структура и конструктор:

```go
const (
	configLockTimeout = 30 * time.Second
	configLockPoll    = 50 * time.Millisecond
)

// ConfigService handles vpn-director configuration operations
type ConfigService struct {
	scriptsDir     string
	defaultDataDir string
	lockTimeout    time.Duration // how long UpdateVPNConfig waits for the lock
	lockPoll       time.Duration // interval between LOCK_NB attempts
}

// NewConfigService creates a new ConfigService
func NewConfigService(scriptsDir, defaultDataDir string) *ConfigService {
	return &ConfigService{
		scriptsDir:     scriptsDir,
		defaultDataDir: defaultDataDir,
		lockTimeout:    configLockTimeout,
		lockPoll:       configLockPoll,
	}
}

// LockPath returns the lock file guarding vpn-director.json. It lives next to
// the config, so dev mode locks inside testdata/dev. /var/lock/vpn-director.lock
// remains the shell script's own lock and Go never touches it.
func (s *ConfigService) LockPath() string {
	return filepath.Join(s.scriptsDir, ".vpn-director.json.lock")
}
```

методы (после `SaveVPNConfig`):

```go
// UpdateVPNConfig implements ConfigStore; see the interface for the contract.
func (s *ConfigService) UpdateVPNConfig(fn func(cfg *vpnconfig.VPNDirectorConfig) error) error {
	unlock, err := s.lockConfig()
	if err != nil {
		return err
	}
	defer unlock()

	cfg, err := s.LoadVPNConfig()
	if err != nil {
		return fmt.Errorf("%w: %w", ErrConfigLoad, err)
	}
	if err := fn(cfg); err != nil {
		return err
	}
	if err := s.SaveVPNConfig(cfg); err != nil {
		return fmt.Errorf("save config: %w", err)
	}
	return nil
}

// lockConfig takes an exclusive flock on LockPath, polling LOCK_NB every
// lockPoll for up to lockTimeout. flock locks belong to the open file
// description, so two ConfigService values in one process contend exactly
// like the bot and the Web UI do across processes.
func (s *ConfigService) lockConfig() (unlock func(), err error) {
	f, err := os.OpenFile(s.LockPath(), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open config lock: %w", err)
	}
	deadline := time.Now().Add(s.lockTimeout)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			f.Close()
			return nil, fmt.Errorf("lock config: %w", err)
		}
		if time.Now().After(deadline) {
			f.Close()
			return nil, ErrConfigLockTimeout
		}
		time.Sleep(s.lockPoll)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}
```

- [ ] **Step 4: Метод в пяти моках**

`server/internal/webapi/test_helpers_test.go` (импорты `fmt`, `os`, `service`): в `mockConfig` добавить поле `updateErr error // returned by UpdateVPNConfig before fn runs, e.g. service.ErrConfigLockTimeout` и метод:

```go
// UpdateVPNConfig mirrors the real contract: a load failure (err or no cfg)
// comes back wrapped in service.ErrConfigLoad before fn runs; an fn error
// skips the save; otherwise the mutated cfg is recorded as savedCfg and
// saveVPNCfgErr, if set, is returned after it.
func (m *mockConfig) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	if m.err != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.err)
	}
	if m.cfg == nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, os.ErrNotExist)
	}
	if err := fn(m.cfg); err != nil {
		return err
	}
	m.savedCfg = m.cfg
	return m.saveVPNCfgErr
}
```

`server/internal/handler/status_test.go` (импорты `fmt`, `os`, `service`):

```go
// UpdateVPNConfig: this mock holds no vpn-director.json, like a router before
// its first configure, so every update stops at the load step.
func (m *mockConfigStore) UpdateVPNConfig(func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.err != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.err)
	}
	return fmt.Errorf("%w: %w", service.ErrConfigLoad, os.ErrNotExist)
}
```

`server/internal/handler/clients_test.go` (импорты `fmt`, `service`):

```go
func (m *mockConfigClients) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.loadErr != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.loadErr)
	}
	if err := fn(m.vpnConfig); err != nil {
		return err
	}
	m.savedConfig = m.vpnConfig
	return m.saveErr
}
```

`server/internal/wizard/server_test.go` (импорты `fmt`, `os`, `service`):

```go
func (m *mockConfigStore) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.err != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.err)
	}
	if m.vpnConfig == nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, os.ErrNotExist)
	}
	return fn(m.vpnConfig)
}
```

`server/internal/wizard/apply_test.go` (импорты `fmt`, `os`, `service`):

```go
func (m *trackingConfigStore) UpdateVPNConfig(fn func(*vpnconfig.VPNDirectorConfig) error) error {
	if m.loadErr != nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, m.loadErr)
	}
	if m.vpnConfig == nil {
		return fmt.Errorf("%w: %w", service.ErrConfigLoad, os.ErrNotExist)
	}
	if err := fn(m.vpnConfig); err != nil {
		return err
	}
	m.saveConfigCalled = true
	m.savedConfig = m.vpnConfig
	return m.saveErr
}
```

- [ ] **Step 5: gitignore для лок-файла dev-режима**

В `server/.gitignore` после `testdata/dev/*.log`:

```
testdata/dev/.vpn-director.json.lock
```

- [ ] **Step 6: Собрать и прогнать модуль**

Run (через `claude-forge:build`): `cd server && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: всё зелёное; пять новых тестов `TestConfigService_UpdateVPNConfig_*` PASS (тест с двумя горутинами — без потерь обновлений: 50 записей).

- [ ] **Step 7: gofmt и коммит**

```bash
files="server/internal/service/interfaces.go server/internal/service/config.go server/internal/service/config_test.go \
  server/internal/webapi/test_helpers_test.go server/internal/handler/status_test.go server/internal/handler/clients_test.go \
  server/internal/wizard/server_test.go server/internal/wizard/apply_test.go"
gofmt -l $files   # должно быть пусто
git add $files server/.gitignore
git commit -m "feat(service): add UpdateVPNConfig, a flock-guarded load-mutate-save for vpn-director.json" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 7: Писатели Web UI → `updateAndApply`

**Files:**
- Modify: `server/internal/webapi/apply.go` (весь файл), `handler_clients.go:44-200`, `handler_excludes.go:52-165`, `handler_servers.go:36-98`, `:237-250`
- Test: `server/internal/webapi/apply_test.go` (переименовать и дополнить)

**Interfaces:**
- Consumes: `ConfigStore.UpdateVPNConfig`, `service.ErrConfigLoad` (задача 6); `lockLongOp` (задача 4).
- Produces: `updateAndApply(deps *Deps, mutate func(cfg *vpnconfig.VPNDirectorConfig) error) error`; `type httpError struct{ status int; msg string }` (возвращается из `mutate`, чтобы ответить 409/404 из-под лока); `writeSaveApplyResult(w, err)` понимает `*httpError`, `*errSavedNotApplied`, `service.ErrConfigLoad`. `saveAndApply` удаляется.

Контракт ответов не меняется: 200 `{"ok":true}`; 500 `failed to save configuration`; 500 `configuration saved, but apply failed: <строка>` + `saved: true`; 500 `failed to load configuration`; 409/404 — как раньше, только проверка теперь под локом (check-and-set атомарен).

- [ ] **Step 1: Переписать тесты `apply_test.go`**

Переименовать `TestSaveAndApply_OK/SaveError/ApplyError` в `TestUpdateAndApply_*` и заменить вызовы `saveAndApply(deps, cfg)` на `updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil })`. Добавить:

```go
func TestUpdateAndApply_HTTPErrorFromMutateIsAnsweredAsIs(t *testing.T) {
	deps := newTestDeps(t)
	mc := &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}}
	deps.Config = mc
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error {
		return &httpError{status: http.StatusConflict, msg: "client already configured for xray"}
	}))

	if rec.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d: %s", rec.Code, rec.Body.String())
	}
	if mc.savedCfg != nil {
		t.Error("a rejected mutation must not be saved")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run after a rejected mutation, got %d calls", vpn.applyCalls)
	}
	var resp map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "client already configured for xray" {
		t.Errorf("unexpected error text: %q", resp["error"])
	}
}

func TestUpdateAndApply_LoadError(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{err: errors.New("read failed")}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "failed to load configuration" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run when the config could not be loaded, got %d calls", vpn.applyCalls)
	}
}

func TestUpdateAndApply_LockTimeoutIsASaveFailure(t *testing.T) {
	deps := newTestDeps(t)
	deps.Config = &mockConfig{cfg: &vpnconfig.VPNDirectorConfig{}, updateErr: service.ErrConfigLockTimeout}
	vpn := &mockVPN{}
	deps.VPN = vpn

	rec := httptest.NewRecorder()
	writeSaveApplyResult(rec, updateAndApply(deps, func(*vpnconfig.VPNDirectorConfig) error { return nil }))

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	var resp map[string]interface{}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["error"] != "failed to save configuration" {
		t.Errorf("unexpected error text: %v", resp["error"])
	}
	if _, has := resp["saved"]; has {
		t.Error("saved must be absent when nothing was written")
	}
	if vpn.applyCalls != 0 {
		t.Errorf("Apply() must not run after a lock timeout, got %d calls", vpn.applyCalls)
	}
}
```

Run (через `claude-forge:build`): `cd server && go vet ./internal/webapi/`
Expected: `undefined: updateAndApply`, `undefined: httpError`.

- [ ] **Step 2: Переписать `apply.go`**

```go
package webapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/service"
	"github.com/zinin/asuswrt-merlin-vpn-director/server/internal/vpnconfig"
)

// errSavedNotApplied marks an updateAndApply failure where vpn-director.json
// was written but `vpn-director.sh apply` failed. The client must learn that
// the change is on disk and only the apply needs a retry.
type errSavedNotApplied struct{ cause error }

func (e *errSavedNotApplied) Error() string { return "saved but not applied: " + e.cause.Error() }
func (e *errSavedNotApplied) Unwrap() error { return e.cause }

// httpError carries a client-facing status and message out of an
// UpdateVPNConfig callback, so a handler can reject a change (409, 404)
// from inside the locked section and writeSaveApplyResult answers with it.
type httpError struct {
	status int
	msg    string
}

func (e *httpError) Error() string { return e.msg }

// updateAndApply mutates vpn-director.json under the config lock and then
// runs `vpn-director.sh apply`, mirroring the bot, so the config on disk
// always matches the kernel state. Errors from UpdateVPNConfig (load, lock,
// mutate, save) are returned as is; an apply failure is wrapped in
// *errSavedNotApplied.
func updateAndApply(deps *Deps, mutate func(cfg *vpnconfig.VPNDirectorConfig) error) error {
	if err := deps.Config.UpdateVPNConfig(mutate); err != nil {
		return err
	}
	if err := deps.VPN.Apply(); err != nil {
		return &errSavedNotApplied{cause: err}
	}
	return nil
}

// writeSaveApplyResult maps an updateAndApply result to the HTTP response:
// 200 {"ok":true}; the status and text of an *httpError returned by mutate;
// 500 "failed to load configuration" when vpn-director.json could not be
// read; 500 with "saved": true and the last line of the apply output when
// only the apply failed; otherwise 500 "failed to save configuration" with
// the cause (a lock timeout, a full disk) in the log. Only pass it the
// result of updateAndApply: answer validation errors with jsonError directly.
func writeSaveApplyResult(w http.ResponseWriter, err error) {
	if err == nil {
		jsonOK(w, map[string]bool{"ok": true})
		return
	}
	var he *httpError
	if errors.As(err, &he) {
		jsonError(w, he.status, he.msg)
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
	if errors.Is(err, service.ErrConfigLoad) {
		jsonError(w, http.StatusInternalServerError, "failed to load configuration")
		return
	}
	slog.Warn("configuration update failed", "error", err)
	jsonError(w, http.StatusInternalServerError, "failed to save configuration")
}
```

- [ ] **Step 3: Клиенты**

В `handler_clients.go` каждый из четырёх хендлеров после валидации запроса заменяет блок «`LoadVPNConfig` → проверка → мутация → `writeSaveApplyResult(w, saveAndApply(deps, cfg))`» на один вызов. `handleAddClient` (после проверки `validRoutes`):

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if existing, found := findClient(cfg, ip); found {
				return &httpError{status: http.StatusConflict, msg: fmt.Sprintf("client already configured for %s", existing.route)}
			}
			if req.Route == "xray" {
				cfg.Xray.Clients = append(cfg.Xray.Clients, ip)
				return nil
			}
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
			return nil
		}))
```

`handlePauseClient` (после `clientAddrFromQuery`):

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			existing, found := findClient(cfg, ip)
			if !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			// Drop any equivalent spelling and write the stored one in its place.
			// lib/config.sh subtracts paused_clients from the clients arrays by
			// exact string, and CollectClients reports Paused by exact lookup, so
			// an entry spelled differently from the client pauses nothing while
			// reporting success. Replacing rather than skipping keeps this
			// idempotent and repairs a mismatched legacy entry on the first pause.
			cfg.PausedClients = append(removeAddr(cfg.PausedClients, ip), existing.stored)
			return nil
		}))
```

`handleResumeClient`:

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if _, found := findClient(cfg, ip); !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			cfg.PausedClients = removeAddr(cfg.PausedClients, ip)
			return nil
		}))
```

`handleDeleteClient`:

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if _, found := findClient(cfg, ip); !found {
				return &httpError{status: http.StatusNotFound, msg: "client not found"}
			}
			cfg.Xray.Clients = removeAddr(cfg.Xray.Clients, ip)
			for name, tunnel := range cfg.TunnelDirector.Tunnels {
				tunnel.Clients = removeAddr(tunnel.Clients, ip)
				cfg.TunnelDirector.Tunnels[name] = tunnel
			}
			cfg.PausedClients = removeAddr(cfg.PausedClients, ip)
			return nil
		}))
```

Строки `unlock := lockLongOp(w, deps, applyDeadline); defer unlock()` из задачи 4 остаются в начале каждого хендлера.

- [ ] **Step 4: Исключения**

`handler_excludes.go`: `handleUpdateExcludeSets` после `normalizeExcludeSets`:

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			cfg.Xray.ExcludeSets = sets
			return nil
		}))
```

`handleAddExcludeIP` после нормализации:

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			if !containsAddr(cfg.Xray.ExcludeIPs, ip) {
				cfg.Xray.ExcludeIPs = append(cfg.Xray.ExcludeIPs, ip)
			}
			return nil
		}))
```

`handleDeleteExcludeIP`:

```go
		writeSaveApplyResult(w, updateAndApply(deps, func(cfg *vpnconfig.VPNDirectorConfig) error {
			cfg.Xray.ExcludeIPs = removeAddr(cfg.Xray.ExcludeIPs, ip)
			return nil
		}))
```

- [ ] **Step 5: Серверы**

`handleSelectServer` — заменить блок `LoadVPNConfig` … `SaveVPNConfig` на:

```go
		// Set xray.servers to ALL servers' IPs, not just the selected one
		// (parity with the import path). xray.servers feeds the TPROXY bypass
		// set; dropping the other endpoints on a switch can cause a routing loop.
		err = deps.Config.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
			cfg.Xray.Servers = collectServerIPs(servers)
			return nil
		})
		if err != nil {
			if errors.Is(err, service.ErrConfigLoad) {
				jsonError(w, http.StatusInternalServerError, "failed to load vpn config")
			} else {
				jsonError(w, http.StatusInternalServerError, "failed to save vpn config")
			}
			return
		}
```

`syncXrayServers`:

```go
// syncXrayServers updates xray.servers with the IPs of all given servers under
// the config lock. A load or save failure is returned so the caller can
// surface it instead of silently leaving xray.servers stale.
func syncXrayServers(config service.ConfigStore, servers []vpnconfig.Server) error {
	err := config.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.Xray.Servers = collectServerIPs(servers)
		return nil
	})
	if err != nil {
		return fmt.Errorf("sync xray.servers: %w", err)
	}
	return nil
}
```

- [ ] **Step 6: Прогнать пакет**

Run (через `claude-forge:build`): `cd server && go vet ./internal/webapi/ && go test -count=1 ./internal/webapi/`
Expected: PASS. Существующие тесты 409/404 (`savedCfg == nil`, `applyCalls == 0`), `_LoadError` («failed to load configuration»), `_SavedButApplyFailed`, `TestSyncXrayServers_*` проходят без изменений; `grep -n saveAndApply server/internal/webapi/*.go` пуст.

- [ ] **Step 7: gofmt и коммит**

```bash
files="server/internal/webapi/apply.go server/internal/webapi/apply_test.go server/internal/webapi/handler_clients.go \
  server/internal/webapi/handler_excludes.go server/internal/webapi/handler_servers.go"
gofmt -l $files   # должно быть пусто
git add $files
git commit -m "refactor(webapi): route every config mutation through UpdateVPNConfig" \
  -m "The 409 and 404 checks move inside the locked callback, so check-and-set is atomic against the bot. Response contract unchanged." \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 8: Писатели бота → `UpdateVPNConfig`

**Files:**
- Modify: `server/internal/handler/handler.go` (хелпер сообщения), `server/internal/handler/clients.go:117-165`, `:225-260`, `:360-407`, `server/internal/handler/exclude.go:170-192`, `server/internal/handler/import.go:107-133`, `server/internal/wizard/apply.go:48-150`
- Test: `server/internal/handler/clients_test.go`, `server/internal/handler/exclude_test.go`, `server/internal/wizard/apply_test.go` (существующие + 2 новых)

**Interfaces:**
- Consumes: `UpdateVPNConfig`, `service.ErrConfigLoad` (задача 6).
- Produces: `handler.configUpdateError(err error) string` — текст для Telegram: `Config load error: …` при `ErrConfigLoad`, иначе `Save error: …`. Пред-проверки (устаревшая клавиатура) остаются на `LoadVPNConfig` вне лока; мутация повторяется на свежем `cfg` внутри `fn`.

Замечание по спеке: она перечисляет `handler/xray.go` среди писателей `xray.servers`, но текущий `/xray` бота конфиг не пишет (только `GenerateConfig` + `RestartXray`) — менять там нечего.

- [ ] **Step 1: Падающие тесты**

В `handler/clients_test.go` добавить:

```go
func TestClientsHandler_Pause_SaveErrorIsReported(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{Xray: vpnconfig.XrayConfig{Clients: []string{"192.168.50.10"}}},
		saveErr:   errors.New("disk full"),
	}
	vpn := &mockVPNClients{}
	h := NewClientsHandler(&Deps{Sender: sender, Config: config, VPN: vpn})

	h.HandleCallback(&tgbotapi.CallbackQuery{
		Data:    "clients:pause:192.168.50.10",
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 100}},
	})

	if n := len(sender.plainTexts); n == 0 || !strings.Contains(sender.plainTexts[n-1], "Save error: disk full") {
		t.Errorf("expected the save error to reach the user, got %v", sender.plainTexts)
	}
}
```

(`mockSenderClients.plainTexts` накапливает тексты `SendPlain`; `exclude_test.go` использует тот же мок.)

В `handler/exclude_test.go` добавить:

```go
func TestExcludeHandler_Done_SaveErrorIsReported(t *testing.T) {
	sender := &mockSenderClients{}
	config := &mockConfigClients{
		vpnConfig: &vpnconfig.VPNDirectorConfig{Xray: vpnconfig.XrayConfig{ExcludeIPs: []string{"1.2.3.4"}}},
		saveErr:   errors.New("disk full"),
	}
	deps := &Deps{Sender: sender, Config: config, VPN: &mockVPNClients{}}
	h := NewExcludeHandler(deps)

	h.HandleExclude(&tgbotapi.Message{Chat: &tgbotapi.Chat{ID: 100}})
	h.HandleCallback(&tgbotapi.CallbackQuery{
		Data:    "exclip:done",
		Message: &tgbotapi.Message{MessageID: 7, Chat: &tgbotapi.Chat{ID: 100}},
	})

	if n := len(sender.plainTexts); n == 0 || !strings.Contains(sender.plainTexts[n-1], "Save error: disk full") {
		t.Errorf("expected the save error to reach the user, got %v", sender.plainTexts)
	}
}
```

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/handler/ -run 'SaveErrorIsReported' -v`
Expected: оба теста проходят уже сейчас (старый `SaveVPNConfig` возвращает `saveErr`) — они пришпиливают сообщение на время миграции; RED здесь даёт компилятор в шаге 4 после удаления `SaveVPNConfig` (задача 9). Продолжать.

- [ ] **Step 2: Хелпер сообщения**

В `server/internal/handler/handler.go` (импорты `errors`, `fmt`; `service` уже есть):

```go
// configUpdateError phrases an UpdateVPNConfig failure the way the bot has
// always reported the two halves of a save: read problems and write problems.
func configUpdateError(err error) string {
	if errors.Is(err, service.ErrConfigLoad) {
		return fmt.Sprintf("Config load error: %v", err)
	}
	return fmt.Sprintf("Save error: %v", err)
}
```

- [ ] **Step 3: `handler/clients.go`**

`handlePauseResume`: оставить загрузку и проверку `exists`; заменить блок «мутация + `SaveVPNConfig`» на:

```go
	err = h.deps.Config.UpdateVPNConfig(func(c *vpnconfig.VPNDirectorConfig) error {
		if pause {
			found := false
			for _, p := range c.PausedClients {
				if p == ip {
					found = true
					break
				}
			}
			if !found {
				c.PausedClients = append(c.PausedClients, ip)
			}
		} else {
			c.PausedClients = removeString(c.PausedClients, ip)
		}
		cfg = c // render the list from what was actually saved
		return nil
	})
	if err != nil {
		h.deps.Sender.SendPlain(chatID, configUpdateError(err))
		return
	}
```

`handleRemove`: оставить поиск `route`; заменить мутацию + сохранение на:

```go
	err = h.deps.Config.UpdateVPNConfig(func(c *vpnconfig.VPNDirectorConfig) error {
		if route == "xray" {
			c.Xray.Clients = removeString(c.Xray.Clients, ip)
		} else if tunnel, ok := c.TunnelDirector.Tunnels[route]; ok {
			tunnel.Clients = removeString(tunnel.Clients, ip)
			c.TunnelDirector.Tunnels[route] = tunnel
		}
		c.PausedClients = removeString(c.PausedClients, ip)
		cfg = c
		return nil
	})
	if err != nil {
		h.deps.Sender.SendPlain(chatID, configUpdateError(err))
		return
	}
```

`handleAddRoute`: оставить загрузку и проверку существования туннеля; заменить мутацию + сохранение на:

```go
	err = h.deps.Config.UpdateVPNConfig(func(c *vpnconfig.VPNDirectorConfig) error {
		if route == "xray" {
			c.Xray.Clients = append(c.Xray.Clients, ip)
			cfg = c
			return nil
		}
		tunnel, ok := c.TunnelDirector.Tunnels[route]
		if !ok {
			return fmt.Errorf("tunnel %s no longer exists", route)
		}
		tunnel.Clients = append(tunnel.Clients, ip)
		c.TunnelDirector.Tunnels[route] = tunnel
		cfg = c
		return nil
	})
	if err != nil {
		h.deps.Sender.SendPlain(chatID, configUpdateError(err))
		return
	}
```

Во всех трёх местах последующие `h.deps.VPN.Apply()` и рендер списка из `cfg` остаются как были.

- [ ] **Step 4: `handler/exclude.go`, `handler/import.go`, `wizard/apply.go`**

`exclude.go`, `saveAndApply`: заменить `LoadVPNConfig` … `SaveVPNConfig` на:

```go
	err := h.deps.Config.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
		cfg.Xray.ExcludeIPs = state.GetExcludeIPs()
		return nil
	})
	if err != nil {
		h.sender.SendPlain(chatID, configUpdateError(err))
		return
	}
	h.sender.SendPlain(chatID, "vpn-director.json updated")
```

`import.go`: оставить вычисление `serverIPs` (цикл dedupe + `sort.Strings`), заменить `if vpnCfg, err := …LoadVPNConfig(); … SaveVPNConfig` на:

```go
	// Auto-sync xray.servers with IPs from all imported servers. A missing
	// vpn-director.json is not an error here: /import works before the first
	// configure, and the wizard writes xray.servers itself.
	err = h.deps.Config.UpdateVPNConfig(func(vpnCfg *vpnconfig.VPNDirectorConfig) error {
		vpnCfg.Xray.Servers = serverIPs
		return nil
	})
	if err != nil && !errors.Is(err, service.ErrConfigLoad) {
		h.deps.Sender.Send(msg.Chat.ID, telegram.EscapeMarkdownV2(
			fmt.Sprintf("Warning: servers imported but xray.servers sync failed: %v", err)))
	}
```

(импорты `errors`, `service`; цикл сбора `serverIPs` переехал выше вызова.)

`wizard/apply.go`, `Apply`: убрать первый `LoadVPNConfig` (загрузку серверов оставить первой), после вычисления `excl`, `tunnels`, `serverIPs`, `excludeIPs` заменить блок «Update config … Save config» на:

```go
	// Update config under the cross-process lock
	err = a.config.UpdateVPNConfig(func(vpnCfg *vpnconfig.VPNDirectorConfig) error {
		vpnCfg.Xray.Clients = xrayClients
		vpnCfg.Xray.ExcludeSets = excl
		vpnCfg.Xray.ExcludeIPs = excludeIPs
		vpnCfg.Xray.Servers = serverIPs
		vpnCfg.TunnelDirector.Tunnels = tunnels
		return nil
	})
	if err != nil {
		if errors.Is(err, service.ErrConfigLoad) {
			a.sender.SendPlain(chatID, fmt.Sprintf("Config load error: %v", err))
		} else {
			a.sender.SendPlain(chatID, fmt.Sprintf("Save error: %v", err))
		}
		return err
	}
	a.sender.SendPlain(chatID, "vpn-director.json updated")
```

(импорт `errors`.)

- [ ] **Step 5: Прогнать пакеты**

Run (через `claude-forge:build`): `cd server && go vet ./... && go test -count=1 ./internal/handler/ ./internal/wizard/`
Expected: PASS. `TestApplier_Apply_ClearsStateOnError` («clears state even on config load error») проходит: `loadErr` теперь срабатывает на `LoadServers`, ошибка возвращается, состояние очищено. Тест `Save error: disk full` (`apply_test.go:615`) проходит через `saveErr` мока.

- [ ] **Step 6: gofmt и коммит**

```bash
files="server/internal/handler/handler.go server/internal/handler/clients.go server/internal/handler/exclude.go \
  server/internal/handler/import.go server/internal/wizard/apply.go server/internal/handler/clients_test.go \
  server/internal/handler/exclude_test.go"
gofmt -l $files   # должно быть пусто
git add $files
git commit -m "refactor(bot): route the wizard, clients, exclude and import writers through UpdateVPNConfig" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 9: `jwt_secret` через `UpdateVPNConfig`; `SaveVPNConfig` покидает интерфейс

**Files:**
- Modify: `server/cmd/webui/main.go:55-103`
- Modify: `server/internal/service/interfaces.go` (интерфейс), `server/internal/service/config.go` (`SaveVPNConfig` → `saveVPNConfig`)
- Modify: моки — `webapi/test_helpers_test.go`, `handler/status_test.go`, `handler/clients_test.go`, `wizard/server_test.go`, `wizard/apply_test.go` (удалить `SaveVPNConfig`)

**Interfaces:**
- Consumes: `UpdateVPNConfig` (задача 6); все писатели уже мигрированы (задачи 7–8).
- Produces: `service.ConfigStore` без `SaveVPNConfig` — единственный путь записи `vpn-director.json` из Go проходит через лок, и это гарантирует компилятор. `ConfigService.saveVPNConfig` — неэкспортированный, используется только из `UpdateVPNConfig`.

- [ ] **Step 1: Web UI генерирует `jwt_secret` под локом**

В `main.go` перенести вычисление `scriptsDir`/`defaultDataDir` и создание `configSvc` выше загрузки конфига и заменить блок загрузки и автогенерации:

```go
	// Derive scripts directory from --config path so runtime reads/writes
	// honour the flag instead of hardcoding /opt/vpn-director.
	scriptsDir := filepath.Dir(*configPath)
	defaultDataDir := filepath.Join(scriptsDir, "data")
	configSvc := service.NewConfigService(scriptsDir, defaultDataDir)

	// Load config
	vpnCfg, err := configSvc.LoadVPNConfig()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	if vpnCfg.WebUI.Port == 0 {
		vpnCfg.WebUI.Port = 8444
	}
	if vpnCfg.WebUI.CertFile == "" {
		vpnCfg.WebUI.CertFile = "/opt/vpn-director/certs/server.crt"
	}
	if vpnCfg.WebUI.KeyFile == "" {
		vpnCfg.WebUI.KeyFile = "/opt/vpn-director/certs/server.key"
	}

	// Auto-generate JWT secret if empty. The write goes through the config
	// lock like every other writer; if another process filled the secret in
	// the meantime, that one wins and is used here as well.
	if vpnCfg.WebUI.JWTSecret == "" {
		slog.Warn("jwt_secret not set, generating random secret")
		secret := make([]byte, 32)
		if _, err := rand.Read(secret); err != nil {
			slog.Error("failed to generate jwt secret", "error", err)
			os.Exit(1)
		}
		vpnCfg.WebUI.JWTSecret = base64.StdEncoding.EncodeToString(secret)
		err := configSvc.UpdateVPNConfig(func(cfg *vpnconfig.VPNDirectorConfig) error {
			if cfg.WebUI.JWTSecret == "" {
				cfg.WebUI.JWTSecret = vpnCfg.WebUI.JWTSecret
			}
			vpnCfg.WebUI.JWTSecret = cfg.WebUI.JWTSecret
			return nil
		})
		if err != nil {
			slog.Warn("failed to save auto-generated jwt_secret", "error", err)
			// Continue anyway — secret is in memory for this session
		}
	}
```

Ниже удалить старые строки `scriptsDir := …`, `defaultDataDir := …`, `configSvc := …` (они переехали). `ensureDevFiles` продолжает звать `vpnconfig.SaveVPNDirectorConfig` напрямую — это создание файла до старта сервисов, лок здесь не нужен.

- [ ] **Step 2: Удалить `SaveVPNConfig` из интерфейса, сервиса и моков**

`interfaces.go`: удалить строку `SaveVPNConfig(*vpnconfig.VPNDirectorConfig) error` и дописать в комментарий к `UpdateVPNConfig`:

```go
	// There is deliberately no SaveVPNConfig: every writer goes through
	// UpdateVPNConfig, so no code path can skip the lock.
```

`config.go`: переименовать `SaveVPNConfig` в `saveVPNConfig` (комментарий: `// saveVPNConfig writes the VPN Director configuration; callers hold the config lock.`) и вызывать его из `UpdateVPNConfig`.

Удалить методы `SaveVPNConfig` из `mockConfig`, `mockConfigStore` (handler), `mockConfigClients`, `mockConfigStore` (wizard), `trackingConfigStore`. Поля `saveVPNCfgErr`, `saveErr`, `savedCfg`, `savedConfig`, `saveConfigCalled` остаются — их использует `UpdateVPNConfig`.

- [ ] **Step 3: Проверить, что писателей мимо лока не осталось**

Run: `git grep -n "SaveVPNConfig" -- server`
Expected: пусто. (`SaveVPNDirectorConfig` в `vpnconfig` и в `ensureDevFiles` — не считается, это другой идентификатор.)

- [ ] **Step 4: Собрать и прогнать модуль**

Run (через `claude-forge:build`): `cd server && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: всё зелёное.

- [ ] **Step 5: gofmt и коммит**

```bash
files="server/cmd/webui/main.go server/internal/service/interfaces.go server/internal/service/config.go \
  server/internal/webapi/test_helpers_test.go server/internal/handler/status_test.go server/internal/handler/clients_test.go \
  server/internal/wizard/server_test.go server/internal/wizard/apply_test.go"
gofmt -l $files   # должно быть пусто
git add $files
git commit -m "refactor(service): drop SaveVPNConfig from ConfigStore so no writer can bypass the config lock" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 10: Лог-файл Web UI, `log_level`, `DefaultMaxSize`, ротация в обоих демонах

**Files:**
- Modify: `server/internal/paths/paths.go`, `server/internal/paths/paths_test.go`
- Modify: `server/internal/logging/rotation.go`, `server/internal/logging/rotation_test.go`
- Modify: `server/internal/vpnconfig/vpnconfig.go:25-30`, `vpnconfig_test.go` (`TestLoadVPNDirectorConfig_WithWebUI`)
- Modify: `server/cmd/bot/main.go:37`, `:106`; `server/cmd/webui/main.go` (логгер, уровень, ротация, dev-конфиг)
- Modify: `README.md:125-134`, `README.ru.md:125-134`

**Interfaces:**
- Consumes: `logging.NewSlogLogger`, `(*Logger).SetLevel`, `(*Logger).StartRotation` (существуют).
- Produces: `paths.Paths.WebUILogPath` (`/tmp/vpn-director-webui.log`; dev `testdata/dev/webui.log`); `func (p Paths) RotatedLogs() []string` — единый список для обоих демонов; `logging.DefaultMaxSize int64 = 200 * 1024`; `vpnconfig.WebUIConfig.LogLevel string` (`log_level`, опционально, по умолчанию info). Задача 11 добавляет источник `webui` поверх `WebUILogPath`.

- [ ] **Step 1: Падающие тесты**

`paths_test.go`: в обе таблицы добавить строку `{"WebUILogPath", p.WebUILogPath, "/tmp/", "vpn-director-webui.log"}` (в `TestDevPaths` — `"testdata/dev/", "webui.log"`); в `TestDefaultNotEmpty` добавить проверки `p.XrayLogPath` и `p.WebUILogPath`; добавить (импорт `reflect`):

```go
func TestRotatedLogs(t *testing.T) {
	p := Default()
	want := []string{p.BotLogPath, p.VPNLogPath, p.WebUILogPath, p.XrayLogPath}
	if got := p.RotatedLogs(); !reflect.DeepEqual(got, want) {
		t.Errorf("RotatedLogs() = %v, want %v", got, want)
	}
	for _, path := range p.RotatedLogs() {
		if path == "" {
			t.Error("RotatedLogs contains an empty path")
		}
	}
}
```

`rotation_test.go`:

```go
func TestDefaultMaxSize(t *testing.T) {
	// Documented in README: logs are truncated at 200 KB.
	if DefaultMaxSize != 200*1024 {
		t.Errorf("DefaultMaxSize = %d, want %d", DefaultMaxSize, 200*1024)
	}
}
```

`vpnconfig_test.go`, `TestLoadVPNDirectorConfig_WithWebUI`: добавить в JSON-фикстуру поле `"log_level": "debug"` внутри `webui` и проверку:

```go
	if cfg.WebUI.LogLevel != "debug" {
		t.Errorf("LogLevel = %q, want %q", cfg.WebUI.LogLevel, "debug")
	}
```

Run (через `claude-forge:build`): `cd server && go vet ./internal/paths/ ./internal/logging/ ./internal/vpnconfig/`
Expected: `p.WebUILogPath undefined`, `undefined: DefaultMaxSize`, `cfg.WebUI.LogLevel undefined`.

- [ ] **Step 2: Пути, константа, поле**

`paths.go`: поле `WebUILogPath   string // /tmp/vpn-director-webui.log` после `VPNLogPath`; в `Default()` — `WebUILogPath:   "/tmp/vpn-director-webui.log",`; в `DevPaths()` — `WebUILogPath:   "testdata/dev/webui.log",`; метод:

```go
// RotatedLogs lists every log file the daemons truncate at
// logging.DefaultMaxSize. The bot and the Web UI rotate the same list;
// os.Truncate is idempotent, so two processes rotating at once are safe.
func (p Paths) RotatedLogs() []string {
	return []string{p.BotLogPath, p.VPNLogPath, p.WebUILogPath, p.XrayLogPath}
}
```

`rotation.go` перед `TruncateIfNeeded`:

```go
// DefaultMaxSize is the size at which StartRotation truncates a log file.
// /tmp is tmpfs on the router, so the logs compete with everything else for RAM.
const DefaultMaxSize int64 = 200 * 1024
```

`vpnconfig.go`, `WebUIConfig`:

```go
	LogLevel  string `json:"log_level,omitempty"` // debug, info, warn, error; empty means info
```

- [ ] **Step 3: Бот использует общий список и константу**

`cmd/bot/main.go`: удалить `const maxLogSize = 200 * 1024`; строку 106 заменить на

```go
	logger.StartRotation(ctx, p.RotatedLogs(), logging.DefaultMaxSize, time.Minute)
```

- [ ] **Step 4: Web UI пишет лог**

`cmd/webui/main.go` — импорт `logging`; сразу после ветки `if *devFlag {…} else { p = paths.Default() }` и до первого `slog.Info`:

```go
	// Log file first, like the bot, so a config load failure is logged too.
	slogger, logger, err := logging.NewSlogLogger(p.WebUILogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logging: %v\n", err)
		os.Exit(1)
	}
	defer logger.Close()
	slog.SetDefault(slogger)
```

(далее `vpnCfg, err := configSvc.LoadVPNConfig()` переходит на `=`, поскольку `err` уже объявлен.) После установки значений по умолчанию для `Port`/`CertFile`/`KeyFile`:

```go
	logger.SetLevel(vpnCfg.WebUI.LogLevel) // "" keeps info
```

После `ctx, cancel := signal.NotifyContext(...)`/`defer cancel()`:

```go
	logger.StartRotation(ctx, p.RotatedLogs(), logging.DefaultMaxSize, time.Minute)
```

В `ensureDevFiles` в литерал `WebUI: vpnconfig.WebUIConfig{…}` добавить `LogLevel:  "debug",`.

- [ ] **Step 5: README**

В `README.md` и `README.ru.md` в JSON-примере секции `webui` (строки ~125–130) после `"jwt_secret": ""` добавить `,` и строку `"log_level": "info"`; после предложения про `jwt_secret` (строка 134) добавить:

EN: `` `log_level` accepts `debug`, `info`, `warn`, `error` (default `info`). The Web UI logs to `/tmp/vpn-director-webui.log`; all logs are truncated at 200 KB. ``

RU: `` `log_level` принимает `debug`, `info`, `warn`, `error` (по умолчанию `info`). Web UI пишет лог в `/tmp/vpn-director-webui.log`; все логи обрезаются при 200 КБ. ``

- [ ] **Step 6: Собрать и прогнать**

Run (через `claude-forge:build`): `cd server && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: всё зелёное, `TestRotatedLogs`, `TestDefaultMaxSize`, обновлённый `_WithWebUI` PASS.

- [ ] **Step 7: gofmt и коммит**

```bash
files="server/internal/paths/paths.go server/internal/paths/paths_test.go server/internal/logging/rotation.go \
  server/internal/logging/rotation_test.go server/internal/vpnconfig/vpnconfig.go server/internal/vpnconfig/vpnconfig_test.go \
  server/cmd/bot/main.go server/cmd/webui/main.go"
gofmt -l $files   # должно быть пусто
git add $files README.md README.ru.md
git commit -m "feat(webui): write a log file with a configurable level and rotate it alongside the other logs" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 11: Источник `webui` в `/api/logs`, `/logs` бота и вкладке Logs

**Files:**
- Modify: `server/cmd/webui/main.go` (`Deps.LogPaths`), `server/internal/webapi/test_helpers_test.go` (`LogPaths`), `server/internal/webapi/handler_logs_test.go:60`, `:165` (+1 тест)
- Modify: `server/internal/handler/misc.go:66-80`, `:96-106`; `server/internal/handler/misc_test.go` (`_DefaultArgs`, `_LinesOnly`, +1 тест)
- Modify: `web/src/components/LogsTab.vue:5`
- Modify: `README.md:116`, `:170`; `README.ru.md:116`, `:170`; `.claude/rules/telegram-bot.md:76`

**Interfaces:**
- Consumes: `paths.Paths.WebUILogPath` (задача 10).
- Produces: ключ `webui` в `Deps.LogPaths`; `/logs webui [N]` в боте; опция `webui` в селекторе вкладки Logs. Порядок вывода `/logs all`: bot, vpn, xray, webui.

- [ ] **Step 1: Падающие тесты**

`webapi/handler_logs_test.go`: в двух циклах `for _, key := range []string{"vpn", "xray", "bot"}` добавить `"webui"`; в `TestHandleLogs_InvalidSourceListsValidOnes` ожидаемый текст ошибки → `unknown source: valid values are bot, vpn, webui, xray` (имена сортируются); добавить:

```go
func TestHandleLogs_ReadsWebUIPath(t *testing.T) {
	deps := newTestDeps(t)
	logs := &mockLogs{output: "webui line"}
	deps.Logs = logs

	req := httptest.NewRequest("GET", "/api/logs?source=webui", nil)
	rec := httptest.NewRecorder()
	handleLogs(deps).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if len(logs.paths) != 1 || logs.paths[0] != "/tmp/test-webui.log" {
		t.Errorf("expected read of /tmp/test-webui.log, got %v", logs.paths)
	}
}
```

`handler/misc_test.go`: в `TestMiscHandler_HandleLogs_DefaultArgs` и `_LinesOnly` добавить `WebUILogPath: "/tmp/webui.log"` в `testPaths`, ожидать 4 вызова и проверить `logReader.calls[3].path == "/tmp/webui.log"`; добавить:

```go
func TestMiscHandler_HandleLogs_SourceWebUI(t *testing.T) {
	sender := &mockSender{}
	logReader := &mockLogReader{output: "log"}
	testPaths := paths.Paths{
		BotLogPath:   "/tmp/bot.log",
		VPNLogPath:   "/tmp/vpn.log",
		XrayLogPath:  "/tmp/xray-error.log",
		WebUILogPath: "/tmp/webui.log",
	}
	deps := &Deps{Sender: sender, Logs: logReader, Paths: testPaths}
	h := NewMiscHandler(deps)

	msg := &tgbotapi.Message{
		Chat: &tgbotapi.Chat{ID: 100},
		Text: "/logs webui",
		Entities: []tgbotapi.MessageEntity{
			{Type: "bot_command", Offset: 0, Length: 5},
		},
	}
	h.HandleLogs(msg)

	if len(logReader.calls) != 1 {
		t.Fatalf("expected 1 log read call, got %d", len(logReader.calls))
	}
	if logReader.calls[0].path != "/tmp/webui.log" {
		t.Errorf("expected webui log path, got %q", logReader.calls[0].path)
	}
}
```

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/webapi/ -run HandleLogs && go test -count=1 ./internal/handler/ -run HandleLogs`
Expected: FAIL — нет ключа `webui`, `_InvalidSourceListsValidOnes` не находит `webui` в списке, `/logs webui` отвечает usage-сообщением, `_DefaultArgs` видит 3 вызова вместо 4.

- [ ] **Step 2: Реализация**

`cmd/webui/main.go`, `LogPaths`: добавить `"webui": p.WebUILogPath,`. `webapi/test_helpers_test.go`, `newTestDeps`: добавить `"webui": "/tmp/test-webui.log",`.

`handler/misc.go`: `case "bot", "vpn", "xray", "webui", "all":`; usage-строка → `"Usage: \`/logs [bot|vpn|xray|webui|all] [lines]\`"`; после блока `xray`:

```go
	if source == "webui" || source == "all" {
		h.sendLogFile(msg.Chat.ID, h.deps.Paths.WebUILogPath, "Web UI", lines)
	}
```

`web/src/components/LogsTab.vue:5`: `const logSources = ['vpn', 'xray', 'webui', 'bot'] as const`.

- [ ] **Step 3: Документация**

`README.md:116` → `| **Logs** | Log viewer (bot, vpn, xray, webui) |`; `README.ru.md:116` → `| **Logs** | Просмотр логов (бот, vpn, xray, webui) |`; `README.md:170`, `README.ru.md:170`, `.claude/rules/telegram-bot.md:76` → `` `/logs [bot\|vpn\|xray\|webui\|all] [N]` `` (остальные ячейки без изменений).

Проверить, что старой сигнатуры не осталось: `git grep -nF 'bot\|vpn\|xray\|all]' -- README.md README.ru.md .claude` → пусто.

- [ ] **Step 4: Прогнать Go и сборку фронта**

Run (через `claude-forge:build`): `cd server && go test -count=1 ./internal/webapi/ ./internal/handler/` и `cd web && npm run build`
Expected: Go зелёный; `vue-tsc -b` без ошибок, `vite build` собирает бандл.

- [ ] **Step 5: gofmt и коммит**

```bash
files="server/cmd/webui/main.go server/internal/webapi/test_helpers_test.go server/internal/webapi/handler_logs_test.go \
  server/internal/handler/misc.go server/internal/handler/misc_test.go"
gofmt -l $files   # должно быть пусто
git add $files web/src/components/LogsTab.vue README.md README.ru.md .claude/rules/telegram-bot.md
git commit -m "feat(logs): expose the Web UI log as the webui source in the API, the bot and the Logs tab" \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 12: Невалидный код страны в `xray.exclude_sets` не валит apply

**Files:**
- Modify: `router/opt/vpn-director/lib/tproxy.sh:103-146` (новый хелпер + `_tproxy_check_required_ipsets`), `:315-370` (`_tproxy_setup_iptables`), `:492-510` (`tproxy_get_required_ipsets`)
- Modify: `server/internal/webapi/handler_excludes.go:33-35` (комментарий)
- Modify: `.claude/rules/xray-tproxy.md:113-116` (Fail-Safe), `:138-146` (таблица внутренних функций)
- Test: `router/test/unit/tproxy.bats` (+3 теста)

**Interfaces:**
- Consumes: `_is_valid_country_code` и `ALL_COUNTRY_CODES` из `lib/ipset.sh` (уже загружается до `tproxy.sh`).
- Produces: `_tproxy_exclude_sets [-q]` — печатает валидные коды из `XRAY_EXCLUDE_SETS` одной строкой через пробел в нижнем регистре; без `-q` пишет `log -l WARN "Ignoring invalid country code '<code>' in xray.exclude_sets"` за каждый отброшенный код. `tproxy_get_required_ipsets`, `_tproxy_check_required_ipsets`, `_tproxy_setup_iptables` работают только с этим списком.

Почему так: `parse_exclude_sets_from_json` (`lib/ipset.sh:227-241`) уже отбрасывает невалидные коды для туннелей, а `tproxy_get_required_ipsets` — нет. Один код `xx` в `xray.exclude_sets` попадал в `_ensure_ipsets` (`vpn-director.sh:126-131`), `ipset_ensure` возвращал 1, и под `set -euo pipefail` `cmd_apply` обрывался до первого правила — а на буте `S99vpn-director` ещё и не ставил cron. Фильтра в одном `tproxy_get_required_ipsets` мало: `_tproxy_check_required_ipsets` иначе увидит `xx`, не найдёт ipset и мягко выйдет без TPROXY вообще, а `_tproxy_setup_iptables` оборвётся с `not found; aborting`. Поэтому все три места читают один отфильтрованный список; предупреждение пишется один раз — из `tproxy_get_required_ipsets`, который в `cmd_apply` вызывается первым. Поведение для пользователя Web UI меняется: неизвестный код сохраняется, apply отвечает 200, код игнорируется с WARN в `/tmp/vpn-director.log` — это осознанная замена премисы спеки §4.4 («ошибку поймает `ipset_ensure`»), согласованная автором.

- [ ] **Step 1: Падающие bats-тесты**

Добавить в `router/test/unit/tproxy.bats` после теста `tproxy_get_required_ipsets: handles empty exclude sets`:

```bash
@test "tproxy_get_required_ipsets: drops an unknown country code with a WARN" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    source "$LIB_DIR/ipset.sh" --source-only
    export XRAY_EXCLUDE_SETS="ru xx US"
    source "$LIB_DIR/tproxy.sh" --source-only

    result=$(tproxy_get_required_ipsets 2>/dev/null)
    [ "$result" = $'ru\nus' ]
    grep -q "WARN.*Ignoring invalid country code 'xx' in xray.exclude_sets" "$LOG_FILE"
}

@test "_tproxy_check_required_ipsets: ignores an unknown country code" {
    load_common
    source "$LIB_DIR/firewall.sh"
    export VPD_CONFIG_FILE="$TEST_ROOT/fixtures/vpn-director.json"
    source "$LIB_DIR/ipset.sh" --source-only
    export XRAY_EXCLUDE_SETS="ru xx"
    source "$LIB_DIR/tproxy.sh" --source-only

    # The ipset mock knows ru but not xx; before the filter this returned 1.
    run _tproxy_check_required_ipsets
    assert_success
}

@test "tproxy_apply: applies rules when xray.exclude_sets holds an unknown code" {
    load_tproxy_module
    export XRAY_EXCLUDE_SETS="ru xx"

    run tproxy_apply
    assert_success
    assert_output --partial "Added exclusion for ipset: ru"
    assert_output --partial "Xray TPROXY routing applied successfully"
    refute_output --partial "aborting"
}
```

Run: `bats router/test/unit/tproxy.bats`
Expected: три новых теста падают (`xx` в выводе; `_tproxy_check_required_ipsets` возвращает 1; `tproxy_apply` уходит в мягкий отказ без «applied successfully»).

- [ ] **Step 2: Хелпер и три места в `tproxy.sh`**

Перед `_tproxy_check_required_ipsets` (после `_tproxy_resolve_exclude_set`):

```bash
# -------------------------------------------------------------------------------------------------
# _tproxy_exclude_sets [-q] - print the configured exclusion country codes on one line
# -------------------------------------------------------------------------------------------------
# Lower-cases each entry of XRAY_EXCLUDE_SETS and drops codes that are not in
# ALL_COUNTRY_CODES, mirroring parse_exclude_sets_from_json for tunnels. Each
# dropped code is logged at WARN unless -q is given, so the three callers of one
# apply produce a single warning. Before this filter a single bad code (e.g. "xx"
# typed in the Web UI) made ipset_ensure fail and cmd_apply abort before any rule
# was written; on boot that also skipped the nightly update cron.
# -------------------------------------------------------------------------------------------------
_tproxy_exclude_sets() {
    local quiet=0 set_key
    local -a exclude_sets_array valid=()

    [[ ${1:-} == "-q" ]] && quiet=1

    [[ -z ${XRAY_EXCLUDE_SETS:-} ]] && return 0
    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"

    for set_key in "${exclude_sets_array[@]}"; do
        [[ -n $set_key ]] || continue
        set_key=$(printf '%s' "$set_key" | tr 'A-Z' 'a-z')
        if ! _is_valid_country_code "$set_key"; then
            (( quiet )) || log -l WARN "Ignoring invalid country code '$set_key' in xray.exclude_sets"
            continue
        fi
        valid+=("$set_key")
    done

    (( ${#valid[@]} )) && printf '%s\n' "${valid[*]}"
    return 0
}
```

`_tproxy_check_required_ipsets`: заменить

```bash
    # Handle empty XRAY_EXCLUDE_SETS
    [[ -z ${XRAY_EXCLUDE_SETS:-} ]] && return 0

    read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"
```
на
```bash
    read -ra exclude_sets_array <<< "$(_tproxy_exclude_sets -q)"
```

`_tproxy_setup_iptables`: заменить

```bash
    # Handle empty XRAY_EXCLUDE_SETS
    if [[ -n ${XRAY_EXCLUDE_SETS:-} ]]; then
        read -ra exclude_sets_array <<< "$XRAY_EXCLUDE_SETS"
    else
        exclude_sets_array=()
    fi
```
на
```bash
    read -ra exclude_sets_array <<< "$(_tproxy_exclude_sets -q)"
```

`tproxy_get_required_ipsets` целиком:

```bash
tproxy_get_required_ipsets() {
    local set_key
    local -a exclude_sets_array

    read -ra exclude_sets_array <<< "$(_tproxy_exclude_sets)"

    for set_key in "${exclude_sets_array[@]}"; do
        [[ -n $set_key ]] || continue
        printf '%s\n' "$set_key"
    done
}
```

Обновить строку 20 шапки `tproxy.sh` (`tproxy_get_required_ipsets() - return list of exclude ipsets`) → `… - return list of valid exclude ipsets (unknown codes dropped with a WARN)` и добавить строку `#   _tproxy_exclude_sets [-q]     - validated, lower-cased XRAY_EXCLUDE_SETS on one line` рядом с `_tproxy_resolve_exclude_set`.

- [ ] **Step 3: Комментарий в Go**

`server/internal/webapi/handler_excludes.go:33-35`:

```go
// normalizeExcludeSets trims, lowercases, validates and de-duplicates country
// codes, keeping first-occurrence order. Unknown codes (e.g. "xx") pass here;
// lib/tproxy.sh drops them at apply time with a WARN in the VPN Director log.
```

- [ ] **Step 4: Прогнать shell-тесты**

Run: `bats router/test/unit/ && bats router/test/integration/`
Expected: все зелёные, включая три новых и старые `tproxy_get_required_ipsets: returns exclude sets` / `handles empty exclude sets`.

Документация правил, `.claude/rules/xray-tproxy.md`: в секции «Fail-Safe» (строки 113–116) после пункта `- Required exclusion ipsets not found` добавить:

```
- Unknown country codes in `xray.exclude_sets` are not "required": `_tproxy_exclude_sets` drops them with a WARN before the check, so a typo cannot abort apply
```

В таблице «Internal functions» (строки 138–146) после строки `_tproxy_check_required_ipsets()` добавить:

```
| `_tproxy_exclude_sets([-q])` | Validated, lower-cased `XRAY_EXCLUDE_SETS` on one line; `-q` suppresses the WARN per dropped code |
```

- [ ] **Step 5: Коммит**

```bash
gofmt -l server/internal/webapi/handler_excludes.go
git add router/opt/vpn-director/lib/tproxy.sh router/test/unit/tproxy.bats server/internal/webapi/handler_excludes.go \
        .claude/rules/xray-tproxy.md
git commit -m "fix(tproxy): ignore unknown country codes in xray.exclude_sets instead of aborting apply" \
  -m "The tunnel path already filters exclude codes through ALL_COUNTRY_CODES; the TPROXY path did not, so one bad code stopped cmd_apply before its first rule and, on boot, the update cron. Filter once, warn once, and let the three consumers share the list. This closes the same hole for the bot." \
  -m "Claude-Session: <URL текущей сессии>"
```

---

### Task 13: Финальная проверка блока 2

**Files:** без изменений кода, кроме правок, которые выявит проверка.

- [ ] **Step 1: Go целиком**

Run (через `claude-forge:build`): `cd server && go build ./... && go vet ./... && go test -count=1 ./...`
Expected: все пакеты `ok`, ни одного `(cached)`, ни одного `FAIL`/`SKIP`.

- [ ] **Step 2: gofmt по затронутым файлам**

Run: `git diff --name-only 7becf18..HEAD -- 'server/*.go' | xargs gofmt -l`
Expected: пусто.

- [ ] **Step 3: Bats целиком**

Run: `bats router/test/`
Expected: все тесты зелёные (unit 127 + 3 + 4 новых, integration + 4 новых, `common.bats` и остальные). Если появится `Executed N instead of expected M` — прогнать повторно: известная гонка харнесса на фиксированных `/tmp`-путях, не связанная с ветками (см. follow-ups).

- [ ] **Step 4: Фронт**

Run (через `claude-forge:build`): `cd web && npm run build`
Expected: `vue-tsc -b` без ошибок, `vite build` успешен.

- [ ] **Step 5: Сверка с инвариантами**

```bash
git grep -n "SaveVPNConfig" -- server                       # пусто
git grep -n "writeTimeout" -- server                        # пусто
git grep -n "shell.Exec(" -- server                         # пусто (остался только ExecContext)
git grep -nF 'bot\|vpn\|xray\|all]' -- README.md README.ru.md .claude          # пусто
git status --short                                          # только untracked-файлы автора
```

- [ ] **Step 6: Ручная проверка в dev-режиме (человек)**

`cd server && go run ./cmd/webui --dev` и `cd web && npm run dev`, `http://localhost:5173`, `admin`/`admin`:
- появляются `server/testdata/dev/webui.log` (с записями запросов) и `server/testdata/dev/.vpn-director.json.lock`; оба не видны в `git status`;
- вкладка Logs: в селекторе есть `webui`, «All» показывает четыре секции;
- Clients: добавить `192.168.50.77` в `xray` → 200, список обновился; `vpn-director.json` в `testdata/dev` имеет права `-rw-------`;
- Status → Apply отвечает за секунды (dev-мок), в `webui.log` нет строки `failed to extend write deadline`;
- параллельно `cd server && go run ./cmd/bot --dev` (если настроен `testdata/dev/telegram-bot.json`): `/logs webui` присылает содержимое лога Web UI.

Итог блока 2 фиксируется в ledger; `docs/superpowers/` остаётся в дереве до финальной уборки после блока 4.

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
- **Поведение.** Глобальный `WriteTimeout` вернулся к 30 с; долгие хендлеры продлевают дедлайн ответа сами (лимит команды + 30 с, импорт 2 мин).
- **Поведение (задача 12).** Неизвестный двухбуквенный код страны в `xray.exclude_sets` больше не обрывает apply и не срывает бут: он отбрасывается с `WARN Ignoring invalid country code` в `/tmp/vpn-director.log`, API отвечает 200. Это заменяет премису спеки §4.4 про `ipset_ensure`.
- **API.** Проверки 409/404 в мутациях клиентов выполняются под локом конфига; тексты и коды не менялись. Ошибка чтения конфига в мутации — 500 `failed to load configuration`; таймаут лока — 500 `failed to save configuration` с причиной в логе Web UI.
- **Интерфейсы Go.** `service.ShellExecutor.Exec` принимает `context.Context`; `service.ConfigStore` теряет `SaveVPNConfig` и получает `UpdateVPNConfig`.
