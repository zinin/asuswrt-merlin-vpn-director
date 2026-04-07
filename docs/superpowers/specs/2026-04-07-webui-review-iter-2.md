# Review Iteration 2 — 2026-04-07 17:00

## Источник

- Design: `docs/superpowers/specs/2026-04-07-webui-design.md`
- Plan: `docs/superpowers/plans/2026-04-07-webui-implementation.md`
- Review agents: codex-executor, gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
- Merged output: (inline — agents returned results directly)

## Замечания

### [SYNC-1] План не синхронизирован с дизайном после итерации 1

> Код в плане (router.go, handlers, api.ts, jwt.go) содержит устаревшие реализации — path params вместо query, hand-rolled JWT, stubs вместо полных реализаций, отсутствующие маршруты.

**Источник:** Alibaba GLM, Qwen, Gemini, Kimi, GLM (5/7)
**Статус:** Автоисправлено
**Ответ:** Добавлена секция "Design Review Sync" в заголовок плана с полным списком расхождений (19 пунктов). Код в плане — baseline, реализатор должен сверяться с design spec.
**Действие:** Plan updated — added sync note header

---

### [DESIGN-1] flock() описан как реализованный, но его нет

> Design spec утверждает "Implemented in shared service.ConfigService", но ConfigService использует bare os.WriteFile.

**Источник:** GLM (1/7)
**Статус:** Автоисправлено
**Ответ:** Исправлено — "must be added to shared ConfigService". Добавлен sync.Mutex для shell operations.
**Действие:** Design updated — concurrency section reworded

---

### [SEC-1] jwt_secret пустой → crash loop

> main.go делает os.Exit(1) при пустом jwt_secret, но installer зависит от jq для генерации. Нет jq → нет секрета → crash loop.

**Источник:** Gemini, Kimi (2/7)
**Статус:** Автоисправлено
**Ответ:** Go server auto-generates jwt_secret via crypto/rand if empty, saves to config. Убрана зависимость от jq.
**Действие:** Design updated — added auto-generation note in HTTPS section

---

### [SEC-2] TLS cert без SAN — браузеры не примут

> openssl генерирует сертификат только с CN, но Chrome/Firefox валидируют SAN.

**Источник:** Codex (1/7)
**Статус:** Автоисправлено
**Ответ:** Добавлен -addext "subjectAltName=IP:LAN_IP,DNS:hostname,IP:127.0.0.1"
**Действие:** Design updated — installer cert generation command

---

### [SEC-3] crypt() зависит от libc, поддерживает только $5$

> SHA-256 only не работает на роутерах с MD5 ($1$) или SHA-512 ($6$). cgo нужен для libc crypt.

**Источник:** Codex, GLM, Kimi (3/7)
**Статус:** Автоисправлено
**Ответ:** Pure Go crypt library, поддержка $1$/$5$/$6$ MCF formats. No cgo.
**Действие:** Design updated — shadow verification section

---

### [SEC-4] Import URL — SSRF risk

> POST /api/servers/import делает server-side fetch без ограничений. SSRF к loopback/LAN.

**Источник:** Codex (1/7)
**Статус:** Автоисправлено
**Ответ:** HTTPS only, deny private IPs, 10s timeout, 1MB limit.
**Действие:** Design updated — import endpoint description

---

### [ARCH-1] Operation serialization для shell commands

> apply, restart, stop могут пересечься при параллельных запросах из SPA и бота.

**Источник:** Codex (1/7)
**Статус:** Автоисправлено
**Ответ:** sync.Mutex в webui для всех mutating shell operations. Достаточно для single-user home router.
**Действие:** Design updated — concurrency section

---

### [PLAN-1] Go module rename недооценён

> Task 1 описывает git mv, но не упоминает обновление import paths во всех Go файлах.

**Источник:** GLM (1/7)
**Статус:** Автоисправлено
**Ответ:** Добавлен в sync note: "Mass-update import paths across all files"
**Действие:** Plan updated — sync note

---

### [MISC] Мелкие исправления через sync note

- removeString: exact == вместо EqualFold
- ClientInfo.paused: wire up paused_clients
- xray log source: добавить в logPaths
- PREARGS: убрать nohup, rc.func daemonizes
- Cookie MaxAge: align with JWT duration parameter

**Статус:** Все включены в sync note плана.

---

### [DISMISSED] Отклонённые

- **Error envelope format** (Codex, GLM) — Текущий `{"error": "message"}` достаточен для v1. Формализуем при добавлении mobile app.
- **Integration tests** (Codex) — Unit tests + manual smoke на роутере достаточно для v1.
- **IPv6 support** (Qwen) — YAGNI, роутеры в основном IPv4 LAN.
- **Config editing via Web UI** (Qwen) — Конфиг редактируется через отдельные endpoints (clients, excludes, servers), raw config view — read only. Это by design.
- **Go 1.25 doesn't exist** (Kimi) — Future project, will use latest stable at implementation time.
- **CertFile existence check** (Kimi) — TLS ListenAndServe will fail with clear error if files missing. Acceptable.

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/superpowers/specs/2026-04-07-webui-design.md | flock wording, jwt auto-gen, TLS SAN, pure Go crypt, SSRF protection, operation mutex |
| docs/superpowers/plans/2026-04-07-webui-implementation.md | 19-point sync note header |

## Статистика

- Всего замечаний (deduplicated): 12
- Автоисправлено: 9
- Обсуждено с пользователем: 0
- Отклонено: 6
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
- Агенты: codex-executor, gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
