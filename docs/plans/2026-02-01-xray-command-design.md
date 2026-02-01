# Команда `/xray` — Быстрое переключение Xray сервера

## Проблема

Сейчас для смены Xray сервера нужно пройти весь `/configure` wizard (4 шага), хотя требуется только переключить сервер без изменения клиентов и исключений.

## Решение

Новая команда `/xray` для быстрого переключения сервера в один клик.

## Поведение

1. Пользователь вводит `/xray`
2. Бот показывает все серверы кнопками в 2 колонки (inline keyboard)
3. Пользователь нажимает на сервер
4. Бот сразу:
   - Генерирует новый `/opt/etc/xray/config.json` из template
   - Выполняет `vpn-director.sh restart xray`
5. Бот отправляет "✓ Переключено на {название сервера}"

## UI

```
Выберите сервер:

[Netherlands, Amsterdam] [Germany, Frankfurt]
[USA, New York]          [Japan, Tokyo]
[UK, London]             [France, Paris]
...
```

- Все серверы на одном экране
- Кнопки в 2 колонки
- Без пагинации
- Без подтверждения — клик = применение

## Обработка ошибок

| Ситуация | Сообщение |
|----------|-----------|
| Нет серверов | "Серверы не найдены. Используйте /import для импорта" |
| Ошибка генерации конфига | "Ошибка: {текст}" |
| Ошибка перезапуска | "Ошибка перезапуска: {текст}" |

## Что меняется / не меняется

**Меняется**:
- `/opt/etc/xray/config.json` — адрес, порт, UUID нового сервера

**НЕ меняется**:
- `vpn-director.json` — clients, exclude_sets остаются прежними

## Реализация

### Новые файлы

**`internal/handler/xray.go`**:

```go
type XrayHandler struct {
    config ConfigStore
    xray   XrayGenerator
    vpn    VPNDirector
    bot    BotAPI
}

func NewXrayHandler(config ConfigStore, xray XrayGenerator, vpn VPNDirector, bot BotAPI) *XrayHandler

// HandleXray обрабатывает команду /xray
// — загружает servers.json
// — формирует inline keyboard в 2 колонки
// — отправляет "Выберите сервер:" с клавиатурой
func (h *XrayHandler) HandleXray(msg *tgbotapi.Message)

// HandleCallback обрабатывает выбор сервера
// — загружает servers.json
// — вызывает xray.GenerateConfig(servers[serverIndex])
// — вызывает vpn.RestartXray()
// — удаляет сообщение с клавиатурой
// — отправляет "✓ Переключено на {server.Name}"
func (h *XrayHandler) HandleCallback(callback *tgbotapi.CallbackQuery, serverIndex int)
```

### Изменения в существующих файлах

**`internal/bot/router.go`**:
- Добавить case `"xray"` в switch команд
- Добавить обработку callback с префиксом `xray:select:`

**`internal/bot/bot.go`**:
- Добавить `XrayHandler` в структуру `Bot`
- Инициализировать в конструкторе

### Callback формат

```
xray:select:{index}
```

Где `index` — позиция сервера в массиве `servers.json` (0-based).

### Зависимости

| Интерфейс | Методы | Источник |
|-----------|--------|----------|
| `ConfigStore` | `LoadServers()` | `service/config.go` |
| `XrayGenerator` | `GenerateConfig(server)` | `service/xray.go` |
| `VPNDirector` | `RestartXray()` | `service/vpndirector.go` |
| `BotAPI` | `Send()`, `Request()` | tgbotapi |

### Поведение после выбора

1. Удалить сообщение со списком серверов (или отредактировать)
2. Показать результат переключения

## Примеры сообщений

**Успех**:
```
✓ Переключено на Netherlands, Amsterdam
```

**Ошибка — нет серверов**:
```
Серверы не найдены. Используйте /import для импорта
```

**Ошибка — сбой перезапуска**:
```
Ошибка перезапуска: xray service not running
```
