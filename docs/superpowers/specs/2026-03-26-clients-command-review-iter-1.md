# Review Iteration 1 — 2026-03-26 23:30

## Источник

- Design: `docs/superpowers/specs/2026-03-26-clients-command-design.md`
- Plan: `docs/superpowers/plans/2026-03-26-clients-command.md`
- Review agents: codex-executor (gpt-5.4), gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
- Merged output: `docs/superpowers/specs/2026-03-26-clients-command-review-merged-iter-1.md`

## Замечания

### [NEW-1] Нормализация IP при проверке дубликатов и хранении

> IP format mismatch: xray stores "192.168.50.10", tunnel_director stores "192.168.50.10/32". Exact string matching in paused_clients and duplicate checks fails across formats.

**Источник:** ALL 7 agents (codex ARCH-1, gemini TYPE-1, glm ARCH-1, albb-glm ARCH-1, albb-qwen ARCHITECTURE-1, albb-kimi DATA-1, albb-minimax CRITICAL-2)
**Статус:** Новое
**Ответ:** Нормализовать при хранении. Всегда хранить IP без /32 suffix для single-host. Добавить normalizeIP() = TrimSuffix(ip, "/32"). Применять при добавлении и при сравнении.
**Действие:** Обновлены design spec (правила хранения, нормализация) и план (normalizeIP() функция, убрана логика добавления /32 для tunnel_director, обновлена проверка дубликатов через normalizeIP).

---

### [NEW-2] Missing return после ошибки apply

> В handlePauseResume и handleRemove после VPN.Apply() error отправляется сообщение об ошибке, но нет return — EditMessage выполняется вопреки спецификации "don't update keyboard".

**Источник:** gemini-executor (TYPE-4)
**Статус:** Новое
**Ответ:** Добавить return после отправки ошибки apply.
**Действие:** Добавлен `return` после Apply error во всех 3 методах (handlePauseResume, handleRemove, handleAddRoute) в плане. Обновлена спецификация — выделено "return immediately".

---

### [NEW-3] Фантомные записи в paused_clients при stale keyboard

> При нажатии Pause на старой клавиатуре после удаления клиента, IP добавится в paused_clients без проверки существования.

**Источник:** gemini-executor (TYPE-2)
**Статус:** Новое
**Ответ:** Проверять существование через CollectClients перед pause/resume. Если не найден — обновить список молча.
**Действие:** Добавлена проверка существования IP в handlePauseResume в плане. Обновлена спецификация — "verify IP exists via CollectClients".

---

### [NEW-4] Race condition в load-modify-save

> Concurrent load-modify-save can lose updates.

**Источник:** codex, glm, albb-glm, albb-qwen, albb-kimi, albb-minimax (6 из 7)
**Статус:** Новое
**Ответ:** Принять как known limitation. Бот обрабатывает события последовательно (одна горутина). Это существующий паттерн (ExcludeHandler, wizard apply).
**Действие:** Нет изменений в документах. Задокументировано как accepted limitation.

---

### [NEW-5] Тест на сохранение поля exclude при jq фильтрации

> to_entries/from_entries может потерять поля туннеля (exclude и другие).

**Источник:** codex SHELL-2, glm SHELL-2, albb-kimi SHELL-1, albb-minimax CRITICAL-1
**Статус:** Новое
**Ответ:** Добавить bats тест. Код корректный (map(.value.clients=...) меняет только .clients), но тест полезен для уверенности.
**Действие:** Добавлен тест "paused_clients filtering preserves tunnel exclude field" в план (Task 3).

## Отклонённые / false positive замечания

- **albb-minimax CRITICAL-1** (to_entries loses data): FALSE POSITIVE — `map(.value.clients = ...)` модифицирует только `.clients`, все остальные поля сохраняются.
- **albb-minimax HIGH-1** (DataDir omitempty): FALSE POSITIVE — `data_dir` намеренно обязательный.
- **codex ERR-2** (remove must clean paused_clients): Уже реализовано в плане — handleRemove вызывает removeString на PausedClients.
- **Callback 64-byte limit** (5 agents): Максимальная длина ~42 байта. IPv6 не поддерживается. Не проблема.
- **Apply failure inconsistency** (6 agents): Существующий паттерн. Config saved but apply failed — пользователь видит ошибку, может retry через /status.

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| `docs/superpowers/specs/2026-03-26-clients-command-design.md` | Нормализация IP (хранить без /32), обновлены правила, добавлена секция Normalization, уточнена error handling |
| `docs/superpowers/plans/2026-03-26-clients-command.md` | normalizeIP() функция, return после apply error (3 места), проверка существования IP в handlePauseResume, нормализованная проверка дубликатов, тест exclude preservation |

## Статистика

- Всего замечаний: ~65 (суммарно по всем агентам)
- Уникальных после дедупликации: 5
- Новых: 5
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
- Агенты: codex-executor (gpt-5.4), gemini-executor, ccs-executor (glm, albb-glm, albb-qwen, albb-kimi, albb-minimax)
