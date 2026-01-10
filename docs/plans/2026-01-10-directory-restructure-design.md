# Рефакторинг структуры директорий VPN Director

**Дата:** 2026-01-10
**Статус:** Утверждён

## Цель

Консолидировать все скрипты VPN Director в единую директорию `/jffs/scripts/vpn-director/` для упрощения структуры проекта.

## Новая структура

```
/jffs/scripts/vpn-director/
├── configure.sh                    # Мастер настройки
├── ipset_builder.sh                # Сборка ipset'ов
├── tunnel_director.sh              # Маршрутизация через VPN
├── xray_tproxy.sh                  # Прозрачный прокси Xray
│
├── configs/
│   ├── config-tunnel-director.sh.template
│   └── config-xray.sh.template
│
└── utils/
    ├── common.sh                   # Общие системные утилиты
    ├── firewall.sh                 # iptables хелперы
    ├── shared.sh                   # VPN-director специфичные функции
    └── send-email.sh               # Email уведомления
```

### Файлы вне vpn-director (системные хуки)

```
/jffs/scripts/firewall-start        # Хук перезагрузки firewall
/jffs/scripts/services-start        # Хук завершения загрузки
/jffs/configs/profile.add           # Shell alias
/opt/etc/xray/config.json           # Конфиг Xray сервера
```

## Миграция файлов

| Старый путь | Новый путь |
|-------------|------------|
| `firewall/ipset_builder.sh` | `vpn-director/ipset_builder.sh` |
| `firewall/tunnel_director.sh` | `vpn-director/tunnel_director.sh` |
| `firewall/fw_shared.sh` | `vpn-director/utils/shared.sh` |
| `firewall/config.sh.template` | `vpn-director/configs/config-tunnel-director.sh.template` |
| `xray/xray_tproxy.sh` | `vpn-director/xray_tproxy.sh` |
| `xray/config.sh.template` | `vpn-director/configs/config-xray.sh.template` |
| `utils/common.sh` | `vpn-director/utils/common.sh` |
| `utils/firewall.sh` | `vpn-director/utils/firewall.sh` |
| `utils/send-email.sh` | `vpn-director/utils/send-email.sh` |
| `utils/configure.sh` | `vpn-director/configure.sh` |

### Удаляемые директории

- `/jffs/scripts/firewall/`
- `/jffs/scripts/xray/`
- `/jffs/scripts/utils/`

## Изменения в скриптах

### Source paths (все исполняемые скрипты)

```bash
# Было:
. /jffs/scripts/utils/common.sh
. /jffs/scripts/utils/firewall.sh
DIR="$(get_script_dir)"
. "$DIR/config.sh"
. "$DIR/fw_shared.sh"

# Станет:
. /jffs/scripts/vpn-director/utils/common.sh
. /jffs/scripts/vpn-director/utils/firewall.sh
. /jffs/scripts/vpn-director/utils/shared.sh
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh
# или config-xray.sh для xray_tproxy.sh
```

### firewall-start

```bash
#!/bin/sh
# Apply Tunnel Director rules
/jffs/scripts/vpn-director/tunnel_director.sh
```

### services-start

```bash
#!/bin/sh
# Builds ipsets + starts Tunnel Director and Xray TPROXY
(/jffs/scripts/vpn-director/ipset_builder.sh -t -x) &

# Cron: update ipsets daily at 03:00
cru a update_ipsets "0 3 * * * /jffs/scripts/vpn-director/ipset_builder.sh -u -t -x"

# Startup notification
(sleep 60; /jffs/scripts/vpn-director/utils/send-email.sh "Startup Notification" \
    "I've just started up and got connected to the internet.") &
```

### profile.add

```bash
alias ipt='/jffs/scripts/vpn-director/ipset_builder.sh -t'
```

## Изменения в install.sh

### Директории

```bash
mkdir -p "$JFFS_DIR/vpn-director/configs"
mkdir -p "$JFFS_DIR/vpn-director/utils"
```

### Список файлов для загрузки

```bash
"jffs/scripts/vpn-director/configure.sh"
"jffs/scripts/vpn-director/ipset_builder.sh"
"jffs/scripts/vpn-director/tunnel_director.sh"
"jffs/scripts/vpn-director/xray_tproxy.sh"
"jffs/scripts/vpn-director/configs/config-tunnel-director.sh.template"
"jffs/scripts/vpn-director/configs/config-xray.sh.template"
"jffs/scripts/vpn-director/utils/common.sh"
"jffs/scripts/vpn-director/utils/firewall.sh"
"jffs/scripts/vpn-director/utils/shared.sh"
"jffs/scripts/vpn-director/utils/send-email.sh"
"jffs/scripts/firewall-start"
"jffs/scripts/services-start"
"jffs/configs/profile.add"
```

### Сообщения после установки

```
/jffs/scripts/vpn-director/configure.sh
/jffs/scripts/vpn-director/configs/config-xray.sh
/jffs/scripts/vpn-director/configs/config-tunnel-director.sh
```

## Обновление документации

### CLAUDE.md

Обновить:
- Секцию Commands с новыми путями
- Таблицу Architecture
- Секцию Config Files

### .claude/rules/

Обновить пути в файлах:
- `tunnel-director.md`
- `ipset-builder.md`
- `xray-tproxy.md`

## План реализации

1. Создать новую структуру директорий в репозитории
2. Переместить файлы согласно таблице миграции
3. Обновить все source пути в скриптах
4. Обновить системные хуки (firewall-start, services-start)
5. Обновить profile.add
6. Обновить install.sh
7. Обновить документацию (CLAUDE.md, .claude/rules/)
8. Удалить старые директории
9. Протестировать установку и работу скриптов
