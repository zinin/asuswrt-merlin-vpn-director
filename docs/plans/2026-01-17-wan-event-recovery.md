# WAN Event Recovery Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Автоматически восстанавливать iptables/routing правила Xray TPROXY и Tunnel Director после переподключения WAN кабеля.

**Architecture:** Добавляем хук `wan-event`, который вызывается Asuswrt-Merlin при изменении состояния WAN. При событии `connected` перезапускаем `tunnel_director.sh` и `xray_tproxy.sh`. Также исправляем баг с `restart` командой, которая блокировалась из-за lock-файла.

**Tech Stack:** Bash, Asuswrt-Merlin hooks, iptables, iproute2

---

## Task 1: Создать wan-event хук

**Files:**
- Create: `jffs/scripts/wan-event`

**Step 1: Создать файл wan-event**

```bash
#!/bin/sh

###################################################################################################
# wan-event - Asuswrt-Merlin hook invoked when WAN state changes
# Arguments: $1 = WAN unit (0, 1, ...), $2 = event type
###################################################################################################

SCRIPT_DIR="/jffs/scripts/vpn-director"

# Skip if Entware not ready (during early boot)
[ -x /opt/bin/bash ] || exit 0

case "$2" in
    connected)
        # Restore routing rules after WAN reconnect
        "$SCRIPT_DIR/tunnel_director.sh"
        "$SCRIPT_DIR/xray_tproxy.sh" start
        ;;
esac
```

**Step 2: Проверить синтаксис**

Run: `bash -n jffs/scripts/wan-event`
Expected: No output (syntax OK)

**Step 3: Commit**

```bash
git add jffs/scripts/wan-event
git commit -m "feat: add wan-event hook for WAN reconnect recovery"
```

---

## Task 2: Исправить restart в xray_tproxy.sh

**Files:**
- Modify: `jffs/scripts/vpn-director/xray_tproxy.sh:340-344`

**Step 1: Заменить restart case**

Найти строки 340-344:
```bash
    restart)
        "$0" stop
        sleep 1
        "$0" start
        ;;
```

Заменить на:
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

**Step 2: Проверить синтаксис**

Run: `bash -n jffs/scripts/vpn-director/xray_tproxy.sh`
Expected: No output (syntax OK)

**Step 3: Commit**

```bash
git add jffs/scripts/vpn-director/xray_tproxy.sh
git commit -m "fix: resolve restart command blocking on lock file

Call teardown/setup functions directly instead of spawning
child processes that compete for the same lock."
```

---

## Task 3: Обновить инсталлятор

**Files:**
- Modify: `install.sh:133-146`

**Step 1: Добавить wan-event в список скриптов**

Найти строки 133-146 (список скриптов в download_scripts):
```bash
    for script in \
        "jffs/scripts/vpn-director/ipset_builder.sh" \
        ...
        "jffs/scripts/firewall-start" \
        "jffs/configs/profile.add"
```

Добавить `"jffs/scripts/wan-event"` после `"jffs/scripts/firewall-start"`:
```bash
    for script in \
        "jffs/scripts/vpn-director/ipset_builder.sh" \
        "jffs/scripts/vpn-director/tunnel_director.sh" \
        "jffs/scripts/vpn-director/xray_tproxy.sh" \
        "jffs/scripts/vpn-director/configure.sh" \
        "jffs/scripts/vpn-director/import_server_list.sh" \
        "jffs/scripts/vpn-director/vpn-director.json.template" \
        "jffs/scripts/vpn-director/utils/common.sh" \
        "jffs/scripts/vpn-director/utils/firewall.sh" \
        "jffs/scripts/vpn-director/utils/shared.sh" \
        "jffs/scripts/vpn-director/utils/config.sh" \
        "jffs/scripts/vpn-director/utils/send-email.sh" \
        "jffs/scripts/firewall-start" \
        "jffs/scripts/wan-event" \
        "jffs/configs/profile.add"
```

**Step 2: Проверить синтаксис**

Run: `bash -n install.sh`
Expected: No output (syntax OK)

**Step 3: Commit**

```bash
git add install.sh
git commit -m "feat(install): add wan-event hook to installation"
```

---

## Task 4: Тестирование на роутере

**Step 1: Скопировать файлы на роутер**

```bash
scp jffs/scripts/wan-event admin@router:/jffs/scripts/wan-event
scp jffs/scripts/vpn-director/xray_tproxy.sh admin@router:/jffs/scripts/vpn-director/xray_tproxy.sh
ssh admin@router "chmod +x /jffs/scripts/wan-event"
```

**Step 2: Проверить restart**

Run on router:
```bash
/jffs/scripts/vpn-director/xray_tproxy.sh restart
```
Expected: Успешный перезапуск без ошибки "Another instance is already running"

**Step 3: Проверить симуляцию wan-event**

Run on router:
```bash
/jffs/scripts/wan-event 0 connected
/jffs/scripts/vpn-director/xray_tproxy.sh status | head -20
```
Expected: Все правила на месте (ip rule 200, route table 100, iptables chain)

**Step 4: Проверить реальное отключение WAN**

1. Отключить WAN кабель на 30 секунд
2. Подключить обратно
3. Подождать 10 секунд
4. Run: `/jffs/scripts/vpn-director/xray_tproxy.sh status`

Expected: Все правила восстановлены автоматически

---

## Task 5: Финальный коммит и очистка

**Step 1: Проверить статус**

Run: `git status`
Expected: Чистое рабочее дерево (все закоммичено)

**Step 2: Удалить дизайн-документ (опционально)**

Дизайн-документ можно оставить для истории или удалить:
```bash
# Если решите удалить:
git rm docs/plans/2026-01-17-wan-event-recovery-design.md
git commit -m "docs: remove superseded design document"
```

**Step 3: Создать PR или merge**

```bash
git log --oneline -5  # Проверить коммиты
```
