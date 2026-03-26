# Review Iteration 1 — 2026-03-26 20:15

## Источник

- Design: `docs/superpowers/specs/2026-03-26-exclude-ips-and-multi-resolve-design.md`
- Plan: `docs/superpowers/plans/2026-03-26-exclude-ips-and-multi-resolve.md`
- Review agents: codex-executor (gpt-5.4), gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
- Merged output: `docs/superpowers/specs/2026-03-26-exclude-ips-and-multi-resolve-review-merged-iter-1.md`

## Замечания

### [ARCH-1] Обратная совместимость ip→ips в servers.json

> Все 7 моделей: удаление поля `ip` без миграции сломает существующие установки.

**Источник:** codex, gemini, glm, albb-glm, albb-qwen, albb-kimi, albb-minimax
**Статус:** Новое
**Ответ:** Оставить как есть. Пользователь делает /import заново.
**Действие:** Нет изменений.

---

### [EDGE-1] DNS недоступен при apply (загрузка роутера)

> 6/7 моделей: при загрузке DNS может быть недоступен, OpenVPN endpoints не попадут в ipset → петля.

**Источник:** codex, gemini, glm, albb-glm, albb-kimi, albb-minimax
**Статус:** Новое
**Ответ:** Не проблема — Entware стартует после WAN, DNS уже доступен.
**Действие:** Нет изменений.

---

### [SEC-1] Shell-валидация exclude_ips

> 6/7 моделей: бот валидирует, но при ручном редактировании JSON невалидные CIDR попадут в ipset.

**Источник:** codex, glm, albb-glm, albb-kimi, albb-minimax, albb-qwen
**Статус:** Новое
**Ответ:** Добавить валидацию в shell.
**Действие:** Обновлён дизайн: добавлена shell-side validation, обновлён план.

---

### [NAME-1] Переименование XRAY_SERVERS ipset

> 5/7 моделей: название не отражает содержимое после добавления exclude_ips и OpenVPN endpoints.

**Источник:** codex, gemini, glm, albb-qwen, albb-minimax
**Статус:** Новое
**Ответ:** Переименовать в TPROXY_BYPASS. Поле конфига: servers_ipset → bypass_ipset.
**Действие:** Обновлены дизайн и план: XRAY_SERVERS → TPROXY_BYPASS, _tproxy_setup_servers_ipset → _tproxy_setup_bypass_ipset, advanced.xray.servers_ipset → advanced.xray.bypass_ipset.

---

### [ARCH-2] Два источника истины (servers.json vs xray.servers)

> Codex: при /import обновляется servers.json, но xray.servers в конфиге не обновляется до /configure.

**Источник:** codex
**Статус:** Новое
**Ответ:** Авто-sync xray.servers при /import.
**Действие:** Обновлён дизайн: добавлена секция /import auto-sync.

---

### [EDGE-3] OpenVPN endpoint DDNS обновляется только при apply

> 3/7 моделей: endpoint может смениться между apply.

**Источник:** codex, albb-kimi, albb-glm
**Статус:** Новое
**Ответ:** Не нужно. Обновление при apply достаточно.
**Действие:** Нет изменений.

---

### [MISS-2] Lockfile для concurrent apply

> 2/7 моделей: нет блокировки при одновременных операциях.

**Источник:** codex, albb-minimax
**Статус:** Новое
**Ответ:** Не нужно. Один пользователь, один бот, race маловероятен.
**Действие:** Нет изменений.

---

### Информационные (не требуют решения)

Следующие замечания отмечены, но не требуют изменений:
- **exclude_ips vs exclude_sets naming** (5/7) — принято, разные суффиксы отражают разную семантику
- **Duplicate IP counting in logs** (4/7) — ipset дедуплицирует, лог показывает attempts
- **ipset size/RAM** (4/7) — TPROXY_BYPASS обычно <100 entries, не проблема
- **Rollback/atomic swap** (4/7) — over-engineering для данного scope
- **IPv6 not supported** (3/7) — IPv4 only, документировано
- **WireGuard support** (2/7) — пользователь сказал "пока не надо"
- **DNS timeout** (6/7) — Entware стартует после WAN

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| design.md | XRAY_SERVERS → TPROXY_BYPASS, servers_ipset → bypass_ipset, добавлена shell validation, добавлен /import auto-sync, обновлены тесты |
| plan.md | XRAY_SERVERS → TPROXY_BYPASS, _tproxy_setup_servers_ipset → _tproxy_setup_bypass_ipset, обновлена архитектура |

## Статистика

- Всего замечаний: ~86 (с дубликатами между моделями)
- Уникальных тем: 14
- Новых: 14
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
- Агенты: codex-executor, gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
