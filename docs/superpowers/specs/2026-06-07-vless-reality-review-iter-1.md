# Review Iteration 1 — 2026-06-07 13:04

## Источник

- Design: `docs/superpowers/specs/2026-06-07-vless-reality-design.md`
- Plan: `docs/superpowers/plans/2026-06-07-vless-reality-vision.md`
- Review agents: claude (self), codex (gpt-5.5 xhigh), alibaba/qwen, ollama/kimi (4 завершили); zai/glm, deepseek/v4-pro, ollama/minimax (3 не дали вывода)
- Merged output: `docs/superpowers/specs/2026-06-07-vless-reality-review-merged-iter-1.md`

## Замечания

### [AUTO-A] `configure.sh` не имеет `SCRIPT_DIR` (Task 7 source-инструкция невыполнима)

> codex C2 / qwen C1 / kimi C1: `. "$SCRIPT_DIR/lib/xrayconf.sh"` под `set -u` упадёт — нет ни `SCRIPT_DIR`, ни sourcing `common.sh`.

**Источник:** codex, qwen, kimi (3/4) · verified (`configure.sh:23` = `VPD_DIR`, нет `SCRIPT_DIR`)
**Статус:** Автоисправлено
**Ответ:** Task 7 Step 1 переписан: `. "$VPD_DIR/lib/xrayconf.sh"` после строки `VPD_DIR=...`; добавлено пояснение про self-contained xrayconf.sh.
**Действие:** план Task 7 Step 1.

---

### [AUTO-B] Пропущен второй dev-template `server/testdata/dev/xray.template.json`

> codex C3: содержит те же `{{...}}` placeholders, грузится в dev-режиме (`paths.go:34` → `bot.go:96`, `webui/main.go:106`); после перехода Go на `json.Unmarshal` dev-mode сломается на парсинге.

**Источник:** codex (1/4) · verified (файл существует, `{{XRAY_SERVER_PORT}}` невалиден; consumers подтверждены)
**Статус:** Автоисправлено
**Ответ:** Task 1 — добавлен Step 3b (конвертация dev-template в valid JSON с `outbounds:[]`), файл добавлен в Files и в `git add` Step 5; design Files Touched дополнен.
**Действие:** план Task 1, design §Files Touched.

---

### [AUTO-C] bats Task 5: `run bash -c "...|func"` не видит sourced-функцию

> codex C4 / qwen W3 / claude C2: child-shell не наследует функции из `setup()` → `command not found`.

**Источник:** codex, qwen, claude (3/4) · verified (семантика bash; claude ошибочно считал тесты проходящими)
**Статус:** Автоисправлено
**Ответ:** все вызовы заменены на `run xrayconf_build_outbound <<< "$server"` / `run xrayconf_generate "$tmpl" <<< "$server"`; добавлена поясняющая заметка; generate-тест усилен проверкой сохранения `inbounds`/`routing`.
**Действие:** план Task 5 Step 1.

---

### [AUTO-D] Расхождение формулировки `outbounds[0]` vs реальный `.outbounds = [$ob]`

> claude C1 / codex: дизайн/план говорят «replace outbounds[0]», Task 5 реализует `.outbounds = [$ob]`.

**Источник:** claude, codex (2/4)
**Статус:** Автоисправлено
**Ответ:** унифицировано на «replace the `outbounds` array wholesale» + `'.outbounds = [$ob]'` (симметрично Go, заменяющему весь массив) в design (таблица решений, §3) и в plan Architecture.
**Действие:** design таблица + §3; plan Architecture.

---

### [AUTO-E] Дизайн говорит «byte-identical», план — «semantically identical»

> claude C7 + session-context: живое противоречие в документе.

**Источник:** claude (1/4) + session-context
**Статус:** Автоисправлено
**Ответ:** в design заменено на «semantically identical (parsed structure, not byte order)» в таблице решений, §3 (legacy output) и §4.
**Действие:** design таблица, §3, §4.

---

### [AUTO-K] Усиление тестов

> claude 12 / kimi D,Q5 / qwen S3: нет assert на `network`; нет cross-проверки отсутствия `tlsSettings`/`realitySettings`; нет проверки сохранения `inbounds`/`routing`; reality-тесты на синтетическом URI без `headerType`.

**Источник:** claude, kimi, qwen (3/4)
**Статус:** Автоисправлено
**Ответ:** Task 4 — assert `network=="tcp"` + отсутствие `tlsSettings` в reality, отсутствие `realitySettings` в tls; Task 1 — assert `inbounds|length==2`; Task 5 — generate-тест проверяет сохранение `inbounds`/`routing`; Task 2/Task 6 — в reality-URI добавлен `headerType=none` (покрытие коллизии `type` vs `headerType`).
**Действие:** план Task 1, 2, 4, 6.

---

### [AUTO-L] Не задокументирован осознанный пропуск `headerType`/`encryption`

> claude 4/10 + kimi: `headerType=none` есть во всех 81 строке подписки, но не парсится; читатель сочтёт упущением.

**Источник:** claude, kimi (2/4) · verified по `decoded.txt`
**Статус:** Автоисправлено
**Ответ:** в design Non-Goals добавлена строка: `headerType` (tcp default none) и `encryption` (always none) намеренно не хранятся.
**Действие:** design §Non-Goals.

---

### [AUTO-M] Миграция: предупреждение + SSH-путь

> claude Q15 / codex / qwen S5 / kimi #9: старый `config.json` остаётся plain-TLS до ре-селекта → бот/Xray сломаны; «rerun /import» недостаточно, если бот недоступен.

**Источник:** claude, codex, qwen, kimi (4/4)
**Статус:** Автоисправлено
**Ответ:** усилены migration-notes в plan Task 8 (SSH-путь `import_server_list.sh` → `configure.sh`, предупреждение про сломанный канал) и в design §4 (⚠️ bullet).
**Действие:** план Task 8; design §4.

---

### [AUTO-N] URI без `#fragment` даёт мусорное имя

> codex: текущий `raw_name="${rest##*#}"` без `#` берёт весь rest; план не тестирует и не чинит.

**Источник:** codex (1/4) · verified (pre-existing; реальные URI всегда с `#name`)
**Статус:** Автоисправлено
**Ответ:** Task 6 Step 3 — добавлен guard `if [[ "$rest" == *#* ]]` вокруг извлечения имени (fallback на server hostname через существующую проверку).
**Действие:** план Task 6 Step 3.

---

### [DISPUTED-I] Починить `.ip` → `.ips` в `configure.sh`

> qwen W2 / kimi #12: `configure.sh:454` строит `xray.servers` через `.ip`, ключ — `ips` → bypass server-IP пустой ([null]); session-context пометил OUT OF SCOPE.

**Источник:** qwen, kimi (2/4) · verified (`:454` `[.[].ip] | unique`; `:118` display)
**Статус:** Обсуждено с пользователем
**Ответ:** Вариант A — включить фикс (пользователь подтвердил пересмотр out-of-scope).
**Действие:** план Task 7 — добавлен Step 4b (`:454` → `[.[].ips[]] | unique`, `:118` → `.ips|join`), смоук в Step 5, commit-msg обновлён, снята пометка out-of-scope в Self-Review.

---

### [DISPUTED-F] Shell `_vless_query_get` не делает URL-decode

> qwen C2 / codex / kimi C2: Go (`url.ParseQuery`) декодирует, shell — нет → потенциальная Go/shell-асимметрия.

**Источник:** qwen, codex, kimi (3/4) · verified (текущая подписка URL-safe; де-факто расхождения нет)
**Статус:** Обсуждено с пользователем
**Ответ:** Вариант A — документировать ограничение (реальные параметры URL-safe; YAGNI; decode добавить при появлении `%`-значений).
**Действие:** design §2 — добавлена заметка про отсутствие URL-decode и условие для перехода к `_url_decode`.

---

### [DISPUTED-G] Неизвестный `network`/`security` молча генерит сломанный конфиг

> codex C1 / claude Q13 / kimi Q4: `network=ws` → нет `wsSettings`; неизвестный `security` → молча legacy TLS.

**Источник:** codex, claude, kimi (3/4)
**Статус:** Обсуждено с пользователем
**Ответ:** Вариант A — fail-fast валидация (отвергать заведомо-битое, не добавляя транспорты; усиливает YAGNI).
**Действие:** план Task 4 (Go `validateStreamParams` + 2 теста), Task 5 (shell-проверка в `xrayconf_build_outbound` + 2 bats-теста); design §3 — заметка про fail-fast.

---

### [DISMISSED] Отклонённые замечания

| Замечание | Источник | Причина отклонения |
|---|---|---|
| `_vless_query_get` substring-коллизия ключей (`s` ⊂ `sid`) | kimi C3 | Ложное: trailing `=` в `&$2=` делает матч точным (`&s=` не матчит `&sid=`); verified claude+codex |
| Default `fingerprint` для reality | codex / kimi #5 / qwen S4 | Подписка всегда шлёт `fp=firefox`; design = emit-when-present; дефолт замаскировал бы битую запись (подключение с неверным fp хуже явного отказа) |
| `ToVPNConfig` метод vs `vpnconfig.FromVLESS` функция | qwen W1/Q2 / codex | Стиль; import-cycle отсутствует (verified); выбор плана осознан и эквивалентен |
| `publicKey` vs `password` / версия xray-core | codex / kimi Q1 | `publicKey` — корректное поле realitySettings для текущего Xray-core |
| `trimempty` неуниверсален для `false`/`0` | qwen W5 | Через `trimempty` идут только строковые/массивные поля; `port` — top-level, всегда присутствует |
| `while ... done | jq -s` subshell (счётчики) | qwen W4 | Новый builder не зависит от post-loop счётчиков; count читается из файла отдельным jq |
| Пустой `realitySettings: {}` | kimi #4 | Не возникает на реальных данных (reality всегда с pbk/sid/sni); тот же класс, что fp-default |
| «busybox/jq portability» формулировка | kimi 7/G | Документы не утверждают busybox-jq; jq на роутере — Entware-бинарник |
| Дублирование `trimempty` в двух файлах | claude 9 | Неотъемлемо для two-language sync; поведение идентично (verified) |

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| `…/specs/2026-06-07-vless-reality-design.md` | D (outbounds wording), E (byte→semantically ×3), L (Non-Goals headerType/encryption), B (Files Touched dev-template), M (§4 ⚠️ migration), F (§2 URL-decode note), G (§3 fail-fast note) |
| `…/plans/2026-06-07-vless-reality-vision.md` | A (Task 7 $VPD_DIR), B (Task 1 dev-template Step 3b + Files + commit), C (Task 5 herestring + note), D (Architecture wording), K (Task 1/2/4/6 test asserts + headerType), M (Task 8 migration), N (Task 6 fragment guard), I (Task 7 Step 4b .ip→.ips + smoke + Self-Review note), G (Task 4 Go validate + tests, Task 5 shell validate + tests) |
| `…/specs/2026-06-07-vless-reality-review-merged-iter-1.md` | merged review (4 обзора + 3 провала) |

## Статистика

- Всего уникальных замечаний: 21
- Автоисправлено (без обсуждения): 9 (A, B, C, D, E, K, L, M, N)
- Авто-применено после анализа: 0
- Обсуждено с пользователем: 3 (I, F, G — все Вариант A)
- Отклонено: 9
- Повторов (автоответ): 0 (итерация 1)
- Пользователь сказал «стоп»: Нет
- Агенты: claude, codex, alibaba/qwen, ollama/kimi (завершили); zai/glm, deepseek/v4-pro, ollama/minimax (не дали вывода)
