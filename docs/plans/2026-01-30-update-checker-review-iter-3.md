# Review Iteration 3 — 2026-01-30 14:40

## Источник

- Design: `docs/plans/2026-01-30-update-checker-design.md`
- Plan: `docs/plans/2026-01-30-update-checker.md`
- Codex output: `~/.claude/codex-interaction/2026-01-30-14-34-22-design-review-update-checker-iter-3/output.txt`

## Замечания

### [MEDIUM-1] Возможна паника при частично повреждённом chats.json (значения null)

> Если `chats.json` содержит валидный JSON с `null` для отдельного пользователя (например `"john_doe": null`), `json.Unmarshal` создаст запись с `nil` значением. В `RecordInteraction`, `MarkNotified` и `SetInactive` планируется прямое разыменование `record`, что приведёт к panic.

**Статус:** Новое
**Ответ:** Удалять nil записи при load()
**Действие:** Добавлен цикл в `load()` после Unmarshal для удаления nil записей из map с логированием WARN.

---

### [LOW-2] В chatstore/store.go используется slog.Warn, но нет импорта

> В плане в `load()` вызывается `slog.Warn`, но в импортах отсутствует `log/slog`, что приведёт к ошибке компиляции.

**Статус:** Новое
**Ответ:** Добавить импорт
**Действие:** Добавлен `"log/slog"` в import block в Task 2.

---

### [LOW-3] Нет тестов для нового callback-роутинга update:*

> План добавляет обработку `update:run` в `router.go` и `handler/update.go`, но не добавляет тестов, покрывающих новый путь.

**Статус:** Новое
**Ответ:** Добавить тесты
**Действие:** Добавлены тесты `TestUpdateHandler_HandleCallback_UpdateRun` и `TestUpdateHandler_HandleCallback_IgnoresOtherCallbacks` в Task 8.

---

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/plans/2026-01-30-update-checker.md | Task 2: добавить import "log/slog" |
| docs/plans/2026-01-30-update-checker.md | Task 2: добавить удаление nil записей в load() |
| docs/plans/2026-01-30-update-checker.md | Task 8: добавить тесты для HandleCallback |

## Статистика

- Всего замечаний: 3
- Новых: 3
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
