# Миграция на Entware init.d

## Проблема

После миграции скриптов с ash на bash (Entware) перестал работать запуск при загрузке роутера:

- Скрипты используют `#!/usr/bin/env bash` (Entware bash)
- При boot `services-start` и `firewall-start` выполняются до монтирования Entware
- `env bash` находит `/bin/bash` (busybox) вместо `/opt/bin/bash` (Entware)
- Скрипты фейлятся или работают некорректно на busybox bash

## Решение

Перенести запуск vpn-director в Entware init.d систему, которая гарантированно выполняется после инициализации Entware.

## Архитектура

### Новый файл: `/opt/etc/init.d/S99vpn-director`

```sh
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SCRIPT_DIR="/jffs/scripts/vpn-director"

start() {
    # Build ipsets, then start tunnel_director + xray_tproxy
    "$SCRIPT_DIR/ipset_builder.sh" -t -x

    # Cron job: update ipsets daily at 03:00
    cru a update_ipsets "0 3 * * * $SCRIPT_DIR/ipset_builder.sh -u -t -x"

    # Startup notification
    "$SCRIPT_DIR/utils/send-email.sh" "Startup Notification" \
        "I've just started up and got connected to the internet."
}

stop() {
    "$SCRIPT_DIR/xray_tproxy.sh" stop
    cru d update_ipsets
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    *)       echo "Usage: $0 {start|stop|restart}" ;;
esac
```

**Ключевые моменты:**
- `#!/bin/sh` — стандарт для init.d скриптов
- `PATH=/opt/bin:...` — `env bash` в вызываемых скриптах найдёт Entware bash
- Запускается после rc.unslung, когда Entware полностью готов

### Изменения в `/jffs/scripts/firewall-start`

```sh
#!/bin/sh

# Skip if Entware not ready (during early boot)
[ -x /opt/bin/bash ] || exit 0

# Apply Tunnel Director rules (for firewall reload events)
/jffs/scripts/vpn-director/tunnel_director.sh
```

**Логика:**
- При boot: Entware не готов → тихо выходит → init.d позже всё сделает
- При runtime firewall reload: Entware готов → запускает tunnel_director

### services-start

Не используется vpn-director. Никакой логики vpn-director там нет.

## Роли файлов

| Файл | Роль |
|------|------|
| `/opt/etc/init.d/S99vpn-director` | Точка входа при загрузке. Запускает ipset_builder -t -x, cron job, email. |
| `/jffs/scripts/firewall-start` | Только для runtime firewall reload. Проверка Entware + tunnel_director. |
| `/jffs/scripts/services-start` | Не используется vpn-director. |

## Порядок при загрузке

```
1. Router boot
2. services-start выполняется        → vpn-director там нет
3. firewall-start выполняется        → /opt/bin/bash не существует, exit 0
4. USB монтируется, Entware готов
5. rc.unslung start
6. S24xray start                     → Xray демон запускается
7. S99vpn-director start             → ipset_builder -t -x, cron, email
```

## При runtime firewall reload

```
1. Пользователь меняет настройки в WebUI
2. firewall-start вызывается         → Entware готов, tunnel_director.sh выполняется
```

## Изменения в install.sh

**Убираем:**
- Логику модификации services-start
- Логику добавления cron job в services-start
- Логику добавления email notification в services-start

**Добавляем:**
- Создание `/opt/etc/init.d/S99vpn-director` с chmod +x

**Модифицируем:**
- firewall-start — добавляем проверку Entware перед вызовом tunnel_director.sh
