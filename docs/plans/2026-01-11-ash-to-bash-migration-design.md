# Миграция с ash на bash

## Мотивация

- Использование современных bash-фич (массивы, associative arrays, `[[ ]]`)
- Устранение проблем со стабильностью ash
- Улучшение отладки и удобства разработки
- Унификация стека (bash 5.2.32 доступен через Entware)

## Приоритеты

1. **Безопасность** — убрать `eval`, заменить word splitting на массивы
2. **Читаемость** — associative arrays, here strings, именованные параметры
3. **Отладка** — информативный PS4, stack traces в логах

## Порядок миграции

```
Этап 1: Утилиты (фундамент)
├── utils/common.sh
├── utils/firewall.sh
├── utils/config.sh
├── utils/shared.sh
└── utils/send-email.sh

Этап 2: Основные скрипты
├── ipset_builder.sh
├── tunnel_director.sh
├── xray_tproxy.sh
└── import_server_list.sh

Этап 3: Установка/конфигурация
├── configure.sh
└── install.sh
```

## Стандарт для всех скриптов

```bash
#!/usr/bin/env bash
set -euo pipefail

# Улучшенная отладка (DEBUG=1)
if [[ ${DEBUG:-0} == 1 ]]; then
    set -x
    PS4='+$(date +%H:%M:%S) ${BASH_SOURCE[0]##*/}:${LINENO} ${FUNCNAME[0]:-main}() '
fi
```

## Изменения по безопасности

### Замена eval

До:
```bash
eval "set -- $rest"
```

После:
```bash
read -ra args <<< "$rest"
set -- "${args[@]}"
```

### Замена word splitting на массивы

До:
```bash
IFS=','; set -- $ports; IFS=$IFS_SAVE
for tok in "$@"; do
```

После:
```bash
IFS=',' read -ra tokens <<< "$ports"
for tok in "${tokens[@]}"; do
```

### [[ ]] вместо [ ]

До:
```bash
if [ "$use_v6" -eq 1 ] && [ -n "$ip" ]; then
```

После:
```bash
if [[ $use_v6 -eq 1 && -n $ip ]]; then
```

## Изменения для читаемости

### Associative arrays для конфигурации

```bash
declare -A CONFIG
CONFIG[data_dir]=$(jq -r '.tunnel_director.data_dir' "$CONFIG_FILE")
CONFIG[log_level]=$(jq -r '.log_level' "$CONFIG_FILE")
```

### readarray вместо while read

До:
```bash
v6_list="$(resolve_ip -6 -q -g -a "$host" || true)"
while IFS= read -r ip6; do
    ...
done <<< "$v6_list"
```

После:
```bash
readarray -t v6_list < <(resolve_ip -6 -q -g -a "$host" || true)
for ip6 in "${v6_list[@]}"; do
```

### Here strings

До:
```bash
printf '%s\n' "$out" | grep -E -- "$pattern"
```

После:
```bash
grep -E -- "$pattern" <<< "$out"
```

## Улучшения отладки

### Stack trace в логах ошибок

```bash
log_error_trace() {
    local msg=$1
    local i
    log -l ERROR "$msg"
    for ((i=1; i<${#FUNCNAME[@]}; i++)); do
        log -l ERROR "  at ${BASH_SOURCE[i]##*/}:${BASH_LINENO[i-1]} ${FUNCNAME[i]}()"
    done
}
```

### Числовые уровни логирования

```bash
# LOG_LEVEL: 0=ERROR, 1=WARN, 2=INFO, 3=DEBUG, 4=TRACE
declare -i LOG_LEVEL=${LOG_LEVEL:-2}
```

## Тестирование

### Фреймворк

bats-core с bats-assert и bats-support.

```bash
sudo apt install bats bats-assert bats-support
```

### Структура тестов

```
test/
├── mocks/                    # Фейки системных команд
│   ├── nvram
│   ├── iptables
│   ├── ip6tables
│   ├── nslookup
│   └── logger
├── fixtures/                 # Тестовые данные
│   ├── hosts
│   └── vpn-director.json
├── test_helper.bash          # Общие setup/teardown
├── common.bats
├── firewall.bats
└── config.bats
```

### Тестовый хелпер

```bash
# test/test_helper.bash
setup() {
    export PATH="$BATS_TEST_DIRNAME/mocks:$PATH"
    export TEST_MODE=1
    export HOSTS_FILE="$BATS_TEST_DIRNAME/fixtures/hosts"
}

load_common() {
    source "$BATS_TEST_DIRNAME/../jffs/scripts/vpn-director/utils/common.sh"
}
```

### Пример теста

```bash
# test/common.bats
load 'test_helper'

@test "is_lan_ip: 192.168.x.x is private" {
    load_common
    run is_lan_ip 192.168.1.100
    [ "$status" -eq 0 ]
}
```

### Запуск

```bash
bats test/           # Все тесты
bats test/common.bats # Один файл
bats --verbose-run test/  # Verbose
```

## Стратегия тестирования каждого этапа

1. `bash -n script.sh` — синтаксическая проверка
2. `shellcheck -s bash script.sh` — статический анализ
3. `bats test/` — юнит-тесты локально
4. Функциональный тест на роутере

## Коммиты

```
1. refactor(utils): migrate common.sh to bash
2. refactor(utils): migrate firewall.sh to bash
3. refactor(utils): migrate config.sh, shared.sh, send-email.sh to bash
4. refactor: migrate ipset_builder.sh to bash
5. refactor: migrate tunnel_director.sh to bash
6. refactor: migrate xray_tproxy.sh to bash
7. refactor: migrate import_server_list.sh to bash
8. refactor: migrate configure.sh and install.sh to bash
```

## Откат

Каждый этап — отдельный коммит:
```bash
git revert HEAD
```

## После миграции

Обновить документацию:
- `CLAUDE.md` — shebang: `#!/usr/bin/env bash`
- `.claude/rules/shell-conventions.md` — новые bash-паттерны
