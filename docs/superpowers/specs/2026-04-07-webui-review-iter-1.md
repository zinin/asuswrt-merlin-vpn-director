# Review Iteration 1 — 2026-04-07 16:30

## Источник

- Design: `docs/superpowers/specs/2026-04-07-webui-design.md`
- Plan: `docs/superpowers/plans/2026-04-07-webui-implementation.md`
- Review agents: codex-executor, gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
- Merged output: `docs/superpowers/specs/2026-04-07-webui-review-merged-iter-1.md`

## Замечания

### [SEC-1] GET /api/config выдаёт jwt_secret

> Эндпоинт /api/config возвращает полный конфиг, включая jwt_secret. Утечка секрета позволяет подделывать JWT токены.

**Источник:** Codex, GLM, Alibaba GLM, Alibaba Qwen, Kimi, MiniMax (6/7)
**Статус:** Автоисправлено
**Ответ:** Redact sensitive fields (jwt_secret) from /api/config response
**Действие:** Design updated — endpoint description now says "redacted: no jwt_secret, no sensitive fields"

---

### [SEC-2] Нет rate limiting на /api/login

> Brute-force атаки на login endpoint без ограничений. На роутере с ограниченными ресурсами crypt() вызовы особенно дороги.

**Источник:** Codex, GLM, Alibaba GLM, Qwen, Kimi, MiniMax (6/7)
**Статус:** Автоисправлено
**Ответ:** In-memory counter per IP, 5 attempts/min, 30s lockout
**Действие:** Design updated — added rate limiting section to Authentication

---

### [SEC-3] Hand-rolled JWT реализация

> Собственная реализация JWT — anti-pattern. Потенциальные edge cases, нет стандартных claims.

**Источник:** Codex, Alibaba GLM (2/7)
**Статус:** Обсуждено с пользователем
**Ответ:** Использовать golang-jwt/jwt/v5
**Действие:** Design updated — JWT via golang-jwt/jwt library. Plan updated — Task 4 uses library instead of hand-rolled.

---

### [SEC-4] JWT secret в plaintext в конфиге

> jwt_secret хранится в vpn-director.json без защиты.

**Источник:** Codex, GLM, Alibaba GLM, Qwen, Kimi, MiniMax (6/7)
**Статус:** Обсуждено с пользователем
**Ответ:** Оставить в конфиге — приемлемо для домашнего роутера. jwt_secret уже redacted из /api/config ответа.
**Действие:** Без изменений (решение пользователя)

---

### [API-1] CIDR в DELETE URL path ломает маршрутизацию

> DELETE /api/clients/192.168.50.0/24 — слеш в CIDR ломает URL routing.

**Источник:** Codex, GLM, Kimi (3/7)
**Статус:** Автоисправлено
**Ответ:** Перенос IP/CIDR в query parameter: DELETE /api/clients?ip=...
**Действие:** Design updated — clients и excludes/ips DELETE endpoints используют query parameter

---

### [API-2] Отсутствуют POST /api/apply и POST /api/ipsets/update

> Кнопки Apply и Update IPsets на Status page, но нет REST endpoints.

**Источник:** Gemini, GLM, Alibaba GLM, Kimi (4/7)
**Статус:** Автоисправлено
**Ответ:** Добавлены POST /api/apply и POST /api/ipsets/update
**Действие:** Design updated — added to Status & Control table

---

### [API-3] Отсутствуют pause/resume endpoints для клиентов

> UI показывает Paused/Active статус, но нет API для управления.

**Источник:** Gemini, Kimi (2/7)
**Статус:** Автоисправлено
**Ответ:** Добавлены POST /api/clients/pause?ip=... и /resume
**Действие:** Design updated — added to Clients table

---

### [ARCH-1] Race condition при concurrent config modification

> Telegram bot и WebUI могут одновременно писать vpn-director.json.

**Источник:** Codex, GLM, Alibaba GLM, Kimi (4/7)
**Статус:** Автоисправлено
**Ответ:** File locking via flock() на /tmp/vpn-director.lock в shared ConfigService
**Действие:** Design updated — added Concurrency section

---

### [ARCH-2] SPA fallback не возвращает index.html

> Прямой доступ по URL вернёт 404 вместо index.html.

**Источник:** GLM (1/7)
**Статус:** Автоисправлено
**Ответ:** Proper SPA fallback: non-/api/ requests → index.html
**Действие:** Design updated — added SPA fallback note

---

### [COMP-1] Import и Update — заглушки

> handleImportServers возвращает 501, handleUpdate отсутствует. Заявлена полная паритетность с ботом.

**Источник:** Codex, GLM, MiniMax, Qwen (4/7)
**Статус:** Автоисправлено
**Ответ:** Помечены как полноценные задачи, не stubs
**Действие:** Plan updated — "Known gaps" переформулированы как обязательные к реализации

---

### [ARCH-3] Business logic в handlers вместо service layer

> Логика добавления/удаления клиентов и туннелей в HTTP handlers, а не в shared service.

**Источник:** Codex (1/7)
**Статус:** Отклонено
**Ответ:** Валидное замечание, но рефакторинг service layer выходит за scope этой задачи. При реализации handlers будут использовать существующие service methods. Если потребуется — вынесем в service layer по ходу.
**Действие:** Без изменений

---

### [SEC-5] CSRF protection

> Cookie-based auth с mutation endpoints без CSRF-защиты.

**Источник:** Codex (1/7)
**Статус:** Отклонено
**Ответ:** SameSite=Strict уже в дизайне — это эффективная CSRF-защита для modern browsers. Дополнительно Origin header check в middleware. Достаточно для домашнего роутера.
**Действие:** Без изменений (SameSite=Strict already specified)

---

### [SEC-6] WAN access по умолчанию

> Сервер на 0.0.0.0 доступен из WAN с self-signed cert.

**Источник:** Codex, Alibaba GLM (2/7)
**Статус:** Отклонено
**Ответ:** Пользователь явно запросил WAN-доступ. Iptables по умолчанию блокирует входящие на WAN, пользователь сам решает открывать ли порт.
**Действие:** Без изменений (решение пользователя)

---

### [PERF-1] Двойное потребление RAM (два Go-процесса)

> Два Go-бинарника на 256-512MB роутере.

**Источник:** Gemini, Qwen (2/7)
**Статус:** Отклонено
**Ответ:** Go runtime ~10-15MB на процесс. На 256MB роутере это ~5-6% RAM — приемлемо. Объединение в один процесс усложняет архитектуру без значимой выгоды.
**Действие:** Без изменений

---

### [PERF-2] Status возвращает raw text вместо structured JSON

> Frontend должен парсить текстовый вывод vpn-director.sh status.

**Источник:** Gemini, GLM (2/7)
**Статус:** Отклонено
**Ответ:** В v1 достаточно raw text. Структурированный status потребует изменений в shell-скриптах (отдельная задача). Фронтенд может отображать preformatted text.
**Действие:** Без изменений (отложено на v2)

---

### [MISC] Отклонённые Minor/Info замечания

Следующие замечания отклонены как YAGNI, out of scope, или неприменимые:

- **API versioning** (/api/v1/) — YAGNI для v1, один клиент
- **Vue component tests** — out of scope для v1
- **Code splitting** — single bundle ~100KB, роутер справится
- **Certificate lifetime 10y** — нормально для self-signed на домашнем роутере
- **Health check endpoint** — nice to have, не блокирует
- **Deep linking / URL hash** — nice to have
- **API pagination** — маленькие наборы данных
- **MIPS architecture** — out of scope, только ARM
- **Go 1.25 version** — будущий проект, корректно
- **Shadow hash format** — runtime detection при реализации
- **Browser caching** — стандартные Cache-Control headers
- **Installer partial failure** — уже обрабатывается (return 0 при отсутствии webui)
- **Tunnel deletion loses excludes** — будет учтено при реализации handleDeleteClient

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/superpowers/specs/2026-04-07-webui-design.md | Redacted /api/config, rate limiting, JWT library, CIDR in query params, apply/ipsets/pause endpoints, file locking, SPA fallback |
| docs/superpowers/plans/2026-04-07-webui-implementation.md | JWT library instead of hand-rolled, import/update fully implemented |

## Статистика

- Всего замечаний (deduplicated): 16
- Автоисправлено: 9
- Обсуждено с пользователем: 2
- Отклонено: 5 (+ ~13 minor/info batch)
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
- Агенты: codex-executor, gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
