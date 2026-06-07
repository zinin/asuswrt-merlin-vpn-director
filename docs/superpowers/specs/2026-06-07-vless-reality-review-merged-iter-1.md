# Merged Design Review — Iteration 1 (vless-reality)

- **Date:** 2026-06-07
- **Design:** `docs/superpowers/specs/2026-06-07-vless-reality-design.md`
- **Plan:** `docs/superpowers/plans/2026-06-07-vless-reality-vision.md`
- **Reviewers (default preset):** claude (self), codex (gpt-5.5 xhigh), zai/glm, alibaba/qwen, deepseek/v4-pro, ollama/kimi, ollama/minimax

## Reviewer status

| Reviewer | Result |
|---|---|
| claude (self-review) | ✅ completed |
| codex (gpt-5.5) | ✅ completed |
| alibaba/qwen | ✅ completed (retry) |
| ollama/kimi | ✅ completed (retry) |
| zai/glm | ❌ failed — runaway extended-thinking loop, no output (both attempts) |
| deepseek/v4-pro | ❌ failed — stream truncated mid-thinking (both attempts) |
| ollama/minimax | ❌ failed — timed out in thinking phase |

---

## claude (self-review)

Проверено по факту: `vless/parser.go`, `vpnconfig.go`, `service/xray.go`, `config.json.template`, `import_server_list.sh`, `configure.sh`, `handler/import.go`, `webapi/handler_servers.go`, `updater/downloader.go`, `install.sh`, `test_helper.bash`, `import_server_list.bats`, реальная подписка (`decoded.txt`), окружение (jq-1.7, bats 1.x).

### Critical Issues
1. **Несоответствие `xrayconf_generate` Task 5 vs Task 7 / дизайн.** Task 5 реализует `.outbounds = [$ob]`, дизайн §3 и план говорят «replace `outbounds[0]` wholesale» (`.outbounds[0] = $ob`). На пустом массиве оба эквивалентны; нужно унифицировать формулировку. Рекомендация: оставить `.outbounds = [$ob]`, поправить текст дизайна/плана.
2. **`xrayconf.bats` переопределяет `setup()` из `test_helper.bash`** (ставит mocks/PATH/HOSTS_FILE). Для xrayconf случайно безвредно, но нарушает конвенцию (остальные unit-bats зовут `load_*`-хелперы) и создаёт ловушку для будущих тестов с resolve. Fix: убрать `setup()` override либо добавить `load_xrayconf_module` в helper.
3. **Task 8 путает номера строк install.sh vs downloader.go** (стр. 146 — из downloader.go, реальная `lib/tproxy.sh` в install.sh = 147). Развести нумерацию.

### Concerns
4. `headerType=none` во ВСЕХ 81 строках реальной подписки — не парсится/не генерируется. Для tcp корректно (Xray default `tcpSettings.header.type="none"`), но стоит задокументировать, чтобы не выглядело упущением.
5. Подписка БЕЗ `alpn`; порядок параметров не алфавитный (`pbk`/`sid` в конце). Парсер безопасен (коллизия `type` vs `headerType` исключена ведущим `&`). Тестовые URI стоит дополнить реальным `headerType=none`.
6. Тест `step_parse_and_save_servers` (Task 6 Step 5) — убедиться, что вызывается `load_import_server_list` и порядок `VPD_CONFIG`. Литеральный `1.2.3.4` резолвится через fast-path (mock не нужен — в плане верно).
7. Дизайн §57 «byte-identical» → надо «semantically identical» (живое противоречие в документе).
8. Минор: `load '../test_helper.bash'` vs конвенция `load '../test_helper'` (обе работают).

### Suggestions
9. Отметить, что `trimempty`/`with_entries(select(...))` в `lib/xrayconf.sh` и в builder-е `import_server_list.sh` должны быть идентичны по поведению.
10. Задокументировать осознанный пропуск `headerType`/`encryption` (tcp default header=none, encryption всегда none).
11. `ToVPNConfig`: оба call-site (`import.go:98-104`, `handler_servers.go:169-175`) — идентичные литералы; `ResolveIPs` вызывается до конвертации; import-cycle отсутствует.
12. Добавить assert на `streamSettings.network` в Go reality-тест; заменить синтетический URI реальной строкой подписки.

### Questions
13. Валидировать `network != tcp`? Сейчас ws/grpc даст `network:"ws"` без `wsSettings` — молча сломанный конфиг.
14. `spiderX`/`show` для reality — достаточно ли Xray-дефолтов (для текущей подписки да)?
15. Миграция: старый сгенерированный `config.json` (plain TLS) остаётся на диске до ре-импорта/ре-селекта → бот/TPROXY сломаны до ручного действия. Предупредить пользователя через Telegram/`/update`?

---

## codex (gpt-5.5 xhigh)

### Critical Issues
1. **План нарушает «generate tcp only».** Go (`buildOutbound`) и shell jq берут `network` из URI как есть; для `type=ws/grpc` будет неполный outbound без `wsSettings/grpcSettings`. Неизвестный `security` падает в legacy TLS. Fix: разрешить только `""|tcp` для network и `""|tls|reality` для security; остальное — ошибкой.
2. **`configure.sh` source-инструкция неверна.** Нет `SCRIPT_DIR` и нет sourcing `common.sh`; под `set -u` wizard упадёт. Использовать `. "$VPD_DIR/lib/xrayconf.sh"`.
3. **Пропущен второй Xray template:** `server/testdata/dev/xray.template.json:39` содержит те же placeholders, используется через `paths.DevPaths` (`paths.go:34`), `bot.go:96`, webui `main.go:106`. После перехода `xray.go` на `json.Unmarshal` dev-mode сломается. Его тоже сделать valid JSON с `outbounds: []`.
4. **Тесты `xrayconf.bats` не работают:** `setup()` source-ит функцию, но `run bash -c "... | xrayconf_build_outbound"` запускает новый bash без неё. Fix: `run xrayconf_build_outbound <<< "$server"` / `run xrayconf_generate "$tmpl" <<< "$server"`.

### Concerns
- REALITY без `fp` тихо даст invalid config (Go запишет пустой Fingerprint, shell `trimempty` удалит). По Project X docs `fingerprint` client-side required. Лучше default `chrome` или ошибка.
- Shell parser ломает URI без `#fragment`: текущий код берёт весь rest как имя. `vless://uuid@host:443?type=tcp&security=tls` → мусорное имя. План не тестирует; нужен `if [[ "$rest" == *#* ]]`.
- `_vless_query_get` не URL-decode-ит значения. Порядок/missing/trailing `&` ок, но `alpn=h2%2Chttp%2F1.1` разойдётся с Go `url.ParseQuery`.
- `publicKey` vs `password`: уточнить target Xray-core/Entware версию.
- Миграция «rerun `/import`» недостаточна, если бот недоступен через сломанный proxy — нужен явный SSH-путь (`import_server_list.sh` → `configure.sh`/WebUI).

### Suggestions
- Тесты на unsupported `network/security`, URI без fragment, URL-encoded query, valid dev template, endpoint-level import (Telegram/WebAPI).
- `ToVPNConfig` в `vless` не создаёт цикл (vpnconfig импортирует только stdlib); чище — converter helper вне `vpnconfig`.
- Делать именно «replace `outbounds[0]`», а не `.outbounds = [$ob]` (сейчас эквивалентно из-за пустого массива).
- jq feasible: локально jq-1.7, `with_entries`, `--argjson`, `.value != []` работают.

### Questions
- Минимальная версия `xray-core` на целевых роутерах Entware (влияет на publicKey/password)?
- Unsupported `type`/`security` отклонять при import или generation? (рекомендует — при generation.)

---

## alibaba/qwen (qwen3.7-plus)

### Critical Issues
- **C1.** `configure.sh` не имеет `SCRIPT_DIR` и не импортирует `lib/common.sh` — инструкция Task 7 невыполнима буквально; под `set -u` → `SCRIPT_DIR: unbound variable`. Fix: `. "$VPD_DIR/lib/xrayconf.sh"`.
- **C2.** Shell `_vless_query_get` не делает URL-decode — сломает REALITY `public_key` с `%2B` (Go `url.ParseQuery` декодирует). Реальная Go/shell-асимметрия. Предложен bash percent-decode helper (`%2B`→`+`, `%2F`→`/`, `%3D`→`=`, `%25` последним) + unit-тесты в обоих путях.

### Concerns
- **W1.** `ToVPNConfig` как метод на `vless.Server` создаёт зависимость парсера от `vpnconfig` (цикла нет, но запах). Предложение: `vpnconfig.FromVLESS(s)`.
- **W2.** `configure.sh` `[.[].ip]` — ключ называется `ips` (массив) → `xray.servers` всегда пусто → TPROXY bypass по IP сервера не работает (вероятный корень частичной маскировки). Plan out-of-scope, но фикс почти бесплатный.
- **W3.** bats `bash -c "... '$server' ..."` хрупок к апострофам/`$` в JSON; лучше через `env`.
- **W4.** `while ... done | jq -s` уходит в subshell (счётчики не видны после pipe) — зафиксировать в комментарии.
- **W5.** `trimempty` корректен для строк/массивов, но не универсален для `false`/`0`.

### Suggestions
S1 — bats-тест на URL-encoded `pbk`; S2 — не игнорировать err от `url.ParseQuery`; S3 — assert на ОТСУТСТВИЕ `tlsSettings` в reality-тесте (и наоборот); S4 — ранний warning при `reality` без `public_key`; S5 — migration-note «old config.json works as TLS until re-select»; S6 — типизированный slice вместо `map[string]interface{}`; S7 — `#name` fragment-логика корректна.

### Questions
Q1 — есть ли реальные подписки с `%2B` в `pbk`; Q2 — почему `ToVPNConfig` метод, а не функция; Q3 — нужен ли мок `resolve_ip` для literal IP; Q4 — консолидировать `SELECTED_SERVER_JSON`; Q5 — обновить `.claude/rules/xray-tproxy.md`; Q6 — валиден ли шаблон с пустым `outbounds:[]` для Xray; Q7 — нет ли ещё одного consumer'а `config.json.template`.

---

## ollama/kimi (kimi-k2.6:cloud)

### Critical Issues
1. **`configure.sh` нет `SCRIPT_DIR`** — план ссылается на несуществующую переменную; раскроется в пустую строку. Fix: добавить `SCRIPT_DIR=...` или использовать `$VPD_DIR`.
2. **`_vless_query_get` не декодирует URL-encoding** — расхождение с Go-путём; для `alpn`/нестандартного `sni` Go и shell запишут разное.
3. **`_vless_query_get` ложное совпадение по подстроке ключа** (`*"&$2="*`, ключ `s` ⊂ `sid`/`sni`/`security`). [ПРИМЕЧАНИЕ оркестратора: ложное срабатывание — trailing `=` делает матч точным; verified.]
4. **Пустой `realitySettings: {}`** при отсутствии всех reality-полей — Xray вряд ли примет. Fix: `| if . == {} then empty else . end`.

### Concerns
5. Нет default для `fingerprint` в REALITY (в некоторых версиях обязателен).
6. `_vless_query_get` обрезает значение на неэкранированном `&` (усиливает #2).
7. jq portability — не busybox-проблема: на роутере jq это Entware-бинарник (1.6+), не applet. Убрать упоминание busybox для jq.
8. Go `TestGenerateConfig_Reality` не проверяет `network` (default `tcp`).
9. Миграция: старый `config.json` с `{{...}}` остаётся после апгрейда.
10. `import_server_list.bats` уже содержит `?type=tcp&security=tls` — убедиться, что `cut -f5` вернёт `tls` (регрессия).
11. `ToVPNConfig` цикла не создаёт (verified).
12. `configure.sh` стр. 118/454 `.ip` вместо `.ips` — pre-existing; при правке вокруг той же функции починить заодно.

### Suggestions
A — точный split по `&` (цикл `while IFS='=' read -r k v`); B — минимальный busybox-safe URL-decode; C — guard от пустых `*Settings`; D — `network` в Go reality-тест; E — `.ip`→`.ips` в configure.sh; F — `jq -e '.inbounds | length == 2'` в `xray_template.bats`; G — переформулировать «busybox/jq» → «jq ≥ 1.4 (Entware)».

### Questions
Q1 — Xray default для `fingerprint`? Q2 — хранить `spiderX`/`show` на будущее? Q3 — параметры подписки, требующие URL-decode? Q4 — `xtls-rprx-vision-udp443`/валидация `flow`? Q5 — тест, что `xrayconf_generate` сохраняет `inbounds`/`routing`?

---

## zai/glm

❌ Failed — no usable review. Both attempts (1800s, then 3000s): competent codebase exploration (42 / 31+ tool calls) then a runaway extended-thinking loop (thinking tokens climbing past ~7850), zero `result` events, no `output.txt`. Model-side convergence failure on glm-5.1 for the broad combined review prompt; not a harness/auth issue.

---

## Failed reviewers

- **zai/glm:** no output. Both attempts (1800s, 3000s): competent exploration then runaway extended-thinking loop, zero `result` events. Model-side convergence failure on the broad prompt; retry with a narrower single-focus prompt if its input is wanted later.
- **deepseek/v4-pro:** no output. Both attempts: context-gathering succeeded, then stream truncated mid-thinking before final answer. Provider/endpoint stability issue on long reasoning task.
- **ollama/minimax:** no output. Ran ~6 min, climbed to ~4764 thinking tokens, never emitted final answer before timeout. Model stalled in extended thinking.
