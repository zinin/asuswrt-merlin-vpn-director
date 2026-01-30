# Review Iteration 2 — 2026-01-30 14:30

## Источник

- Design: `docs/plans/2026-01-30-update-checker-design.md`
- Plan: `docs/plans/2026-01-30-update-checker.md`
- Codex output: `~/.claude/codex-interaction/2026-01-30-14-22-24-design-review-update-checker-iter-2/output.txt`

## Замечания

### [MEDIUM-1] Incomplete MarkdownV2 escaping in notifications

> The plan proposes a custom `escape()` function for MarkdownV2, but it does not escape the backslash `\`. If `release.Body` contains backslashes (paths, commands, code), Telegram may return a parsing error and notifications will not be sent. This also duplicates the existing helper `telegram.EscapeMarkdownV2` which has the correct character list.

**Статус:** Новое
**Ответ:** Использовать существующий telegram.EscapeMarkdownV2
**Действие:** Убрана локальная функция `escape()` из Task 7, добавлен импорт `internal/telegram` и использование `telegram.EscapeMarkdownV2()` для всех строк в formatNotification.

---

### [MEDIUM-2] Risk of panic due to `null` in chats.json

> In the planned `chatstore.load()`, JSON errors are ignored and there is no check for nil map. If `chats.json` contains valid JSON `null` (or the file is partially corrupted), `json.Unmarshal` will set `s.users == nil`, and then `RecordInteraction` will panic when writing to the map.

**Статус:** Новое
**Ответ:** Добавить проверку nil + логирование, невалидный конфиг считать пустым конфигом
**Действие:** Обновлён метод `load()` в Task 2: добавлена обработка ошибки json.Unmarshal с логированием WARN, добавлена проверка `if s.users == nil` после unmarshal.

---

### [LOW-3] No test for user blocking scenario

> The UpdateChecker tests lack verification of the `isBlockedError`/`SetInactive` branch, even though this is key logic for deactivating users.

**Статус:** Новое
**Ответ:** Добавить тест
**Действие:** Добавлен тест `TestChecker_SetsInactiveOnBlock` в Task 4 с mock sender, возвращающим "Forbidden: bot was blocked by the user".

---

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/plans/2026-01-30-update-checker.md | Task 7: использовать telegram.EscapeMarkdownV2 вместо локального escape() |
| docs/plans/2026-01-30-update-checker.md | Task 2: добавить проверку nil и логирование в load() |
| docs/plans/2026-01-30-update-checker.md | Task 4: добавить TestChecker_SetsInactiveOnBlock |

## Статистика

- Всего замечаний: 3
- Новых: 3
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
