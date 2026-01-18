# Entware Init System: Complete Guide

Documentation for the Entware service initialization system on ASUS routers with Merlin firmware (and other Entware-enabled devices).

## Overview

Entware uses a simple SysV-style init script system based on:
- **rc.unslung** — orchestrator that launches all services
- **rc.func** — library of functions for process management
- **S\*\* scripts** — individual service scripts

The entire system is located in `/opt/etc/init.d/`.

## Architecture

```
/opt/etc/init.d/
├── rc.unslung        # Orchestrator (launches all S* scripts)
├── rc.func           # Function library (start, stop, check, etc.)
├── S01syslog-ng      # Service with priority 01 (starts first)
├── S24xray           # Service with priority 24
├── S80nginx          # Service with priority 80
└── S99myservice      # Service with priority 99 (starts last)
```

### Execution Order

- **start/check**: S01 → S24 → S80 → S99 (ascending order)
- **stop/kill/restart**: S99 → S80 → S24 → S01 (descending order)

## rc.unslung — Main Orchestrator

This script manages all services. Full source code:

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

### Key Points:

1. **Environment cleanup** — `unset LD_LIBRARY_PATH` and `LD_PRELOAD` for security
2. **Script discovery** — finds all executable `S*` files in `/opt/etc/init.d/`
3. **Sorting** — adds `-r` for reverse order on stop/restart/kill
4. **Source instead of fork** — scripts are sourced via `.` for speed
5. **Argument passing** — each script receives `$ACTION` and `$CALLER`

### Usage:

```sh
/opt/etc/init.d/rc.unslung start      # Start all services
/opt/etc/init.d/rc.unslung stop       # Stop all services
/opt/etc/init.d/rc.unslung restart    # Restart all services
/opt/etc/init.d/rc.unslung check      # Check status of all services
/opt/etc/init.d/rc.unslung kill       # Force kill all services
/opt/etc/init.d/rc.unslung reconfigure # Send SIGHUP to all services
```

## rc.func — Function Library

This is the heart of the service management system. Full source code:

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

### Configuration Variables

An S* script must set these variables before sourcing rc.func:

| Variable | Required | Description | Example |
|----------|----------|-------------|---------|
| `ENABLED` | Yes | Whether service is enabled | `yes` or `no` |
| `PROCS` | Yes | Process name (for pidof) | `xray` |
| `ARGS` | Yes | Command line arguments | `run -confdir /opt/etc/xray` |
| `DESC` | Yes | Description for output and logs | `$PROCS` or `"My Service"` |
| `PREARGS` | No | Prefix before command | `nice -n 10` |
| `PRECMD` | No | Command to run before start | `mkdir -p /var/run/myservice` |
| `POSTCMD` | No | Command to run after start | `sleep 2` |
| `CRITICAL` | No | Run from cron | `yes` |

### Functions

#### start()

```
1. Checks ENABLED=yes (otherwise return 8)
2. Checks pidof $PROC — if process exists, exits (already running)
3. Executes $PRECMD (if set)
4. Launches: $PREARGS $PROC $ARGS &
5. Waits up to 10 seconds for PID to appear
6. Executes $POSTCMD (if set)
7. Checks pidof — if empty, failed (return 255)
8. Logs result via logger
```

#### stop()

```
1. Sends SIGTERM: killall $PROC
2. Waits up to 10 seconds for PID to disappear
3. Checks result
```

#### kill()

```
1. Sends SIGKILL: killall -9 $PROC
2. No waiting — immediate termination
```

#### check()

```
1. Executes pidof $PROC
2. If PID exists — alive (return 0)
3. If no PID — dead (return 1)
```

#### reconfigure()

```
1. Sends SIGHUP: killall -SIGHUP $PROC
2. Service should reload configuration
```

## Status Detection Mechanism

**The entire system is based on the `pidof` command:**

```sh
pidof xray
# If process is alive: returns PID, e.g., "12345"
# If process is dead: returns empty string
```

### Characteristics:

- **No PID files** — only process name lookup in /proc
- **No systemd/supervisord** — simple shell scripts
- **No healthcheck** — only checks "process exists"
- **Process name** must match `$PROCS` (binary name)

### Command Reference

| Command | Signal | Timeout | Success Check |
|---------|--------|---------|---------------|
| `start` | — | 10 sec | `pidof $PROC` not empty |
| `stop` | SIGTERM | 10 sec | `pidof $PROC` empty |
| `kill` | SIGKILL | 0 sec | `pidof $PROC` empty |
| `check` | — | — | `pidof $PROC` not empty |
| `restart` | SIGTERM | 10 sec | — |
| `reconfigure` | SIGHUP | — | — |

## S* Script Examples

### Standard Script (uses rc.func)

File: `/opt/etc/init.d/S24xray`

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

**What happens when `S24xray start` is called:**

1. Variables are set (PROCS=xray, ARGS=...)
2. rc.func is sourced
3. rc.func sees ACTION=start
4. start() function executes
5. Actually runs: `xray run -confdir /opt/etc/xray &`
6. Waits up to 10 sec for PID to appear
7. Output: `Starting xray... done.` or `Starting xray... failed.`

### Standalone Script (without rc.func)

File: `/opt/etc/init.d/S80nginx`

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

Some services (nginx, mysql) have their own management mechanisms and don't use rc.func.

### Script with Additional Options

```sh
#!/bin/sh

ENABLED=yes
PROCS=myservice
ARGS="--config /opt/etc/myservice.conf --daemon"
PREARGS="nice -n 10"                              # Run with lower priority
PRECMD="mkdir -p /opt/var/run/myservice"          # Create directory before start
POSTCMD="sleep 2"                                 # Wait after start
DESC="My Custom Service"
PATH=/opt/sbin:/opt/bin:/usr/sbin:/usr/bin:/sbin:/bin

. /opt/etc/init.d/rc.func
```

## Creating a New Service

### Step 1: Create the Script

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

### Step 2: Make it Executable

```sh
chmod +x /opt/etc/init.d/S50myservice
```

### Step 3: Test

```sh
/opt/etc/init.d/S50myservice start
/opt/etc/init.d/S50myservice check
/opt/etc/init.d/S50myservice stop
```

### Binary Requirements:

1. **Must daemonize** — start and stay in background
2. **Process name = PROCS** — for pidof to work correctly
3. **Don't exit immediately** — otherwise start() will consider launch failed

## Router Integration (ASUS Merlin)

### Auto-start on Boot

On ASUS routers with Merlin, Entware is launched via `/jffs/scripts/post-mount`:

```sh
#!/bin/sh
. /jffs/addons/amtm/mount-entware.mod # Added by amtm
```

The `mount-entware.mod` script does:

```sh
#!/bin/sh

MOUNT_PATH="${1:-/mnt/MERLIN}"

mount_entware(){
    # Clear /tmp/opt if it's not a symlink
    [ ! -L /tmp/opt ] && rm -rf /tmp/opt

    # Create symlink to Entware
    ln -nsf "${opkgFile%/bin/opkg}" /tmp/opt

    # Start all services
    /opt/etc/init.d/rc.unslung start "$0"
}

# Find opkg on mounted disk
opkgFile=$(/usr/bin/find "$MOUNT_PATH/entware/bin/opkg" 2> /dev/null)

# If found and Entware not yet running — mount and start
if [ "$opkgFile" ] && [ ! -d /opt/bin ]; then
    mount_entware
fi
```

### Shutdown on Reboot

File `/jffs/scripts/services-stop`:

```sh
#!/bin/sh
/opt/etc/init.d/rc.unslung stop
```

## Debugging

### Check Status of All Services

```sh
/opt/etc/init.d/rc.unslung check
```

### Check Specific Service

```sh
/opt/etc/init.d/S24xray check
pidof xray
ps | grep xray
```

### View Startup Logs

```sh
logread | grep -i "started\|failed"
```

### Manual Start for Debugging

```sh
# Run in foreground to see errors
xray run -confdir /opt/etc/xray

# Or with output to file
xray run -confdir /opt/etc/xray > /tmp/xray.log 2>&1 &
cat /tmp/xray.log
```

### Common Problems

| Problem | Cause | Solution |
|---------|-------|----------|
| `failed` on start | Binary not found or config error | Run manually to see error |
| `already running` | Process already running | Use `check` or `pidof` |
| `failed` on stop | Process not responding to SIGTERM | Use `kill` (SIGKILL) |
| Service doesn't start on boot | ENABLED=no or script not executable | Check `ENABLED` and `chmod +x` |

## System Limitations

1. **No dependencies** — can't specify "start after service X" (only via number)
2. **No healthcheck** — only checks PID existence, not actual health
3. **No auto-restart** — if service crashes, it won't restart automatically
4. **One process = one service** — multi-process services not supported
5. **Fixed process name** — pidof searches by exact binary name

## Comparison with Other Systems

| Feature | Entware rc.func | systemd | supervisord |
|---------|-----------------|---------|-------------|
| Dependencies | No (order only) | Yes | No |
| Healthcheck | No | Yes | Yes |
| Auto-restart | No | Yes | Yes |
| Logging | logger | journald | Built-in |
| PID tracking | pidof | PID file / cgroups | PID file |
| Complexity | Minimal | High | Medium |
| Resources | ~0 | Significant | Moderate |

The Entware system is optimized for low-power devices (routers, NAS) where simplicity and minimal resource consumption are important.
