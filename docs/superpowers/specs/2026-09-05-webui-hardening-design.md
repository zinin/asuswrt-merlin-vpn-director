# VPN Director Web UI: исправления поведения, надёжность, самообновление

Дата: 2026-09-05
Ветка блока 1: `feature/webui-behavior`
Статус: дизайн согласован в брейнсторме, ждёт ревью спека

## 1. Контекст

Web UI живёт в `server/cmd/webui` (Go, HTTPS, `net/http`), `server/internal/webapi`
(роутер, middleware, обработчики), `server/internal/auth` (проверка пароля по
`/etc/shadow`, JWT) и `web/` (Vue 3 SPA, встраивается в бинарник через
`go:embed`). Сервисный слой `server/internal/service` общий с Telegram-ботом.
Web UI влит в `master` 2026-04-07, документирован в README, ставится
`install.sh`, запускается init-скриптом `S98vpn-director-webui`.

Аудит кода 2026-09-05 нашёл 22 проблемы. Они делятся на четыре группы:
поведение, которое обманывает пользователя; надёжность двух демонов над
одним конфигом; самообновление, которое не знает о Web UI; долг в
безопасности, документации и репозитории.

### 1.1. Найденные проблемы

Поведение:

1. `POST /api/update` отдаёт заглушку со статусом 200 и `ok:false`
   (`webapi/handler_logs.go:88`), а фронт пишет «Update completed»
   (`web/src/components/SettingsTab.vue:46`).
2. Самообновление бота не трогает Web UI: список файлов
   (`updater/downloader.go:25`) без бинарника webui и без
   `S98vpn-director-webui`, скрипт `update_script.sh.tmpl` копирует и
   перезапускает только бота.
3. «Update IPsets» вызывает `apply` вместо `update`
   (`webapi/handler_status.go:65`); только `cmd_update` выставляет
   `IPSET_FORCE_UPDATE=1` (`vpn-director.sh:283`).
4. Клиент в туннель создаёт туннель с пустым `exclude`
   (`webapi/handler_clients.go:100`); визард бота наследует `exclude_sets`
   и добавляет `/32` (`wizard/apply.go:81`, `:100`).
5. Клиент добавляется в маршрут без проверки других маршрутов
   (`webapi/handler_clients.go:89`); бот отказывает
   (`handler/clients.go:318`). IP оказывается и в xray, и в туннеле.
6. Валидация клиентов принимает IPv6 и `/32` без нормализации
   (`webapi/handler_clients.go:11`); бот принимает только IPv4 и срезает
   `/32` (`handler/clients.go:404`).
7. Мутации клиентов и исключений только сохраняют JSON; бот после каждой
   зовёт `Apply()` (`handler/clients.go:165`, `handler/exclude.go:178`).
8. Вкладка Status грузит статус и внешний IP через `Promise.all`
   (`StatusTab.vue:13`): ошибка `curl ifconfig.me` прячет статус.
9. Источник логов `xray` читает `/tmp/xray-access.log`
   (`webapi/handler_logs.go:9`), который никто не пишет: в шаблоне Xray
   нет секции `log`, вывод Xray теряется в init-скрипте Entware.
10. Коды стран не валидируются (`webapi/handler_excludes.go:36`); фронт
    приводит к верхнему регистру (`ExclusionsTab.vue:34`), бот и шаблон
    хранят нижний; неверный код валит `ipset_ensure`, и apply прерывается.
11. Импорт выбрасывает ошибки разбора подписки
    (`webapi/handler_servers.go:158`); бот их показывает.

Надёжность:

12. У Web UI нет файла лога: init-скрипт шлёт вывод в `/dev/null`
    (`S98vpn-director-webui:28`), `cmd/webui/main.go` не поднимает
    `internal/logging`.
13. `WriteTimeout` 30 секунд (`webapi/server.go:33`) при `shell.Exec` без
    таймаута (`shell/shell.go:10`): долгий apply обрывает ответ, зависший
    скрипт держит `OpMutex` навсегда.
14. Конфиг пишется через `os.WriteFile` без блокировки и без атомарности
    (`vpnconfig.go:131`); бот и Web UI работают над одним файлом.
15. `acquire_lock` в `lib/common.sh:432` при занятом локе выходит с кодом 0:
    параллельный apply от Web UI молча пропускается, Go считает его
    успешным.

Долг:

16. Middleware принимает Bearer (`webapi/middleware.go:44`), но логин не
    отдаёт токен в теле (убрано в 5da62dd).
17. `vpn-director.json` с `jwt_secret` и `servers.json` с UUID пишутся 0644.
18. JWT не отзывается при смене пароля роутера.
19. Сервер слушает `:8444` на всех интерфейсах.
20. `install.sh` не запускает Web UI после установки, README обещает
    «installed automatically».
21. CI собирает Go 1.24 при `go 1.25.5` в `go.mod`.
22. `CLAUDE.md` пишет «future WebUI», нет `.claude/rules/webui.md`; в дереве
    лежат неотслеживаемые `server/bot`, `server/webui`, `review.diff`,
    `test_exit.sh`.

## 2. Объём и порядок

Работа разбита на четыре блока. Каждый блок идёт в своей ветке от `master`
со своим PR, детальный план пишется перед началом блока. Спек коммитится в
ветку блока 1, перед PR удаляется из дерева и остаётся доступен через
`git show <hash>:docs/superpowers/specs/2026-09-05-webui-hardening-design.md`.

| Блок | Ветка | Содержание | Зависит от |
|------|-------|------------|------------|
| 1 | `feature/webui-behavior` | проблемы 1, 3–11 | нет |
| 2 | `feature/webui-reliability` | проблемы 12–15, 17 | блок 1 (авто-apply делает таймауты значимыми) |
| 3 | `feature/unified-updater` | проблема 2, настоящий `/api/update` | блок 2 (лог Web UI) |
| 4 | `feature/webui-housekeeping` | проблемы 18, 20–22 | блоки 1–3 (сводная документация) |

Вне объёма, решено в брейнсторме:

- редактирование `exclude` отдельных туннелей в UI (только наследование при
  создании);
- выдача JWT для API-клиентов (`POST /api/token`); Bearer в middleware
  остаётся как есть, проблема 16 закрывается документацией;
- опция `webui.listen` (проблема 19);
- access-лог Xray (только error-лог);
- блокировка для shell-писателей конфига (`configure.sh` интерактивный);
- тестовый фреймворк для Vue.

## 3. Принятые решения

| Вопрос | Решение |
|--------|---------|
| Применение изменений | авто-apply после каждой мутации, как в боте |
| Исключения туннелей | новый туннель получает копию `xray.exclude_sets` |
| Bearer | middleware остаётся, токен в теле не выдаём |
| Логи Xray | error-лог в файл через секцию `log` шаблона, access выключен |
| Безопасность | права 0600 и отзыв JWT при смене пароля |
| Самообновление | общий системный updater, бот и Web UI как адаптеры |
| Уведомление об обновлении из Web UI | бот пишет всем активным чатам, фронт опрашивает версию, баннер о новой версии |
| Документы | один спек, план на каждый блок перед блоком |

## 4. Блок 1. Поведение Web UI

### 4.1. Авто-apply

Новый хелпер в `webapi`:

```go
// saveAndApply persists cfg and applies it. It returns a typed error so the
// handler can tell "not saved" from "saved but not applied".
func saveAndApply(deps *Deps, cfg *vpnconfig.VPNDirectorConfig) error
```

Контракт ответов для всех мутаций:

| Исход | Код | Тело |
|-------|-----|------|
| сохранено и применено | 200 | `{"ok": true}` |
| ошибка сохранения | 500 | `{"error": "failed to save configuration"}` |
| сохранено, apply упал | 500 | `{"error": "configuration saved, but apply failed: <строка>", "saved": true}` |

`<строка>` это последняя непустая строка вывода `vpn-director.sh`, обрезанная
до 200 символов. Хелпер используют `POST /api/clients`,
`POST /api/clients/pause`, `POST /api/clients/resume`, `DELETE /api/clients`,
`POST /api/excludes/sets`, `POST /api/excludes/ips`,
`DELETE /api/excludes/ips`. Импорт серверов apply не делает, как и в боте:
bypass-набор пересобирается при выборе сервера через `restart xray`.

Фронт при ошибке с `saved: true` показывает сообщение сервера и всё равно
перечитывает список, чтобы сохранённое изменение было видно. Кнопка Apply на
вкладке Status служит повтором.

### 4.2. Update IPsets

Интерфейс `service.VPNDirector` получает `Update() error`;
`VPNDirectorService.Update` вызывает `vpn-director.sh update`.
`handleUpdateIPsets` переключается на него, ошибка отдаётся как
`failed to update ipsets: <строка>`. Моки в `webapi/test_helpers_test.go`,
`handler/xray_test.go`, `handler/status_test.go`, `handler/clients_test.go`,
`wizard/apply_test.go` получают метод. Dev-исполнитель уже отвечает на
`update`.

### 4.3. Клиенты

Общая валидация переезжает в `vpnconfig`:

```go
// NormalizeClientAddr trims s, accepts an IPv4 address or IPv4 CIDR,
// strips a trailing /32 and returns the canonical string.
func NormalizeClientAddr(s string) (string, error)
```

Ошибка валидации имеет текст `invalid IPv4 address or CIDR`. Бот заменяет
свои `isValidIPOrCIDR` и `normalizeIP` в `handler/clients.go` на эту функцию.

`POST /api/clients`:

1. нормализует `ip`, при ошибке 400;
2. проверяет `route` как сейчас;
3. собирает клиентов через `vpnconfig.CollectClients` и сравнивает адреса
   после нормализации обеих сторон, потому что старые конфиги хранят и
   `1.2.3.4`, и `1.2.3.4/32`; совпадение даёт
   409 `{"error": "client already configured for <route>"}`;
4. для маршрута xray добавляет адрес в `xray.clients`; для туннеля создаёт
   туннель, если его нет, с `Clients: [addr]` и `Exclude` как копия
   `xray.exclude_sets`;
5. вызывает `saveAndApply`.

`POST /api/clients/pause`, `POST /api/clients/resume`, `DELETE /api/clients`
нормализуют параметр `ip` (400 при ошибке) и отвечают 404
`{"error": "client not found"}`, если адрес не настроен ни в одном маршруте.
Удаление по-прежнему убирает адрес из всех маршрутов и из `paused_clients`.

Список маршрутов во фронте не меняется: xray, wgc1–wgc5, ovpnc1–ovpnc5.

### 4.4. Исключения

`POST /api/excludes/sets` обрабатывает каждый элемент: обрезает пробелы,
приводит к нижнему регистру, проверяет `^[a-z]{2}$`, иначе 400
`{"error": "invalid country code: <значение>"}`. Дубли убираются с
сохранением порядка. Затем `saveAndApply`. Ошибку неизвестного кода
(например `xx`) поймает `ipset_ensure` при apply, и она вернётся в ответе
как «saved, but apply failed».

`POST /api/excludes/ips` и `DELETE /api/excludes/ips` нормализуют адрес через
`NormalizeClientAddr`.

Фронт вводит и показывает коды в нижнем регистре; проверка дублей сравнивает
после приведения к нижнему регистру.

### 4.5. Вкладка Status

`StatusTab.vue` грузит статус и внешний IP через `Promise.allSettled`.
Ошибка IP показывает в карточке «unavailable» и текст ошибки, статус
отображается независимо.

### 4.6. Логи Xray

Шаблон `router/opt/etc/xray/config.json.template` получает секцию верхнего
уровня:

```json
"log": {
  "loglevel": "warning",
  "access": "none",
  "error": "/tmp/xray-error.log"
}
```

Оба генератора конфига (`service/xray.go` и `lib/xrayconf.sh`) копируют шаблон
и заменяют только `outbounds`, секция проходит без изменений кода.

Пути логов перестают быть константами в `webapi`: `paths.Paths` получает
`XrayLogPath` (`/tmp/xray-error.log`, в dev `testdata/dev/xray-error.log`),
`main.go` собирает карту источников и кладёт её в `Deps.LogPaths`. Источник
`xray` в `/api/logs` читает error-лог. Бот добавляет источник `xray` в
`/logs` и путь в список ротации (`cmd/bot/main.go:106`).

Release notes: на установленных роутерах секция появится в `config.json`
после следующего выбора сервера в боте или Web UI.

### 4.7. Импорт

При пустом результате разбора подписки ответ 400 содержит до трёх ошибок
`DecodeSubscription`:
`no VLESS servers found in subscription: <e1>; <e2>; <e3>`.

### 4.8. Заглушка Update

`POST /api/update` отвечает 501
`{"error": "self-update via Web UI is not supported yet"}`. Фронт показывает
текст сервера. Блок 3 заменяет заглушку.

### 4.9. Мелочи API и SPA

- Неизвестный путь под `/api/` отдаёт JSON 404 `{"error": "not found"}`:
  в защищённом mux регистрируется fallback-паттерн `/api/`.
- `spaHandler` ставит `Cache-Control: no-cache` для `index.html` и
  `Cache-Control: public, max-age=31536000, immutable` для `/assets/`, чтобы
  после обновления бинарника браузер не держал старый бандл.

### 4.10. Тесты блока 1

- `webapi`: табличные тесты на 400, 404, 409, 501, на тело ответа при
  провале apply, на fallback 404 и заголовки кеша.
- `vpnconfig`: тесты `NormalizeClientAddr` (IPv4, CIDR, `/32`, IPv6, мусор,
  пробелы).
- бот: тесты `handler/clients.go` после перехода на общую функцию.
- bats `router/test/unit/xray_template.bats`: секция `log` в шаблоне и в
  сгенерированном конфиге.
- фронт: `npm run build` (`vue-tsc`) без ошибок, ручная проверка в
  dev-режиме по чек-листу плана.

## 5. Блок 2. Надёжность

### 5.1. Логи Web UI

`paths.Paths` получает `WebUILogPath` (`/tmp/vpn-director-webui.log`, в dev
`testdata/dev/webui.log`). `WebUIConfig` получает `LogLevel string`
(`log_level`, необязательное, по умолчанию info). `cmd/webui/main.go`
поднимает `logging.NewSlogLogger` до загрузки конфига, как бот, затем
выставляет уровень. Константа размера лога 200 KB переезжает в
`logging.DefaultMaxSize`. Оба демона крутят `StartRotation` над одним списком:
лог бота, лог shell, лог Web UI, error-лог Xray. Обрезка через `os.Truncate`
идемпотентна, одновременная ротация из двух процессов безопасна.

Источник `webui` появляется в `/api/logs` и в `/logs` бота.

### 5.2. Таймауты команд

`service.ShellExecutor` получает контекст:

```go
type ShellExecutor interface {
    Exec(ctx context.Context, name string, args ...string) (*shell.Result, error)
}
```

`shell.ExecContext` строит `exec.CommandContext`, ставит `cmd.Cancel` на
SIGTERM и `cmd.WaitDelay` 10 секунд, чтобы скрипт успел убрать временные
файлы. Истечение контекста возвращает ошибку `command timed out after <d>`.

Сервисы задают лимит на команду и строят контекст сами от
`context.Background()`, а не от контекста HTTP-запроса: закрытая вкладка не
должна убивать apply на середине.

| Команда | Лимит |
|---------|-------|
| status | 30 с |
| apply, restart, stop, restart xray | 5 мин |
| update | 15 мин |
| curl (внешний IP) | 15 с |
| tail (логи) | 10 с |

Меняются `devmode.Executor`, `devmode/executor_test.go`,
`service/vpndirector_test.go` и остальные моки исполнителя.

### 5.3. Таймауты HTTP

Хелпер в `webapi`:

```go
// extendWriteDeadline pushes the response deadline past the server-wide
// WriteTimeout for handlers that run long shell commands.
func extendWriteDeadline(w http.ResponseWriter, d time.Duration)
```

Реализация через `http.NewResponseController(w).SetWriteDeadline`; ошибку
«не поддерживается» от `httptest.ResponseRecorder` хелпер игнорирует.
Дедлайн равен лимиту команды плюс 30 секунд, для импорта 2 минуты (загрузка
подписки 10 секунд плюс DNS по каждому серверу). Хелпер вызывают apply,
restart, stop, обновление ipset-ов, выбор сервера, импорт и все мутации с
авто-apply.
Глобальный `WriteTimeout` 30 секунд остаётся для остальных.

### 5.4. Атомарная запись и блокировка конфига

`vpnconfig.SaveVPNDirectorConfig` и `SaveServers` пишут через временный файл
в том же каталоге, `Sync`, `Chmod(0600)`, `Rename`. Права 0600 закрывают
проблему 17.

`service.ConfigStore` получает метод:

```go
// UpdateVPNConfig runs fn under an exclusive cross-process lock:
// lock, load, fn, save, unlock. Readers stay lock-free because Save is atomic.
UpdateVPNConfig(fn func(cfg *vpnconfig.VPNDirectorConfig) error) error
```

Лок-файл `.vpn-director.json.lock` лежит в каталоге конфига, поэтому в
dev-режиме он в `testdata/dev`. Блокировка через `syscall.Flock(LOCK_EX)` с
опросом `LOCK_NB` каждые 50 мс до 30 секунд, после чего ошибка
`config lock timeout`. Файл `/var/lock/vpn-director.lock` остаётся локом
shell-скрипта, Go его не трогает.

На `UpdateVPNConfig` переводятся все Go-писатели: обработчики Web UI
(клиенты, исключения, выбор сервера, синхронизация `xray.servers` при
импорте, автогенерация `jwt_secret` в `main.go`, для чего `ConfigService`
создаётся до неё) и бот (`handler/clients.go`,
`handler/exclude.go`, `wizard/apply.go`, синхронизация `xray.servers` в
`handler/import.go` и `handler/xray.go`). Шесть моков `ConfigStore` получают
метод: мок вызывает `fn` на своём `cfg` и записывает результат в `savedCfg`.

### 5.5. Ожидание лока в vpn-director.sh

`vpn-director.sh` получает опцию `--wait[=SEC]`, по умолчанию 120 секунд;
`parse_option` экспортирует `VPD_LOCK_WAIT`. `acquire_lock` в `lib/common.sh`
при заданной переменной ждёт лок циклом `flock -n` с шагом 1 секунда, потому
что BusyBox `flock` не имеет `-w`; по истечении срока пишет
`log -l ERROR "Timed out waiting for lock"` и выходит с кодом 1. Без
переменной поведение прежнее: `flock -n` и выход с кодом 0.

`VPNDirectorService` передаёт `--wait` первым аргументом в apply, restart,
stop и update. `devmode.Executor.mockVPNDirector` пропускает аргументы,
начинающиеся с `--`, прежде чем брать команду. Хуки `firewall-start`,
`wan-event` и `S99vpn-director` не меняются.

### 5.6. Тесты блока 2

- `shell`: `ExecContext` со спящей командой и коротким лимитом, проверка
  текста ошибки и завершения процесса.
- `vpnconfig`: атомарная запись, права 0600, отсутствие временных файлов
  после успеха и после ошибки.
- `service`: `UpdateVPNConfig` из двух горутин с отдельными дескрипторами
  лока, таймаут лока.
- `webapi`: хелпер дедлайна на `httptest`, тесты обработчиков с моками.
- bats `router/test/unit/lock.bats`: `acquire_lock` при занятом локе в
  режимах по умолчанию и с `VPD_LOCK_WAIT`, включая код выхода по таймауту.

## 6. Блок 3. Единое самообновление

### 6.1. Пакет updater

- `scriptFiles` дополняется `router/opt/etc/init.d/S98vpn-director-webui`.
- `DownloadRelease` качает оба бинарника: `telegram-bot-<arch>` и
  `webui-<arch>` для arm64 и arm. Отсутствие любого ассета в релизе это
  ошибка загрузки.
- `RunUpdateScript` принимает структуру:

```go
type RunOptions struct {
    OldVersion string
    NewVersion string
    ChatID     int64  // 0 when the update was started from the Web UI
    Initiator  string // "bot" or "webui"
}
```

### 6.2. Скрипт обновления

Шаблон `update_script.sh.tmpl` работает со списком демонов. Каждый демон
описан именем, путём бинарника и init-скриптом; работающий процесс
определяется через `pgrep -f` по полному пути, как сейчас:
`telegram-bot` (`/opt/vpn-director/telegram-bot`, `S98telegram-bot`) и
`webui` (`/opt/vpn-director/webui`, `S98vpn-director-webui`).

Шаги:

1. запомнить, какие демоны работают;
2. снять оба с monit, если monit есть;
3. остановить работающих, дождаться выхода, при необходимости `kill -9`;
4. скопировать скрипты, шаблоны, init-скрипты, хуки и оба бинарника;
5. выставить права;
6. записать `notify.json`;
7. снять lock;
8. вернуть monit;
9. запустить тех демонов, что работали до обновления.

Скрипт остаётся `#!/bin/sh` с `set -e`. Восстановление через
`trap ... EXIT` с проверкой кода выхода, потому что ash не знает `ERR`: при
ненулевом коде скрипт запускает обратно демонов, которые работали, пишет
причину в `update.log` и записывает `notify.json` со `status: failed`.

Формат `notify.json`:

```json
{"chat_id": 0, "old_version": "v1.2.0", "new_version": "v1.3.0",
 "status": "ok", "initiator": "webui"}
```

### 6.3. Пакет updateflow

Новый пакет `server/internal/updateflow` выносит оркестрацию из
`handler/update.go`:

```go
func New(upd updater.Updater, currentVersion string, devMode bool) *Flow

type CheckResult struct {
    Current, Latest string
    UpdateAvailable bool
    Changelog       string
    CheckedAt       time.Time
}

// Check returns the cached result when it is younger than 30 minutes.
// force bypasses the cache, but not more often than once a minute.
func (f *Flow) Check(ctx context.Context, force bool) (CheckResult, error)

type StartResult struct {
    From, To string
}

// Start runs the pre-flight checks synchronously, creates the lock and
// launches download + update script in a goroutine. progress receives
// human-readable status lines.
func (f *Flow) Start(ctx context.Context, chatID int64, progress func(string)) (StartResult, error)
```

Ошибки типизированы: `ErrDevMode`, `ErrDevVersion`, `ErrInProgress`,
`ErrUpToDate`, обёртка для сбоев GitHub. Провал загрузки или запуска скрипта
чистит файлы и снимает lock, как сейчас. Бот превращается в адаптер: тексты
сообщений сохраняются, прогресс уходит в чат. Web UI пишет прогресс в лог.
`updatechecker` бота не меняется.

### 6.4. Web API

| Метод | Путь | Ответ |
|-------|------|-------|
| GET | `/api/update/check` | `CheckResult` в JSON; `?force=1` обходит кеш с минутным порогом; в dev `{"update_available": false, "dev": true}` |
| POST | `/api/update` | 202 `{"ok": true, "from": ..., "to": ...}`; 200 `{"ok": true, "update_available": false}`; 400 dev-режим или версия `dev`; 409 обновление идёт; 502 GitHub недоступен |
| GET | `/api/update/status` | `{"in_progress": bool}` по lock-файлу |

Заглушка 501 из блока 1 удаляется.

### 6.5. Фронт

Вкладка Settings показывает текущую и последнюю версии, свёрнутый changelog
и кнопку «Update to vX», активную только при `update_available`. После
подтверждения и ответа 202 экран «Updating, the server is restarting», опрос
`/api/version` каждые 3 секунды с `skipAuthRedirect`, чтобы ошибки соединения
во время перезапуска не выбрасывали на логин. Условие выхода: версия равна
`to` или прошло 5 минут. Затем перезагрузка страницы ради нового бандла.
Cookie переживает обновление, секрет в конфиге не меняется.

В шапке баннер «Version vX available, see Settings» по результату `check`
при загрузке приложения.

### 6.6. Бот

`startup.CheckAndSendNotify` получает chatstore. При `chat_id` 0 бот пишет
всем активным пользователям, каталог обновления чистится после первой
успешной отправки. При `status: failed` текст сообщения: `Update failed:
see /tmp/vpn-director-update/update.log`. В dev-режиме chatstore нет, и
уведомление при `chat_id` 0 пропускается с записью в лог.

`/update` из бота обновляет и Web UI без изменений в обработчике.

### 6.7. Одноразовая миграция

Первый переход на версию с этим блоком выполнит старый updater бота, который
не трогает Web UI. Release notes этой версии просят один раз перезапустить
`install.sh`. Дальше оба пути обновляют всё.

### 6.8. Тесты блока 3

- golden-тест шаблона скрипта с двумя демонами и trap-веткой;
- downloader на два ассета через `httptest`;
- `Flow` на моке `Updater` по состояниям: dev, идёт обновление, уже последняя,
  провал загрузки с очисткой, кеш и порог `force`;
- обработчики API на коды ответов;
- `CheckAndSendNotify` с `chat_id` 0, со статусом failed и без chatstore;
- адаптация тестов `handler/update.go`.

Сценарий перезапуска проверяется на роутере: в dev-режиме `POST /api/update`
отвечает 400.

## 7. Блок 4. Безопасность и хозяйство

### 7.1. Отзыв JWT при смене пароля

`ShadowAuth.Fingerprint(username) (string, error)` возвращает первые 8 байт
SHA-256 от хеша пароля из `/etc/shadow` в hex. Логин вычисляет отпечаток
для введённого имени, `JWTService.Create(subject, fingerprint)` кладёт его в
claim `pwh`; `Validate` возвращает его в `Claims`. `authMiddleware` получает
`ShadowAuth` вторым аргументом и сверяет claim с текущим отпечатком при
каждом запросе, несовпадение или отсутствие claim дают 401. Токены, выданные до обновления,
потребуют одного повторного логина. Кеш по mtime не заводится.

### 7.2. Установщик

После `setup_webui_config` новая функция `start_webui` запускает
`S98vpn-director-webui start`, если Web UI не работает, и печатает адрес с
LAN IP. Текст next steps перестаёт называть запуск опциональным.

### 7.3. CI

- `telegram-bot.yml`: `go-version-file: server/go.mod`.
- новый `test.yml` на push и pull request: `go vet ./...` и `go test ./...`
  в `server/`, `npm ci` и `npm run build` в `web/`, `bats router/test/unit`.

### 7.4. Документация

- `CLAUDE.md`: строки про `server/cmd/webui`, `internal/webapi`,
  `internal/auth`, `web/`; команды `make build-webui` и dev-режим; секция
  `webui` конфига с `log_level`; убрать «future WebUI».
- новый `.claude/rules/webui.md`: архитектура, таблица API, аутентификация,
  dev-режим, сборка с embed, поток обновления, Bearer как поддержанный вход
  без выдачи токена.
- README: строка вкладки Logs (`bot, vpn, webui, xray`), раздел Web UI про
  обновление и `log_level`.

Каждый блок правит документацию, которую задел; блок 4 сводит остальное.

### 7.5. Уборка

`server/.gitignore` получает `/bot` и `/webui`. Кандидаты на удаление, решение
за автором в момент выполнения: `review.diff`, `test_exit.sh`,
`docs/superpowers/plans/2026-03-26-exclude-ips-and-multi-resolve-continuation-prompt.md`.

## 8. Затрагиваемые файлы

| Блок | Файлы |
|------|-------|
| 1 | `server/internal/webapi/{handler_clients,handler_excludes,handler_status,handler_servers,handler_logs,router}.go` и тесты; `server/internal/service/{interfaces,vpndirector}.go`; `server/internal/vpnconfig/validate.go` (новый) и тест; `server/internal/handler/clients.go`, `misc.go` и тесты; `server/internal/paths/paths.go`; `server/cmd/webui/main.go`; `server/cmd/bot/main.go`; `router/opt/etc/xray/config.json.template`; `router/test/unit/xray_template.bats`; `web/src/components/{StatusTab,ClientsTab,ExclusionsTab,LogsTab,SettingsTab}.vue`; `web/src/api.ts` |
| 2 | `server/internal/shell/shell.go`; `server/internal/service/*.go`; `server/internal/devmode/executor.go`; `server/internal/vpnconfig/vpnconfig.go`; `server/internal/logging/logger.go`; `server/internal/paths/paths.go`; `server/cmd/webui/main.go`; `server/internal/webapi/{server,handler_*}.go`; `server/internal/handler/{clients,exclude,import,xray}.go`; `server/internal/wizard/apply.go`; все моки; `router/opt/vpn-director/vpn-director.sh`; `router/opt/vpn-director/lib/common.sh`; `router/test/unit/lock.bats` (новый) |
| 3 | `server/internal/updater/{downloader,script,updater}.go`, `update_script.sh.tmpl`; `server/internal/updateflow/` (новый); `server/internal/handler/update.go`; `server/internal/startup/notify.go`; `server/internal/bot/bot.go`; `server/internal/webapi/handler_update.go` (новый), `router.go`; `web/src/{App.vue,api.ts,types.ts}`, `components/SettingsTab.vue` |
| 4 | `server/internal/auth/{shadow,jwt}.go`; `server/internal/webapi/{middleware,handler_auth}.go`; `install.sh`; `.github/workflows/{telegram-bot,test}.yml`; `CLAUDE.md`; `.claude/rules/webui.md` (новый); `README.md`; `server/.gitignore` |

## 9. Риски

| Риск | Смягчение |
|------|-----------|
| Авто-apply добавляет секунды ожидания на каждый клик | принято как паритет с ботом; кнопки блокируются на время запроса |
| SIGTERM по таймауту прерывает apply на середине | лимит 5 минут это страховка от зависания; повторный apply идемпотентен |
| Переписанный скрипт обновления ломает критический путь | golden-тесты, trap-восстановление, ручная проверка на роутере до тега |
| Старый updater не обновит Web UI при первом переходе | release notes с одноразовым `install.sh` |
| GitHub API без токена: 60 запросов в час | кеш 30 минут и минутный порог для `force` |
| flock на файловой системе `/opt` | USB ext4 и JFFS поддерживают flock; в dev-режиме лок лежит в `testdata/dev` |
| Токены до блока 4 теряют силу | один повторный логин, отражено в release notes |
