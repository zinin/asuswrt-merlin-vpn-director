[🇬🇧 English](README.md) | 🇷🇺 Русский

# VPN Director для Asuswrt-Merlin

Выборочная маршрутизация трафика через Xray TPROXY и OpenVPN-туннели.

## Возможности

- **Xray TPROXY**: Прозрачный прокси для выбранных LAN-клиентов через VLESS
- **Tunnel Director**: Маршрутизация трафика через OpenVPN/WireGuard по назначению
- **Маршрутизация по странам**: Направление трафика напрямую или через VPN в зависимости от географии назначения
- **Telegram-бот**: Удалённое управление через Telegram (статус, настройка, перезапуск)
- **Простая установка**: Установка одной командой с интерактивной настройкой

## Быстрая установка

```bash
curl -fsSL \
  -H "Cache-Control: no-cache" \
  -H "Pragma: no-cache" \
  -H "If-Modified-Since: Thu, 01 Jan 1970 00:00:00 GMT" \
  "https://raw.githubusercontent.com/zinin/asuswrt-merlin-vpn-director/master/install.sh?v=$(date +%s)" \
| /usr/bin/env bash
```

После установки:

1. Импортируйте VLESS-серверы (опционально):
   ```bash
   /opt/vpn-director/import_server_list.sh
   ```

2. Запустите мастер настройки:
   ```bash
   /opt/vpn-director/configure.sh
   ```

3. Настройте Telegram-бота (опционально):
   ```bash
   /opt/vpn-director/setup_telegram_bot.sh
   ```

## Требования

- Прошивка Asuswrt-Merlin
- Установленный Entware
- Необходимые пакеты:
  ```bash
  opkg install curl coreutils-base64 coreutils-sha256sum gawk jq xray-core procps-ng-pgrep
  ```
- OpenVPN-клиент, настроенный в интерфейсе роутера (для Tunnel Director)

### Опционально

- `opkg install wget-ssl` — более быстрая и надёжная загрузка файлов зон стран (рекомендуется)
- `opkg install openssl-util` — для email-уведомлений
- `opkg install monit` — для автоматического перезапуска Xray при падении (см. [Мониторинг процессов](#мониторинг-процессов))
- `opkg install coreutils-tr` — исправляет баг команды `tr` (стандартный busybox `tr` портит символы при некоторых локалях)

## Ручная настройка

После установки конфигурационные файлы находятся:

- `/opt/vpn-director/vpn-director.json` — общая конфигурация (Xray + Tunnel Director)
- `/opt/etc/xray/config.json` — конфигурация сервера Xray

## Команды

```bash
# CLI VPN Director
/opt/vpn-director/vpn-director.sh status              # Показать статус
/opt/vpn-director/vpn-director.sh apply               # Применить конфигурацию
/opt/vpn-director/vpn-director.sh stop                # Остановить все компоненты
/opt/vpn-director/vpn-director.sh restart             # Перезапустить всё
/opt/vpn-director/vpn-director.sh update              # Обновить ipsets + применить

# Отдельные компоненты
/opt/vpn-director/vpn-director.sh status tunnel       # Только статус Tunnel Director
/opt/vpn-director/vpn-director.sh status ipset        # Только статус IPSet
/opt/vpn-director/vpn-director.sh restart xray        # Перезапустить только Xray TPROXY

# Опции (можно использовать с любой командой)
/opt/vpn-director/vpn-director.sh -v status           # Подробный вывод
/opt/vpn-director/vpn-director.sh -f apply            # Принудительное применение
/opt/vpn-director/vpn-director.sh --dry-run apply     # Показать, что будет сделано

# Импорт серверов
/opt/vpn-director/import_server_list.sh
```

## Telegram-бот

Удалённое управление через Telegram с авторизацией по имени пользователя.

### Настройка

1. Создайте бота через [@BotFather](https://t.me/BotFather) и получите токен
2. Запустите скрипт настройки:
   ```bash
   /opt/vpn-director/setup_telegram_bot.sh
   ```
3. Введите токен бота и разрешённые имена пользователей (без @)

### Команды бота

| Команда | Описание |
|---------|----------|
| `/status` | Статус VPN Director |
| `/servers` | Список серверов |
| `/import <url>` | Импорт VLESS-подписки (авто-синхронизация xray.servers) |
| `/exclude` | Управление исключёнными IP/CIDR |
| `/configure` | Мастер настройки |
| `/restart` | Перезапустить VPN Director |
| `/stop` | Остановить VPN Director |
| `/logs [bot\|vpn\|all] [N]` | Последние логи (по умолчанию: all, 20 строк) |
| `/ip` | Внешний IP |
| `/update` | Обновить до последней версии |
| `/version` | Версия бота |

### Мастер настройки

Команда `/configure` запускает 5-шаговый мастер:
1. Выбор сервера Xray
2. Выбор исключений по странам (по ISO-коду страны)
3. Исключение конкретных IP/CIDR из прокси
4. Добавление LAN-клиентов с маршрутизацией (Xray/OpenVPN/WireGuard)
5. Проверка и применение

## Как это работает

### Xray TPROXY

Трафик от указанных LAN-клиентов прозрачно перенаправляется через Xray с помощью TPROXY. Прокси использует протокол VLESS поверх TLS для подключения к вашему VPN-серверу.

### Tunnel Director

Маршрутизирует трафик от указанных LAN-клиентов через туннели OpenVPN/WireGuard в зависимости от назначения. Настраиваемые исключения позволяют направлять трафик к выбранным странам напрямую для оптимальной производительности.

### IPSet по странам

Списки IP-адресов стран загружаются автоматически из нескольких источников с резервным переключением:
1. GeoLite2 через GitHub (firehol/blocklist-ipsets) — наиболее точный
2. IPDeny через зеркало на GitHub — не заблокирован в большинстве регионов
3. IPDeny напрямую — может быть заблокирован в некоторых регионах
4. Ручной ввод — интерактивный запрос, если все источники недоступны

## Скрипты автозапуска

Проект использует Entware init.d для автоматического запуска:

| Скрипт | Когда вызывается | Назначение |
|--------|------------------|------------|
| `/opt/etc/init.d/S99vpn-director` | После инициализации Entware | Запускает `vpn-director.sh apply` для инициализации всех компонентов |
| `/jffs/scripts/firewall-start` | После применения правил файрвола | Повторно применяет конфигурацию после перезагрузки файрвола |
| `/jffs/scripts/wan-event` | При подключении WAN | Запускает `vpn-director.sh apply` при подключении WAN |

**Примечание:** Скрипт init.d проверяет доступность bash из Entware перед запуском скриптов vpn-director.

Для включения пользовательских скриптов: Administration -> System -> Enable JFFS custom scripts and configs -> Yes

## Мониторинг процессов

Xray и Telegram-бот могут иногда падать. Используйте monit для автоматического перезапуска.

### Настройка

1. Установите monit:
   ```bash
   opkg install monit
   ```

2. Создайте конфигурации в `/opt/etc/monit.d/`:

   **xray:**
   ```
   check process xray matching "xray"
       start program = "/opt/etc/init.d/S24xray start"
       stop program = "/opt/etc/init.d/S24xray stop"
       if does not exist then restart
   ```

   **telegram-bot:**
   ```
   check process telegram-bot matching "telegram-bot"
       start program = "/opt/etc/init.d/S98telegram-bot start"
       stop program = "/opt/etc/init.d/S98telegram-bot stop"
       if does not exist then restart
   ```

3. Включите директорию конфигов в `/opt/etc/monitrc`:
   ```
   include /opt/etc/monit.d/*
   ```

4. Отредактируйте `/opt/etc/monitrc`, установите интервал проверки:
   ```
   set daemon 30    # проверка каждые 30 секунд
   ```

5. Перезапустите monit:
   ```bash
   /opt/etc/init.d/S99monit restart
   ```

6. Проверьте:
   ```bash
   monit status
   ```

## Лицензия

Copyright (C) 2026 Alexander Zinin <mail@zinin.ru>

Лицензировано под GNU Affero General Public License v3.0 или более поздней версии
(AGPL-3.0-or-later). См. `LICENSE`.
