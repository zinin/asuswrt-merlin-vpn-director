# VPN Director Refactoring Design

**Дата:** 2026-01-19
**Статус:** Draft

## Проблема

Текущая архитектура имеет несколько недостатков:

1. **Неудобство управления** — нужно помнить разные скрипты и флаги (`ipset_builder.sh -t`, `xray_tproxy.sh status`, etc.)
2. **Запутанные зависимости** — `ipset_builder.sh` знает про TD и Xray через флаги `-t`/`-x`, хотя это "строитель ipsets"
3. **Дублирование логики** — оба скрипта (TD и Xray) делают похожие вещи: создают chains, применяют rules, проверяют ipsets

## Решение

Полная реорганизация с модульной архитектурой и единой точкой входа.

---

## 1. Общая архитектура

### Структура файлов

```
jffs/scripts/vpn-director/
├── vpn-director.sh              # Единая точка входа (CLI)
├── lib/
│   ├── common.sh                # Существующий (logging, locking, tmp files)
│   ├── firewall.sh              # Существующий (iptables helpers)
│   ├── config.sh                # Существующий (загрузка vpn-director.json)
│   ├── ipset.sh                 # НОВЫЙ: построение и управление ipsets
│   ├── tunnel.sh                # НОВЫЙ: логика Tunnel Director
│   └── tproxy.sh                # НОВЫЙ: логика Xray TPROXY
├── vpn-director.json            # Конфиг
└── data/                        # Данные (ipset dumps, servers.json)
```

### Удаляемые файлы

- `ipset_builder.sh` → логика переезжает в `lib/ipset.sh`
- `tunnel_director.sh` → логика переезжает в `lib/tunnel.sh`
- `xray_tproxy.sh` → логика переезжает в `lib/tproxy.sh`
- `utils/shared.sh` → объединяется с другими lib-файлами

### Принцип разделения

| Модуль | Ответственность |
|--------|-----------------|
| `ipset.sh` | Скачивание, создание, restore/dump ipsets. Не знает про TD/Xray |
| `tunnel.sh` | Chains TUN_DIR_*, fwmark routing, ip rules |
| `tproxy.sh` | Chain XRAY_TPROXY, TPROXY routing, клиентские ipsets |
| `vpn-director.sh` | CLI parsing, оркестрация вызовов модулей |

---

## 2. CLI интерфейс

### Подкоманды

```bash
vpn-director <command> [component] [options]
```

| Команда | Компоненты | Что делает |
|---------|------------|------------|
| `status` | `[tunnel\|xray\|ipset]` | Показать состояние (всё или конкретный) |
| `apply` | `[tunnel\|xray]` | Применить конфиг (ipsets подтягиваются автоматически если нужны) |
| `stop` | `[tunnel\|xray]` | Остановить компонент(ы) |
| `restart` | `[tunnel\|xray]` | stop + apply |
| `update` | — | Скачать свежие ipsets + apply всё |

**Примечания:**
- `update` — только глобальная команда (без компонента)
- `status ipset` — показать ipsets, но нет `apply ipset`, `stop ipset`
- Ipsets автоматически подтягиваются при `apply` если нужны (из кэша или скачиваются)

### Флаги

| Флаг | Описание |
|------|----------|
| `-f, --force` | Принудительно (игнорировать hash-проверки) |
| `-q, --quiet` | Минимальный вывод |
| `-v, --verbose` | Подробный вывод (DEBUG mode) |
| `--dry-run` | Показать что будет сделано, без применения |

### Примеры использования

```bash
vpn-director status              # Полный статус
vpn-director apply               # Применить всё (idempotent)
vpn-director restart xray        # Перезапустить только Xray TPROXY
vpn-director update              # Скачать свежие ipsets + apply
vpn-director apply --dry-run     # Показать план без применения
```

---

## 3. Структура модулей

### lib/ipset.sh

Ответственность: построение и управление ipsets. **Не знает про tunnel/xray.**

```bash
# Публичные функции (вызываются из vpn-director.sh)
ipset_status()              # Вывести список ipsets, размеры, возраст кэша
ipset_ensure <set_keys...>  # Убедиться что ipsets существуют (из кэша или скачать)
ipset_update <set_keys...>  # Принудительно скачать свежие данные
ipset_cleanup               # Удалить ipsets которые больше не нужны

# Внутренние функции
_download_zone <country>    # Скачать zone-файл с IPdeny
_restore_from_cache <set>   # Восстановить из dump
_save_to_cache <set>        # Сохранить dump
_create_ipset <name> <file> # Создать ipset из файла
_create_combo <name> <sets> # Создать list:set из нескольких ipsets
_derive_set_name <keys>     # Обработка длинных имён (>31 char)
```

### lib/tunnel.sh

Ответственность: Tunnel Director chains и routing.

```bash
# Публичные функции
tunnel_status()      # Показать chains TUN_DIR_*, ip rules, fwmarks
tunnel_apply()       # Применить правила из конфига (idempotent)
tunnel_stop()        # Удалить все chains и ip rules

# Внутренние функции
_parse_rule <rule>          # Парсинг строки table:src:excl:set:set_excl
_setup_chain <idx> <rule>   # Создать chain TUN_DIR_<idx>
_setup_ip_rule <idx> <tbl>  # Создать ip rule с fwmark
_get_required_ipsets()      # Вернуть список ipsets нужных для правил
```

### lib/tproxy.sh

Ответственность: Xray TPROXY chain и routing.

```bash
# Публичные функции
tproxy_status()      # Показать chain XRAY_TPROXY, routing, процесс xray
tproxy_apply()       # Применить правила (idempotent)
tproxy_stop()        # Удалить chain и routing

# Внутренние функции
_setup_routing()            # ip route/rule для TPROXY
_teardown_routing()         # Удалить routing
_setup_clients_ipset()      # Создать XRAY_CLIENTS ipset
_setup_servers_ipset()      # Создать XRAY_SERVERS ipset
_get_required_ipsets()      # Вернуть список exclude ipsets
```

---

## 4. Оркестрация

### Порядок операций

Компоненты имеют зависимости: **ipset → tunnel → tproxy**

| Команда | Порядок выполнения |
|---------|-------------------|
| `apply` | ipset_ensure → tunnel_apply → tproxy_apply |
| `apply tunnel` | ipset_ensure (только нужные для tunnel) → tunnel_apply |
| `apply xray` | ipset_ensure (только нужные для xray) → tproxy_apply |
| `stop` | tproxy_stop → tunnel_stop (обратный порядок) |
| `stop tunnel` | tunnel_stop |
| `restart` | stop → apply |
| `update` | ipset_update → tunnel_apply → tproxy_apply |
| `status` | ipset_status + tunnel_status + tproxy_status |

### Определение нужных ipsets

```bash
apply() {
    local component="${1:-all}"
    local required_ipsets=""

    case "$component" in
        all)
            required_ipsets="$(tunnel_get_required_ipsets) $(tproxy_get_required_ipsets)"
            ;;
        tunnel)
            required_ipsets="$(tunnel_get_required_ipsets)"
            ;;
        xray)
            required_ipsets="$(tproxy_get_required_ipsets)"
            ;;
    esac

    # Дедупликация и ensure
    ipset_ensure $required_ipsets

    # Применить компоненты
    # ...
}
```

### Обработка ошибок

| Ситуация | Поведение |
|----------|-----------|
| IPdeny недоступен, но есть кэш | Использовать кэш, вывести warning |
| IPdeny недоступен, кэша нет | Ошибка, прервать apply для зависимого компонента |
| xt_TPROXY module недоступен | Пропустить tproxy_apply, вывести warning |
| VPN client не активен | tunnel_apply применяет правила, но трафик не пойдёт (warning) |

### Идемпотентность

Все `*_apply()` функции идемпотентны:
- Проверяют текущее состояние (hash конфига, существующие chains)
- Если ничего не изменилось — выходят без действий
- Если изменилось — пересоздают с нуля (delete + create)

---

## 5. Интеграция с системой

### Entware init.d (S99vpn-director)

```bash
#!/bin/sh

ENABLED=yes
PROCS=""  # Не демон, просто скрипт
DESC="VPN Director"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

VPD="/jffs/scripts/vpn-director/vpn-director.sh"

start() {
    echo "Starting $DESC..."
    $VPD apply
    cru a vpn_director_update "0 3 * * * $VPD update"
    /jffs/scripts/vpn-director/lib/notify.sh "Startup" "VPN Director started"
}

stop() {
    echo "Stopping $DESC..."
    $VPD stop
    cru d vpn_director_update
}

case "$1" in
    start)   start ;;
    stop)    stop ;;
    restart) stop; start ;;
    status)  $VPD status ;;
    *)       echo "Usage: $0 {start|stop|restart|status}" ;;
esac
```

### firewall-start hook

```bash
#!/bin/sh
# /jffs/scripts/firewall-start
# Вызывается Merlin при перезагрузке firewall

[ -x /opt/bin/bash ] || exit 0
/jffs/scripts/vpn-director/vpn-director.sh apply
```

### wan-event hook

```bash
#!/bin/sh
# /jffs/scripts/wan-event
# Вызывается при изменении состояния WAN

[ -x /opt/bin/bash ] || exit 0
case "$2" in
    connected)
        /jffs/scripts/vpn-director/vpn-director.sh apply
        ;;
esac
```

### Алиас в profile.add

```bash
alias vpd='/jffs/scripts/vpn-director/vpn-director.sh'
# Использование: vpd status, vpd apply, vpd update
```

### Логирование

Без изменений — используется существующий `log()` из `lib/common.sh`:
- Файл: `/tmp/vpn_director.log`
- Ротация: 100KB

---

## 6. План миграции

### Файлы: удаление и создание

| Действие | Файл | Примечание |
|----------|------|------------|
| **Удалить** | `ipset_builder.sh` | Логика → `lib/ipset.sh` |
| **Удалить** | `tunnel_director.sh` | Логика → `lib/tunnel.sh` |
| **Удалить** | `xray_tproxy.sh` | Логика → `lib/tproxy.sh` |
| **Удалить** | `utils/shared.sh` | Функции распределяются по lib/* |
| **Создать** | `vpn-director.sh` | Единая точка входа |
| **Создать** | `lib/ipset.sh` | Модуль ipsets |
| **Создать** | `lib/tunnel.sh` | Модуль Tunnel Director |
| **Создать** | `lib/tproxy.sh` | Модуль Xray TPROXY |
| **Переименовать** | `utils/` → `lib/` | Унификация naming |
| **Обновить** | `firewall-start` | Вызов `vpn-director.sh apply` |
| **Обновить** | `wan-event` | Вызов `vpn-director.sh apply` |
| **Обновить** | `S99vpn-director` | Новые команды |
| **Обновить** | `profile.add` | Алиас `vpd` |

### Перенос логики

| Из файла | Функция/блок | В файл |
|----------|--------------|--------|
| `ipset_builder.sh` | `download_file()`, `build_ipset()`, `restore_dump()` | `lib/ipset.sh` |
| `ipset_builder.sh` | `parse_country_codes()`, `parse_combo_from_rules()` | `lib/ipset.sh` |
| `tunnel_director.sh` | `table_allowed()`, `resolve_set_name()` | `lib/tunnel.sh` |
| `tunnel_director.sh` | Основной цикл apply | `tunnel_apply()` в `lib/tunnel.sh` |
| `xray_tproxy.sh` | `setup_*()`, `teardown_*()` | `lib/tproxy.sh` |
| `xray_tproxy.sh` | `show_status()` | `tproxy_status()` в `lib/tproxy.sh` |
| `utils/shared.sh` | `derive_set_name()` | `lib/ipset.sh` |

### Сохраняемые файлы (без изменений)

- `lib/common.sh` (бывший `utils/common.sh`)
- `lib/firewall.sh` (бывший `utils/firewall.sh`)
- `lib/config.sh` (бывший `utils/config.sh`)
- `utils/send-email.sh` → `lib/notify.sh` (опционально переименовать)

---

## 7. Тестирование

### Новая структура тестов

```
test/
├── test_helper.bash          # Без изменений
├── mocks/                    # Без изменений
├── fixtures/                 # Без изменений
├── unit/
│   ├── ipset.bats            # Тесты lib/ipset.sh
│   ├── tunnel.bats           # Тесты lib/tunnel.sh
│   └── tproxy.bats           # Тесты lib/tproxy.sh
├── integration/
│   └── vpn_director.bats     # Тесты CLI и оркестрации
└── e2e/
    └── scenarios.bats        # End-to-end сценарии (опционально)
```

### Подход к тестированию

| Уровень | Что тестируем | Как |
|---------|--------------|-----|
| **Unit** | Отдельные функции в lib/*.sh | `--source-only` для загрузки без выполнения |
| **Integration** | CLI команды, порядок вызовов | Mock модули, проверка вызовов |
| **E2E** | Полные сценарии | Docker/VM с реальными ipsets (опционально) |

### Миграция существующих тестов

| Старый файл | Действие |
|-------------|----------|
| `ipset_builder.bats` | Разделить на `unit/ipset.bats` + `integration/vpn_director.bats` |
| `tunnel_director.bats` | Перенести в `unit/tunnel.bats` |
| `xray_tproxy.bats` | Перенести в `unit/tproxy.bats` |

---

## 8. Порядок реализации

1. **Создать lib/ipset.sh** — перенести логику из `ipset_builder.sh`
2. **Создать lib/tunnel.sh** — перенести логику из `tunnel_director.sh`
3. **Создать lib/tproxy.sh** — перенести логику из `xray_tproxy.sh`
4. **Создать vpn-director.sh** — CLI и оркестрация
5. **Обновить hooks** — `firewall-start`, `wan-event`, `S99vpn-director`
6. **Переименовать utils/ → lib/** — унификация
7. **Мигрировать тесты** — адаптировать под новую структуру
8. **Удалить старые скрипты** — после проверки работоспособности
9. **Обновить install.sh** — новые пути и файлы
10. **Обновить документацию** — CLAUDE.md, .claude/rules/*

---

## 9. Риски

| Риск | Митигация |
|------|-----------|
| Регрессия в логике при переносе | Сначала тесты, потом рефакторинг |
| Сломается на production роутере | Тестировать в dev environment, делать backup |
| Забытые вызовы старых скриптов | Grep по репозиторию перед удалением |
