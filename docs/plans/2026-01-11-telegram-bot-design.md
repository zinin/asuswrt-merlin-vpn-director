# Telegram Bot для VPN Director

Дизайн-документ для Telegram бота управления VPN Director на роутерах Asuswrt-Merlin.

## Общая архитектура

### Компоненты системы

```
┌─────────────────────────────────────────────────────────────┐
│                      Роутер Asus                            │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌─────────────────┐         ┌─────────────────────────┐   │
│  │  Telegram Bot   │────────▶│  vpn-director.json      │   │
│  │  (Go binary)    │         │  servers.json           │   │
│  └────────┬────────┘         │  xray/config.json       │   │
│           │                  └─────────────────────────┘   │
│           │ exec                                            │
│           ▼                                                 │
│  ┌─────────────────┐                                       │
│  │  Bash Scripts   │                                       │
│  │  xray_tproxy.sh │                                       │
│  │  ipset_builder  │                                       │
│  └─────────────────┘                                       │
│                                                             │
└─────────────────────────────────────────────────────────────┘
           ▲
           │ Telegram API (HTTPS)
           ▼
┌─────────────────┐
│  Telegram User  │
└─────────────────┘
```

### Разделение ответственности

| Слой | Ответственность |
|------|-----------------|
| **Go Bot** | Telegram API, парсинг VLESS, генерация JSON, UI wizard |
| **Bash Scripts** | iptables, ipset, запуск/остановка Xray, системные операции |
| **JSON конфиги** | Единый источник правды для обоих слоёв |

### Файловая структура на роутере

```
/jffs/scripts/vpn-director/
├── telegram-bot              # Go бинарник
├── telegram-bot.json         # Конфиг бота (token, users)
├── vpn-director.json         # Основной конфиг
├── data/
│   └── servers.json          # Импортированные серверы
├── xray_tproxy.sh
├── ipset_builder.sh
└── ...
```

## Структура Go-проекта

### Расположение в репозитории

```
telegram-bot/
├── cmd/
│   └── bot/
│       └── main.go           # Точка входа
├── internal/
│   ├── config/               # Загрузка telegram-bot.json
│   ├── bot/                  # Инициализация, polling, handlers
│   ├── wizard/               # Состояние wizard, шаги конфигурации
│   ├── vless/                # Парсинг VLESS URI
│   ├── vpnconfig/            # Работа с JSON конфигами
│   └── shell/                # Вызов bash-скриптов
├── go.mod
├── go.sum
└── Makefile
```

Конкретная структура файлов определится при реализации.

### Зависимости

```go
require (
    github.com/go-telegram-bot-api/telegram-bot-api/v5
)
```

Минимум зависимостей — только Telegram API. JSON парсинг через стандартную библиотеку.

### Сборка

```makefile
build-arm64:
	GOOS=linux GOARCH=arm64 go build -o bin/telegram-bot-arm64 ./cmd/bot

build-arm:
	GOOS=linux GOARCH=arm GOARM=7 go build -o bin/telegram-bot-arm ./cmd/bot
```

## Команды бота

| Команда | Описание | Реализация |
|---------|----------|------------|
| `/start` | Приветствие, список команд | Статичное сообщение |
| `/import <url>` | Импорт серверов | Go: fetch + parse VLESS + write servers.json |
| `/configure` | Wizard конфигурации | Go: inline-кнопки, генерация JSON |
| `/status` | Статус Xray | `xray_tproxy.sh status` |
| `/servers` | Список серверов | Go: читает servers.json |
| `/restart_xray` | Перезапуск Xray | `xray_tproxy.sh restart` |
| `/stop_xray` | Остановить Xray | `xray_tproxy.sh stop` |
| `/start_xray` | Запустить Xray | `xray_tproxy.sh start` |
| `/logs` | Последние строки лога | Go: читает /tmp/vpn_director.log |
| `/ip` | Внешний IP | `curl -s ifconfig.me` |

### Примеры ответов

**`/status`:**
```
Xray: ✓ работает
Сервер: nl-1.example.com:443
Клиенты: 192.168.1.100, 192.168.1.101
Исключения: ru, ua
```

**`/servers`:**
```
Серверы (3):
1. nl-1 — nl-1.example.com (1.2.3.4)
2. de-2 — de-2.example.com (5.6.7.8)
3. us-1 — us-1.example.com (9.10.11.12)
```

## Wizard конфигурации

### Шаги wizard `/configure`

```
Шаг 1: Выбор сервера
┌────────────────────────────────────────┐
│ Выберите Xray сервер:                  │
│                                        │
│ [nl-1 (1.2.3.4)]  [de-2 (5.6.7.8)]    │
│ [us-1 (9.10.11.12)]                    │
└────────────────────────────────────────┘

Шаг 2: Исключения (мультивыбор)
┌────────────────────────────────────────┐
│ Исключить из прокси:                   │
│                                        │
│ [✓ ru]  [✓ ua]  [  by]  [  kz]        │
│ [  de]  [  fr]  [  nl]  [  pl]        │
│                                        │
│ [Готово]                               │
└────────────────────────────────────────┘

Шаг 3: Добавление клиентов (цикл)
┌────────────────────────────────────────┐
│ Клиенты для Xray: (пока нет)           │
│                                        │
│ [Добавить клиента]  [Готово]           │
└────────────────────────────────────────┘

Шаг 3.1: Ввод IP клиента
┌────────────────────────────────────────┐
│ Введите IP адрес клиента:              │
│ (например: 192.168.1.100)              │
└────────────────────────────────────────┘
→ Пользователь вводит текстом

Шаг 4: Подтверждение
┌────────────────────────────────────────┐
│ Конфигурация:                          │
│ • Сервер: nl-1 (1.2.3.4)              │
│ • Исключения: ru, ua                   │
│ • Клиенты: 192.168.1.100              │
│                                        │
│ [Применить]  [Отмена]                  │
└────────────────────────────────────────┘

Шаг 5: Применение
┌────────────────────────────────────────┐
│ ⏳ Применяю конфигурацию...            │
│ ✓ vpn-director.json обновлён          │
│ ✓ xray/config.json обновлён           │
│ ✓ Xray перезапущен                    │
│                                        │
│ Готово!                                │
└────────────────────────────────────────┘
```

### Хранение состояния

```go
type WizardState struct {
    ChatID      int64
    Step        string
    Server      *Server
    Exclusions  []string
    Clients     []string
}

// В памяти: map[chatID]*WizardState
// При перезапуске бота состояние теряется
```

## Импорт серверов

### Команда `/import <url>`

**Сценарий:**
```
Пользователь: /import https://example.com/subscription
Бот: ⏳ Загружаю список серверов...
Бот: ✓ Импортировано 5 серверов:
     1. nl-1 — nl-1.example.com
     2. de-2 — de-2.example.com
     3. us-1 — us-1.example.com
     4. uk-1 — uk-1.example.com
     5. fr-1 — fr-1.example.com
```

### Логика (аналог import_server_list.sh)

1. Fetch URL → получить base64 строку
2. Decode base64 → список VLESS URI (по строкам)
3. Для каждого URI:
   - Парсинг: `vless://uuid@server:port?params#name`
   - Извлечение: uuid, server, port, name
   - Резолв IP: `net.LookupIP(server)`
   - Валидация: port числовой, uuid не пустой
4. Сохранить в servers.json

### Структура servers.json

```json
[
  {
    "address": "nl-1.example.com",
    "port": 443,
    "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "name": "nl-1",
    "ip": "1.2.3.4"
  }
]
```

### Обработка ошибок

| Ситуация | Ответ бота |
|----------|------------|
| URL недоступен | ❌ Не удалось загрузить: connection refused |
| Не base64 | ❌ Неверный формат: ожидается base64 |
| Нет валидных серверов | ❌ Не найдено ни одного VLESS сервера |
| Частичный успех | ⚠️ Импортировано 3 из 5 (2 с ошибками) |

## Скрипт setup_telegram_bot.sh

### Сценарий работы

```bash
$ ./setup_telegram_bot.sh

VPN Director Telegram Bot Setup
================================

Введите токен бота: 123456789:ABCdefGHI...
Введите username (без @): myusername
Добавить ещё? [y/N]: n

✓ Конфиг создан: telegram-bot.json
✓ Бот перезапущен

Готово! Напишите боту /start
```

### Поведение бота при старте

- Строка запуска бота всегда есть в services-start (добавляется install.sh)
- Бот при старте проверяет наличие telegram-bot.json
- Если конфига нет — тихо завершается
- Если конфиг есть — работает

```go
config, err := LoadConfig("telegram-bot.json")
if os.IsNotExist(err) {
    os.Exit(0)  // Конфига нет — тихо выходим
}
// Конфиг есть — работаем
```

## Авторизация

### Конфиг telegram-bot.json

```json
{
  "bot_token": "123456789:ABCdefGHI...",
  "allowed_users": ["username1", "username2"]
}
```

### Проверка доступа

```go
func (b *Bot) isAuthorized(username string) bool {
    for _, allowed := range b.config.AllowedUsers {
        if strings.EqualFold(allowed, username) {
            return true
        }
    }
    return false
}
```

Проверка выполняется для каждого сообщения и callback от inline-кнопок.

### Логирование

```
[INFO] Бот запущен
[INFO] Авторизован: myusername
[WARN] Отказано в доступе: unknown_user
[INFO] Команда /status от myusername
[ERROR] Ошибка выполнения xray_tproxy.sh: exit code 1
```

## Вызов bash-скриптов

```go
type ShellResult struct {
    Output   string
    ExitCode int
}

func Exec(command string, args ...string) (*ShellResult, error) {
    cmd := exec.Command(command, args...)
    output, err := cmd.CombinedOutput()

    exitCode := 0
    if exitErr, ok := err.(*exec.ExitError); ok {
        exitCode = exitErr.ExitCode()
    }

    return &ShellResult{
        Output:   string(output),
        ExitCode: exitCode,
    }, nil
}
```

### Использование

```go
const scriptsDir = "/jffs/scripts/vpn-director"

func XrayStatus() (string, error) {
    result, err := shell.Exec(scriptsDir + "/xray_tproxy.sh", "status")
    return result.Output, err
}

func XrayRestart() error {
    result, err := shell.Exec(scriptsDir + "/xray_tproxy.sh", "restart")
    if result.ExitCode != 0 {
        return fmt.Errorf("exit code %d: %s", result.ExitCode, result.Output)
    }
    return nil
}
```

## Работа с конфигами

### Чтение servers.json

```go
type Server struct {
    Address string `json:"address"`
    Port    int    `json:"port"`
    UUID    string `json:"uuid"`
    Name    string `json:"name"`
    IP      string `json:"ip"`
}

func LoadServers(dataDir string) ([]Server, error) {
    path := filepath.Join(dataDir, "servers.json")
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, err
    }
    var servers []Server
    return servers, json.Unmarshal(data, &servers)
}
```

### Обновление vpn-director.json

```go
func UpdateVPNDirectorConfig(configPath string,
    server Server, excludeSets []string, clients []string) error {

    data, _ := os.ReadFile(configPath)
    var config map[string]interface{}
    json.Unmarshal(data, &config)

    xray := config["xray"].(map[string]interface{})
    xray["clients"] = clients
    xray["exclude_sets"] = excludeSets
    xray["servers"] = []string{server.IP}

    output, _ := json.MarshalIndent(config, "", "  ")
    return os.WriteFile(configPath, output, 0644)
}
```

### Генерация xray/config.json

```go
func GenerateXrayConfig(templatePath, outputPath string, server Server) error {
    template, _ := os.ReadFile(templatePath)

    config := string(template)
    config = strings.ReplaceAll(config, "{{XRAY_SERVER_ADDRESS}}", server.Address)
    config = strings.ReplaceAll(config, "{{XRAY_SERVER_PORT}}", strconv.Itoa(server.Port))
    config = strings.ReplaceAll(config, "{{XRAY_SERVER_UUID}}", server.UUID)

    return os.WriteFile(outputPath, []byte(config), 0644)
}
```

## CI/CD

### GitHub Actions workflow

```yaml
# .github/workflows/telegram-bot.yml
name: Build Telegram Bot

on:
  push:
    tags:
      - 'v*'

jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: '1.21'

      - name: Build arm64
        run: |
          cd telegram-bot
          GOOS=linux GOARCH=arm64 go build -o ../bin/telegram-bot-arm64 ./cmd/bot

      - name: Build arm
        run: |
          cd telegram-bot
          GOOS=linux GOARCH=arm GOARM=7 go build -o ../bin/telegram-bot-arm ./cmd/bot

      - name: Upload to Release
        uses: softprops/action-gh-release@v1
        with:
          files: |
            bin/telegram-bot-arm64
            bin/telegram-bot-arm
```

### Скачивание в install.sh

```bash
ARCH=$(uname -m)
case "$ARCH" in
    aarch64) BOT_BINARY="telegram-bot-arm64" ;;
    armv7l)  BOT_BINARY="telegram-bot-arm" ;;
    *)       echo "Архитектура $ARCH не поддерживается"; exit 1 ;;
esac

RELEASE_URL="https://github.com/zinin/asuswrt-merlin-vpn-director/releases/latest/download"
curl -fsSL "$RELEASE_URL/$BOT_BINARY" -o "$JFFS_DIR/vpn-director/telegram-bot"
chmod +x "$JFFS_DIR/vpn-director/telegram-bot"
```

## Итоговая структура репозитория

```
asuswrt-merlin-vpn-director/
├── install.sh                          # Обновить: скачивает бота
├── setup_telegram_bot.sh               # Новый: создаёт конфиг бота
│
├── telegram-bot/                       # Новая папка
│   ├── cmd/bot/main.go
│   ├── internal/...
│   ├── go.mod
│   ├── go.sum
│   └── Makefile
│
├── .github/workflows/
│   └── telegram-bot.yml                # Новый: CI для сборки
│
├── jffs/scripts/vpn-director/
│   ├── xray_tproxy.sh
│   ├── ipset_builder.sh
│   ├── tunnel_director.sh
│   ├── configure.sh                    # Остаётся как CLI альтернатива
│   ├── import_server_list.sh           # Остаётся как CLI альтернатива
│   ├── vpn-director.json.template
│   └── utils/...
│
├── config/
│   └── xray.json.template
│
└── test/...
```

## Резюме

### Что будет создано

| Компонент | Описание |
|-----------|----------|
| `telegram-bot/` | Go-приложение для Telegram бота |
| `setup_telegram_bot.sh` | Скрипт создания конфига бота |
| `.github/workflows/telegram-bot.yml` | CI для сборки бинарников |
| Изменения в `install.sh` | Скачивание бота, строка в services-start |

### Принципы

- Go для данных, bash для применения
- Авторизация по username
- Бот опционален: без конфига — тихо выходит
- Минимум зависимостей
- CLI и бот работают параллельно, делают одно и то же

### Порядок реализации

1. Go-приложение (структура, команды, wizard)
2. setup_telegram_bot.sh
3. CI workflow
4. Изменения в install.sh
5. Тестирование на роутере
