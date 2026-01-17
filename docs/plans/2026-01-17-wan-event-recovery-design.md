# WAN Event Recovery Design

**Дата:** 2026-01-17
**Статус:** Approved

## Обзор проблемы

При отключении/подключении WAN кабеля Asuswrt-Merlin генерирует события `wan-event`, которые приводят к сбросу:
- `ip rule` (правило 200 с fwmark 0x100)
- `ip route table 100` (local default dev lo)
- Правил в цепочке `XRAY_TPROXY` (цепочка остаётся, но пустая)

Текущий хук `firewall-start` **не вызывается** при WAN reconnect — он срабатывает только при полной перезагрузке firewall (reboot, `service restart_firewall`).

Дополнительно: команда `restart` в `xray_tproxy.sh` не работает из-за ошибки в реализации lock-механизма.

## Решение

1. **Добавить `wan-event` хук** — перезапускает `tunnel_director.sh` и `xray_tproxy.sh` при событии `connected`

2. **Исправить `restart` в `xray_tproxy.sh`** — вызывать функции `teardown_*` и `setup_*` напрямую вместо запуска дочерних процессов

3. **Обновить инсталлятор** — копировать `wan-event` в `/jffs/scripts/` с перезаписью существующего

## Затронутые файлы

| Файл | Изменение |
|------|-----------|
| `jffs/scripts/wan-event` | Новый файл |
| `jffs/scripts/vpn-director/xray_tproxy.sh` | Исправить restart case |
| `install.sh` | Добавить копирование wan-event |

---

## Реализация wan-event

### Структура скрипта

```bash
#!/bin/sh
# wan-event - Asuswrt-Merlin hook for WAN state changes
# Arguments: $1 = WAN unit (0, 1, ...), $2 = event type

SCRIPT_DIR="/jffs/scripts/vpn-director"

# Skip if Entware not ready
[ -x /opt/bin/bash ] || exit 0

case "$2" in
    connected)
        # Restore routing rules after WAN reconnect
        "$SCRIPT_DIR/tunnel_director.sh"
        "$SCRIPT_DIR/xray_tproxy.sh" start
        ;;
esac
```

### Логика событий

Asuswrt-Merlin передаёт в `wan-event` два аргумента:
- `$1` — номер WAN интерфейса (0, 1, ...)
- `$2` — тип события: `init`, `connecting`, `connected`, `disconnected`, `stopping`, `stopped`

Нас интересует только `connected` — момент когда WAN поднялся и готов к работе.

### Порядок вызова скриптов

1. **tunnel_director.sh** — сначала, чтобы восстановить fwmark-routing для VPN туннелей
2. **xray_tproxy.sh start** — затем, чтобы восстановить TPROXY правила

Порядок важен: Xray TPROXY использует позицию 1 в PREROUTING (перед Tunnel Director), но сам скрипт идемпотентен и корректно обработает любой порядок вызова.

### Проверка Entware

Строка `[ -x /opt/bin/bash ] || exit 0` — защита от раннего вызова во время загрузки, когда Entware ещё не смонтирован.

---

## Исправление restart в xray_tproxy.sh

### Текущая проблема

```bash
# Строки 340-344 в xray_tproxy.sh
restart)
    "$0" stop    # Запускает новый процесс, который блокируется на acquire_lock
    sleep 1
    "$0" start   # Тоже блокируется
    ;;
```

`acquire_lock` вызывается в строке 43, **до** обработки команды. Дочерние процессы `$0 stop` и `$0 start` пытаются взять тот же lock `/var/lock/xray_tproxy.lock`, который уже занят родителем.

### Решение

Заменить вызов дочерних процессов на прямой вызов функций:

```bash
restart)
    log "Restarting Xray TPROXY routing..."
    teardown_iptables
    teardown_routing
    sleep 1

    if ! check_tproxy_module; then
        exit 1
    fi

    if ! check_required_ipsets; then
        log -l WARN "Required ipsets not ready; exiting without applying rules"
        exit 0
    fi

    setup_routing
    setup_clients_ipset
    setup_servers_ipset
    setup_iptables
    log "Xray TPROXY routing restarted successfully"
    ;;
```

---

## Изменения в инсталляторе

### Изменения

Добавить копирование `wan-event` рядом с `firewall-start`:

```bash
# В секции копирования хуков (после firewall-start)
cp "$REPO_DIR/jffs/scripts/wan-event" /jffs/scripts/wan-event
chmod +x /jffs/scripts/wan-event
```

### Поведение при существующем файле

Перезаписывать без вопросов:
- Инсталлятор неинтерактивный (`curl | bash`)
- Предполагается, что пользователь не имеет своих хуков
- При обновлении vpn-director нужно обновить и wan-event

---

## Тестирование

### Ручное тестирование

1. **Тест restart:**
   ```bash
   /jffs/scripts/vpn-director/xray_tproxy.sh restart
   # Ожидание: успешный перезапуск без ошибки lock
   ```

2. **Тест wan-event:**
   ```bash
   # Симуляция события
   /jffs/scripts/wan-event 0 connected
   /jffs/scripts/vpn-director/xray_tproxy.sh status
   # Ожидание: все правила на месте
   ```

3. **Тест реального отключения:**
   - Отключить WAN кабель на 30 секунд
   - Подключить обратно
   - Проверить `xray_tproxy.sh status`
   - Ожидание: правила восстановлены автоматически

---

## План реализации

| # | Задача |
|---|--------|
| 1 | Создать `jffs/scripts/wan-event` |
| 2 | Исправить `restart` case в `xray_tproxy.sh` |
| 3 | Обновить `install.sh` — добавить копирование wan-event |
| 4 | Протестировать на роутере |
