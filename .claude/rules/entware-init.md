# Entware Init System: Полное руководство

Документация по системе инициализации сервисов Entware на роутерах ASUS с прошивкой Merlin (и других устройствах с Entware).

## Обзор

Entware использует простую систему init-скриптов в стиле SysV, основанную на:
- **rc.unslung** — оркестратор, запускающий все сервисы
- **rc.func** — библиотека функций для управления процессами
- **S\*\* скрипты** — индивидуальные скрипты сервисов

Вся система находится в `/opt/etc/init.d/`.

## Архитектура

```
/opt/etc/init.d/
├── rc.unslung        # Оркестратор (запускает все S* скрипты)
├── rc.func           # Библиотека функций (start, stop, check, etc.)
├── S01syslog-ng      # Сервис с приоритетом 01 (запускается первым)
├── S24xray           # Сервис с приоритетом 24
├── S80nginx          # Сервис с приоритетом 80
└── S99myservice      # Сервис с приоритетом 99 (запускается последним)
```

### Порядок выполнения

- **start/check**: S01 → S24 → S80 → S99 (по возрастанию номера)
- **stop/kill/restart**: S99 → S80 → S24 → S01 (по убыванию номера)

## rc.unslung — Главный оркестратор

Этот скрипт управляет всеми сервисами. Полный исходный код:

```sh
#!/bin/sh

PATH=/opt/sbin:/opt/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

# Start/stop all init scripts in /opt/etc/init.d including symlinks
# starting them in numerical order and
# stopping them in reverse numerical order

unset LD_LIBRARY_PATH
unset LD_PRELOAD

ACTION=$1
CALLER=$2

if [ $# -lt 1 ]; then
    printf "Usage: $0 {start|stop|restart|reconfigure|check|kill}\n" >&2
    exit 1
fi

[ $ACTION = stop -o $ACTION = restart -o $ACTION = kill ] && ORDER="-r"

for i in $(/opt/bin/find /opt/etc/init.d/ -perm '-u+x' -name 'S*' | sort $ORDER ) ;
do
    case "$i" in
        S* | *.sh )
            # Source shell script for speed.
            trap "" INT QUIT TSTP EXIT
            . $i $ACTION $CALLER
            ;;
        *)
            # No sh extension, so fork subprocess.
            $i $ACTION $CALLER
            ;;
    esac
done
```

### Ключевые моменты:

1. **Очистка окружения** — `unset LD_LIBRARY_PATH` и `LD_PRELOAD` для безопасности
2. **Поиск скриптов** — находит все исполняемые файлы `S*` в `/opt/etc/init.d/`
3. **Сортировка** — при stop/restart/kill добавляется `-r` для обратного порядка
4. **Source вместо fork** — скрипты подключаются через `.` для скорости
5. **Передача аргументов** — каждый скрипт получает `$ACTION` и `$CALLER`

### Использование:

```sh
/opt/etc/init.d/rc.unslung start      # Запустить все сервисы
/opt/etc/init.d/rc.unslung stop       # Остановить все сервисы
/opt/etc/init.d/rc.unslung restart    # Перезапустить все сервисы
/opt/etc/init.d/rc.unslung check      # Проверить статус всех сервисов
/opt/etc/init.d/rc.unslung kill       # Принудительно убить все сервисы
/opt/etc/init.d/rc.unslung reconfigure # Отправить SIGHUP всем сервисам
```

## rc.func — Библиотека функций

Это сердце системы управления сервисами. Полный исходный код:

```sh
#!/bin/sh

ACTION=$1
CALLER=$2

ansi_red="\033[1;31m";
ansi_white="\033[1;37m";
ansi_green="\033[1;32m";
ansi_yellow="\033[1;33m";
ansi_blue="\033[1;34m";
ansi_bell="\007";
ansi_blink="\033[5m";
ansi_std="\033[m";
ansi_rev="\033[7m";
ansi_ul="\033[4m";

start() {
    [ "$CRITICAL" != "yes" -a "$CALLER" = "cron" ] && return 7
    [ "$ENABLED" != "yes" ] && return 8
    echo -e -n "$ansi_white Starting $DESC... $ansi_std"
    if [ -n "`pidof $PROC`" ]; then
        echo -e "            $ansi_yellow already running. $ansi_std"
        return 0
    fi
    $PRECMD > /dev/null 2>&1
    $PREARGS $PROC $ARGS > /dev/null 2>&1 &
    COUNTER=0
    LIMIT=10
    while [ -z "`pidof $PROC`" -a "$COUNTER" -le "$LIMIT" ]; do
        sleep 1;
        COUNTER=`expr $COUNTER + 1`
    done
    $POSTCMD > /dev/null 2>&1

    if [ -z "`pidof $PROC`" ]; then
        echo -e "            $ansi_red failed. $ansi_std"
        logger "Failed to start $DESC from $CALLER."
        return 255
    else
        echo -e "            $ansi_green done. $ansi_std"
        logger "Started $DESC from $CALLER."
        return 0
    fi
}

stop() {
    case "$ACTION" in
        stop | restart)
            echo -e -n "$ansi_white Shutting down $PROC... $ansi_std"
            killall $PROC 2>/dev/null
            COUNTER=0
            LIMIT=10
            while [ -n "`pidof $PROC`" -a "$COUNTER" -le "$LIMIT" ]; do
                sleep 1;
                COUNTER=`expr $COUNTER + 1`
            done
            ;;
        kill)
            echo -e -n "$ansi_white Killing $PROC... $ansi_std"
            killall -9 $PROC 2>/dev/null
            ;;
    esac

    if [ -n "`pidof $PROC`" ]; then
        echo -e "            $ansi_red failed. $ansi_std"
        return 255
    else
        echo -e "            $ansi_green done. $ansi_std"
        return 0
    fi
}

check() {
    echo -e -n "$ansi_white Checking $DESC... "
    if [ -n "`pidof $PROC`" ]; then
        echo -e "            $ansi_green alive. $ansi_std";
        return 0
    else
        echo -e "            $ansi_red dead. $ansi_std";
        return 1
    fi
}

reconfigure() {
    SIGNAL=SIGHUP
    echo -e "$ansi_white Sending $SIGNAL to $PROC... $ansi_std"
    killall -$SIGNAL $PROC 2>/dev/null
}

for PROC in $PROCS; do
    case $ACTION in
        start)
            start
            ;;
        stop | kill )
            check && stop
            ;;
        restart)
            check > /dev/null && stop
            start
            ;;
        check)
            check
            ;;
        reconfigure)
            reconfigure
            ;;
        *)
            echo -e "$ansi_white Usage: $0 (start|stop|restart|check|kill|reconfigure)$ansi_std"
            exit 1
            ;;
    esac
done
```

### Переменные конфигурации

S* скрипт должен установить эти переменные перед подключением rc.func:

| Переменная | Обязательная | Описание | Пример |
|------------|--------------|----------|--------|
| `ENABLED` | Да | Включён ли сервис | `yes` или `no` |
| `PROCS` | Да | Имя процесса (для pidof) | `xray` |
| `ARGS` | Да | Аргументы командной строки | `run -confdir /opt/etc/xray` |
| `DESC` | Да | Описание для вывода и логов | `$PROCS` или `"My Service"` |
| `PREARGS` | Нет | Префикс перед командой | `nice -n 10` |
| `PRECMD` | Нет | Команда перед запуском | `mkdir -p /var/run/myservice` |
| `POSTCMD` | Нет | Команда после запуска | `sleep 2` |
| `CRITICAL` | Нет | Запускать из cron | `yes` |

### Функции

#### start()

```
1. Проверяет ENABLED=yes (иначе return 8)
2. Проверяет pidof $PROC — если процесс есть, выходит (already running)
3. Выполняет $PRECMD (если задано)
4. Запускает: $PREARGS $PROC $ARGS &
5. Ждёт до 10 секунд появления PID
6. Выполняет $POSTCMD (если задано)
7. Проверяет pidof — если пусто, failed (return 255)
8. Логирует результат через logger
```

#### stop()

```
1. Отправляет SIGTERM: killall $PROC
2. Ждёт до 10 секунд исчезновения PID
3. Проверяет результат
```

#### kill()

```
1. Отправляет SIGKILL: killall -9 $PROC
2. Не ждёт — немедленное завершение
```

#### check()

```
1. Выполняет pidof $PROC
2. Если PID есть — alive (return 0)
3. Если PID нет — dead (return 1)
```

#### reconfigure()

```
1. Отправляет SIGHUP: killall -SIGHUP $PROC
2. Сервис должен перечитать конфигурацию
```

## Механизм определения статуса

**Вся система основана на команде `pidof`:**

```sh
pidof xray
# Если процесс жив: возвращает PID, например "12345"
# Если процесс мёртв: возвращает пустую строку
```

### Особенности:

- **Нет PID-файлов** — только поиск по имени процесса в /proc
- **Нет systemd/supervisord** — простые shell-скрипты
- **Нет healthcheck** — проверяется только "процесс существует"
- **Имя процесса** должно совпадать с `$PROCS` (имя бинарника)

### Таблица команд

| Команда | Сигнал | Таймаут | Проверка успеха |
|---------|--------|---------|-----------------|
| `start` | — | 10 сек | `pidof $PROC` не пусто |
| `stop` | SIGTERM | 10 сек | `pidof $PROC` пусто |
| `kill` | SIGKILL | 0 сек | `pidof $PROC` пусто |
| `check` | — | — | `pidof $PROC` не пусто |
| `restart` | SIGTERM | 10 сек | — |
| `reconfigure` | SIGHUP | — | — |

## Примеры S* скриптов

### Стандартный скрипт (использует rc.func)

Файл: `/opt/etc/init.d/S24xray`

```sh
#!/bin/sh

ENABLED=yes
PROCS=xray
ARGS="run -confdir /opt/etc/xray"
PREARGS=""
DESC=$PROCS
PATH=/opt/sbin:/opt/bin:/opt/usr/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

**Что происходит при `S24xray start`:**

1. Устанавливаются переменные (PROCS=xray, ARGS=...)
2. Source'ится rc.func
3. rc.func видит ACTION=start
4. Выполняется функция start()
5. Реально запускается: `xray run -confdir /opt/etc/xray &`
6. Ожидание до 10 сек появления PID
7. Вывод: `Starting xray... done.` или `Starting xray... failed.`

### Standalone скрипт (без rc.func)

Файл: `/opt/etc/init.d/S80nginx`

```sh
#!/bin/sh

case $1 in
    start)
        nginx && echo 'Nginx started.'
        ;;
    stop)
        nginx -s quit && echo 'Nginx gracefully stopped.'
        ;;
    restart)
        nginx -s stop && nginx && echo 'Nginx restarted.'
        ;;
    reload)
        nginx -s reload && echo 'Nginx configuration reload.'
        ;;
    reopen)
        nginx -s reopen && echo 'Nginx log files reopened.'
        ;;
    test)
        nginx -t
        ;;
esac
```

Некоторые сервисы (nginx, mysql) имеют собственные механизмы управления и не используют rc.func.

### Скрипт с дополнительными опциями

```sh
#!/bin/sh

ENABLED=yes
PROCS=myservice
ARGS="--config /opt/etc/myservice.conf --daemon"
PREARGS="nice -n 10"                              # Запуск с пониженным приоритетом
PRECMD="mkdir -p /opt/var/run/myservice"          # Создать директорию перед запуском
POSTCMD="sleep 2"                                 # Подождать после запуска
DESC="My Custom Service"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

## Создание нового сервиса

### Шаг 1: Создать скрипт

```sh
cat > /opt/etc/init.d/S50myservice << 'EOF'
#!/bin/sh

ENABLED=yes
PROCS=myservice
ARGS="--config /opt/etc/myservice.conf"
PREARGS=""
DESC=$PROCS
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
EOF
```

### Шаг 2: Сделать исполняемым

```sh
chmod +x /opt/etc/init.d/S50myservice
```

### Шаг 3: Проверить

```sh
/opt/etc/init.d/S50myservice start
/opt/etc/init.d/S50myservice check
/opt/etc/init.d/S50myservice stop
```

### Требования к бинарнику:

1. **Должен демонизироваться** — запускаться и оставаться в фоне
2. **Имя процесса = PROCS** — для корректной работы pidof
3. **Не завершаться сразу** — иначе start() посчитает запуск неудачным

## Интеграция с роутером (ASUS Merlin)

### Автозапуск при загрузке

На роутерах ASUS с Merlin Entware запускается через `/jffs/scripts/post-mount`:

```sh
#!/bin/sh
. /jffs/addons/amtm/mount-entware.mod # Added by amtm
```

Скрипт `mount-entware.mod` выполняет:

```sh
#!/bin/sh

MOUNT_PATH="${1:-/mnt/MERLIN}"

mount_entware(){
    # Очистить /tmp/opt если это не symlink
    [ ! -L /tmp/opt ] && rm -rf /tmp/opt

    # Создать symlink на Entware
    ln -nsf "${opkgFile%/bin/opkg}" /tmp/opt

    # Запустить все сервисы
    /opt/etc/init.d/rc.unslung start "$0"
}

# Найти opkg на смонтированном диске
opkgFile=$(/usr/bin/find "$MOUNT_PATH/entware/bin/opkg" 2> /dev/null)

# Если найден и Entware ещё не запущен — монтировать и запустить
if [ "$opkgFile" ] && [ ! -d /opt/bin ]; then
    mount_entware
fi
```

### Остановка при выключении

Файл `/jffs/scripts/services-stop`:

```sh
#!/bin/sh
/opt/etc/init.d/rc.unslung stop
```

## Отладка

### Проверить статус всех сервисов

```sh
/opt/etc/init.d/rc.unslung check
```

### Проверить конкретный сервис

```sh
/opt/etc/init.d/S24xray check
pidof xray
ps | grep xray
```

### Посмотреть логи запуска

```sh
logread | grep -i "started\|failed"
```

### Ручной запуск для отладки

```sh
# Запустить в foreground для просмотра ошибок
xray run -confdir /opt/etc/xray

# Или с выводом в файл
xray run -confdir /opt/etc/xray > /tmp/xray.log 2>&1 &
cat /tmp/xray.log
```

### Частые проблемы

| Проблема | Причина | Решение |
|----------|---------|---------|
| `failed` при start | Бинарник не найден или ошибка конфига | Запустить вручную для просмотра ошибки |
| `already running` | Процесс уже запущен | `check` или `pidof` |
| `failed` при stop | Процесс не отвечает на SIGTERM | Использовать `kill` (SIGKILL) |
| Сервис не запускается при загрузке | ENABLED=no или скрипт не исполняемый | Проверить `ENABLED` и `chmod +x` |

## Ограничения системы

1. **Нет зависимостей** — нельзя указать "запустить после сервиса X" (только через номер)
2. **Нет healthcheck** — проверяется только существование PID, не работоспособность
3. **Нет автоперезапуска** — если сервис упал, он не перезапустится автоматически
4. **Один процесс = один сервис** — не поддерживаются сервисы с несколькими процессами
5. **Имя процесса фиксировано** — pidof ищет по точному имени бинарника

## Сравнение с другими системами

| Функция | Entware rc.func | systemd | supervisord |
|---------|-----------------|---------|-------------|
| Зависимости | Нет (только порядок) | Да | Нет |
| Healthcheck | Нет | Да | Да |
| Автоперезапуск | Нет | Да | Да |
| Логирование | logger | journald | Встроенное |
| PID tracking | pidof | PID file / cgroups | PID file |
| Сложность | Минимальная | Высокая | Средняя |
| Ресурсы | ~0 | Значительные | Умеренные |

Система Entware оптимизирована для маломощных устройств (роутеры, NAS) где важны простота и минимальное потребление ресурсов.
