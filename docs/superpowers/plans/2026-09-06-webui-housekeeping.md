# Web UI: безопасность и хозяйство (блок 4) — план реализации

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Закрыть последний блок работы над Web UI: привязать JWT к текущему паролю роутера, довести установщик до работающего Web UI без ручного шага, поднять CI до реальной проверки трёх стеков, свести документацию всех четырёх блоков и разобрать долг, накопленный ревью блоков 2 и 3.

**Architecture:** `ShadowAuth` учится считать отпечаток хеша пароля; `JWTService` кладёт его в claim `pwh`, а `authMiddleware` сверяет с текущим на каждом запросе — токен живёт ровно до смены пароля. Установщик получает `start_webui` и становится тестируемым через `--source-only`, как остальные скрипты репозитория. CI распадается на три параллельные джобы (Go, web, bats), для чего пакет `updater` сначала уводится с сети: боевой хост `raw.githubusercontent.com` становится инъектируемым, как уже инъектируемы API-хост, архитектура и интерпретатор. Фронт получает типизированный API-клиент, а состояние обновления переезжает из компонента в модуль, чтобы уход с вкладки Settings не убивал прогресс.

**Tech Stack:** Go 1.25 (`crypto/sha256`, `encoding/hex`, `errors.Is`, `regexp`, `httptest.NewTLSServer`), `github.com/golang-jwt/jwt/v5`, Bash 5 (`install.sh`, `lib/common.sh`), BusyBox ash (init-скрипты), Bats + bats-support/bats-assert, GitHub Actions, Vue 3 + TypeScript (axios, `vue-tsc`).

**Spec:** `docs/superpowers/specs/2026-09-05-webui-hardening-design.md`, секция 7 «Блок 4. Безопасность и хозяйство». Секции 1–3 — контекст и принятые решения; §8 — таблица затрагиваемых файлов; §9 — риски. Секции 4–6 (блоки 1–3) выполнены и служат источником уже существующих API.

**Status:** Tasks 1–16 done (implementation HEAD `60dc171`; after mesh-review auto-fixes HEAD `3ef2916`). «Проверка блока» remains. Mesh-review of all four blocks ran on `feature/webui-behavior` vs `master` with autodecide. Ход выполнения блока 4 — в `.superpowers/sdd/2026-09-06-webui-housekeeping/progress.md` (каталог gitignored).

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

✅ Done — see commit(s): `51eba29`

---

### Task 9: три мелочи webapi

✅ Done — see commit(s): `0113725`

---

### Task 10: честные ошибки и честные имена тестов

✅ Done — see commit(s): `fc6d6dc`, `5ad27e1`

---

### Task 11: shell роутера — валидация, граница теста, шаблон, шапка

✅ Done — see commit(s): `2ab6163`, `858dd6a`

---

### Task 12: типизированный API-клиент

✅ Done — see commit(s): `512dfc5`

---

### Task 13: обновление переживает уход с вкладки Settings

✅ Done — see commit(s): `47b222a`

---

### Task 14: три полировки интерфейса

✅ Done — see commit(s): `d7a56bb`, `4b44681`

---

### Task 15: сводная документация четырёх блоков

✅ Done — see commit(s): `efc6041`

---

### Task 16: уборка репозитория

✅ Done — see commit(s): `4140e6e`

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
