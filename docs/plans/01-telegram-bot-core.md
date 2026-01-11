# Telegram Bot: Ядро приложения

## Контекст проекта

VPN Director — система управления VPN на роутерах Asus (Asuswrt-Merlin). Сейчас управление через bash-скрипты в консоли. Добавляем Telegram бота как альтернативный интерфейс.

**Архитектура:** Go для данных и UI, bash для применения (iptables, xray).

**Расположение:** `telegram-bot/` в корне репозитория.

## Цель этого модуля

Создать базовую структуру Go-приложения и реализовать простые команды, не требующие сложной логики.

## Что нужно реализовать

### Структура проекта

```
telegram-bot/
├── cmd/bot/main.go
├── internal/
│   ├── config/          # Загрузка telegram-bot.json
│   ├── bot/             # Telegram bot, handlers, auth
│   └── shell/           # Вызов bash-скриптов
├── go.mod
└── Makefile
```

### Конфиг telegram-bot.json

```json
{
  "bot_token": "123456789:ABCdefGHI...",
  "allowed_users": ["username1", "username2"]
}
```

Путь: `/jffs/scripts/vpn-director/telegram-bot.json`

### Поведение при старте

```go
// Если конфига нет — тихо выходим (бот опционален)
config, err := LoadConfig(configPath)
if os.IsNotExist(err) {
    os.Exit(0)
}
```

### Авторизация

Проверка username отправителя против `allowed_users` (case-insensitive).
Неавторизованным — ответ "⛔ Доступ запрещён".

### Команды для реализации

| Команда | Описание | Реализация |
|---------|----------|------------|
| `/start` | Приветствие, список команд | Статичное сообщение |
| `/status` | Статус Xray | `xray_tproxy.sh status` |
| `/start_xray` | Запустить Xray | `xray_tproxy.sh start` |
| `/stop_xray` | Остановить Xray | `xray_tproxy.sh stop` |
| `/restart_xray` | Перезапуск Xray | `xray_tproxy.sh restart` |
| `/logs` | Последние строки лога | Читать `/tmp/vpn_director.log` |
| `/ip` | Внешний IP | `curl -s ifconfig.me` |

### Вызов bash-скриптов

```go
const scriptsDir = "/jffs/scripts/vpn-director"

func Exec(command string, args ...string) (output string, exitCode int, err error)
```

### Примеры ответов

**`/start`:**
```
VPN Director Bot

Команды:
/status — статус Xray
/servers — список серверов
/import <url> — импорт серверов
/configure — настройка
/start_xray — запустить
/stop_xray — остановить
/restart_xray — перезапустить
/logs — логи
/ip — внешний IP
```

**`/status`:**
```
Xray: ✓ работает
Сервер: nl-1.example.com:443
Клиенты: 192.168.1.100
Исключения: ru, ua
```

**`/ip`:**
```
Внешний IP: 1.2.3.4
```

### Сборка

```makefile
build-arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/telegram-bot-arm ./cmd/bot
```

## Зависимости

```go
require github.com/go-telegram-bot-api/telegram-bot-api/v5
```

## Выходные артефакты

- Компилируемое Go-приложение
- Makefile для кросс-компиляции
- Работающие команды: `/start`, `/status`, `/start_xray`, `/stop_xray`, `/restart_xray`, `/logs`, `/ip`

## Зависимости от других модулей

- Нет зависимостей, это базовый модуль
