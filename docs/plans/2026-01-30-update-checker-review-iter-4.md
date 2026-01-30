# Review Iteration 4 — 2026-01-30 15:05

## Источник

- Design: `docs/plans/2026-01-30-update-checker-design.md`
- Plan: `docs/plans/2026-01-30-update-checker.md`
- Codex output: `~/.claude/codex-interaction/2026-01-30-14-51-50-design-review-update-checker-iter-4/output.txt`

## Замечания

### [MEDIUM-1] Task 4: тесты используют Send(), но позже интерфейс меняется на SendWithKeyboard

> Тест использует `mockSender.Send()`, а в Task 7 интерфейс меняется на `SendWithKeyboard`. Тесты не скомпилируются при последовательном выполнении задач.

**Статус:** Новое
**Ответ:** Сразу использовать SendWithKeyboard в Task 4
**Действие:** Обновлён Task 4: интерфейс Sender теперь использует SendWithKeyboard, mockSender реализует SendWithKeyboard, добавлен импорт tgbotapi. formatNotification сразу возвращает (string, keyboard).

---

### [MEDIUM-2] Task 8: тесты используют неверную сигнатуру NewUpdateHandler

> Пример использует `NewUpdateHandler(sender, executor, updaterSvc, version)`, но фактическая сигнатура — `NewUpdateHandler(sender, upd, devMode, version)`.

**Статус:** Новое
**Ответ:** Переписать на существующие моки
**Действие:** Исправлены тесты в Task 8: используется корректная сигнатура `NewUpdateHandler(sender, upd, false, version)` и существующие моки `mockUpdater`, `mockUpdateSender`.

---

### [MEDIUM-3] ChatStore не нормализует username

> Auth использует lowercase, ChatStore хранит как есть. Возможны дубликаты при разном регистре.

**Статус:** Новое
**Ответ:** Нормализовать в lowercase
**Действие:** Добавлен `strings.ToLower(username)` во все методы ChatStore: RecordInteraction, MarkNotified, IsNotified, SetInactive. Добавлен импорт "strings".

---

### [LOW-4] Дизайн использует несуществующие API: CompareVersions, paths.DataDir

> В дизайне указан `updater.CompareVersions()` (должен быть `ShouldUpdate`) и `paths.DataDir` (должен быть `p.DefaultDataDir`).

**Статус:** Новое
**Ответ:** Исправить дизайн
**Действие:** Обновлён дизайн-документ: `CompareVersions()` → `ShouldUpdate()`, `paths.DataDir` → `p.DefaultDataDir`.

---

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/plans/2026-01-30-update-checker.md | Task 4: Sender interface → SendWithKeyboard, mockSender обновлён |
| docs/plans/2026-01-30-update-checker.md | Task 4: formatNotification сразу с keyboard и telegram.EscapeMarkdownV2 |
| docs/plans/2026-01-30-update-checker.md | Task 2: добавлен import "strings", нормализация username |
| docs/plans/2026-01-30-update-checker.md | Task 8: исправлена сигнатура NewUpdateHandler в тестах |
| docs/plans/2026-01-30-update-checker-design.md | CompareVersions → ShouldUpdate, paths.DataDir → p.DefaultDataDir |

## Статистика

- Всего замечаний: 4
- Новых: 4
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
