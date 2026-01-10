# Import Server List — Design

## Overview

Разделение `configure.sh` на два независимых скрипта:
- `import_server_list.sh` — импорт VLESS серверов
- `configure.sh` — конфигурация клиентов и правил

## User Flow

```
install.sh → import_server_list.sh → configure.sh
```

## File Structure

```
jffs/scripts/vpn-director/
├── import_server_list.sh    # NEW: импорт VLESS серверов
├── configure.sh             # MODIFIED: убран импорт, читает servers.json
├── vpn-director.json        # MODIFIED: ipset_dump_dir → data_dir
└── data/                    # NEW: default path
    ├── servers.json         # список серверов (JSON)
    └── ipsets/              # дампы ipset
        ├── ru-ipdeny.dump
        └── ...
```

## Config Changes

**vpn-director.json.template:**
```json
{
  "tunnel_director": {
    "rules": [],
    "data_dir": "/jffs/scripts/vpn-director/data"
  },
  ...
}
```

- `ipset_dump_dir` → `data_dir`
- Default path: `/jffs/scripts/vpn-director/data`

## import_server_list.sh

### Functionality

1. Запрашивает путь к файлу или URL с VLESS серверами
2. Скачивает/читает и декодирует base64
3. Парсит VLESS URI, резолвит IP-адреса
4. Сохраняет `data/servers.json`
5. Обновляет `xray.servers` в `vpn-director.json`

### Input Format

Base64-encoded file with vless:// URIs (one per line).

### Output Format (servers.json)

```json
[
  {
    "address": "srv1.example.com",
    "port": 443,
    "uuid": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx",
    "name": "Server Name",
    "ip": "1.2.3.4"
  }
]
```

### Behavior

- Always overwrites existing `servers.json` (no confirmation)
- Automatically updates `xray.servers` array in `vpn-director.json`

### Functions (moved from configure.sh)

- `parse_vless_uri()` — парсинг URI
- `step_get_vless_file()` → main input logic
- `step_parse_vless_servers()` → parsing and IP resolution

## configure.sh Changes

### Removed Steps

- Step 1: Get VLESS file
- Step 2: Parsing Servers

### New Steps

1. Select Xray Server (reads from `servers.json`)
2. Configure Xray Exclusions
3. Configure Clients
4. Show Summary
5. Generate Configs
6. Apply Rules

### Startup Check

```sh
SERVERS_FILE="$DATA_DIR/servers.json"
if [ ! -f "$SERVERS_FILE" ]; then
    print_error "Server list not found. Run import_server_list.sh first."
    exit 1
fi
```

### Removed Functions

- `parse_vless_uri()`
- `step_get_vless_file()`
- `step_parse_vless_servers()`

### Modified Functions

- `step_select_xray_server()` — reads from `servers.json` instead of temp file

## Other File Changes

### ipset_builder.sh

- Read `tunnel_director.data_dir` instead of `tunnel_director.ipset_dump_dir`
- Dump path: `$DATA_DIR/ipsets/` (with subdirectory)
- Create `ipsets/` subdirectory if not exists

### utils/config.sh

- Update parameter name if referenced

### Documentation

- `.claude/rules/ipset-builder.md` — update paths
- `CLAUDE.md` — add `import_server_list.sh`, update `data_dir`

## Implementation Order

1. Create `import_server_list.sh`
2. Update `vpn-director.json.template`
3. Update `ipset_builder.sh`
4. Update `configure.sh`
5. Update documentation
