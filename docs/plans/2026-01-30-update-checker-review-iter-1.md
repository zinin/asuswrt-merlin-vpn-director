# Review Iteration 1 — 2026-01-30 14:15

## Источник

- Design: `docs/plans/2026-01-30-update-checker-design.md`
- Plan: `docs/plans/2026-01-30-update-checker.md`
- Codex output: `~/.claude/codex-interaction/2026-01-30-14-06-55-design-review-update-checker-iter-1/output.txt`

## Замечания

### [HIGH-1] План Task 1 предлагает заменить весь config.go, риск потери полей

> В шаге 3 Task 1 предлагается полностью заменить `config.go` минимальным struct'ом. Если в текущем проекте уже есть дополнительные поля/валидации, то это нарушит обратную совместимость.

**Статус:** Новое
**Ответ:** Замечание не применимо. Новый конфиг содержит все поля старого (`BotToken`, `AllowedUsers`, `LogLevel`) плюс новое `UpdateCheckInterval`. Структура расширяется, а не заменяется.
**Действие:** Нет изменений (ложное срабатывание)

---

### [HIGH-2] telegram.MessageSender не содержит SendWithKeyboard

> В Task 7 `updatechecker.Sender` требует `SendWithKeyboard`, а в Task 6 передаётся `b.Sender()` (тип `telegram.MessageSender`). Если интерфейс не содержит этот метод, сборка не пройдёт.

**Статус:** Новое
**Ответ:** Расширить MessageSender
**Действие:** Проверил — `telegram.MessageSender` УЖЕ содержит `SendWithKeyboard`. Обновил Task 7 для использования существующего метода.

---

### [MEDIUM-3] Несоответствие callback ID: update:confirm в дизайне vs update:start в плане

> Дизайн говорит про reuse `update:confirm`, а план внедряет новый `update:start`.

**Статус:** Новое
**Ответ:** Использовать существующий механизм
**Действие:** Изменил callback ID на `update:run`, обновил Task 7 и Task 8. Дизайн исправлен — убрана привязка к конкретному callback ID.

---

### [MEDIUM-4] MarkdownV2 escaping без явного parse mode

> В `formatNotification` добавлено экранирование MarkdownV2, но план не гарантирует ParseMode.

**Статус:** Новое
**Ответ:** Явно выставить ParseMode
**Действие:** `telegram.Sender.SendWithKeyboard` уже устанавливает `ParseMode: MarkdownV2`. Добавил комментарий в план.

---

### [LOW-5] Обрезка changelog по байтам может ломать UTF-8

> `changelog[:maxChangelogLength]` режет строку по байтам. Для UTF-8 это может обрезать посреди руны.

**Статус:** Новое
**Ответ:** Обрезать по рунам
**Действие:** Обновил код в Task 7 — используется `[]rune` для корректной обрезки.

---

### [LOW-6] Дизайн задаёт интерфейс ChatStore, план использует *chatstore.Store

> Дизайн предусматривает интерфейс, но план привязывает к конкретной реализации.

**Статус:** Новое
**Ответ:** Использовать интерфейс
**Действие:** Обновил Task 4 — `updatechecker.Checker` использует интерфейс `ChatStore`, а не конкретный `*chatstore.Store`.

---

## Изменения в документах

| Файл | Изменение |
|------|-----------|
| docs/plans/2026-01-30-update-checker-design.md | Убрана привязка к конкретному callback ID |
| docs/plans/2026-01-30-update-checker.md | Task 4: ChatStore интерфейс вместо конкретного типа |
| docs/plans/2026-01-30-update-checker.md | Task 7: UTF-8 safe обрезка, callback update:run |
| docs/plans/2026-01-30-update-checker.md | Task 8: UpdateRouterHandler.HandleCallback |

## Статистика

- Всего замечаний: 6
- Новых: 6
- Повторов (автоответ): 0
- Пользователь сказал "стоп": Нет
