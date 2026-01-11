# Telegram Bot: Импорт серверов

## Контекст проекта

VPN Director — система управления VPN на роутерах Asus. Telegram бот — альтернативный интерфейс к bash-скриптам.

Этот модуль реализует импорт VLESS серверов — аналог `import_server_list.sh`, но на Go.

## Цель этого модуля

Реализовать команду `/import <url>` и команду `/servers` для просмотра списка.

## Что нужно реализовать

### Команда `/import <url>`

**Сценарий:**
```
Пользователь: /import https://example.com/subscription
Бот: ⏳ Загружаю список серверов...
Бот: ✓ Импортировано 5 серверов:
     1. nl-1 — nl-1.example.com
     2. de-2 — de-2.example.com
     3. us-1 — us-1.example.com
```

### Логика импорта

1. **Fetch URL** — HTTP GET, получить тело ответа
2. **Decode base64** — тело закодировано в base64
3. **Парсинг VLESS URI** — по строкам, формат:
   ```
   vless://uuid@server:port?params#name
   ```
4. **Для каждого URI:**
   - Извлечь: uuid, server (address), port, name
   - Резолв IP: `net.LookupIP(server)` — получить IPv4
   - Валидация: port числовой, uuid не пустой
5. **Сохранить в servers.json**

### Парсинг VLESS URI

```
vless://xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx@nl-1.example.com:443?type=tcp&security=tls#Server%20Name
       └──────────── uuid ────────────────┘ └──── address ────┘ └port┘ └─ params ─┘ └─ name ─┘
```

- UUID: между `vless://` и `@`
- Address: между `@` и `:`
- Port: между `:` и `?`
- Name: после `#` (URL-decoded, фильтровать эмодзи)

### Структура servers.json

Путь: `$data_dir/servers.json` (обычно `/jffs/scripts/vpn-director/data/servers.json`)

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

### Команда `/servers`

**Ответ:**
```
Серверы (3):
1. nl-1 — nl-1.example.com (1.2.3.4)
2. de-2 — de-2.example.com (5.6.7.8)
3. us-1 — us-1.example.com (9.10.11.12)
```

Если серверов нет:
```
Серверы не импортированы.
Используйте /import <url>
```

### Обработка ошибок

| Ситуация | Ответ бота |
|----------|------------|
| URL не указан | ❌ Использование: /import <url> |
| URL недоступен | ❌ Не удалось загрузить: connection refused |
| Не base64 | ❌ Неверный формат: ожидается base64 |
| Нет валидных серверов | ❌ Не найдено ни одного VLESS сервера |
| Частичный успех | ⚠️ Импортировано 3 из 5 (2 с ошибками) |

### Где взять data_dir

Читать из `/jffs/scripts/vpn-director/vpn-director.json`:

```json
{
  "data_dir": "/jffs/scripts/vpn-director/data",
  ...
}
```

## Структура кода

```
internal/
├── vless/
│   └── parser.go        # Парсинг VLESS URI
└── vpnconfig/
    └── servers.go       # Чтение/запись servers.json
```

## Выходные артефакты

- Работающая команда `/import <url>`
- Работающая команда `/servers`
- Парсер VLESS URI
- Модуль работы с servers.json

## Зависимости от других модулей

- **01-telegram-bot-core** — базовая структура, авторизация, bot handlers
