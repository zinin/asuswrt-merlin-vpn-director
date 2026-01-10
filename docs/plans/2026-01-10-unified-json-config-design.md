# Unified JSON Config Design

Объединение конфигов `config-xray.sh` и `config-tunnel-director.sh` в единый JSON-файл.

## Мотивация

- Один конфиг удобнее двух
- Всё управляется из одного места
- JSON — стандартный формат, чище shell-скриптов

## Структура файлов

### Новые файлы

| Путь | Назначение |
|------|------------|
| `vpn-director.json` | Единый конфиг (в корне vpn-director/) |
| `vpn-director.json.template` | Шаблон для configure.sh |
| `utils/config.sh` | Загрузчик: читает JSON, экспортирует переменные |

### Удаляемые файлы

| Путь |
|------|
| `configs/config-xray.sh.template` |
| `configs/config-tunnel-director.sh.template` |

## Формат vpn-director.json

```json
{
  "tunnel_director": {
    "rules": [
      "wgc1:192.168.50.0/24::us,ca"
    ],
    "ipset_dump_dir": "/jffs/ipset_builder"
  },
  "xray": {
    "clients": ["192.168.1.10"],
    "servers": ["1.2.3.4"],
    "exclude_sets": ["ru"]
  },
  "advanced": {
    "xray": {
      "tproxy_port": 12345,
      "route_table": 100,
      "rule_pref": 200,
      "fwmark": "0x100",
      "fwmark_mask": "0x100",
      "chain": "XRAY_TPROXY",
      "clients_ipset": "XRAY_CLIENTS",
      "servers_ipset": "XRAY_SERVERS"
    },
    "tunnel_director": {
      "chain_prefix": "TUN_DIR_",
      "pref_base": 16384,
      "mark_mask": "0x00ff0000",
      "mark_shift": 16
    },
    "boot": {
      "min_time": 120,
      "wait_delay": 60
    }
  }
}
```

### Принципы

- Пользовательские настройки на верхнем уровне (`tunnel_director`, `xray`)
- Технические параметры в `advanced` — редко меняются
- Массивы для списков (rules, clients, servers, exclude_sets)
- Hex-значения как строки (`"0x100"`) — сохраняют читаемость

## Загрузчик config.sh

```bash
#!/usr/bin/env ash
# config.sh — загружает vpn-director.json и экспортирует переменные

set -euo pipefail

CONFIG_FILE="/jffs/scripts/vpn-director/vpn-director.json"

# Проверка наличия конфига
if [ ! -f "$CONFIG_FILE" ]; then
    echo "ERROR: Config not found: $CONFIG_FILE" >&2
    exit 1
fi

# Проверка валидности JSON
if ! jq empty "$CONFIG_FILE" 2>/dev/null; then
    echo "ERROR: Invalid JSON: $CONFIG_FILE" >&2
    exit 1
fi

# Хелпер для чтения значений
cfg() { jq -r "$1 // empty" "$CONFIG_FILE"; }
cfg_arr() { jq -r "$1 // [] | .[]" "$CONFIG_FILE" | tr '\n' ' ' | sed 's/ $//'; }

# === Tunnel Director ===
TUN_DIR_RULES=$(cfg_arr '.tunnel_director.rules')
IPS_BDR_DIR=$(cfg '.tunnel_director.ipset_dump_dir')

# === Xray ===
XRAY_CLIENTS=$(cfg_arr '.xray.clients')
XRAY_SERVERS=$(cfg_arr '.xray.servers')
XRAY_EXCLUDE_SETS=$(cfg_arr '.xray.exclude_sets')

# === Advanced: Xray ===
XRAY_TPROXY_PORT=$(cfg '.advanced.xray.tproxy_port')
XRAY_ROUTE_TABLE=$(cfg '.advanced.xray.route_table')
XRAY_RULE_PREF=$(cfg '.advanced.xray.rule_pref')
XRAY_FWMARK=$(cfg '.advanced.xray.fwmark')
XRAY_FWMARK_MASK=$(cfg '.advanced.xray.fwmark_mask')
XRAY_CHAIN=$(cfg '.advanced.xray.chain')
XRAY_CLIENTS_IPSET=$(cfg '.advanced.xray.clients_ipset')
XRAY_SERVERS_IPSET=$(cfg '.advanced.xray.servers_ipset')

# === Advanced: Tunnel Director ===
TUN_DIR_CHAIN_PREFIX=$(cfg '.advanced.tunnel_director.chain_prefix')
TUN_DIR_PREF_BASE=$(cfg '.advanced.tunnel_director.pref_base')
TUN_DIR_MARK_MASK=$(cfg '.advanced.tunnel_director.mark_mask')
TUN_DIR_MARK_SHIFT=$(cfg '.advanced.tunnel_director.mark_shift')

# === Advanced: Boot ===
MIN_BOOT_TIME=$(cfg '.advanced.boot.min_time')
BOOT_WAIT_DELAY=$(cfg '.advanced.boot.wait_delay')

# === readonly ===
readonly \
    TUN_DIR_RULES IPS_BDR_DIR \
    XRAY_CLIENTS XRAY_SERVERS XRAY_EXCLUDE_SETS \
    XRAY_TPROXY_PORT XRAY_ROUTE_TABLE XRAY_RULE_PREF \
    XRAY_FWMARK XRAY_FWMARK_MASK XRAY_CHAIN \
    XRAY_CLIENTS_IPSET XRAY_SERVERS_IPSET \
    TUN_DIR_CHAIN_PREFIX TUN_DIR_PREF_BASE \
    TUN_DIR_MARK_MASK TUN_DIR_MARK_SHIFT \
    MIN_BOOT_TIME BOOT_WAIT_DELAY
```

### Особенности

- `cfg()` — читает скалярные значения, возвращает пустую строку если нет
- `cfg_arr()` — читает массив, соединяет через пробел
- Валидация JSON при загрузке
- Все переменные readonly в конце

## Изменения в скриптах

### xray_tproxy.sh, tunnel_director.sh, ipset_builder.sh

```bash
# Было:
. /jffs/scripts/vpn-director/configs/config-xray.sh
# или
. /jffs/scripts/vpn-director/configs/config-tunnel-director.sh

# Стало:
. /jffs/scripts/vpn-director/utils/config.sh
```

### Изменение итерации

Сейчас правила — многострочная строка:
```bash
echo "$TUN_DIR_RULES" | while read -r rule; do
```

С новым форматом (пробелы):
```bash
for rule in $TUN_DIR_RULES; do
```

Аналогично для `XRAY_CLIENTS`, `XRAY_SERVERS`, `XRAY_EXCLUDE_SETS`.

## configure.sh

Интерактивный скрипт запрашивает:
1. IP клиентов для Xray
2. IP серверов Xray
3. Страны для исключения
4. Правила Tunnel Director

Затем:
- Формирует JSON-массивы из ввода
- Генерирует JSON через `jq` или подставляет в шаблон
- Сохраняет в `vpn-director.json`
- Валидирует результат через `jq empty`

## Порядок реализации

1. Создать `utils/config.sh`
2. Создать `vpn-director.json.template`
3. Обновить скрипты (source + итерация)
4. Обновить `configure.sh`
5. Удалить старые конфиги
6. Протестировать
